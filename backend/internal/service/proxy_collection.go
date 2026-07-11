package service

import (
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
	mu           sync.Mutex
	runningTests map[int]bool
	httpClient   *http.Client
	clashBaseURL string
}

var ErrSystemProxyCollectionProtected = errors.New("系统默认策略组不可编辑")

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
	logging.Info("proxy_collection.create", "创建代理集合: %s", req.Name)
	if IsSystemProxyCollectionName(req.Name) {
		return nil, ErrSystemProxyCollectionProtected
	}

	// 验证类型
	if !isSupportedGroupType(req.Type) {
		return nil, fmt.Errorf("无效的集合类型: %s", req.Type)
	}
	if err := normalizeCollectionHealthSettings(&req); err != nil {
		return nil, err
	}

	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}

	// 验证节点数量（manual 模式）
	if sourceType == "manual" && len(req.NodeUIDs) == 0 {
		return nil, fmt.Errorf("至少需要选择一个节点")
	}

	// 验证节点组引用（node_groups 模式）
	if sourceType == "node_groups" && len(req.ReferencedGroupIDs) == 0 {
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
		RouteRuleIDs:       string(routeRuleIDsJSON),
		NodeUIDs:           string(nodeUIDsJSON),
		TestURL:            req.TestURL,
		TestInterval:       req.TestInterval,
		Tolerance:          req.Tolerance,
		Enabled:            req.Enabled,
	}

	if err := s.store.CreateProxyCollection(pc); err != nil {
		return nil, err
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

// Update 更新代理集合
func (s *ProxyCollectionService) Update(id int, req model.ProxyCollectionRequest) error {
	logging.Info("proxy_collection.update", "更新代理集合: %d", id)
	existing, err := s.store.GetProxyCollection(id)
	if err != nil {
		return err
	}
	if IsSystemProxyCollectionName(existing.Name) || IsSystemProxyCollectionName(req.Name) {
		return ErrSystemProxyCollectionProtected
	}

	// 验证类型
	if !isSupportedGroupType(req.Type) {
		return fmt.Errorf("无效的集合类型: %s", req.Type)
	}
	if err := normalizeCollectionHealthSettings(&req); err != nil {
		return err
	}

	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}

	// 验证节点数量（manual 模式）
	if sourceType == "manual" && len(req.NodeUIDs) == 0 {
		return fmt.Errorf("至少需要选择一个节点")
	}

	// 验证节点组引用（node_groups 模式）
	if sourceType == "node_groups" && len(req.ReferencedGroupIDs) == 0 {
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
		RouteRuleIDs:       string(routeRuleIDsJSON),
		NodeUIDs:           string(nodeUIDsJSON),
		TestURL:            req.TestURL,
		TestInterval:       req.TestInterval,
		Tolerance:          req.Tolerance,
		Enabled:            req.Enabled,
	}

	if err := s.store.UpdateProxyCollection(id, pc); err != nil {
		return err
	}
	s.refreshHealthCheckJob(id)
	return nil
}

func isSupportedGroupType(groupType string) bool {
	return groupType == "selector" || groupType == "urltest"
}

// Delete 删除代理集合
func (s *ProxyCollectionService) Delete(id int) error {
	logging.Info("proxy_collection.delete", "删除代理集合: %d", id)
	existing, err := s.store.GetProxyCollection(id)
	if err != nil {
		return err
	}
	if IsSystemProxyCollectionName(existing.Name) {
		return ErrSystemProxyCollectionProtected
	}
	if err := s.store.DeleteProxyCollection(id); err != nil {
		return err
	}
	s.removeHealthCheckJob(id)
	return nil
}

// ToggleEnabled 切换启用状态
func (s *ProxyCollectionService) ToggleEnabled(id int) error {
	pc, err := s.store.GetProxyCollection(id)
	if err != nil {
		return err
	}
	if IsSystemProxyCollectionName(pc.Name) {
		return ErrSystemProxyCollectionProtected
	}

	pc.Enabled = !pc.Enabled
	if err := s.store.UpdateProxyCollection(id, pc); err != nil {
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
	switch name {
	case "全球直连", "应用净化":
		return true
	default:
		return false
	}
}
