package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/robfig/cron/v3"

	"github.com/ackwrap/ackwrap/internal/geoquery"
	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/parser"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

// SystemAdBlockRouteRuleName 系统默认广告拦截规则名称。
// 智能快速配置时自动创建，并绑定到系统默认策略组 应用净化。
const SystemAdBlockRouteRuleName = "广告拦截"

// SystemRuleAdBlockKey 系统默认广告拦截规则内部标识。
const SystemRuleAdBlockKey = "ad_block"

// ErrSystemRouteRuleProtected 系统默认规则不可删除或修改名称/匹配，仅允许启停。
var ErrSystemRouteRuleProtected = errors.New("系统默认规则不可删除或编辑，只能启用或停用")

// IsSystemRouteRuleKey 判断规则是否为系统默认规则。
func IsSystemRouteRuleKey(systemKey string) bool {
	switch strings.TrimSpace(systemKey) {
	case SystemRuleAdBlockKey:
		return true
	default:
		return false
	}
}

// IsSystemRouteRuleName 仅用于阻止用户创建占用系统默认显示名的普通规则。
func IsSystemRouteRuleName(name string) bool {
	return strings.TrimSpace(name) == SystemAdBlockRouteRuleName
}

type RouteRuleService struct {
	store       *store.Store
	paths       *paths.Paths
	realtime    *RealtimeService
	cron        *cron.Cron
	ruleEntries map[int64]cron.EntryID
	geoEntries  map[int64]cron.EntryID
	mu          sync.Mutex
}

func NewRouteRuleService(s *store.Store, p *paths.Paths, rt *RealtimeService) *RouteRuleService {
	return &RouteRuleService{
		store:       s,
		paths:       p,
		realtime:    rt,
		cron:        cron.New(cron.WithSeconds()),
		ruleEntries: make(map[int64]cron.EntryID),
		geoEntries:  make(map[int64]cron.EntryID),
	}
}

func (svc *RouteRuleService) StartScheduler() {
	items, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		logging.Error("route_rule_subscription.scheduler", "load rule subscriptions failed: %v", err)
	}
	for i := range items {
		svc.scheduleRuleSubscriptionJob(&items[i])
	}
	geoItems, err := svc.store.ListGeoAssets()
	if err != nil {
		logging.Error("geo.scheduler", "load geo assets failed: %v", err)
	}
	for i := range geoItems {
		svc.scheduleGeoAssetJob(&geoItems[i])
	}
	svc.cron.Start()
	logging.Info("route_rule.scheduler", "started with %d rule jobs and %d geo jobs", len(items), len(geoItems))
}

func (svc *RouteRuleService) StopScheduler() {
	svc.cron.Stop()
	logging.Info("route_rule.scheduler", "stopped")
}

func (svc *RouteRuleService) scheduleRuleSubscriptionJob(item *model.RouteRuleSubscription) {
	svc.removeRuleSubscriptionJob(item.ID)
	spec, ok := syncScheduleSpec(item.SyncMode, item.SyncTime, item.SyncWeekday)
	if !ok {
		return
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	entryID, err := svc.cron.AddFunc(spec, func() {
		logging.Info("route_rule_subscription.scheduler", "auto updating rule subscription %d (%s)", item.ID, item.Name)
		svc.runRuleSubscriptionSync(item.ID)
	})
	if err != nil {
		logging.Error("route_rule_subscription.scheduler", "add cron job for rule subscription %d: %v", item.ID, err)
		return
	}
	svc.ruleEntries[item.ID] = entryID
}

func (svc *RouteRuleService) removeRuleSubscriptionJob(id int64) {
	svc.mu.Lock()
	entryID, ok := svc.ruleEntries[id]
	if ok {
		delete(svc.ruleEntries, id)
	}
	svc.mu.Unlock()
	if ok {
		svc.cron.Remove(entryID)
	}
}

func (svc *RouteRuleService) refreshRuleSubscriptionJob(id int64) {
	item, err := svc.store.GetRouteRuleSubscription(id)
	if err != nil || item == nil {
		svc.removeRuleSubscriptionJob(id)
		return
	}
	svc.scheduleRuleSubscriptionJob(item)
}

func (svc *RouteRuleService) scheduleGeoAssetJob(item *model.GeoAsset) {
	svc.removeGeoAssetJob(item.ID)
	spec, ok := syncScheduleSpec(item.SyncMode, item.SyncTime, item.SyncWeekday)
	if !ok {
		return
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	entryID, err := svc.cron.AddFunc(spec, func() {
		logging.Info("geo.scheduler", "auto updating geo asset %d (%s)", item.ID, item.Name)
		svc.runGeoAssetSync(item.ID)
	})
	if err != nil {
		logging.Error("geo.scheduler", "add cron job for geo asset %d: %v", item.ID, err)
		return
	}
	svc.geoEntries[item.ID] = entryID
}

func (svc *RouteRuleService) removeGeoAssetJob(id int64) {
	svc.mu.Lock()
	entryID, ok := svc.geoEntries[id]
	if ok {
		delete(svc.geoEntries, id)
	}
	svc.mu.Unlock()
	if ok {
		svc.cron.Remove(entryID)
	}
}

func (svc *RouteRuleService) refreshGeoAssetJob(id int64) {
	item, err := svc.store.GetGeoAsset(id)
	if err != nil || item == nil {
		svc.removeGeoAssetJob(id)
		return
	}
	svc.scheduleGeoAssetJob(item)
}

func syncScheduleSpec(mode string, syncTime string, weekday int) (string, bool) {
	mode = strings.TrimSpace(mode)
	if mode == "" || mode == "off" {
		return "", false
	}
	minute, hour, ok := parseSyncTime(syncTime)
	if !ok {
		return "", false
	}
	if mode == "weekly" {
		return fmt.Sprintf("0 %d %d * * %d", minute, hour, weekday%7), true
	}
	if mode == "monthly" {
		day := weekday
		if day < 1 {
			day = 1
		}
		if day > 31 {
			day = 31
		}
		return fmt.Sprintf("0 %d %d %d * *", minute, hour, day), true
	}
	if mode == "daily" {
		return fmt.Sprintf("0 %d %d * * *", minute, hour), true
	}
	return "", false
}

func (svc *RouteRuleService) List() ([]model.RouteRule, error) {
	logging.Info("route_rule.list", "listing route rules")
	return svc.store.ListRouteRules()
}

func (svc *RouteRuleService) Create(req *model.RouteRuleRequest) (*model.RouteRule, error) {
	if IsSystemRouteRuleName(req.Name) {
		return nil, ErrSystemRouteRuleProtected
	}
	if err := svc.validateRouteRule(req); err != nil {
		return nil, err
	}
	logging.Info("route_rule.create", "creating route rule: %s", req.Name)
	return svc.store.CreateRouteRule(req)
}

func (svc *RouteRuleService) Update(id int64, req *model.RouteRuleRequest) (*model.RouteRule, error) {
	existing, err := svc.store.GetRouteRule(id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, fmt.Errorf("route rule not found")
	}
	// 系统默认规则只允许启停，其余字段强制保持原值。
	if IsSystemRouteRuleKey(existing.SystemKey) {
		logging.Info("route_rule.update", "updating system route rule enabled only: %d (%s)", id, existing.Name)
		item, err := svc.store.UpdateRouteRule(id, &model.RouteRuleRequest{
			Name:     existing.Name,
			Enabled:  req.Enabled,
			Priority: existing.Priority,
			RuleType: existing.RuleType,
			Values:   existing.Values,
			Outbound: existing.Outbound,
			Invert:   existing.Invert,
		})
		if err != nil {
			return nil, err
		}
		if item == nil {
			return nil, fmt.Errorf("route rule not found")
		}
		return item, nil
	}
	if IsSystemRouteRuleName(req.Name) {
		return nil, ErrSystemRouteRuleProtected
	}
	if err := svc.validateRouteRule(req); err != nil {
		return nil, err
	}
	logging.Info("route_rule.update", "updating route rule: %d", id)
	item, err := svc.store.UpdateRouteRule(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("route rule not found")
	}
	return item, nil
}

func (svc *RouteRuleService) Delete(id int64) (*model.ActionResponse, error) {
	logging.Info("route_rule.delete", "deleting route rule: %d", id)
	existing, err := svc.store.GetRouteRule(id)
	if err != nil {
		return nil, err
	}
	if existing != nil && IsSystemRouteRuleKey(existing.SystemKey) {
		return nil, ErrSystemRouteRuleProtected
	}
	if err := svc.store.DeleteRouteRule(id); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "route rule deleted"}, nil
}

func (svc *RouteRuleService) Reorder(req *model.RouteRuleReorderRequest) (*model.ActionResponse, error) {
	if len(req.IDs) == 0 {
		return nil, fmt.Errorf("rule ids are required")
	}
	logging.Info("route_rule.reorder", "reordering %d route rules", len(req.IDs))
	if err := svc.store.ReorderRouteRules(req.IDs); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "route rules reordered"}, nil
}

func (svc *RouteRuleService) Preview() (*model.RouteRulePreviewResponse, error) {
	return svc.PreviewWithBaseURL("")
}

func (svc *RouteRuleService) PreviewWithBaseURL(baseURL string) (*model.RouteRulePreviewResponse, error) {
	logging.Info("route_rule.preview", "previewing route rules")
	items, err := svc.store.ListRouteRules()
	if err != nil {
		return nil, err
	}
	subscriptions, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		return nil, err
	}
	ruleSets := make([]map[string]any, 0)
	ruleSetTags := make(map[string]bool)
	for _, item := range subscriptions {
		if !item.Enabled {
			continue
		}
		if ruleSetTags[item.Tag] {
			continue
		}
		format := item.Format
		ruleURL := routeRuleSubscriptionContentURL(baseURL, item.ID)
		downloadDetour := "direct"
		if item.Format == "clash" {
			format = "source"
		}
		ruleSet := map[string]any{
			"tag":             item.Tag,
			"type":            "remote",
			"format":          format,
			"url":             ruleURL,
			"download_detour": downloadDetour,
		}
		ruleSetTags[item.Tag] = true
		ruleSets = append(ruleSets, ruleSet)
	}
	rules := make([]map[string]any, 0)
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		key := routeRuleSingboxKey(item.RuleType)
		values := item.Values
		if item.RuleType == "mixed" {
			mixedRules, err := mixedSingboxRouteRules(item.Values, item.Outbound, item.Invert)
			if err != nil {
				return nil, err
			}
			rules = append(rules, mixedRules...)
			ruleSets = addMixedGeneratedRuleSets(ruleSets, ruleSetTags, item.Values)
			continue
		}
		if item.RuleType == "geoip" || item.RuleType == "geosite" {
			key = "rule_set"
			values = generatedGeoRuleSetTags(item.RuleType, item.Values)
			ruleSets = appendGeneratedGeoRuleSets(ruleSets, ruleSetTags, item.RuleType, item.Values)
		}
		rule := map[string]any{key: values, "outbound": item.Outbound}
		if item.Invert {
			rule["invert"] = true
		}
		rules = append(rules, rule)
	}
	return &model.RouteRulePreviewResponse{Rules: rules, RuleSets: ruleSets}, nil
}

func (svc *RouteRuleService) ListSubscriptions() ([]model.RouteRuleSubscription, error) {
	logging.Info("route_rule_subscription.list", "listing route rule subscriptions")
	return svc.store.ListRouteRuleSubscriptions()
}

func (svc *RouteRuleService) CreateSubscription(req *model.RouteRuleSubscriptionRequest) (*model.RouteRuleSubscription, error) {
	if err := validateRouteRuleSubscription(req); err != nil {
		return nil, err
	}
	if err := svc.ensureRouteRuleSubscriptionTagUnique(0, req.Tag); err != nil {
		return nil, err
	}
	logging.Info("route_rule_subscription.create", "creating route rule subscription: %s", req.Name)
	item, err := svc.store.CreateRouteRuleSubscription(req)
	if err != nil {
		return nil, err
	}
	svc.scheduleRuleSubscriptionJob(item)
	go svc.runRuleSubscriptionSync(item.ID)
	return item, nil
}

func (svc *RouteRuleService) UpdateSubscription(id int64, req *model.RouteRuleSubscriptionRequest) (*model.RouteRuleSubscription, error) {
	if err := validateRouteRuleSubscription(req); err != nil {
		return nil, err
	}
	if err := svc.ensureRouteRuleSubscriptionTagUnique(id, req.Tag); err != nil {
		return nil, err
	}
	logging.Info("route_rule_subscription.update", "updating route rule subscription: %d", id)
	old, _ := svc.store.GetRouteRuleSubscription(id)
	item, err := svc.store.UpdateRouteRuleSubscription(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("route rule subscription not found")
	}
	svc.refreshRuleSubscriptionJob(id)
	if old == nil || old.URL != item.URL || old.Format != item.Format || old.UseProxy != item.UseProxy {
		go svc.runRuleSubscriptionSync(id)
	}
	return item, nil
}

func (svc *RouteRuleService) DeleteSubscription(id int64) (*model.ActionResponse, error) {
	logging.Info("route_rule_subscription.delete", "deleting route rule subscription: %d", id)
	svc.removeRuleSubscriptionJob(id)
	if err := svc.store.DeleteRouteRuleSubscription(id); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "route rule subscription deleted"}, nil
}

func (svc *RouteRuleService) SyncSubscription(id int64) (*model.ActionResponse, error) {
	item, err := svc.store.GetRouteRuleSubscription(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("route rule subscription not found")
	}
	logging.Info("route_rule_subscription.sync", "syncing route rule subscription: %d", id)
	go svc.runRuleSubscriptionSync(id)
	return &model.ActionResponse{Success: true, Message: "route rule subscription sync started"}, nil
}

func (svc *RouteRuleService) SyncAllSubscriptions() (*model.ActionResponse, error) {
	items, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		return nil, err
	}
	go func() {
		for _, item := range items {
			svc.runRuleSubscriptionSync(item.ID)
		}
	}()
	return &model.ActionResponse{Success: true, Message: "route rule subscriptions sync started"}, nil
}

func (svc *RouteRuleService) ListGeoAssets() ([]model.GeoAsset, error) {
	logging.Info("geo.list", "listing geo assets")
	return svc.store.ListGeoAssets()
}

func (svc *RouteRuleService) UpdateGeoAsset(id int64, req *model.GeoAssetRequest) (*model.GeoAsset, error) {
	if err := validateGeoAssetRequest(req); err != nil {
		return nil, err
	}
	logging.Info("geo.update", "updating geo asset: %d", id)
	old, _ := svc.store.GetGeoAsset(id)
	item, err := svc.store.UpdateGeoAsset(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("geo asset not found")
	}
	svc.refreshGeoAssetJob(id)
	if old == nil || old.URL != item.URL || old.UseProxy != item.UseProxy {
		go svc.runGeoAssetSync(id)
	}
	return item, nil
}

func (svc *RouteRuleService) SyncGeoAsset(id int64) (*model.ActionResponse, error) {
	item, err := svc.store.GetGeoAsset(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("geo asset not found")
	}
	logging.Info("geo.sync", "syncing geo asset: %d", id)
	go svc.runGeoAssetSync(id)
	return &model.ActionResponse{Success: true, Message: "geo asset sync started"}, nil
}

func (svc *RouteRuleService) SyncAllGeoAssets() (*model.ActionResponse, error) {
	items, err := svc.store.ListGeoAssets()
	if err != nil {
		return nil, err
	}
	go func() {
		for _, item := range items {
			svc.runGeoAssetSync(item.ID)
		}
	}()
	return &model.ActionResponse{Success: true, Message: "geo assets sync started"}, nil
}

func (svc *RouteRuleService) GeoLookup(target string, dnsServer string) (*model.GeoLookupResponse, error) {
	target = normalizeGeoLookupTarget(target)
	dnsServer = strings.TrimSpace(dnsServer)
	if target == "" {
		return nil, fmt.Errorf("查询目标不能为空")
	}

	resp := &model.GeoLookupResponse{
		Target:         target,
		DNSServer:      dnsServer,
		ResolvedIPs:    []string{},
		GeoAssets:      []model.GeoLookupAssetStatus{},
		Capabilities:   []string{"geoip", "geosite"},
		GeoIPMatches:   []string{},
		GeositeMatches: []string{},
	}
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return nil, err
	}

	var geoipPath, geositePath string
	for _, asset := range assets {
		ready := asset.LocalPath != ""
		if ready {
			if _, err := os.Stat(asset.LocalPath); err != nil {
				ready = false
			}
		}
		status := model.GeoLookupAssetStatus{Type: asset.Type, Name: asset.Name, Ready: ready, LocalPath: asset.LocalPath, UpdatedAt: asset.CachedUpdatedAt, Error: asset.SyncError}
		resp.GeoAssets = append(resp.GeoAssets, status)
		if ready && asset.Type == "geoip" {
			geoipPath = asset.LocalPath
		}
		if ready && asset.Type == "geosite" {
			geositePath = asset.LocalPath
		}
	}

	if addr, err := netip.ParseAddr(target); err == nil {
		resp.TargetType = "ip"
		resp.ResolvedIPs = []string{addr.String()}
	} else {
		resp.TargetType = "domain"
		host := strings.TrimSuffix(strings.ToLower(target), ".")
		ips, err := lookupIPsWithDNSServer(host, dnsServer)
		if err == nil {
			for _, ip := range ips {
				if addr, ok := netIPToAddr(ip); ok {
					resp.ResolvedIPs = append(resp.ResolvedIPs, addr.String())
				}
			}
		}
		if geositePath != "" && isQualifiedDomain(host) {
			matches, err := lookupGeositeCodes(geositePath, host)
			if err != nil {
				resp.Message = appendLookupMessage(resp.Message, "geosite 查询失败: "+err.Error())
			} else {
				resp.GeositeMatches = matches
			}
		}
	}

	if geoipPath != "" && len(resp.ResolvedIPs) > 0 {
		reader, err := geoquery.OpenGeoIP(geoipPath)
		if err != nil {
			resp.Message = appendLookupMessage(resp.Message, "geoip 查询失败: "+err.Error())
		} else {
			defer reader.Close()
			seen := make(map[string]bool)
			for _, ip := range resp.ResolvedIPs {
				addr, err := netip.ParseAddr(ip)
				if err != nil {
					continue
				}
				addr = addr.Unmap()
				code := reader.Lookup(addr)
				entry := ip + " => " + code
				if !seen[entry] {
					seen[entry] = true
					resp.GeoIPMatches = append(resp.GeoIPMatches, entry)
				}
			}
		}
	}

	if resp.Message == "" {
		resp.Message = "查询完成"
	}
	return resp, nil
}

func normalizeGeoLookupTarget(target string) string {
	target = strings.TrimSpace(target)
	if target == "" {
		return ""
	}
	trimmedHost := strings.Trim(target, "[]")
	if addr, err := netip.ParseAddr(trimmedHost); err == nil {
		return addr.Unmap().String()
	}
	if strings.Contains(target, "://") {
		if parsed, err := url.Parse(target); err == nil && parsed.Hostname() != "" {
			return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		}
	}
	if host, _, err := net.SplitHostPort(target); err == nil {
		host = strings.Trim(host, "[]")
		if addr, err := netip.ParseAddr(host); err == nil {
			return addr.Unmap().String()
		}
		return strings.TrimSuffix(strings.ToLower(host), ".")
	}
	if strings.ContainsAny(target, "/?#") {
		if parsed, err := url.Parse("https://" + target); err == nil && parsed.Hostname() != "" {
			return strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
		}
	}
	target = strings.SplitN(target, "/", 2)[0]
	target = strings.SplitN(target, "?", 2)[0]
	target = strings.SplitN(target, "#", 2)[0]
	if host, _, err := net.SplitHostPort(target); err == nil {
		target = host
	}
	return strings.TrimSuffix(strings.ToLower(strings.Trim(target, "[]")), ".")
}

func isQualifiedDomain(host string) bool {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	return strings.Contains(host, ".")
}

func (svc *RouteRuleService) GeoTags(assetType string, query string, limit int) (*model.GeoTagsResponse, error) {
	assetType = strings.ToLower(strings.TrimSpace(assetType))
	query = strings.ToLower(strings.TrimSpace(query))
	if assetType == "" {
		assetType = "geosite"
	}
	if assetType != "geosite" && assetType != "geoip" {
		return nil, fmt.Errorf("unsupported geo tag type: %s", assetType)
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}

	tags, ready, err := svc.loadGeoTags(assetType)
	if err != nil {
		return nil, err
	}
	resp := &model.GeoTagsResponse{Type: assetType, Tags: []string{}, Ready: ready}
	if !ready {
		resp.Message = assetType + " 数据库未就绪"
		return resp, nil
	}
	resp.Total = len(tags)
	for _, tag := range tags {
		if query != "" && !strings.Contains(strings.ToLower(tag), query) {
			continue
		}
		resp.Tags = append(resp.Tags, tag)
		if len(resp.Tags) >= limit {
			break
		}
	}
	resp.Message = "查询完成"
	return resp, nil
}

func (svc *RouteRuleService) GeoDomains(tag string, limit int, offset int) (*model.GeoDomainsResponse, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" {
		return nil, fmt.Errorf("geosite tag 不能为空")
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	geositePath, ready, err := svc.geositePath()
	if err != nil {
		return nil, err
	}
	resp := &model.GeoDomainsResponse{Tag: tag, Items: []model.GeoDomainItem{}, Suggestions: []string{}, Limit: limit, Offset: offset, Ready: ready}
	if !ready {
		resp.Message = "geosite 数据库未就绪"
		return resp, nil
	}

	reader, codes, err := geoquery.OpenGeosite(geositePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	codeExists := false
	for _, code := range codes {
		if code == tag {
			codeExists = true
		}
		if strings.Contains(code, tag) && len(resp.Suggestions) < 30 {
			resp.Suggestions = append(resp.Suggestions, code)
		}
	}
	if !codeExists {
		sort.Strings(resp.Suggestions)
		resp.Message = "未找到精确 geosite tag"
		return resp, nil
	}
	resp.Suggestions = []string{}

	items, err := reader.Read(tag)
	if err != nil {
		resp.Message = "未找到精确 geosite tag"
		return resp, nil
	}
	resp.Total = len(items)
	if offset > len(items) {
		offset = len(items)
		resp.Offset = offset
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	for _, item := range items[offset:end] {
		resp.Items = append(resp.Items, model.GeoDomainItem{Type: geositeItemTypeLabel(item.Type), Value: item.Value})
	}
	resp.Message = "查询完成"
	return resp, nil
}

func geositeItemTypeLabel(itemType geoquery.GeositeItemType) string {
	switch itemType {
	case geoquery.GeositeRuleTypeDomain:
		return "domain"
	case geoquery.GeositeRuleTypeDomainSuffix:
		return "domain_suffix"
	case geoquery.GeositeRuleTypeDomainKeyword:
		return "domain_keyword"
	case geoquery.GeositeRuleTypeDomainRegex:
		return "domain_regex"
	default:
		return "unknown"
	}
}

func (svc *RouteRuleService) geositePath() (string, bool, error) {
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return "", false, err
	}
	for _, asset := range assets {
		if asset.Type != "geosite" || asset.LocalPath == "" {
			continue
		}
		if _, err := os.Stat(asset.LocalPath); err != nil {
			continue
		}
		return asset.LocalPath, true, nil
	}
	return "", false, nil
}

func (svc *RouteRuleService) loadGeoTags(assetType string) ([]string, bool, error) {
	if assetType == "geoip" {
		return []string{"private", "cn", "hk", "mo", "tw", "us", "jp", "sg", "kr", "de", "fr", "gb", "ru", "in", "br", "au", "ca"}, true, nil
	}
	geositePath, ready, err := svc.geositePath()
	if err != nil {
		return nil, false, err
	}
	if !ready {
		return []string{}, false, nil
	}
	reader, codes, err := geoquery.OpenGeosite(geositePath)
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()
	sort.Strings(codes)
	return codes, true, nil
}

func netIPToAddr(ip net.IP) (netip.Addr, bool) {
	if ip4 := ip.To4(); ip4 != nil {
		return netip.AddrFrom4([4]byte{ip4[0], ip4[1], ip4[2], ip4[3]}), true
	}
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return addr.Unmap(), true
}

func lookupIPsWithDNSServer(host string, dnsServer string) ([]net.IP, error) {
	if dnsServer == "" || dnsServer == "system" {
		return net.LookupIP(host)
	}
	if _, ok := dohEndpointForServer(dnsServer); ok {
		return lookupIPsWithDoH(host, dnsServer)
	}
	server := dnsServer
	if _, _, err := net.SplitHostPort(server); err != nil {
		server = net.JoinHostPort(server, "53")
	}
	dialer := net.Dialer{Timeout: 5 * time.Second}
	resolver := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			return dialer.DialContext(ctx, "udp", server)
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	return resolver.LookupIP(ctx, "ip", host)
}

func lookupIPsWithDoH(host string, dnsServer string) ([]net.IP, error) {
	endpoint, ok := dohEndpointForServer(dnsServer)
	if !ok {
		return nil, fmt.Errorf("unsupported doh server: %s", dnsServer)
	}
	result := make([]net.IP, 0)
	for _, qtype := range []string{"A", "AAAA"} {
		ips, err := lookupDoHType(endpoint, host, qtype)
		if err != nil {
			continue
		}
		result = append(result, ips...)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("doh returned no records")
	}
	return result, nil
}

func dohEndpointForServer(dnsServer string) (string, bool) {
	switch strings.TrimSpace(dnsServer) {
	case "cloudflare-doh":
		return "https://cloudflare-dns.com/dns-query", true
	case "google-doh":
		return "https://dns.google/resolve", true
	case "aliyun-doh":
		return "https://dns.alidns.com/resolve", true
	case "tencent-doh":
		return "https://doh.pub/resolve", true
	case "1.1.1.1", "1.0.0.1":
		return "https://cloudflare-dns.com/dns-query", true
	case "8.8.8.8", "8.8.4.4":
		return "https://dns.google/resolve", true
	case "223.5.5.5", "223.6.6.6":
		return "https://dns.alidns.com/resolve", true
	case "119.29.29.29", "182.254.116.116":
		return "https://doh.pub/resolve", true
	default:
		return "", false
	}
}

func lookupDoHType(endpoint string, host string, qtype string) ([]net.IP, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	query := url.Values{}
	query.Set("name", host)
	query.Set("type", qtype)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+query.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/dns-json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("doh status: %d", resp.StatusCode)
	}
	var body struct {
		Answer []struct {
			Data string `json:"data"`
			Type int    `json:"type"`
		} `json:"Answer"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	ips := make([]net.IP, 0, len(body.Answer))
	for _, answer := range body.Answer {
		if qtype == "A" && answer.Type != 1 {
			continue
		}
		if qtype == "AAAA" && answer.Type != 28 {
			continue
		}
		ip := net.ParseIP(answer.Data)
		if ip != nil {
			ips = append(ips, ip)
		}
	}
	return ips, nil
}

func lookupGeositeCodes(path string, domain string) ([]string, error) {
	reader, codes, err := geoquery.OpenGeosite(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	matches := make([]string, 0)
	for _, code := range codes {
		items, err := reader.Read(code)
		if err != nil {
			continue
		}
		if rule := geoquery.NewGeositeMatcher(items).Match(domain); rule != "" {
			matches = append(matches, code+" ("+rule+")")
		}
	}
	return matches, nil
}

func appendLookupMessage(current string, next string) string {
	if current == "" {
		return next
	}
	return current + "; " + next
}

func (svc *RouteRuleService) runGeoAssetSync(id int64) {
	item, err := svc.store.GetGeoAsset(id)
	if err != nil || item == nil {
		logging.Error("geo.sync", "get geo asset %d failed: %v", id, err)
		return
	}
	if item.SyncStatus == "syncing" {
		return
	}
	if err := svc.store.SetGeoAssetSyncState(id, "syncing", ""); err != nil {
		logging.Error("geo.sync", "set geo sync state failed: %v", err)
		return
	}
	localPath, err := svc.fetchAndCacheGeoAsset(item)
	if err != nil {
		logging.Error("geo.sync", "sync geo asset %d failed: %v", id, err)
		_ = svc.store.SetGeoAssetSyncState(id, "failed", err.Error())
		return
	}
	if _, err := svc.store.UpdateGeoAssetSyncResult(id, localPath); err != nil {
		logging.Error("geo.sync", "update geo sync result failed: %v", err)
	}
}

func (svc *RouteRuleService) fetchAndCacheGeoAsset(item *model.GeoAsset) (string, error) {
	if svc.paths == nil {
		return "", fmt.Errorf("geo directory is not configured")
	}
	body, err := fetchRouteRuleSubscriptionContent(item.URL, item.UseProxy)
	if err != nil {
		return "", err
	}
	localPath := filepath.Join(svc.paths.GeoDir, item.Type+".db")
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return "", fmt.Errorf("create geo dir: %w", err)
	}
	if err := os.WriteFile(localPath, body, 0644); err != nil {
		return "", fmt.Errorf("write geo database: %w", err)
	}
	return localPath, nil
}

func (svc *RouteRuleService) SubscriptionContent(id int64) ([]byte, string, error) {
	item, err := svc.store.GetRouteRuleSubscription(id)
	if err != nil {
		return nil, "", err
	}
	if item == nil {
		return nil, "", fmt.Errorf("route rule subscription not found")
	}
	if item.CachedPath != "" {
		if data, err := os.ReadFile(item.CachedPath); err == nil && len(data) > 0 {
			return data, routeRuleSubscriptionContentType(item.Format), nil
		}
	}

	logging.Info("route_rule_subscription.convert", "loading route rule subscription content: %d", id)
	data, contentType, cachedPath, err := svc.fetchAndCacheRouteRuleSubscription(item)
	if err != nil {
		return nil, "", err
	}
	if _, err := svc.store.UpdateRouteRuleSubscriptionSyncResult(id, cachedPath); err != nil {
		logging.Error("route_rule_subscription.sync", "update sync result failed: %v", err)
	}
	return data, contentType, nil
}

func (svc *RouteRuleService) runRuleSubscriptionSync(id int64) {
	item, err := svc.store.GetRouteRuleSubscription(id)
	if err != nil || item == nil {
		logging.Error("route_rule_subscription.sync", "get route rule subscription %d failed: %v", id, err)
		return
	}
	if item.SyncStatus == "syncing" {
		return
	}
	if err := svc.store.SetRouteRuleSubscriptionSyncState(id, "syncing", 30, ""); err != nil {
		logging.Error("route_rule_subscription.sync", "set sync state failed: %v", err)
		return
	}
	_, _, cachedPath, err := svc.fetchAndCacheRouteRuleSubscription(item)
	if err != nil {
		logging.Error("route_rule_subscription.sync", "sync rule subscription %d failed: %v", id, err)
		_ = svc.store.SetRouteRuleSubscriptionSyncState(id, "failed", 0, err.Error())
		return
	}
	if _, err := svc.store.UpdateRouteRuleSubscriptionSyncResult(id, cachedPath); err != nil {
		logging.Error("route_rule_subscription.sync", "update sync result failed: %v", err)
	}
}

func (svc *RouteRuleService) fetchAndCacheRouteRuleSubscription(item *model.RouteRuleSubscription) ([]byte, string, string, error) {
	if svc.paths == nil {
		return nil, "", "", fmt.Errorf("rules directory is not configured")
	}
	body, err := fetchRouteRuleSubscriptionContent(item.URL, item.UseProxy)
	if err != nil {
		return nil, "", "", err
	}
	data := body
	if item.Format == "source" {
		if !json.Valid(body) {
			return nil, "", "", fmt.Errorf("source rule subscription is not valid json")
		}
	} else if item.Format == "clash" {
		ruleSet, err := parser.ParseClashRuleSetYAML(body)
		if err != nil {
			return nil, "", "", err
		}
		data, err = json.MarshalIndent(ruleSet, "", "  ")
		if err != nil {
			return nil, "", "", fmt.Errorf("marshal converted rule set: %w", err)
		}
	}
	cachedPath := svc.routeRuleSubscriptionCachePath(item)
	if err := os.MkdirAll(filepath.Dir(cachedPath), 0755); err != nil {
		return nil, "", "", fmt.Errorf("create rules cache dir: %w", err)
	}
	if err := os.WriteFile(cachedPath, data, 0644); err != nil {
		return nil, "", "", fmt.Errorf("write rule subscription cache: %w", err)
	}
	return data, routeRuleSubscriptionContentType(item.Format), cachedPath, nil
}

func (svc *RouteRuleService) routeRuleSubscriptionCachePath(item *model.RouteRuleSubscription) string {
	ext := ".srs"
	if item.Format == "source" {
		ext = ".json"
	} else if item.Format == "clash" {
		ext = ".json"
	}
	name := slugRouteRuleSubscriptionTag(item.Tag)
	if name == "" {
		name = fmt.Sprintf("rule-%d", item.ID)
	}
	return filepath.Join(svc.paths.RulesDir, fmt.Sprintf("%d-%s%s", item.ID, name, ext))
}

func routeRuleSubscriptionContentType(format string) string {
	if format == "binary" {
		return "application/octet-stream"
	}
	return "application/json; charset=utf-8"
}

func (svc *RouteRuleService) validateRouteRule(req *model.RouteRuleRequest) error {
	if err := validateRouteRule(req); err != nil {
		return err
	}
	return svc.validateGeoSiteRuleValues(req.RuleType, req.Values)
}

func validateRouteRule(req *model.RouteRuleRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.RuleType = strings.TrimSpace(req.RuleType)
	req.Outbound = strings.TrimSpace(req.Outbound)
	req.Values = store.NormalizeRouteRuleValues(req.Values)
	if req.Name == "" {
		return fmt.Errorf("route rule name is required")
	}
	if !isRouteRuleType(req.RuleType) {
		return fmt.Errorf("unsupported route rule type")
	}
	if len(req.Values) == 0 {
		return fmt.Errorf("route rule values are required")
	}
	switch req.Outbound {
	case "proxy", "direct", "block":
	default:
		return fmt.Errorf("outbound must be proxy, direct, or block")
	}
	if req.RuleType == "mixed" {
		if _, err := parseMixedRouteRuleValues(req.Values); err != nil {
			return err
		}
		return validateMixedRouteRuleValues(req.Values)
	}
	if req.RuleType == "ip_cidr" {
		return validateIPCidrValues(req.Values)
	}
	if req.RuleType == "geoip" {
		return validateGeoIPValues(req.Values)
	}
	if req.Priority <= 0 {
		req.Priority = 0
	}
	return nil
}

func (svc *RouteRuleService) validateGeoSiteRuleValues(ruleType string, values []string) error {
	geositeValues := make([]string, 0)
	if ruleType == "geosite" {
		geositeValues = append(geositeValues, values...)
	}
	if ruleType == "mixed" {
		items, err := parseMixedRouteRuleValues(values)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.RuleType == "geosite" {
				geositeValues = append(geositeValues, item.Value)
			}
		}
	}
	if len(geositeValues) == 0 {
		return nil
	}
	codes, ready, err := svc.loadGeoTags("geosite")
	if err != nil {
		return err
	}
	if !ready {
		return nil
	}
	codeSet := make(map[string]bool, len(codes))
	for _, code := range codes {
		codeSet[strings.ToLower(code)] = true
	}
	for _, value := range geositeValues {
		trimmed := strings.ToLower(strings.TrimSpace(value))
		if strings.HasPrefix(trimmed, "geosite-") {
			trimmed = strings.TrimPrefix(trimmed, "geosite-")
		}
		if trimmed == "" {
			continue
		}
		if !codeSet[trimmed] {
			return fmt.Errorf("geosite tag 不存在: %s", value)
		}
	}
	return nil
}

func routeRuleSingboxKey(ruleType string) string {
	return ruleType
}

func validateRouteRuleSubscription(req *model.RouteRuleSubscriptionRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Tag = strings.TrimSpace(req.Tag)
	req.URL = strings.TrimSpace(req.URL)
	req.Format = strings.TrimSpace(req.Format)
	req.SyncMode = strings.TrimSpace(req.SyncMode)
	req.SyncTime = strings.TrimSpace(req.SyncTime)
	if req.Name == "" {
		return fmt.Errorf("route rule subscription name is required")
	}
	if req.URL == "" {
		return fmt.Errorf("route rule subscription url is required")
	}
	parsed, err := url.Parse(req.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("route rule subscription url must be http or https")
	}
	if req.Tag == "" {
		req.Tag = defaultRouteRuleSubscriptionTag(req.Name, req.URL)
	}
	if req.Tag == "" {
		return fmt.Errorf("route rule subscription tag is required")
	}
	if !isRouteRuleSubscriptionTag(req.Tag) {
		return fmt.Errorf("route rule subscription tag can only contain letters, numbers, dash, and underscore")
	}
	if req.Format == "" || req.Format == "auto" {
		req.Format = detectRouteRuleSubscriptionFormat(req.URL)
	}
	switch req.Format {
	case "binary", "source", "clash":
	default:
		return fmt.Errorf("route rule subscription format must be auto, binary, source, or clash")
	}
	if req.SyncMode == "" {
		req.SyncMode = "daily"
	}
	switch req.SyncMode {
	case "off", "daily", "weekly", "monthly":
	default:
		return fmt.Errorf("route rule subscription sync mode must be off, daily, weekly, or monthly")
	}
	if req.SyncMode != "off" {
		if req.SyncTime == "" {
			req.SyncTime = "04:00:00"
		}
		if !syncTimePattern.MatchString(req.SyncTime) {
			return fmt.Errorf("route rule subscription sync time must be HH:MM:SS")
		}
	}
	if req.SyncMode == "weekly" && (req.SyncWeekday < 0 || req.SyncWeekday > 6) {
		return fmt.Errorf("route rule subscription sync weekday must be 0-6")
	}
	if req.SyncMode == "monthly" && (req.SyncWeekday < 1 || req.SyncWeekday > 31) {
		return fmt.Errorf("route rule subscription sync day of month must be 1-31")
	}
	return nil
}

func validateGeoAssetRequest(req *model.GeoAssetRequest) error {
	req.URL = strings.TrimSpace(req.URL)
	req.SyncMode = strings.TrimSpace(req.SyncMode)
	req.SyncTime = strings.TrimSpace(req.SyncTime)
	if req.URL == "" {
		return fmt.Errorf("geo asset url is required")
	}
	parsed, err := url.Parse(req.URL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("geo asset url must be http or https")
	}
	if req.SyncMode == "" {
		req.SyncMode = "daily"
	}
	switch req.SyncMode {
	case "off", "daily", "weekly", "monthly":
	default:
		return fmt.Errorf("geo asset sync mode must be off, daily, weekly, or monthly")
	}
	if req.SyncMode != "off" {
		if req.SyncTime == "" {
			req.SyncTime = "03:30:00"
		}
		if !syncTimePattern.MatchString(req.SyncTime) {
			return fmt.Errorf("geo asset sync time must be HH:MM:SS")
		}
	}
	if req.SyncMode == "weekly" && (req.SyncWeekday < 0 || req.SyncWeekday > 6) {
		return fmt.Errorf("geo asset sync weekday must be 0-6")
	}
	if req.SyncMode == "monthly" && (req.SyncWeekday < 1 || req.SyncWeekday > 31) {
		return fmt.Errorf("geo asset sync day of month must be 1-31")
	}
	return nil
}

func fetchRouteRuleSubscriptionContent(rawURL string, useProxy bool) ([]byte, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if useProxy {
		proxyURL, _ := url.Parse("http://127.0.0.1:2080")
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Timeout: 60 * time.Second, Transport: transport}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create rule subscription request: %w", err)
	}
	req.Header.Set("User-Agent", "Ackwrap/0.1")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch rule subscription: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch rule subscription failed: http %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read rule subscription: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("rule subscription response is empty")
	}
	return data, nil
}

func detectRouteRuleSubscriptionFormat(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	path := strings.ToLower(rawURL)
	if err == nil {
		path = strings.ToLower(parsed.Path)
	}
	switch {
	case strings.HasSuffix(path, ".yaml"), strings.HasSuffix(path, ".yml"):
		return "clash"
	case strings.HasSuffix(path, ".json"):
		return "source"
	default:
		return "binary"
	}
}

func routeRuleSubscriptionContentURL(baseURL string, id int64) string {
	path := fmt.Sprintf("/api/v1/rules/subscriptions/%d/content", id)
	if strings.TrimSpace(baseURL) == "" {
		return path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func (svc *RouteRuleService) ensureRouteRuleSubscriptionTagUnique(id int64, tag string) error {
	items, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		return err
	}
	for _, item := range items {
		if item.Tag == tag && item.ID != id {
			return fmt.Errorf("route rule subscription tag already exists")
		}
	}
	return nil
}

func defaultRouteRuleSubscriptionTag(name string, rawURL string) string {
	if tag := slugRouteRuleSubscriptionTag(name); tag != "" {
		return tag
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	path := strings.Trim(parsed.Path, "/")
	if tag := slugRouteRuleSubscriptionTag(path); tag != "" {
		return tag
	}
	return slugRouteRuleSubscriptionTag(parsed.Host)
}

func slugRouteRuleSubscriptionTag(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if r > unicode.MaxASCII {
			continue
		}
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '.' || r == '/' || unicode.IsSpace(r) {
			if b.Len() > 0 && !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func isRouteRuleSubscriptionTag(value string) bool {
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return value != ""
}
