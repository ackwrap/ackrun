package store

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

// CreateProxyCollection 创建代理集合
func (s *Store) CreateProxyCollection(pc *model.ProxyCollection) error {
	now := time.Now().UnixMilli()
	pc.CreatedAt = now
	pc.UpdatedAt = now

	result, err := s.db.Exec(
		`INSERT INTO proxy_collections (name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pc.Name, pc.Type, pc.SourceType, pc.ReferencedGroupIDs, pc.RouteRuleIDs, pc.NodeUIDs, pc.TestURL, pc.TestInterval, pc.Tolerance, boolToInt(pc.Enabled), pc.CreatedAt, pc.UpdatedAt,
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
		`SELECT id, name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, created_at, updated_at
			FROM proxy_collections WHERE id = ?`, id,
	).Scan(&pc.ID, &pc.Name, &pc.Type, &pc.SourceType, &pc.ReferencedGroupIDs, &pc.RouteRuleIDs, &pc.NodeUIDs, &pc.TestURL, &pc.TestInterval, &pc.Tolerance, &enabled, &pc.CreatedAt, &pc.UpdatedAt)

	if err != nil {
		return nil, err
	}

	pc.Enabled = enabled == 1
	return &pc, nil
}

// ListProxyCollections 列出所有代理集合
func (s *Store) ListProxyCollections() ([]*model.ProxyCollection, error) {
	rows, err := s.db.Query(
		`SELECT id, name, type, source_type, referenced_group_ids, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, created_at, updated_at
			FROM proxy_collections ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	collections := []*model.ProxyCollection{}
	for rows.Next() {
		var pc model.ProxyCollection
		var enabled int

		if err := rows.Scan(&pc.ID, &pc.Name, &pc.Type, &pc.SourceType, &pc.ReferencedGroupIDs, &pc.RouteRuleIDs, &pc.NodeUIDs, &pc.TestURL, &pc.TestInterval, &pc.Tolerance, &enabled, &pc.CreatedAt, &pc.UpdatedAt); err != nil {
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

// CleanInvalidNodeUIDs 清理所有策略组中失效的节点 UID
func (s *Store) CleanInvalidNodeUIDs(removedUIDs []string) (int, error) {
	if len(removedUIDs) == 0 {
		return 0, nil
	}

	collections, err := s.ListProxyCollections()
	if err != nil {
		return 0, err
	}

	removedSet := make(map[string]bool)
	for _, uid := range removedUIDs {
		removedSet[uid] = true
	}

	cleanedCount := 0
	for _, pc := range collections {
		if pc.SourceType != "manual" || pc.NodeUIDs == "" || pc.NodeUIDs == "[]" {
			continue
		}

		var currentUIDs []string
		if err := json.Unmarshal([]byte(pc.NodeUIDs), &currentUIDs); err != nil {
			continue
		}

		// 过滤掉失效的 UID
		validUIDs := make([]string, 0)
		hadInvalid := false
		for _, uid := range currentUIDs {
			if removedSet[uid] {
				hadInvalid = true
			} else {
				validUIDs = append(validUIDs, uid)
			}
		}

		if !hadInvalid {
			continue
		}

		// 更新策略组
		validUIDsJSON, _ := json.Marshal(validUIDs)
		pc.NodeUIDs = string(validUIDsJSON)
		if err := s.UpdateProxyCollection(pc.ID, pc); err != nil {
			return cleanedCount, err
		}
		cleanedCount++
	}

	return cleanedCount, nil
}

// AutoAddNewNodes 自动将新增节点加入匹配的策略组
func (s *Store) AutoAddNewNodes(subscriptionID int64, addedUIDs []string) (int, error) {
	if len(addedUIDs) == 0 {
		return 0, nil
	}

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
