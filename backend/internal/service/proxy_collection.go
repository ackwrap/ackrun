package service

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
	"github.com/robfig/cron/v3"
)

// ProxyCollectionService 代理集合服务
type ProxyCollectionService struct {
	store        *store.Store
	realtime     *RealtimeService
	cron         *cron.Cron
	entries      map[int]cron.EntryID
	healthJobsMu sync.Mutex
	mu           sync.Mutex
	runningTests map[int]bool
	httpClient   *http.Client
	clashBaseURL string
}

var (
	ErrSystemProxyCollectionProtected     = errors.New("系统默认策略组不可编辑")
	ErrProxyCollectionRuleBindingInvalid  = errors.New("代理规则绑定无效")
	ErrProxyCollectionRuleNotFound        = errors.New("绑定的路由规则不存在")
	ErrProxyCollectionRuleBindingConflict = errors.New("代理规则绑定冲突")
	ErrProxyCollectionNotFound            = errors.New("代理策略配置不存在")
)

const (
	proxyCollectionSourceManual             = "manual"
	proxyCollectionSourceNodeGroups         = "node_groups"
	proxyCollectionSourceNodeGroupsAndNodes = "node_groups_and_nodes"
)

// NewProxyCollectionService 创建代理集合服务
func NewProxyCollectionService(store *store.Store, realtime *RealtimeService) *ProxyCollectionService {
	return &ProxyCollectionService{
		store:        store,
		realtime:     realtime,
		cron:         cron.New(cron.WithSeconds()),
		entries:      make(map[int]cron.EntryID),
		runningTests: make(map[int]bool),
		httpClient:   &http.Client{Timeout: 10 * time.Second},
	}
}

// Create 创建代理集合
func (s *ProxyCollectionService) Create(req model.ProxyCollectionRequest) (*model.ProxyCollectionWithNodes, error) {
	if err := s.bindCollectionRequestToProxyRule(&req, 0); err != nil {
		return nil, err
	}
	logging.Info("proxy_collection.create", "创建代理集合: %s", req.Name)
	if IsReservedProxyCollectionName(req.Name) {
		return nil, ErrSystemProxyCollectionProtected
	}

	// 验证类型
	if !isSupportedGroupType(req.Type) {
		return nil, fmt.Errorf("无效的集合类型: %s", req.Type)
	}
	if err := normalizeCollectionHealthSettings(&req); err != nil {
		return nil, err
	}

	sourceType, err := normalizeCollectionSourceType(req.SourceType)
	if err != nil {
		return nil, err
	}

	// 验证节点数量（manual 模式）
	if sourceType == proxyCollectionSourceManual && len(req.NodeUIDs) == 0 {
		return nil, fmt.Errorf("至少需要选择一个节点")
	}

	// 验证节点组引用
	if isCollectionGroupSource(sourceType) && len(req.ReferencedGroupIDs) == 0 {
		return nil, fmt.Errorf("至少需要引用一个节点组")
	}

	referencedGroupIDsJSON, _ := json.Marshal(req.ReferencedGroupIDs)
	routeRuleIDsJSON, _ := json.Marshal(req.RouteRuleIDs)
	nodeUIDsJSON, _ := json.Marshal(req.NodeUIDs)

	// 创建集合
	pc := &model.ProxyCollection{
		Name:               req.Name,
		Type:               req.Type,
		SourceType:         sourceType,
		ReferencedGroupIDs: string(referencedGroupIDsJSON),
		RouteRuleID:        req.RouteRuleID,
		RouteRuleIDs:       string(routeRuleIDsJSON),
		NodeUIDs:           string(nodeUIDsJSON),
		TestURL:            req.TestURL,
		TestInterval:       req.TestInterval,
		Tolerance:          req.Tolerance,
		Enabled:            req.Enabled,
	}

	if err := s.store.CreateProxyCollection(pc); err != nil {
		return nil, normalizeProxyCollectionRuleBindingError(err)
	}
	s.refreshHealthCheckJob(pc.ID)

	return s.store.GetProxyCollectionWithNodes(pc.ID)
}

// Get 获取代理集合
func (s *ProxyCollectionService) Get(id int) (*model.ProxyCollectionWithNodes, error) {
	return s.store.GetProxyCollectionWithNodes(id)
}

// List 列出所有代理集合
func (s *ProxyCollectionService) List() ([]*model.ProxyCollectionWithNodes, error) {
	return s.store.ListProxyCollectionsWithNodes()
}

func (s *ProxyCollectionService) Reorder(ids []int) error {
	if len(ids) == 0 {
		return fmt.Errorf("策略组 ID 不能为空")
	}
	seen := make(map[int]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("策略组 ID 无效或重复")
		}
		seen[id] = true
	}
	logging.Info("proxy_collection.reorder", "调整 %d 个策略组的顺序", len(ids))
	return s.store.ReorderProxyCollections(ids)
}

// Update 更新代理集合
func (s *ProxyCollectionService) Update(id int, req model.ProxyCollectionRequest) error {
	logging.Info("proxy_collection.update", "更新代理集合: %d", id)
	existing, err := s.store.GetProxyCollection(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return err
	}
	if IsSystemProxyCollectionName(existing.Name) {
		return ErrSystemProxyCollectionProtected
	}
	if err := s.bindCollectionRequestToProxyRule(&req, id); err != nil {
		return err
	}
	if IsReservedProxyCollectionName(req.Name) {
		return ErrSystemProxyCollectionProtected
	}

	// 验证类型
	if !isSupportedGroupType(req.Type) {
		return fmt.Errorf("无效的集合类型: %s", req.Type)
	}
	if err := normalizeCollectionHealthSettings(&req); err != nil {
		return err
	}

	sourceType, err := normalizeCollectionSourceType(req.SourceType)
	if err != nil {
		return err
	}

	// 验证节点数量（manual 模式）
	if sourceType == proxyCollectionSourceManual && len(req.NodeUIDs) == 0 {
		return fmt.Errorf("至少需要选择一个节点")
	}

	// 验证节点组引用
	if isCollectionGroupSource(sourceType) && len(req.ReferencedGroupIDs) == 0 {
		return fmt.Errorf("至少需要引用一个节点组")
	}

	referencedGroupIDsJSON, _ := json.Marshal(req.ReferencedGroupIDs)
	routeRuleIDsJSON, _ := json.Marshal(req.RouteRuleIDs)
	nodeUIDsJSON, _ := json.Marshal(req.NodeUIDs)

	// 更新集合
	pc := &model.ProxyCollection{
		Name:               req.Name,
		Type:               req.Type,
		SourceType:         sourceType,
		ReferencedGroupIDs: string(referencedGroupIDsJSON),
		RouteRuleID:        req.RouteRuleID,
		RouteRuleIDs:       string(routeRuleIDsJSON),
		NodeUIDs:           string(nodeUIDsJSON),
		TestURL:            req.TestURL,
		TestInterval:       req.TestInterval,
		Tolerance:          req.Tolerance,
		Enabled:            req.Enabled,
	}

	if err := s.store.UpdateProxyCollection(id, pc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return normalizeProxyCollectionRuleBindingError(err)
	}
	s.refreshHealthCheckJob(id)
	return nil
}

func (s *ProxyCollectionService) bindCollectionRequestToProxyRule(req *model.ProxyCollectionRequest, collectionID int) error {
	routeRuleID := req.RouteRuleID
	if routeRuleID <= 0 {
		return fmt.Errorf("%w: 代理集合必须绑定且只能绑定一条代理规则", ErrProxyCollectionRuleBindingInvalid)
	}
	rule, err := s.store.GetRouteRule(routeRuleID)
	if err != nil {
		return err
	}
	if rule == nil {
		return ErrProxyCollectionRuleNotFound
	}
	if rule.IsSystem || rule.Outbound != "proxy" {
		return fmt.Errorf("%w: 代理集合只能绑定非系统代理规则", ErrProxyCollectionRuleBindingInvalid)
	}
	bound, err := s.store.GetProxyCollectionByRouteRuleID(routeRuleID)
	if err != nil {
		return err
	}
	if bound != nil && bound.ID != collectionID {
		return fmt.Errorf("%w: 代理规则已绑定到其他集合", ErrProxyCollectionRuleBindingConflict)
	}
	req.RouteRuleID = routeRuleID
	req.RouteRuleIDs = []int64{routeRuleID}
	req.Name = rule.Name
	if IsReservedProxyCollectionName(req.Name) {
		return ErrSystemProxyCollectionProtected
	}
	return s.validateBoundCollectionOutboundName(req.Name, collectionID)
}

func (s *ProxyCollectionService) validateBoundCollectionOutboundName(name string, collectionID int) error {
	groups, err := s.store.ListNodeGroups()
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.Enabled && strings.TrimSpace(group.Name) == strings.TrimSpace(name) {
			return fmt.Errorf("%w: 策略组名称 %q 与已启用节点组的 outbound tag 冲突", ErrProxyCollectionRuleBindingConflict, name)
		}
	}
	collections, err := s.store.ListProxyCollections()
	if err != nil {
		return err
	}
	for _, collection := range collections {
		if collection.ID != collectionID && collection.Enabled && strings.TrimSpace(collection.Name) == strings.TrimSpace(name) {
			return fmt.Errorf("%w: 策略组名称 %q 与已启用策略组的 outbound tag 冲突", ErrProxyCollectionRuleBindingConflict, name)
		}
	}
	return nil
}

func normalizeProxyCollectionRuleBindingError(err error) error {
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unique constraint") && strings.Contains(message, "proxy_collections.route_rule_id") {
		return fmt.Errorf("%w: 代理规则已绑定到其他集合", ErrProxyCollectionRuleBindingConflict)
	}
	return err
}

func isSupportedGroupType(groupType string) bool {
	return groupType == "selector" || groupType == "urltest"
}

func normalizeCollectionSourceType(sourceType string) (string, error) {
	sourceType = strings.TrimSpace(sourceType)
	if sourceType == "" {
		return proxyCollectionSourceManual, nil
	}
	switch sourceType {
	case proxyCollectionSourceManual, proxyCollectionSourceNodeGroups, proxyCollectionSourceNodeGroupsAndNodes:
		return sourceType, nil
	default:
		return "", fmt.Errorf("无效的节点来源: %s", sourceType)
	}
}

func isCollectionGroupSource(sourceType string) bool {
	return sourceType == proxyCollectionSourceNodeGroups || sourceType == proxyCollectionSourceNodeGroupsAndNodes
}

// Delete 删除代理集合
func (s *ProxyCollectionService) Delete(id int) error {
	logging.Info("proxy_collection.delete", "删除代理集合: %d", id)
	existing, err := s.store.GetProxyCollection(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return err
	}
	if IsSystemProxyCollectionName(existing.Name) {
		return ErrSystemProxyCollectionProtected
	}
	if err := s.store.DeleteProxyCollection(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return err
	}
	s.removeHealthCheckJob(id)
	return nil
}

// ToggleEnabled 切换启用状态
func (s *ProxyCollectionService) ToggleEnabled(id int) error {
	pc, err := s.store.GetProxyCollection(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return err
	}
	if IsSystemProxyCollectionName(pc.Name) {
		return ErrSystemProxyCollectionProtected
	}

	pc.Enabled = !pc.Enabled
	if err := s.store.UpdateProxyCollection(id, pc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrProxyCollectionNotFound
		}
		return err
	}
	s.refreshHealthCheckJob(id)
	return nil
}

func normalizeCollectionHealthSettings(req *model.ProxyCollectionRequest) error {
	if req.TestURL == "" {
		req.TestURL = "https://www.gstatic.com/generate_204"
	}
	parsed, err := url.ParseRequestURI(strings.TrimSpace(req.TestURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("测速 URL 必须是有效的 http/https URL")
	}
	req.TestURL = parsed.String()
	if req.TestInterval == 0 {
		req.TestInterval = 300
	}
	if req.TestInterval < 60 || req.TestInterval > 3600 {
		return fmt.Errorf("测速间隔必须在 60 到 3600 秒之间")
	}
	if req.Tolerance < 0 || req.Tolerance > 1000 {
		return fmt.Errorf("测速容差必须在 0 到 1000 毫秒之间")
	}
	return nil
}

func IsSystemProxyCollectionName(name string) bool {
	switch strings.TrimSpace(name) {
	case "全球直连":
		return true
	default:
		return false
	}
}

func IsReservedProxyCollectionName(name string) bool {
	return IsSystemProxyCollectionName(name)
}
