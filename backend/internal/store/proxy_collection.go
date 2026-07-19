package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

// CreateProxyCollection 创建代理集合
func (s *Store) CreateProxyCollection(pc *model.ProxyCollection) error {
	now := time.Now().UnixMilli()
	pc.CreatedAt = now
	pc.UpdatedAt = now
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(priority), -1) + 1 FROM proxy_collections`).Scan(&pc.Priority); err != nil {
		return err
	}

	result, err := s.db.Exec(
		`INSERT INTO proxy_collections (name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pc.Name, pc.Type, pc.SourceType, pc.ReferencedGroupIDs, pc.RouteRuleIDs, pc.NodeUIDs, pc.TestURL, pc.TestInterval, pc.Tolerance, boolToInt(pc.Enabled), pc.Priority, pc.CreatedAt, pc.UpdatedAt,
	)
	if err != nil {
		return err
	}

	id, _ := result.LastInsertId()
	pc.ID = int(id)
	return nil
}

// GetProxyCollection 获取代理集合
func (s *Store) GetProxyCollection(id int) (*model.ProxyCollection, error) {
	var pc model.ProxyCollection
	var enabled int

	err := s.db.QueryRow(
		`SELECT id, name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at
			FROM proxy_collections WHERE id = ?`, id,
	).Scan(&pc.ID, &pc.Name, &pc.Type, &pc.SourceType, &pc.ReferencedGroupIDs, &pc.RouteRuleIDs, &pc.NodeUIDs, &pc.TestURL, &pc.TestInterval, &pc.Tolerance, &enabled, &pc.Priority, &pc.CreatedAt, &pc.UpdatedAt)

	if err != nil {
		return nil, err
	}

	pc.Enabled = enabled == 1
	return &pc, nil
}

// ListProxyCollections 列出所有代理集合
func (s *Store) ListProxyCollections() ([]*model.ProxyCollection, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at
			FROM proxy_collections ORDER BY CASE WHEN name = '全球直连' THEN 0 ELSE 1 END, priority ASC, id DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collections := []*model.ProxyCollection{}
	for rows.Next() {
		var pc model.ProxyCollection
		var enabled int

		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Type, &pc.SourceType, &pc.ReferencedGroupIDs, &pc.RouteRuleIDs, &pc.NodeUIDs, &pc.TestURL, &pc.TestInterval, &pc.Tolerance, &enabled, &pc.Priority, &pc.CreatedAt, &pc.UpdatedAt); err != nil {
			return nil, err
		}

		pc.Enabled = enabled == 1
		collections = append(collections, &pc)
	}

	return collections, nil
}

// UpdateProxyCollection 更新代理集合
func (s *Store) UpdateProxyCollection(id int, pc *model.ProxyCollection) error {
	pc.UpdatedAt = time.Now().UnixMilli()

	_, err := s.db.Exec(
		`UPDATE proxy_collections SET name = ?, type = ?, source_type = ?, referenced_group_ids = ?, route_rule_ids = ?, node_uids = ?, test_url = ?, test_interval = ?, tolerance = ?, enabled = ?, updated_at = ?
			WHERE id = ?`,
		pc.Name, pc.Type, pc.SourceType, pc.ReferencedGroupIDs, pc.RouteRuleIDs, pc.NodeUIDs, pc.TestURL, pc.TestInterval, pc.Tolerance, boolToInt(pc.Enabled), pc.UpdatedAt, id,
	)

	return err
}

// DeleteProxyCollection 删除代理集合
func (s *Store) DeleteProxyCollection(id int) error {
	_, err := s.db.Exec(`DELETE FROM proxy_collections WHERE id = ?`, id)
	return err
}

func (s *Store) ReorderProxyCollections(ids []int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "proxy_collections", ids); err != nil {
		return err
	}
	var directID int
	err = tx.QueryRow(`SELECT id FROM proxy_collections WHERE name = '全球直连' LIMIT 1`).Scan(&directID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil && (len(ids) == 0 || ids[0] != directID) {
		return fmt.Errorf("全球直连必须保持在策略组第一位")
	}
	now := time.Now().UnixMilli()
	for priority, id := range ids {
		if _, err := tx.Exec(`UPDATE proxy_collections SET priority = ?, updated_at = ? WHERE id = ?`, priority, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetCollectionNodeUIDs 获取集合中的节点 UID 列表
func (s *Store) GetCollectionNodeUIDs(collectionID int) ([]string, error) {
	pc, err := s.GetProxyCollection(collectionID)
	if err != nil {
		return nil, err
	}

	var uids []string
	if pc.NodeUIDs != "" && pc.NodeUIDs != "[]" {
		if err := json.Unmarshal([]byte(pc.NodeUIDs), &uids); err != nil {
			return nil, err
		}
	}
	return uids, nil
}

// ListProxyCollectionsWithNodes 列出所有代理集合及其节点
func (s *Store) ListProxyCollectionsWithNodes() ([]*model.ProxyCollectionWithNodes, error) {
	collections, err := s.ListProxyCollections()
	if err != nil {
		return nil, err
	}

	result := []*model.ProxyCollectionWithNodes{}
	for _, pc := range collections {
		var nodeUIDs []string
		if pc.NodeUIDs != "" && pc.NodeUIDs != "[]" {
			if err := json.Unmarshal([]byte(pc.NodeUIDs), &nodeUIDs); err != nil {
				return nil, err
			}
		}

		var referencedGroupIDs []int64
		if pc.ReferencedGroupIDs != "" && pc.ReferencedGroupIDs != "[]" {
			if err := json.Unmarshal([]byte(pc.ReferencedGroupIDs), &referencedGroupIDs); err != nil {
				return nil, err
			}
		}

		var routeRuleIDs []int64
		if pc.RouteRuleIDs != "" && pc.RouteRuleIDs != "[]" {
			if err := json.Unmarshal([]byte(pc.RouteRuleIDs), &routeRuleIDs); err != nil {
				return nil, err
			}
		}

		var referencedGroups []model.NodeGroup
		for _, gid := range referencedGroupIDs {
			group, err := s.GetNodeGroup(gid)
			if err == nil && group != nil {
				referencedGroups = append(referencedGroups, *group)
			}
		}

		result = append(result, &model.ProxyCollectionWithNodes{
			ProxyCollection:  *pc,
			NodeUIDs:         nodeUIDs,
			ReferencedGroups: referencedGroups,
			RouteRuleIDs:     routeRuleIDs,
		})
	}

	return result, nil
}

type InvalidNodeCleanupResult struct {
	UpdatedCollections int
	DeletedNodeGroups  int
}

// CleanInvalidNodeUIDs 清理失效节点引用、零节点节点组及业务策略组引用。
func (s *Store) CleanInvalidNodeUIDs(removedUIDs []string) (InvalidNodeCleanupResult, error) {
	result := InvalidNodeCleanupResult{}
	if len(removedUIDs) == 0 {
		return result, nil
	}
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	result, err = s.cleanInvalidNodeUIDsTx(tx, removedUIDs)
	if err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) cleanInvalidNodeUIDsTx(tx *sql.Tx, removedUIDs []string) (InvalidNodeCleanupResult, error) {
	result := InvalidNodeCleanupResult{}
	remove, err := globallyMissingNodeUIDsTx(tx, removedUIDs)
	if err != nil {
		return result, err
	}
	emptyGroupIDs, err := s.emptyNodeGroupIDsTx(tx, remove)
	if err != nil {
		return result, err
	}
	if len(remove) > 0 {
		result.UpdatedCollections, err = updateStringJSONRefsTx(tx, "proxy_collections", "node_uids", remove)
		if err != nil {
			return result, err
		}
		if _, err := updateStringJSONRefsTx(tx, "node_groups", "node_uids", remove); err != nil {
			return result, err
		}
	}
	for _, id := range emptyGroupIDs {
		if _, err := tx.Exec(`DELETE FROM node_groups WHERE id = ?`, id); err != nil {
			return result, err
		}
	}
	groupRemove := make(map[int64]bool, len(emptyGroupIDs))
	for _, id := range emptyGroupIDs {
		groupRemove[id] = true
	}
	if _, err := updateIntJSONRefsTx(tx, "proxy_collections", "referenced_group_ids", groupRemove); err != nil {
		return result, err
	}
	result.DeletedNodeGroups = len(emptyGroupIDs)
	return result, nil
}

// AutoAddNewNodes 自动将新增节点加入匹配的策略组
func (s *Store) AutoAddNewNodes(subscriptionID int64, addedUIDs []string) (int, error) {
	if len(addedUIDs) == 0 {
		return 0, nil
	}
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	// 获取新增节点的详细信息
	newNodes, err := s.ListNodesByUIDs(addedUIDs)
	if err != nil {
		return 0, err
	}
	if len(newNodes) == 0 {
		return 0, nil
	}

	collections, err := s.ListProxyCollections()
	if err != nil {
		return 0, err
	}

	updatedCount := 0
	for _, pc := range collections {
		if pc.SourceType != "manual" || pc.NodeUIDs == "" || pc.NodeUIDs == "[]" {
			continue
		}

		var currentUIDs []string
		if err := json.Unmarshal([]byte(pc.NodeUIDs), &currentUIDs); err != nil {
			continue
		}

		if len(currentUIDs) == 0 {
			continue
		}

		// 获取策略组现有节点信息
		existingNodes, err := s.ListNodesByUIDs(currentUIDs)
		if err != nil || len(existingNodes) == 0 {
			continue
		}

		// 分析策略组特征
		feature := analyzeProxyCollectionFeature(existingNodes, subscriptionID)
		if !feature.HasSubscription {
			// 策略组不包含该订阅的节点，跳过
			continue
		}

		// 判断新节点是否匹配并加入
		matchedUIDs := []string{}
		for _, newNode := range newNodes {
			if newNode.SubscriptionID != subscriptionID {
				continue
			}
			if matchesFeature(newNode, feature) {
				matchedUIDs = append(matchedUIDs, newNode.UID)
			}
		}

		if len(matchedUIDs) == 0 {
			continue
		}

		// 更新策略组，追加新节点
		updatedUIDs := append(currentUIDs, matchedUIDs...)
		updatedUIDsJSON, _ := json.Marshal(updatedUIDs)
		pc.NodeUIDs = string(updatedUIDsJSON)
		if err := s.UpdateProxyCollection(pc.ID, pc); err != nil {
			return updatedCount, err
		}
		updatedCount++
	}

	return updatedCount, nil
}

// proxyCollectionFeature 策略组节点特征
type proxyCollectionFeature struct {
	HasSubscription bool     // 是否包含该订阅的节点
	Keywords        []string // 提取的关键词
}

// analyzeProxyCollectionFeature 分析策略组节点特征
func analyzeProxyCollectionFeature(nodes []model.Node, subscriptionID int64) proxyCollectionFeature {
	feature := proxyCollectionFeature{
		Keywords: []string{},
	}

	// 检查是否包含该订阅的节点
	for _, node := range nodes {
		if node.SubscriptionID == subscriptionID {
			feature.HasSubscription = true
			break
		}
	}

	if !feature.HasSubscription {
		return feature
	}

	// 提取所有该订阅节点名称中的关键词
	keywordMap := make(map[string]int)
	for _, node := range nodes {
		if node.SubscriptionID != subscriptionID {
			continue
		}
		// 提取常见地区关键词
		keywords := extractKeywords(node.Name)
		for _, kw := range keywords {
			keywordMap[kw]++
		}
	}

	// 选择出现频率高的关键词作为特征
	for kw, count := range keywordMap {
		if count >= 1 { // 至少出现1次
			feature.Keywords = append(feature.Keywords, kw)
		}
	}

	return feature
}

// extractKeywords 从节点名称提取关键词
func extractKeywords(name string) []string {
	nameLower := strings.ToLower(name)
	keywords := []string{}

	// 常见地区关键词
	regions := []string{
		"香港", "hk", "hong kong",
		"台湾", "台灣", "tw", "taiwan",
		"日本", "jp", "japan", "tokyo",
		"新加坡", "sg", "singapore",
		"美国", "美國", "us", "usa", "america",
		"韩国", "韓國", "kr", "korea",
		"英国", "英國", "uk", "britain",
		"德国", "德國", "de", "germany",
		"法国", "法國", "fr", "france",
		"俄罗斯", "俄羅斯", "ru", "russia",
	}

	for _, region := range regions {
		if strings.Contains(nameLower, region) {
			keywords = append(keywords, region)
		}
	}

	return keywords
}

// matchesFeature 判断节点是否匹配策略组特征
func matchesFeature(node model.Node, feature proxyCollectionFeature) bool {
	if len(feature.Keywords) == 0 {
		// 没有明确特征，自动加入该订阅所有新节点
		return true
	}

	// 检查节点名称是否包含任一特征关键词
	nameLower := strings.ToLower(node.Name)
	for _, kw := range feature.Keywords {
		if strings.Contains(nameLower, kw) {
			return true
		}
	}

	return false
}
