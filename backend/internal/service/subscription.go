package service

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/parser"
	"github.com/ackwrap/ackwrap/internal/store"
)

var syncTimePattern = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d:[0-5]\d$`)

var subscriptionUserAgentOptions = []model.UserAgentOption{
	{Label: "Clash Meta", Value: "clash-meta/2.4.0", Description: "通用 Clash/Mihomo 订阅，兼容性最好"},
	{Label: "Mihomo", Value: "mihomo/1.18.0", Description: "部分机场会按 Mihomo 返回 Clash 格式"},
	{Label: "Clash Verge", Value: "clash-verge/1.7.7", Description: "模拟 Clash Verge 客户端"},
	{Label: "v2rayN", Value: "v2rayN/6.0", Description: "部分机场会按 v2rayN 返回 URI/base64 列表"},
	{Label: "sing-box", Value: "sing-box/1.10.0", Description: "部分订阅服务会返回 sing-box JSON"},
	{Label: "Shadowrocket", Value: "Shadowrocket/1995", Description: "部分移动端订阅兼容格式"},
}

type subscriptionSyncResult struct {
	NodeCount         int
	TrafficUsedBytes  int64
	TrafficTotalBytes int64
	ExpireAt          int64
	Nodes             []model.ParsedNode
	UnsupportedCount  map[string]int
}

type SubscriptionService struct {
	store      *store.Store
	realtime   *RealtimeService
	reconciler *ConfigReconcileService
	cron       *cron.Cron
	entries    map[int64]cron.EntryID
	mu         sync.Mutex
	syncMu     sync.Mutex
	syncing    map[int64]bool
}

func NewSubscriptionService(s *store.Store, rt *RealtimeService) *SubscriptionService {
	return &SubscriptionService{
		store:    s,
		realtime: rt,
		cron:     cron.New(cron.WithSeconds()),
		entries:  make(map[int64]cron.EntryID),
		syncing:  make(map[int64]bool),
	}
}

// SetConfigReconciler 注入统一配置协调器。
func (svc *SubscriptionService) SetConfigReconciler(reconciler *ConfigReconcileService) {
	svc.reconciler = reconciler
}

func (svc *SubscriptionService) StartScheduler() {
	if err := svc.store.ResetInterruptedSubscriptionSyncs(); err != nil {
		logging.Error("subscription.scheduler", "reset interrupted sync states failed: %v", err)
	}
	items, err := svc.store.ListSubscriptions()
	if err != nil {
		logging.Error("subscription.scheduler", "load subscriptions failed: %v", err)
	}
	for i := range items {
		svc.scheduleJob(&items[i])
	}
	svc.cron.Start()
	logging.Info("subscription.scheduler", "started with %d jobs", len(items))
}

func (svc *SubscriptionService) StopScheduler() {
	svc.cron.Stop()
	logging.Info("subscription.scheduler", "stopped")
}

func (svc *SubscriptionService) scheduleJob(sub *model.Subscription) {
	svc.removeJob(sub.ID)
	if sub.SyncMode == "off" {
		return
	}
	minute, hour, ok := parseSyncTime(sub.SyncTime)
	if !ok {
		return
	}
	spec := fmt.Sprintf("0 %d %d * * *", minute, hour)
	if sub.SyncMode == "weekly" {
		cronWeekday := sub.SyncWeekday % 7
		spec = fmt.Sprintf("0 %d %d * * %d", minute, hour, cronWeekday)
	} else if sub.SyncMode == "monthly" {
		day := sub.SyncWeekday
		if day < 1 {
			day = 1
		}
		if day > 31 {
			day = 31
		}
		spec = fmt.Sprintf("0 %d %d %d * *", minute, hour, day)
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	entryID, err := svc.cron.AddFunc(spec, func() {
		logging.Info("subscription.scheduler", "auto syncing subscription %d (%s)", sub.ID, sub.Name)
		svc.runSync(sub.ID)
	})
	if err != nil {
		logging.Error("subscription.scheduler", "add cron job for subscription %d: %v", sub.ID, err)
		return
	}
	svc.entries[sub.ID] = entryID
	logging.Info("subscription.scheduler", "scheduled subscription %d (%s) mode=%s time=%s cron=%s", sub.ID, sub.Name, sub.SyncMode, sub.SyncTime, spec)
}

func (svc *SubscriptionService) removeJob(subID int64) {
	svc.mu.Lock()
	entryID, ok := svc.entries[subID]
	if ok {
		delete(svc.entries, subID)
	}
	svc.mu.Unlock()
	if ok {
		svc.cron.Remove(entryID)
	}
}

func (svc *SubscriptionService) refreshJob(id int64) {
	item, err := svc.store.GetSubscription(id)
	if err != nil || item == nil {
		svc.removeJob(id)
		return
	}
	svc.scheduleJob(item)
}

func (svc *SubscriptionService) List() ([]model.Subscription, error) {
	logging.Info("subscription.list", "listing subscriptions")
	return svc.store.ListSubscriptions()
}

func (svc *SubscriptionService) UserAgentOptions() []model.UserAgentOption {
	return subscriptionUserAgentOptions
}

func (svc *SubscriptionService) Create(req *model.SubscriptionRequest) (*model.Subscription, error) {
	if err := validateSubscription(req); err != nil {
		return nil, err
	}
	logging.Info("subscription.create", "creating subscription: %s", req.Name)
	item, err := svc.store.CreateSubscription(req)
	if err != nil {
		return nil, err
	}
	svc.scheduleJob(item)
	go svc.runSync(item.ID)
	return item, nil
}

func (svc *SubscriptionService) Update(id int64, req *model.SubscriptionRequest) (*model.Subscription, error) {
	if err := validateSubscription(req); err != nil {
		return nil, err
	}
	logging.Info("subscription.update", "updating subscription: %d", id)
	old, _ := svc.store.GetSubscription(id)
	item, err := svc.store.UpdateSubscription(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("subscription not found")
	}
	svc.refreshJob(id)
	if old != nil && old.URL != req.URL {
		logging.Info("subscription.update", "url changed for subscription %d, triggering sync", id)
		go svc.runSync(id)
	}
	return item, nil
}

func (svc *SubscriptionService) Delete(id int64) (*model.ActionResponse, error) {
	logging.Info("subscription.delete", "deleting subscription: %d", id)
	item, err := svc.store.GetSubscription(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("subscription not found")
	}
	if item.URL == "manual://local" {
		if err := svc.store.ClearSubscriptionNodes(id); err != nil {
			return nil, err
		}
		return &model.ActionResponse{Success: true, Message: "local subscription nodes cleared"}, nil
	}
	svc.removeJob(id)
	return &model.ActionResponse{Success: true, Message: "subscription deleted"}, svc.store.DeleteSubscription(id)
}

func (svc *SubscriptionService) Sync(id int64) (*model.ActionResponse, error) {
	logging.Info("subscription.sync", "syncing subscription: %d", id)
	item, err := svc.store.GetSubscription(id)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("subscription not found")
	}
	go svc.runSync(id)
	return &model.ActionResponse{Success: true, Message: "subscription sync started"}, nil
}

func (svc *SubscriptionService) SyncAll() (*model.ActionResponse, error) {
	logging.Info("subscription.sync_all", "syncing all subscriptions")
	items, err := svc.store.ListSubscriptions()
	if err != nil {
		return nil, err
	}
	go func() {
		total := 0
		failed := 0
		for _, item := range items {
			if item.URL == "manual://local" {
				continue
			}
			total++
			svc.runSync(item.ID)
			updated, getErr := svc.store.GetSubscription(item.ID)
			if getErr != nil || updated == nil || updated.SyncStatus != "updated" {
				failed++
			}
		}
		if svc.realtime != nil {
			svc.realtime.Broadcast("subscription.sync_all", map[string]any{
				"status": "completed",
				"total":  total,
				"failed": failed,
			})
		}
	}()
	return &model.ActionResponse{Success: true, Message: "subscriptions sync started"}, nil
}

func (svc *SubscriptionService) runSync(id int64) {
	if !svc.beginSync(id) {
		item, _ := svc.store.GetSubscription(id)
		if item != nil {
			svc.broadcastSync(id, "syncing", item.SyncProgress, "")
		}
		return
	}
	defer svc.endSync(id)

	item, err := svc.store.GetSubscription(id)
	if err != nil || item == nil {
		logging.Error("subscription.sync", "get subscription %d failed: %v", id, err)
		return
	}
	// 1. 记录同步前的节点 UID 列表
	if !svc.updateSyncProgress(id, 5) {
		return
	}
	oldUIDs, err := svc.store.GetSubscriptionNodeUIDs(id)
	if err != nil {
		logging.Error("subscription.sync", "get old UIDs for subscription %d failed: %v", id, err)
	}

	if !svc.updateSyncProgress(id, 15) {
		return
	}
	result, err := svc.fetchAndParse(item.URL, item.UserAgent, item.SyncTimeoutSecs)
	if err != nil {
		logging.Error("subscription.sync", "fetch subscription %d failed: %v", id, err)
		if setErr := svc.store.SetSubscriptionSyncState(id, "failed", 0); setErr != nil {
			logging.Error("subscription.sync", "set failed state failed: %v", setErr)
		}
		svc.broadcastSync(id, "failed", 0, err.Error())
		return
	}
	if !svc.updateSyncProgress(id, 50) {
		return
	}

	// 构建不支持协议的警告消息
	warningMsg := ""
	if len(result.UnsupportedCount) > 0 {
		totalUnsupported := 0
		var parts []string
		for typ, count := range result.UnsupportedCount {
			totalUnsupported += count
			parts = append(parts, fmt.Sprintf("%s: %d", typ, count))
		}
		warningMsg = fmt.Sprintf("已忽略 %d 个不支持的节点 (%s)", totalUnsupported, strings.Join(parts, ", "))
	}

	if err := svc.applyNodeFilters(result); err != nil {
		logging.Error("subscription.sync", "apply node filters for subscription %d failed: %v", id, err)
		if setErr := svc.store.SetSubscriptionSyncState(id, "failed", 0); setErr != nil {
			logging.Error("subscription.sync", "set failed state failed: %v", setErr)
		}
		svc.broadcastSync(id, "failed", 0, err.Error())
		return
	}
	if !svc.updateSyncProgress(id, 65) {
		return
	}
	if !svc.updateSyncProgress(id, 80) {
		return
	}
	if err := svc.store.ReplaceSubscriptionNodes(id, result.Nodes); err != nil {
		logging.Error("subscription.sync", "replace nodes failed: %v", err)
		if setErr := svc.store.SetSubscriptionSyncState(id, "failed", 0); setErr != nil {
			logging.Error("subscription.sync", "set failed state failed: %v", setErr)
		}
		svc.broadcastSync(id, "failed", 0, err.Error())
		return
	}

	// 2. 记录同步后的节点 UID 列表
	newUIDs, err := svc.store.GetSubscriptionNodeUIDs(id)
	if err != nil {
		logging.Error("subscription.sync", "get new UIDs for subscription %d failed: %v", id, err)
	}

	// 3. 对比变化
	added, removed := diffUIDs(oldUIDs, newUIDs)
	hasChanges := len(added) > 0 || len(removed) > 0

	if hasChanges {
		logging.Info("subscription.sync", "订阅 %d 节点变化：新增 %d，删除 %d", id, len(added), len(removed))

		// 4. 清理策略组中失效的节点引用
		if len(removed) > 0 {
			cleanedCount, err := svc.store.CleanInvalidNodeUIDs(removed)
			if err != nil {
				logging.Error("subscription.sync", "clean invalid node UIDs failed: %v", err)
			} else if cleanedCount > 0 {
				logging.Info("subscription.sync", "已清理 %d 个策略组的失效节点引用", cleanedCount)
			}
		}

		// 5. 自动将新增节点加入匹配的策略组
		if len(added) > 0 {
			addedCount, err := svc.store.AutoAddNewNodes(id, added)
			if err != nil {
				logging.Error("subscription.sync", "auto add new nodes failed: %v", err)
			} else if addedCount > 0 {
				logging.Info("subscription.sync", "已自动将新增节点加入 %d 个策略组", addedCount)
			}
		}

	} else {
		logging.Info("subscription.sync", "订阅 %d 节点无变化，跳过配置更新", id)
	}

	if !svc.updateSyncProgress(id, 95) {
		return
	}
	updated, err := svc.store.UpdateSubscriptionSyncResult(id, result.NodeCount, result.TrafficUsedBytes, result.TrafficTotalBytes, result.ExpireAt)
	if err != nil {
		logging.Error("subscription.sync", "update sync result failed: %v", err)
		if setErr := svc.store.SetSubscriptionSyncState(id, "failed", 0); setErr != nil {
			logging.Error("subscription.sync", "set failed state failed: %v", setErr)
		}
		svc.broadcastSync(id, "failed", 0, err.Error())
		return
	}
	if warningMsg != "" {
		svc.broadcastSubscriptionWithWarning(updated, "updated", 100, warningMsg)
	} else {
		svc.broadcastSubscription(updated, "updated", 100)
	}
	if hasChanges && svc.reconciler != nil {
		svc.reconciler.Trigger("subscription.sync")
	}
}

func (svc *SubscriptionService) beginSync(id int64) bool {
	svc.syncMu.Lock()
	defer svc.syncMu.Unlock()
	if svc.syncing[id] {
		logging.Info("subscription.sync", "subscription %d already syncing, skip", id)
		return false
	}
	svc.syncing[id] = true
	return true
}

func (svc *SubscriptionService) endSync(id int64) {
	svc.syncMu.Lock()
	delete(svc.syncing, id)
	svc.syncMu.Unlock()
}

func (svc *SubscriptionService) updateSyncProgress(id int64, progress float64) bool {
	if err := svc.store.SetSubscriptionSyncState(id, "syncing", progress); err != nil {
		logging.Error("subscription.sync", "set sync progress failed: %v", err)
		return false
	}
	svc.broadcastSync(id, "syncing", progress, "")
	return true
}

func (svc *SubscriptionService) applyNodeFilters(result *subscriptionSyncResult) error {
	filters, err := svc.store.ListEnabledNodeFilters()
	if err != nil {
		return err
	}
	if len(filters) == 0 || len(result.Nodes) == 0 {
		return nil
	}
	compiled := make([]compiledNodeFilter, 0, len(filters))
	for _, filter := range filters {
		re, err := regexp.Compile(filter.Pattern)
		if err != nil {
			return fmt.Errorf("invalid node filter %s: %w", filter.Name, err)
		}
		compiled = append(compiled, compiledNodeFilter{filter: filter, regex: re})
	}
	kept := make([]model.ParsedNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		if nodeFiltered(node, compiled) {
			continue
		}
		kept = append(kept, node)
	}
	if len(kept) == 0 {
		return fmt.Errorf("all parsed nodes were filtered by rules")
	}
	result.Nodes = kept
	result.NodeCount = len(kept)
	return nil
}

type compiledNodeFilter struct {
	filter model.NodeFilter
	regex  *regexp.Regexp
}

func nodeFiltered(node model.ParsedNode, filters []compiledNodeFilter) bool {
	for _, filter := range filters {
		if filter.regex.MatchString(nodeFilterValue(node, filter.filter.Target)) {
			return true
		}
	}
	return false
}

func nodeFilterValue(node model.ParsedNode, target string) string {
	switch target {
	case "name":
		return node.Name
	case "type":
		return node.Type
	case "server":
		return node.Server
	case "raw":
		return node.Raw
	case "raw_json":
		return node.RawJSON
	case "all":
		fallthrough
	default:
		return strings.Join([]string{node.Name, node.Type, node.Server, node.Raw, node.RawJSON}, "\n")
	}
}

func (svc *SubscriptionService) broadcastSync(id int64, status string, progress float64, message string) {
	if svc.realtime == nil {
		return
	}
	data := map[string]any{
		"id":       id,
		"status":   status,
		"progress": progress,
	}
	if message != "" {
		data["error"] = message
	}
	svc.realtime.Broadcast("subscription.sync", data)
}

func (svc *SubscriptionService) broadcastSubscription(item *model.Subscription, status string, progress float64) {
	if svc.realtime == nil || item == nil {
		return
	}
	svc.realtime.Broadcast("subscription.sync", map[string]any{
		"id":                  item.ID,
		"status":              status,
		"progress":            progress,
		"node_count":          item.NodeCount,
		"traffic_used_bytes":  item.TrafficUsedBytes,
		"traffic_total_bytes": item.TrafficTotalBytes,
		"expire_at":           item.ExpireAt,
		"last_sync_at":        item.LastSyncAt,
	})
}

func (svc *SubscriptionService) broadcastSubscriptionWithWarning(item *model.Subscription, status string, progress float64, warning string) {
	if svc.realtime == nil || item == nil {
		return
	}
	svc.realtime.Broadcast("subscription.sync", map[string]any{
		"id":                  item.ID,
		"status":              status,
		"progress":            progress,
		"node_count":          item.NodeCount,
		"traffic_used_bytes":  item.TrafficUsedBytes,
		"traffic_total_bytes": item.TrafficTotalBytes,
		"expire_at":           item.ExpireAt,
		"last_sync_at":        item.LastSyncAt,
		"warning":             warning,
	})
}

func (svc *SubscriptionService) fetchAndParse(rawURL string, userAgent string, timeoutSeconds int) (*subscriptionSyncResult, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 60
	}
	client := &http.Client{Timeout: time.Duration(timeoutSeconds) * time.Second}
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("subscription http status: %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, err
	}

	result := parseSubscriptionUserInfo(resp.Header.Get("Subscription-Userinfo"))
	nodes, err := parser.ParseSubscriptionNodes(body)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("subscription contains no supported nodes")
	}

	// 过滤不支持的协议和无法安全等价转换的 Clash 协议变体。
	supportedNodes := make([]model.ParsedNode, 0, len(nodes))
	unsupportedCount := map[string]int{}

	for _, node := range nodes {
		if isUnsupportedNodeType(node.Type) || node.UnsupportedReason != "" {
			unsupportedCount[node.Type]++
			if node.UnsupportedReason != "" {
				logging.Info("subscription.parse", "filtered %s node: %s", node.Type, node.UnsupportedReason)
			}
		} else {
			supportedNodes = append(supportedNodes, node)
		}
	}

	// 记录被过滤的节点
	if len(unsupportedCount) > 0 {
		totalUnsupported := 0
		for typ, count := range unsupportedCount {
			totalUnsupported += count
			logging.Info("subscription.parse", "filtered %d %s nodes (unsupported protocol)", count, typ)
		}
		result.UnsupportedCount = unsupportedCount
	}

	if len(supportedNodes) == 0 {
		return nil, fmt.Errorf("subscription contains no supported nodes after filtering")
	}

	result.Nodes = supportedNodes
	result.NodeCount = len(supportedNodes)
	return result, nil
}

func parseSubscriptionUserInfo(value string) *subscriptionSyncResult {
	result := &subscriptionSyncResult{}
	for _, part := range strings.Split(value, ";") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.ParseInt(strings.TrimSpace(kv[1]), 10, 64)
		if err != nil {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(kv[0])) {
		case "upload":
			result.TrafficUsedBytes += v
		case "download":
			result.TrafficUsedBytes += v
		case "total":
			result.TrafficTotalBytes = v
		case "expire":
			result.ExpireAt = v * 1000
		}
	}
	return result
}

func parseSyncTime(s string) (minute, hour int, ok bool) {
	parts := strings.SplitN(s, ":", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return m, h, true
}

func validateSubscription(req *model.SubscriptionRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.URL = strings.TrimSpace(req.URL)
	req.UserAgent = strings.TrimSpace(req.UserAgent)
	if req.UserAgent == "" {
		req.UserAgent = "clash-meta/2.4.0"
	}
	if req.Name == "" {
		return fmt.Errorf("subscription name is required")
	}
	if req.URL == "" {
		return fmt.Errorf("subscription url is required")
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		return fmt.Errorf("subscription url must start with http:// or https://")
	}
	if req.SyncIntervalMins < 0 {
		return fmt.Errorf("sync interval must be greater than or equal to 0")
	}
	if req.SyncTimeoutSecs <= 0 {
		req.SyncTimeoutSecs = 60
	}
	if req.SyncTimeoutSecs < 5 || req.SyncTimeoutSecs > 300 {
		return fmt.Errorf("sync timeout must be between 5 and 300 seconds")
	}
	req.SyncMode = strings.TrimSpace(req.SyncMode)
	if req.SyncMode == "" {
		req.SyncMode = "off"
	}
	switch req.SyncMode {
	case "off":
		req.SyncTime = ""
		req.SyncWeekday = 0
	case "daily":
		if !syncTimePattern.MatchString(req.SyncTime) {
			return fmt.Errorf("sync time must use HH:mm:ss")
		}
		req.SyncWeekday = 0
	case "weekly":
		if !syncTimePattern.MatchString(req.SyncTime) {
			return fmt.Errorf("sync time must use HH:mm:ss")
		}
		if req.SyncWeekday < 1 || req.SyncWeekday > 7 {
			return fmt.Errorf("sync weekday must be 1-7")
		}
	case "monthly":
		if !syncTimePattern.MatchString(req.SyncTime) {
			return fmt.Errorf("sync time must use HH:mm:ss")
		}
		if req.SyncWeekday < 1 || req.SyncWeekday > 31 {
			return fmt.Errorf("sync day of month must be 1-31")
		}
	default:
		return fmt.Errorf("sync mode must be off, daily, weekly, or monthly")
	}
	return nil
}

// diffUIDs 对比两个 UID 列表的差异
func diffUIDs(oldUIDs, newUIDs []string) (added, removed []string) {
	oldSet := make(map[string]bool)
	for _, uid := range oldUIDs {
		oldSet[uid] = true
	}

	newSet := make(map[string]bool)
	for _, uid := range newUIDs {
		newSet[uid] = true
	}

	// 找出新增的
	for _, uid := range newUIDs {
		if !oldSet[uid] {
			added = append(added, uid)
		}
	}

	// 找出删除的
	for _, uid := range oldUIDs {
		if !newSet[uid] {
			removed = append(removed, uid)
		}
	}

	return added, removed
}
