package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	systemRuleAdBlockKey      = "ad_block"
	systemRuleGlobalDirectKey = "global_direct"
	systemAdBlockName         = "广告拦截"
	systemGlobalDirectName    = "全球直连"
)

func (s *Store) migrateRouteStrategies() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	adBlockID, err := ensureSystemRouteRuleTx(tx, systemRuleAdBlockKey, systemAdBlockName, "geosite", []string{"category-ads-all"}, "block", false)
	if err != nil {
		return err
	}
	globalDirectID, err := ensureSystemRouteRuleTx(tx, systemRuleGlobalDirectKey, systemGlobalDirectName, "fallback", []string{}, "direct", true)
	if err != nil {
		return err
	}
	if err := ensureGlobalDirectCollectionTx(tx, globalDirectID); err != nil {
		return err
	}
	if err := normalizeSystemRouteRuleOrderTx(tx, adBlockID, globalDirectID); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureSystemRouteRuleTx(tx *sql.Tx, systemKey, name, ruleType string, values []string, outbound string, forceEnabled bool) (int64, error) {
	valuesJSON, err := json.Marshal(values)
	if err != nil {
		return 0, err
	}
	var id int64
	err = tx.QueryRow(`SELECT id FROM route_rules WHERE system_key = ?`, systemKey).Scan(&id)
	if err == sql.ErrNoRows {
		err = tx.QueryRow(`SELECT id FROM route_rules WHERE name = ? AND COALESCE(system_key, '') = '' ORDER BY priority ASC, id ASC LIMIT 1`, name).Scan(&id)
		if err == nil {
			_, err = tx.Exec(`UPDATE route_rules SET system_key = ?, updated_at = ? WHERE id = ?`, systemKey, time.Now().UnixMilli(), id)
		}
	}
	if err == sql.ErrNoRows {
		now := time.Now().UnixMilli()
		result, insertErr := tx.Exec(`INSERT INTO route_rules (name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at) VALUES (?, 1, 0, ?, ?, ?, 0, ?, ?, ?)`, name, ruleType, string(valuesJSON), outbound, systemKey, now, now)
		if insertErr != nil {
			return 0, insertErr
		}
		return result.LastInsertId()
	}
	if err != nil {
		return 0, err
	}
	now := time.Now().UnixMilli()
	if forceEnabled {
		_, err = tx.Exec(`UPDATE route_rules SET name = ?, enabled = 1, rule_type = ?, values_json = ?, outbound = ?, invert = 0, updated_at = ?
			WHERE id = ? AND (name <> ? OR enabled <> 1 OR rule_type <> ? OR values_json <> ? OR outbound <> ? OR invert <> 0)`,
			name, ruleType, string(valuesJSON), outbound, now, id, name, ruleType, string(valuesJSON), outbound)
	} else {
		_, err = tx.Exec(`UPDATE route_rules SET name = ?, rule_type = ?, values_json = ?, outbound = ?, invert = 0, updated_at = ?
			WHERE id = ? AND (name <> ? OR rule_type <> ? OR values_json <> ? OR outbound <> ? OR invert <> 0)`,
			name, ruleType, string(valuesJSON), outbound, now, id, name, ruleType, string(valuesJSON), outbound)
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

func ensureGlobalDirectCollectionTx(tx *sql.Tx, routeRuleID int64) error {
	var id int
	err := tx.QueryRow(`SELECT id FROM proxy_collections WHERE name = ? ORDER BY priority ASC, id DESC LIMIT 1`, systemGlobalDirectName).Scan(&id)
	if err == sql.ErrNoRows {
		if _, err := tx.Exec(`UPDATE proxy_collections SET route_rule_id = 0 WHERE route_rule_id = ?`, routeRuleID); err != nil {
			return err
		}
		now := time.Now().UnixMilli()
		legacy, _ := json.Marshal([]int64{routeRuleID})
		_, err = tx.Exec(`INSERT INTO proxy_collections (name, type, source_type, referenced_group_ids, route_rule_id, route_rule_ids, node_uids, test_url, test_interval, tolerance, enabled, priority, created_at, updated_at) VALUES (?, 'selector', 'manual', '[]', ?, ?, '["direct"]', 'https://www.gstatic.com/generate_204', 300, 100, 1, 0, ?, ?)`, systemGlobalDirectName, routeRuleID, string(legacy), now, now)
		return err
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE proxy_collections SET route_rule_id = 0 WHERE route_rule_id = ? AND id <> ?`, routeRuleID, id); err != nil {
		return err
	}
	legacy, _ := json.Marshal([]int64{routeRuleID})
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`UPDATE proxy_collections SET type = 'selector', source_type = 'manual', referenced_group_ids = '[]', route_rule_id = ?, route_rule_ids = ?, node_uids = '["direct"]', enabled = 1, priority = 0, updated_at = ?
		WHERE id = ? AND (type <> 'selector' OR source_type <> 'manual' OR referenced_group_ids <> '[]' OR route_rule_id <> ? OR route_rule_ids <> ? OR node_uids <> '["direct"]' OR enabled <> 1 OR priority <> 0)`,
		routeRuleID, string(legacy), now, id, routeRuleID, string(legacy)); err != nil {
		return err
	}
	_, err = tx.Exec(`UPDATE proxy_collections SET enabled = 0, route_rule_id = 0, route_rule_ids = '[]', updated_at = ?
		WHERE name = ? AND id <> ? AND (enabled <> 0 OR route_rule_id <> 0 OR route_rule_ids <> '[]')`, now, systemGlobalDirectName, id)
	return err
}

func normalizeSystemRouteRuleOrderTx(tx *sql.Tx, adBlockID, globalDirectID int64) error {
	rows, err := tx.Query(`SELECT id FROM route_rules WHERE id NOT IN (?, ?) ORDER BY priority ASC, id ASC`, adBlockID, globalDirectID)
	if err != nil {
		return err
	}
	ids := []int64{adBlockID}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	ids = append(ids, globalDirectID)
	now := time.Now().UnixMilli()
	for index, id := range ids {
		priority := (index + 1) * 10
		if _, err := tx.Exec(`UPDATE route_rules SET priority = ?, updated_at = ? WHERE id = ? AND priority <> ?`, priority, now, id, priority); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) NormalizeSystemRouteRuleOrder() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := normalizeSystemRouteRuleOrderInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func normalizeSystemRouteRuleOrderInTx(tx *sql.Tx) error {
	var adBlockID, globalDirectID int64
	if err := tx.QueryRow(`SELECT id FROM route_rules WHERE system_key = ?`, systemRuleAdBlockKey).Scan(&adBlockID); err != nil {
		return fmt.Errorf("load ad-block system rule: %w", err)
	}
	if err := tx.QueryRow(`SELECT id FROM route_rules WHERE system_key = ?`, systemRuleGlobalDirectKey).Scan(&globalDirectID); err != nil {
		return fmt.Errorf("load global-direct system rule: %w", err)
	}
	return normalizeSystemRouteRuleOrderTx(tx, adBlockID, globalDirectID)
}
