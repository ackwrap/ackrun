package service

import (
	"encoding/json"
	"fmt"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

// ProxyCollectionService 代理集合服务
type ProxyCollectionService struct {
	store *store.Store
}

// NewProxyCollectionService 创建代理集合服务
func NewProxyCollectionService(store *store.Store) *ProxyCollectionService {
	return &ProxyCollectionService{store: store}
}

// Create 创建代理集合
func (s *ProxyCollectionService) Create(req model.ProxyCollectionRequest) (*model.ProxyCollectionWithNodes, error) {
	logging.Info("proxy_collection.create", "创建代理集合: %s", req.Name)

	// 验证类型
	if req.Type != "selector" && req.Type != "urltest" && req.Type != "fallback" {
		return nil, fmt.Errorf("无效的集合类型: %s", req.Type)
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

	// 验证类型
	if req.Type != "selector" && req.Type != "urltest" && req.Type != "fallback" {
		return fmt.Errorf("无效的集合类型: %s", req.Type)
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

	return s.store.UpdateProxyCollection(id, pc)
}

// Delete 删除代理集合
func (s *ProxyCollectionService) Delete(id int) error {
	logging.Info("proxy_collection.delete", "删除代理集合: %d", id)
	return s.store.DeleteProxyCollection(id)
}

// ToggleEnabled 切换启用状态
func (s *ProxyCollectionService) ToggleEnabled(id int) error {
	pc, err := s.store.GetProxyCollection(id)
	if err != nil {
		return err
	}

	pc.Enabled = !pc.Enabled
	return s.store.UpdateProxyCollection(id, pc)
}
