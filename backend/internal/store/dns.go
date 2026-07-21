package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

// DNS Servers

func (s *Store) ListDNSServers() ([]model.DNSServer, error) {
	rows, err := s.db.Query(`SELECT id, tag, enabled, server_type, address, address_resolver, address_strategy, strategy, detour, client_subnet, options_json, priority, created_at, updated_at FROM dns_servers ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []model.DNSServer
	for rows.Next() {
		var srv model.DNSServer
		var enabled int
		if err := rows.Scan(&srv.ID, &srv.Tag, &enabled, &srv.ServerType, &srv.Address, &srv.AddressResolver, &srv.AddressStrategy, &srv.Strategy, &srv.Detour, &srv.ClientSubnet, &srv.OptionsJSON, &srv.Priority, &srv.CreatedAt, &srv.UpdatedAt); err != nil {
			return nil, err
		}
		srv.Enabled = enabled == 1
		servers = append(servers, srv)
	}
	return servers, nil
}

func (s *Store) GetDNSServer(id int64) (*model.DNSServer, error) {
	var srv model.DNSServer
	var enabled int
	err := s.db.QueryRow(`SELECT id, tag, enabled, server_type, address, address_resolver, address_strategy, strategy, detour, client_subnet, options_json, priority, created_at, updated_at FROM dns_servers WHERE id = ?`, id).Scan(&srv.ID, &srv.Tag, &enabled, &srv.ServerType, &srv.Address, &srv.AddressResolver, &srv.AddressStrategy, &srv.Strategy, &srv.Detour, &srv.ClientSubnet, &srv.OptionsJSON, &srv.Priority, &srv.CreatedAt, &srv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	srv.Enabled = enabled == 1
	return &srv, nil
}

func (s *Store) CreateDNSServer(req *model.DNSServerRequest) (*model.DNSServer, error) {
	now := time.Now().Unix()
	var priority int
	if err := s.db.QueryRow(`SELECT COALESCE(MAX(priority), -1) + 1 FROM dns_servers`).Scan(&priority); err != nil {
		return nil, err
	}
	optionsJSON, err := json.Marshal(req.Options)
	if err != nil {
		return nil, err
	}
	if req.Options == nil {
		optionsJSON = []byte("{}")
	}

	result, err := s.db.Exec(`INSERT INTO dns_servers (tag, enabled, server_type, address, address_resolver, address_strategy, strategy, detour, client_subnet, options_json, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Tag, req.Enabled, req.ServerType, req.Address, req.AddressResolver, req.AddressStrategy, req.Strategy, req.Detour, req.ClientSubnet, string(optionsJSON), priority, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetDNSServer(id)
}

func (s *Store) UpdateDNSServer(id int64, req *model.DNSServerRequest) error {
	now := time.Now().Unix()
	optionsJSON, err := json.Marshal(req.Options)
	if err != nil {
		return err
	}
	if req.Options == nil {
		optionsJSON = []byte("{}")
	}

	_, err = s.db.Exec(`UPDATE dns_servers SET tag = ?, enabled = ?, server_type = ?, address = ?, address_resolver = ?, address_strategy = ?, strategy = ?, detour = ?, client_subnet = ?, options_json = ?, updated_at = ? WHERE id = ?`,
		req.Tag, req.Enabled, req.ServerType, req.Address, req.AddressResolver, req.AddressStrategy, req.Strategy, req.Detour, req.ClientSubnet, string(optionsJSON), now, id)
	return err
}

func (s *Store) DeleteDNSServer(id int64) error {
	_, err := s.db.Exec(`DELETE FROM dns_servers WHERE id = ?`, id)
	return err
}

func (s *Store) ReorderDNSServers(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "dns_servers", ids); err != nil {
		return err
	}
	now := time.Now().Unix()
	for priority, id := range ids {
		if _, err := tx.Exec(`UPDATE dns_servers SET priority = ?, updated_at = ? WHERE id = ?`, priority, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DNS Rules

func (s *Store) ListDNSRules() ([]model.DNSRule, error) {
	rows, err := s.db.Query(`SELECT id, enabled, priority, rule_type, conditions_json, server, disable_cache, rewrite_ttl, client_subnet, created_at, updated_at FROM dns_rules ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []model.DNSRule
	for rows.Next() {
		var rule model.DNSRule
		var enabled, disableCache int
		if err := rows.Scan(&rule.ID, &enabled, &rule.Priority, &rule.RuleType, &rule.ConditionsJSON, &rule.Server, &disableCache, &rule.RewriteTTL, &rule.ClientSubnet, &rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		rule.Enabled = enabled == 1
		rule.DisableCache = disableCache == 1
		rules = append(rules, rule)
	}
	return rules, nil
}

func (s *Store) GetDNSRule(id int64) (*model.DNSRule, error) {
	var rule model.DNSRule
	var enabled, disableCache int
	err := s.db.QueryRow(`SELECT id, enabled, priority, rule_type, conditions_json, server, disable_cache, rewrite_ttl, client_subnet, created_at, updated_at FROM dns_rules WHERE id = ?`, id).Scan(&rule.ID, &enabled, &rule.Priority, &rule.RuleType, &rule.ConditionsJSON, &rule.Server, &disableCache, &rule.RewriteTTL, &rule.ClientSubnet, &rule.CreatedAt, &rule.UpdatedAt)
	if err != nil {
		return nil, err
	}
	rule.Enabled = enabled == 1
	rule.DisableCache = disableCache == 1
	return &rule, nil
}

func (s *Store) CreateDNSRule(req *model.DNSRuleRequest) (*model.DNSRule, error) {
	now := time.Now().Unix()
	conditionsJSON, _ := json.Marshal(req.Conditions)
	if conditionsJSON == nil {
		conditionsJSON = []byte("{}")
	}

	result, err := s.db.Exec(`INSERT INTO dns_rules (enabled, priority, rule_type, conditions_json, server, disable_cache, rewrite_ttl, client_subnet, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Enabled, req.Priority, req.RuleType, string(conditionsJSON), req.Server, req.DisableCache, req.RewriteTTL, req.ClientSubnet, now, now)
	if err != nil {
		return nil, err
	}

	id, _ := result.LastInsertId()
	return s.GetDNSRule(id)
}

func (s *Store) UpdateDNSRule(id int64, req *model.DNSRuleRequest) error {
	now := time.Now().Unix()
	conditionsJSON, _ := json.Marshal(req.Conditions)
	if conditionsJSON == nil {
		conditionsJSON = []byte("{}")
	}

	_, err := s.db.Exec(`UPDATE dns_rules SET enabled = ?, priority = ?, rule_type = ?, conditions_json = ?, server = ?, disable_cache = ?, rewrite_ttl = ?, client_subnet = ?, updated_at = ? WHERE id = ?`,
		req.Enabled, req.Priority, req.RuleType, string(conditionsJSON), req.Server, req.DisableCache, req.RewriteTTL, req.ClientSubnet, now, id)
	return err
}

func (s *Store) DeleteDNSRule(id int64) error {
	_, err := s.db.Exec(`DELETE FROM dns_rules WHERE id = ?`, id)
	return err
}

func (s *Store) ReorderDNSRules(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "dns_rules", ids); err != nil {
		return err
	}

	for priority, id := range ids {
		if _, err := tx.Exec(`UPDATE dns_rules SET priority = ? WHERE id = ?`, priority, id); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DNS Global Settings (复用 app_settings 表)

func (s *Store) GetDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	r := &model.DNSGlobalSettings{
		Enabled:          true,
		Final:            "dns_proxy",
		Strategy:         "prefer_ipv4",
		DisableCache:     false,
		DisableExpire:    false,
		IndependentCache: false,
		ReverseMapping:   false,
		CacheCapacity:    4096,
		ClientSubnet:     "",
		FakeIPEnabled:    false,
		FakeIPInet4Range: "198.18.0.0/15",
		FakeIPInet6Range: "fdfe:dcba:9876::/48",
	}

	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'dns_global.%'`)
	if err != nil {
		if err == sql.ErrNoRows {
			return r, nil
		}
		return nil, err
	}
	defer rows.Close()

	seen := make(map[string]bool)
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		seen[key] = true
		switch key {
		case "dns_global.enabled":
			r.Enabled = value == "true"
		case "dns_global.final":
			if value != "" {
				r.Final = value
			}
		case "dns_global.strategy":
			if value != "" {
				r.Strategy = value
			}
		case "dns_global.disable_cache":
			r.DisableCache = value == "true"
		case "dns_global.disable_expire":
			r.DisableExpire = value == "true"
		case "dns_global.independent_cache":
			r.IndependentCache = value == "true"
		case "dns_global.reverse_mapping":
			r.ReverseMapping = value == "true"
		case "dns_global.cache_capacity":
			if n, err := strconv.Atoi(value); err == nil && n > 0 {
				r.CacheCapacity = n
			}
		case "dns_global.client_subnet":
			r.ClientSubnet = value
		case "dns_global.fakeip_enabled":
			r.FakeIPEnabled = value == "true"
		case "dns_global.fakeip_inet4_range":
			if value != "" {
				r.FakeIPInet4Range = value
			}
		case "dns_global.fakeip_inet6_range":
			if value != "" {
				r.FakeIPInet6Range = value
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if seen["dns_global.enabled"] && seen["dns_global.final"] && seen["dns_global.strategy"] &&
		seen["dns_global.disable_cache"] && seen["dns_global.disable_expire"] && seen["dns_global.independent_cache"] &&
		seen["dns_global.reverse_mapping"] && seen["dns_global.client_subnet"] && seen["dns_global.fakeip_enabled"] &&
		seen["dns_global.fakeip_inet4_range"] && seen["dns_global.fakeip_inet6_range"] {
		return r, nil
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	legacy, err := s.GetDNSSettings()
	if err != nil {
		return nil, err
	}
	if !seen["dns_global.enabled"] {
		r.Enabled = legacy.Enabled
	}
	if !seen["dns_global.final"] && legacy.Final != "" {
		r.Final = legacy.Final
	}
	if !seen["dns_global.strategy"] && legacy.Strategy != "" {
		r.Strategy = legacy.Strategy
	}
	if !seen["dns_global.disable_cache"] {
		r.DisableCache = legacy.DisableCache
	}
	if !seen["dns_global.disable_expire"] {
		r.DisableExpire = legacy.DisableExpire
	}
	if !seen["dns_global.independent_cache"] {
		r.IndependentCache = legacy.IndependentCache
	}
	if !seen["dns_global.reverse_mapping"] {
		r.ReverseMapping = legacy.ReverseMapping
	}
	if !seen["dns_global.client_subnet"] {
		r.ClientSubnet = legacy.ClientSubnet
	}
	if !seen["dns_global.fakeip_enabled"] {
		r.FakeIPEnabled = legacy.FakeIPEnabled
	}
	if !seen["dns_global.fakeip_inet4_range"] && legacy.FakeIPInet4Range != "" {
		r.FakeIPInet4Range = legacy.FakeIPInet4Range
	}
	if !seen["dns_global.fakeip_inet6_range"] && legacy.FakeIPInet6Range != "" {
		r.FakeIPInet6Range = legacy.FakeIPInet6Range
	}
	return r, nil
}

func (s *Store) SetDNSGlobalSettings(req *model.DNSGlobalSettings) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"dns.enabled":                   fmt.Sprintf("%t", req.Enabled),
		"dns_global.enabled":            fmt.Sprintf("%t", req.Enabled),
		"dns.final":                     req.Final,
		"dns_global.final":              req.Final,
		"dns.strategy":                  req.Strategy,
		"dns_global.strategy":           req.Strategy,
		"dns.disable_cache":             fmt.Sprintf("%t", req.DisableCache),
		"dns_global.disable_cache":      fmt.Sprintf("%t", req.DisableCache),
		"dns.disable_expire":            fmt.Sprintf("%t", req.DisableExpire),
		"dns_global.disable_expire":     fmt.Sprintf("%t", req.DisableExpire),
		"dns.independent_cache":         fmt.Sprintf("%t", req.IndependentCache),
		"dns_global.independent_cache":  fmt.Sprintf("%t", req.IndependentCache),
		"dns.reverse_mapping":           fmt.Sprintf("%t", req.ReverseMapping),
		"dns_global.reverse_mapping":    fmt.Sprintf("%t", req.ReverseMapping),
		"dns_global.cache_capacity":     fmt.Sprintf("%d", req.CacheCapacity),
		"dns.client_subnet":             req.ClientSubnet,
		"dns_global.client_subnet":      req.ClientSubnet,
		"dns.fakeip_enabled":            fmt.Sprintf("%t", req.FakeIPEnabled),
		"dns_global.fakeip_enabled":     fmt.Sprintf("%t", req.FakeIPEnabled),
		"dns.fakeip_inet4_range":        req.FakeIPInet4Range,
		"dns_global.fakeip_inet4_range": req.FakeIPInet4Range,
		"dns.fakeip_inet6_range":        req.FakeIPInet6Range,
		"dns_global.fakeip_inet6_range": req.FakeIPInet6Range,
	}

	for key, value := range settings {
		if value == "" {
			_, err := s.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
			if err != nil {
				return err
			}
			continue
		}
		_, err := s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now)
		if err != nil {
			return err
		}
	}
	return nil
}
