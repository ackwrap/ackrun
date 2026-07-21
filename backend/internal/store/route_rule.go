package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) ListRouteRules() ([]model.RouteRule, error) {
	rows, err := s.db.Query(`SELECT id, name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at FROM route_rules ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.RouteRule, 0)
	for rows.Next() {
		item, err := scanRouteRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) CreateRouteRule(req *model.RouteRuleRequest) (*model.RouteRule, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	priority := req.Priority
	if priority <= 0 {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(priority), 0) + 10 FROM route_rules`).Scan(&priority); err != nil {
			return nil, err
		}
		if priority <= 0 {
			priority = 10
		}
	}
	valuesJSON, err := json.Marshal(req.Values)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	res, err := tx.Exec(`INSERT INTO route_rules (name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.Name, boolToInt(req.Enabled), priority, req.RuleType, string(valuesJSON), req.Outbound, boolToInt(req.Invert), req.SystemKey, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := normalizeSystemRouteRuleOrderInTx(tx); err != nil {
		return nil, err
	}
	item, err := scanRouteRule(tx.QueryRow(`SELECT id, name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at FROM route_rules WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) GetRouteRule(id int64) (*model.RouteRule, error) {
	row := s.db.QueryRow(`SELECT id, name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at FROM route_rules WHERE id = ?`, id)
	item, err := scanRouteRule(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) SetRouteRuleSystemKey(id int64, systemKey string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE route_rules SET system_key = ?, updated_at = ? WHERE id = ?`, systemKey, now, id)
	return err
}

func (s *Store) UpdateRouteRule(id int64, req *model.RouteRuleRequest) (*model.RouteRule, error) {
	valuesJSON, err := json.Marshal(req.Values)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	result, err := tx.Exec(`UPDATE route_rules SET name = ?, enabled = ?, priority = ?, rule_type = ?, values_json = ?, outbound = ?, invert = ?, updated_at = ? WHERE id = ?`, req.Name, boolToInt(req.Enabled), req.Priority, req.RuleType, string(valuesJSON), req.Outbound, boolToInt(req.Invert), now, id)
	if err != nil {
		return nil, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, nil
	}
	if req.Outbound == "proxy" {
		legacyJSON, _ := json.Marshal([]int64{id})
		if _, err := tx.Exec(`UPDATE proxy_collections SET name = ?, route_rule_ids = ?, updated_at = ? WHERE route_rule_id = ?`, req.Name, string(legacyJSON), now, id); err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE proxy_collections SET route_rule_id = 0, route_rule_ids = '[]', updated_at = ? WHERE route_rule_id = ?`, now, id); err != nil {
			return nil, err
		}
	}
	if err := normalizeSystemRouteRuleOrderInTx(tx); err != nil {
		return nil, err
	}
	item, err := scanRouteRule(tx.QueryRow(`SELECT id, name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at FROM route_rules WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) DeleteRouteRule(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`UPDATE proxy_collections SET route_rule_id = 0, route_rule_ids = '[]', updated_at = ? WHERE route_rule_id = ?`, time.Now().UnixMilli(), id); err != nil {
		return err
	}
	if _, err := updateIntJSONRefsTx(tx, "proxy_collections", "route_rule_ids", map[int64]bool{id: true}); err != nil {
		return err
	}
	result, err := tx.Exec(`DELETE FROM route_rules WHERE id = ?`, id)
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
	if err := normalizeSystemRouteRuleOrderInTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReorderRouteRules(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "route_rules", ids); err != nil {
		return err
	}
	var adBlockID, globalDirectID int64
	if err := tx.QueryRow(`SELECT id FROM route_rules WHERE system_key = ?`, systemRuleAdBlockKey).Scan(&adBlockID); err != nil {
		return err
	}
	if err := tx.QueryRow(`SELECT id FROM route_rules WHERE system_key = ?`, systemRuleGlobalDirectKey).Scan(&globalDirectID); err != nil {
		return err
	}
	if ids[0] != adBlockID {
		return fmt.Errorf("广告拦截必须保持在规则第一位")
	}
	if ids[len(ids)-1] != globalDirectID {
		return fmt.Errorf("全球直连必须保持在规则最后一位")
	}
	now := time.Now().UnixMilli()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE route_rules SET priority = ?, updated_at = ? WHERE id = ?`, (i+1)*10, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type routeRuleScanner interface {
	Scan(dest ...any) error
}

func scanRouteRule(scanner routeRuleScanner) (*model.RouteRule, error) {
	var item model.RouteRule
	var enabled, invert int
	var valuesJSON string
	if err := scanner.Scan(&item.ID, &item.Name, &enabled, &item.Priority, &item.RuleType, &valuesJSON, &item.Outbound, &invert, &item.SystemKey, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(valuesJSON), &item.Values); err != nil {
		return nil, fmt.Errorf("decode route rule values: %w", err)
	}
	item.Enabled = enabled != 0
	item.Invert = invert != 0
	item.IsSystem = item.SystemKey != ""
	return &item, nil
}

func NormalizeRouteRuleValues(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
