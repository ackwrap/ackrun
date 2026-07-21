package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

// NodeGroup CRUD

func (s *Store) ListNodeGroups() ([]model.NodeGroupWithStats, error) {
	rows, err := s.db.Query(`SELECT id, name, type, filter_protocols, filter_subscriptions, filter_include, filter_exclude, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at FROM node_groups ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.NodeGroupWithStats
	for rows.Next() {
		var g model.NodeGroup
		var enabled int
		if err := rows.Scan(&g.ID, &g.Name, &g.Type, &g.FilterProtocols, &g.FilterSubscriptions, &g.FilterInclude, &g.FilterExclude, &g.NodeUIDs, &g.TestURL, &g.TestInterval, &g.Tolerance, &enabled, &g.Priority, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.Enabled = enabled == 1

		// 计算匹配节点数
		matchedCount := s.countMatchedNodes(g.NodeUIDs, g.FilterProtocols, g.FilterSubscriptions, g.FilterInclude, g.FilterExclude)

		groups = append(groups, model.NodeGroupWithStats{
			NodeGroup:        g,
			MatchedNodeCount: matchedCount,
		})
	}
	return groups, nil
}

func (s *Store) GetNodeGroup(id int64) (*model.NodeGroup, error) {
	var g model.NodeGroup
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, type, filter_protocols, filter_subscriptions, filter_include, filter_exclude, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at FROM node_groups WHERE id = ?`, id).
		Scan(&g.ID, &g.Name, &g.Type, &g.FilterProtocols, &g.FilterSubscriptions, &g.FilterInclude, &g.FilterExclude, &g.NodeUIDs, &g.TestURL, &g.TestInterval, &g.Tolerance, &enabled, &g.Priority, &g.CreatedAt, &g.UpdatedAt)
	if err != nil {
		return nil, err
	}
	g.Enabled = enabled == 1
	return &g, nil
}

func (s *Store) CreateNodeGroup(req *model.NodeGroupRequest) (*model.NodeGroup, error) {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	now := time.Now().Unix()
	testURL := req.TestURL
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	testInterval := req.TestInterval
	if testInterval == 0 {
		testInterval = 300
	}
	tolerance := req.Tolerance
	if tolerance == 0 {
		tolerance = 100
	}

	nodeUIDsJSON := marshalNodeUIDs(req.NodeUIDs)
	result, err := s.db.Exec(`INSERT INTO node_groups (name, type, filter_protocols, filter_subscriptions, filter_include, filter_exclude, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.Type, req.FilterProtocols, req.FilterSubscriptions, req.FilterInclude, req.FilterExclude, nodeUIDsJSON, testURL, testInterval, tolerance, req.Enabled, req.Priority, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetNodeGroup(id)
}

func (s *Store) UpdateNodeGroup(id int64, req *model.NodeGroupRequest) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	now := time.Now().Unix()
	testURL := req.TestURL
	if testURL == "" {
		testURL = "https://www.gstatic.com/generate_204"
	}
	testInterval := req.TestInterval
	if testInterval == 0 {
		testInterval = 300
	}
	tolerance := req.Tolerance
	if tolerance == 0 {
		tolerance = 100
	}

	nodeUIDsJSON := marshalNodeUIDs(req.NodeUIDs)
	_, err := s.db.Exec(`UPDATE node_groups SET name = ?, type = ?, filter_protocols = ?, filter_subscriptions = ?, filter_include = ?, filter_exclude = ?, node_uids = ?, test_url = ?, test_interval = ?, tolerance = ?, enabled = ?, priority = ?, updated_at = ? WHERE id = ?`,
		req.Name, req.Type, req.FilterProtocols, req.FilterSubscriptions, req.FilterInclude, req.FilterExclude, nodeUIDsJSON, testURL, testInterval, tolerance, req.Enabled, req.Priority, now, id)
	return err
}

func (s *Store) UpdateNodeGroupFilters(id int64, filterProtocols, filterSubscriptions string) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	result, err := s.db.Exec(`UPDATE node_groups SET filter_protocols = ?, filter_subscriptions = ?, updated_at = ? WHERE id = ?`,
		filterProtocols, filterSubscriptions, time.Now().Unix(), id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteNodeGroup(id int64) error {
	return s.DeleteNodeGroups([]int64{id})
}

func (s *Store) DeleteNodeGroups(ids []int64) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()
	return s.deleteNodeGroups(ids)
}

func (s *Store) deleteNodeGroups(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, id := range ids {
		if _, err := tx.Exec(`DELETE FROM node_groups WHERE id = ?`, id); err != nil {
			return err
		}
	}
	remove := make(map[int64]bool, len(ids))
	for _, id := range ids {
		remove[id] = true
	}
	if _, err := updateIntJSONRefsTx(tx, "proxy_collections", "referenced_group_ids", remove); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteEmptyNodeGroups() ([]int64, error) {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	groups, err := s.ListNodeGroups()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for _, group := range groups {
		if group.MatchedNodeCount == 0 {
			ids = append(ids, group.ID)
		}
	}
	if err := s.deleteNodeGroups(ids); err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *Store) ReorderNodeGroups(ids []int64) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "node_groups", ids); err != nil {
		return err
	}

	for priority, id := range ids {
		if _, err := tx.Exec(`UPDATE node_groups SET priority = ? WHERE id = ?`, priority, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// PreviewNodeGroupMatches 预览节点组匹配的节点
func (s *Store) PreviewNodeGroupMatches(filterProtocols, filterSubscriptions, filterInclude, filterExclude string) ([]model.Node, error) {
	nodes, err := s.ListEnabledNodes()
	if err != nil {
		return nil, err
	}

	return s.filterNodes(nodes, filterProtocols, filterSubscriptions, filterInclude, filterExclude), nil
}

func (s *Store) PreviewNodeGroupManualMatches(nodeUIDs string) ([]model.Node, error) {
	nodes, err := s.ListEnabledNodes()
	if err != nil {
		return nil, err
	}
	return filterNodesByUIDs(nodes, nodeUIDs)
}

// countMatchedNodes 计算匹配节点数
func (s *Store) countMatchedNodes(nodeUIDs, filterProtocols, filterSubscriptions, filterInclude, filterExclude string) int {
	nodes, err := s.ListEnabledNodes()
	if err != nil {
		return 0
	}
	if hasManualNodeUIDs(nodeUIDs) {
		matched, err := filterNodesByUIDs(nodes, nodeUIDs)
		if err != nil {
			return 0
		}
		return len(matched)
	}
	return len(s.filterNodes(nodes, filterProtocols, filterSubscriptions, filterInclude, filterExclude))
}

func hasManualNodeUIDs(nodeUIDs string) bool {
	value := strings.TrimSpace(nodeUIDs)
	return value != "" && value != "[]" && value != "null"
}

func marshalNodeUIDs(uids []string) string {
	if len(uids) == 0 {
		return "[]"
	}
	data, err := json.Marshal(uids)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func filterNodesByUIDs(nodes []model.Node, nodeUIDs string) ([]model.Node, error) {
	var uids []string
	if err := json.Unmarshal([]byte(nodeUIDs), &uids); err != nil {
		return nil, err
	}
	selected := make(map[string]bool, len(uids))
	for _, uid := range uids {
		uid = strings.TrimSpace(uid)
		if uid != "" {
			selected[uid] = true
		}
	}
	result := make([]model.Node, 0, len(selected))
	for _, node := range nodes {
		if selected[node.UID] {
			result = append(result, node)
		}
	}
	return result, nil
}

// ListEnabledNodes returns every enabled node without applying API pagination.
func (s *Store) ListEnabledNodes() ([]model.Node, error) {
	rows, err := s.db.Query(`
		SELECT n.id, n.uid, n.subscription_id, COALESCE(s.name, '') AS subscription_name,
			n.name, n.name_overridden, n.type, n.server, n.server_port, n.raw, n.raw_json, n.enabled, n.preferred,
			n.latency_ms, n.status, n.last_test_at, n.test_latency_ms, n.test_success, n.created_at, n.updated_at
		FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id
		WHERE n.enabled = 1
		ORDER BY n.updated_at DESC, n.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Node, 0)
	for rows.Next() {
		var item model.Node
		if err := scanNode(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// filterNodes 根据多维度 filter 筛选节点
func (s *Store) filterNodes(nodes []model.Node, filterProtocols, filterSubscriptions, filterInclude, filterExclude string) []model.Node {
	var result []model.Node

	// 解析协议列表
	var protocolList []string
	if filterProtocols != "" {
		protocolList = strings.Split(filterProtocols, ",")
	}

	// 解析订阅 ID 列表
	var subscriptionList []string
	if filterSubscriptions != "" {
		subscriptionList = strings.Split(filterSubscriptions, ",")
	}

	// 解析关键词
	includeKeywords := strings.Split(filterInclude, "|")
	excludeKeywords := []string{}
	if filterExclude != "" {
		excludeKeywords = strings.Split(filterExclude, "|")
	}

	for _, node := range nodes {
		// 1. 先按订阅筛选
		if len(subscriptionList) > 0 {
			matched := false
			subscriptionIDStr := fmt.Sprintf("%d", node.SubscriptionID)
			for _, subID := range subscriptionList {
				if strings.TrimSpace(subID) == subscriptionIDStr {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// 2. 再按协议筛选
		if len(protocolList) > 0 {
			matched := false
			for _, protocol := range protocolList {
				if strings.TrimSpace(protocol) == node.Type {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}

		// 3. 先按排除关键词筛选。排除优先，命中任意排除词就直接跳过。
		excludeMatched := false
		for _, keyword := range excludeKeywords {
			keyword = strings.TrimSpace(keyword)
			if keyword == "" {
				continue
			}
			if nodeNameKeywordMatched(node.Name, keyword) {
				excludeMatched = true
				break
			}
		}
		if excludeMatched {
			continue
		}

		// 4. 再按包含关键词筛选。包含词为空表示不过滤；否则命中任意包含词即可。
		if filterInclude != "" {
			includeMatched := false
			for _, keyword := range includeKeywords {
				keyword = strings.TrimSpace(keyword)
				if keyword == "" {
					continue
				}
				if nodeNameKeywordMatched(node.Name, keyword) {
					includeMatched = true
					break
				}
			}
			if !includeMatched {
				continue
			}
		}

		result = append(result, node)
	}

	return result
}

func nodeNameKeywordMatched(name, keyword string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	if keyword == "*" || keyword == ".*" {
		return true
	}

	// 两到三位英文地区短码使用边界匹配，避免 HK 命中 SHK、US 命中 user。
	if isShortASCIIKeyword(keyword) {
		return boundedASCIIContains(name, keyword)
	}

	return strings.Contains(name, keyword)
}

func isShortASCIIKeyword(keyword string) bool {
	if len(keyword) < 2 || len(keyword) > 3 {
		return false
	}
	for _, r := range keyword {
		if r < 'a' || r > 'z' {
			return false
		}
	}
	return true
}

func boundedASCIIContains(text, keyword string) bool {
	start := 0
	for {
		idx := strings.Index(text[start:], keyword)
		if idx < 0 {
			return false
		}
		idx += start
		beforeOK := idx == 0 || !isASCIIAlphaNum(text[idx-1])
		after := idx + len(keyword)
		afterOK := after >= len(text) || !isASCIIAlphaNum(text[after])
		if beforeOK && afterOK {
			return true
		}
		start = idx + len(keyword)
	}
}

func isASCIIAlphaNum(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9')
}

// GetNodeGroupsByIDs 根据 ID 列表获取节点组
func (s *Store) GetNodeGroupsByIDs(ids []int64) ([]model.NodeGroup, error) {
	if len(ids) == 0 {
		return []model.NodeGroup{}, nil
	}

	// 构建 IN 查询
	query := `SELECT id, name, type, filter_protocols, filter_subscriptions, filter_include, filter_exclude, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at FROM node_groups WHERE id IN (`
	args := []interface{}{}
	for i, id := range ids {
		if i > 0 {
			query += ","
		}
		query += "?"
		args = append(args, id)
	}
	query += `) AND enabled = 1 ORDER BY priority ASC, id ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []model.NodeGroup
	for rows.Next() {
		var g model.NodeGroup
		var enabled int
		if err := rows.Scan(&g.ID, &g.Name, &g.Type, &g.FilterProtocols, &g.FilterSubscriptions, &g.FilterInclude, &g.FilterExclude, &g.NodeUIDs, &g.TestURL, &g.TestInterval, &g.Tolerance, &enabled, &g.Priority, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}
		g.Enabled = enabled == 1
		groups = append(groups, g)
	}
	return groups, nil
}

// UpdateProxyCollection 更新以支持新字段
func (s *Store) UpdateProxyCollectionWithGroups(id int, req *model.ProxyCollectionRequest) error {
	now := time.Now().Unix()

	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}

	referencedGroupIDsJSON, _ := json.Marshal(req.ReferencedGroupIDs)
	routeRuleIDsJSON, _ := json.Marshal(req.RouteRuleIDs)

	nodeUIDsJSON, _ := json.Marshal(req.NodeUIDs)

	_, err := s.db.Exec(`UPDATE proxy_collections SET name = ?, type = ?, source_type = ?, referenced_group_ids = ?, route_rule_id = ?, route_rule_ids = ?, node_uids = ?, test_url = ?, test_interval = ?, tolerance = ?, enabled = ?, updated_at = ? WHERE id = ?`,
		req.Name, req.Type, sourceType, string(referencedGroupIDsJSON), req.RouteRuleID, string(routeRuleIDsJSON), string(nodeUIDsJSON), req.TestURL, req.TestInterval, req.Tolerance, req.Enabled, now, id)
	return err
}

// CreateProxyCollectionWithGroups 创建策略组（支持节点组）
func (s *Store) CreateProxyCollectionWithGroups(req *model.ProxyCollectionRequest) (*model.ProxyCollectionWithNodes, error) {
	now := time.Now().Unix()

	sourceType := req.SourceType
	if sourceType == "" {
		sourceType = "manual"
	}

	referencedGroupIDsJSON, _ := json.Marshal(req.ReferencedGroupIDs)
	routeRuleIDsJSON, _ := json.Marshal(req.RouteRuleIDs)

	nodeUIDsJSON, _ := json.Marshal(req.NodeUIDs)

	result, err := s.db.Exec(`INSERT INTO proxy_collections (name, type, source_type, referenced_group_ids, route_rule_id, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Name, req.Type, sourceType, string(referencedGroupIDsJSON), req.RouteRuleID, string(routeRuleIDsJSON), string(nodeUIDsJSON), req.TestURL, req.TestInterval, req.Tolerance, req.Enabled, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetProxyCollectionWithNodes(int(id))
}

// GetProxyCollectionWithNodes 获取策略组及详情（支持节点组）
func (s *Store) GetProxyCollectionWithNodes(id int) (*model.ProxyCollectionWithNodes, error) {
	var c model.ProxyCollection
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, type, source_type, referenced_group_ids, route_rule_id, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at FROM proxy_collections WHERE id = ?`, id).
		Scan(&c.ID, &c.Name, &c.Type, &c.SourceType, &c.ReferencedGroupIDs, &c.RouteRuleID, &c.RouteRuleIDs, &c.NodeUIDs, &c.TestURL, &c.TestInterval, &c.Tolerance, &enabled, &c.Priority, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, err
	}
	c.Enabled = enabled == 1

	result := &model.ProxyCollectionWithNodes{
		ProxyCollection: c,
	}
	if c.RouteRuleIDs != "" && c.RouteRuleIDs != "[]" {
		json.Unmarshal([]byte(c.RouteRuleIDs), &result.RouteRuleIDs)
	}

	// 节点组来源模式需要加载引用的节点组。
	if c.SourceType == "node_groups" || c.SourceType == "node_groups_and_nodes" {
		var groupIDs []int64
		if c.ReferencedGroupIDs != "" && c.ReferencedGroupIDs != "[]" {
			json.Unmarshal([]byte(c.ReferencedGroupIDs), &groupIDs)
		}
		groups, _ := s.GetNodeGroupsByIDs(groupIDs)
		result.ReferencedGroups = groups
	} else {
		// manual 模式，解析 node_uids JSON
		var uids []string
		if c.NodeUIDs != "" && c.NodeUIDs != "[]" {
			json.Unmarshal([]byte(c.NodeUIDs), &uids)
		}
		result.NodeUIDs = uids
	}

	return result, nil
}
