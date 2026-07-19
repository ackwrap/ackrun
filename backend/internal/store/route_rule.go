package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
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
	priority := req.Priority
	if priority <= 0 {
		priority = s.nextRouteRulePriority()
	}
	valuesJSON, err := json.Marshal(req.Values)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`INSERT INTO route_rules (name, enabled, priority, rule_type, values_json, outbound, invert, system_key, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.Name, boolToInt(req.Enabled), priority, req.RuleType, string(valuesJSON), req.Outbound, boolToInt(req.Invert), req.SystemKey, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetRouteRule(id)
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
	now := time.Now().UnixMilli()
	_, err = s.db.Exec(`UPDATE route_rules SET name = ?, enabled = ?, priority = ?, rule_type = ?, values_json = ?, outbound = ?, invert = ?, updated_at = ? WHERE id = ?`, req.Name, boolToInt(req.Enabled), req.Priority, req.RuleType, string(valuesJSON), req.Outbound, boolToInt(req.Invert), now, id)
	if err != nil {
		return nil, err
	}
	return s.GetRouteRule(id)
}

func (s *Store) DeleteRouteRule(id int64) error {
	if _, err := s.db.Exec(`DELETE FROM route_rules WHERE id = ?`, id); err != nil {
		return err
	}
	return s.removeRouteRuleRefsFromProxyCollections(id)
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
	now := time.Now().UnixMilli()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE route_rules SET priority = ?, updated_at = ? WHERE id = ?`, (i+1)*10, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) nextRouteRulePriority() int {
	var priority int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(priority), 0) + 10 FROM route_rules`).Scan(&priority); err != nil || priority <= 0 {
		return 10
	}
	return priority
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
