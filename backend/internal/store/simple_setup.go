package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const simpleSetupStateKey = "simple_setup.state"
const simpleSetupSubscriptionKey = "simple_setup.subscription_id"

func (s *Store) SimpleSetupSubscription() (*model.Subscription, error) {
	var id int64
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, simpleSetupSubscriptionKey).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) SaveSimpleSetupSubscription(req *model.SubscriptionRequest) (*model.Subscription, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var state string
	if err := tx.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, simpleSetupStateKey).Scan(&state); err != nil {
		return nil, err
	}
	if state != "prepared" && state != "applied" && state != "configured" {
		return nil, fmt.Errorf("simple setup has not been prepared")
	}
	var id int64
	err = tx.QueryRow(`SELECT s.id FROM subscriptions s JOIN app_settings a ON s.id = a.value WHERE a.key = ?`, simpleSetupSubscriptionKey).Scan(&id)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if err == sql.ErrNoRows {
		result, err := tx.Exec(`INSERT INTO subscriptions (name, url, user_agent, sync_interval_minutes, sync_mode, sync_time, sync_weekday, sync_timeout_seconds, expire_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.Name, req.URL, req.UserAgent, req.SyncIntervalMins, req.SyncMode, req.SyncTime, req.SyncWeekday, req.SyncTimeoutSecs, req.ExpireAt, now, now)
		if err != nil {
			return nil, err
		}
		id, err = result.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		if _, err := tx.Exec(`UPDATE subscriptions SET name = ?, url = ?, user_agent = ?, sync_interval_minutes = ?, sync_mode = ?, sync_time = ?, sync_weekday = ?, sync_timeout_seconds = ?, expire_at = ?, updated_at = ? WHERE id = ?`,
			req.Name, req.URL, req.UserAgent, req.SyncIntervalMins, req.SyncMode, req.SyncTime, req.SyncWeekday, req.SyncTimeoutSecs, req.ExpireAt, now, id); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, simpleSetupSubscriptionKey, strconv.FormatInt(id, 10), now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

const simpleSetupAvailableQuery = `SELECT NOT (
	EXISTS(SELECT 1 FROM route_rules WHERE COALESCE(system_key, '') = '') OR
	EXISTS(SELECT 1 FROM proxy_collections WHERE name <> '全球直连') OR
	EXISTS(SELECT 1 FROM node_groups) OR EXISTS(SELECT 1 FROM dns_servers) OR
	EXISTS(SELECT 1 FROM dns_rules) OR EXISTS(SELECT 1 FROM dns_hosts) OR
	EXISTS(SELECT 1 FROM subscriptions) OR EXISTS(SELECT 1 FROM nodes) OR
	EXISTS(SELECT 1 FROM route_rule_subscriptions) OR
	EXISTS(SELECT 1 FROM app_settings WHERE key = 'config_generator.request')
)`

func (s *Store) SimpleSetupState() (string, error) {
	var state string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, simpleSetupStateKey).Scan(&state)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return state, err
}

func (s *Store) SetSimpleSetupState(state string) error {
	if state != "prepared" && state != "applied" && state != "configured" {
		return fmt.Errorf("invalid simple setup state %q", state)
	}
	_, err := s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, simpleSetupStateKey, state, time.Now().UnixMilli())
	return err
}

func (s *Store) SimpleSetupAvailable() (bool, error) {
	state, err := s.SimpleSetupState()
	if err != nil {
		return false, err
	}
	if state != "" {
		return true, nil
	}
	var available bool
	err = s.db.QueryRow(simpleSetupAvailableQuery).Scan(&available)
	return available, err
}

func (s *Store) PrepareSimpleSetup() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var state string
	err = tx.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, simpleSetupStateKey).Scan(&state)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if state != "" {
		return nil
	}
	var available bool
	if err := tx.QueryRow(simpleSetupAvailableQuery).Scan(&available); err != nil {
		return err
	}
	if !available {
		return fmt.Errorf("已有专业配置，请在专业模式管理，简易配置不会覆盖现有设置")
	}
	now := time.Now().UnixMilli()
	groupIDs := make([]int64, 0, 2)
	for i, group := range []struct{ name, kind, exclude string }{
		{"自动选择", "urltest", "免费|过期|流量|官网|到期|剩余|套餐|订阅"},
		{"全部节点", "selector", ""},
	} {
		result, err := tx.Exec(`INSERT INTO node_groups (name, type, filter_include, filter_exclude, test_interval, tolerance, enabled, priority, created_at, updated_at)
			VALUES (?, ?, '.*', ?, 600, 100, 1, ?, ?, ?)`, group.name, group.kind, group.exclude, i*10, now, now)
		if err != nil {
			return err
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		groupIDs = append(groupIDs, id)
	}
	groupsJSON, _ := json.Marshal(groupIDs)
	for i, rule := range []struct {
		name, kind, outbound string
		values               []string
	}{
		{"局域网", "ip_cidr", "bypass", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"}},
		{"AI", "geosite", "proxy", []string{"openai", "anthropic", "google-gemini"}},
		{"视频", "geosite", "proxy", []string{"youtube", "netflix", "disney"}},
		{"Google", "geosite", "proxy", []string{"google"}},
		{"社交", "geosite", "proxy", []string{"telegram", "twitter", "facebook", "instagram"}},
		{"开发", "geosite", "proxy", []string{"github", "docker"}},
		{"CN", "mixed", "bypass", []string{"geosite:cn", "geoip:cn"}},
		{"默认代理", "fallback", "proxy", []string{}},
	} {
		valuesJSON, _ := json.Marshal(rule.values)
		result, err := tx.Exec(`INSERT INTO route_rules (name, enabled, priority, rule_type, values_json, outbound, created_at, updated_at)
			VALUES (?, 1, ?, ?, ?, ?, ?, ?)`, rule.name, (i+2)*10, rule.kind, string(valuesJSON), rule.outbound, now, now)
		if err != nil {
			return err
		}
		if rule.outbound != "proxy" {
			continue
		}
		id, err := result.LastInsertId()
		if err != nil {
			return err
		}
		ruleIDsJSON, _ := json.Marshal([]int64{id})
		priority := (i + 1) * 10
		if rule.name == "默认代理" {
			priority = 0
		}
		if _, err := tx.Exec(`INSERT INTO proxy_collections (name, type, source_type, referenced_group_ids, route_rule_id, route_rule_ids, enabled, priority, created_at, updated_at)
			VALUES (?, 'selector', 'node_groups_and_nodes', ?, ?, ?, 1, ?, ?, ?)`, rule.name, string(groupsJSON), id, string(ruleIDsJSON), priority, now, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE route_rules SET enabled = 1, updated_at = ? WHERE system_key = ?`, now, systemRuleAdBlockKey); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE route_rules SET outbound = 'proxy', updated_at = ? WHERE system_key = ?`, now, systemRuleGlobalDirectKey); err != nil {
		return err
	}
	if err := normalizeSystemRouteRuleOrderInTx(tx); err != nil {
		return err
	}
	for i, server := range []struct{ tag, kind, address, detour string }{
		{"dns_local", "udp", "223.5.5.5", "direct"},
		{"dns_proxy", "https", "1.1.1.1", "proxy"},
	} {
		if _, err := tx.Exec(`INSERT INTO dns_servers (tag, server_type, address, detour, priority, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, server.tag, server.kind, server.address, server.detour, i, now, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO dns_rules (conditions_json, server, priority, created_at, updated_at)
		VALUES ('{"geosite":["cn"]}', 'dns_local', 0, ?, ?)`, now, now); err != nil {
		return err
	}
	if _, _, err := ensureDefaultLocalDNSRuleTx(tx); err != nil {
		return err
	}
	requestJSON, err := json.Marshal(model.ConfigGenerateRequest{DefaultOutbound: "默认代理", InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort, LogLevel: "info"})
	if err != nil {
		return err
	}
	for key, value := range map[string]string{
		"inbound.mode": "tun", "proxy.mode": "rule", "dns_global.enabled": "true",
		"dns_global.final": "dns_proxy", "dns_global.proxy_final": "dns_proxy",
		"dns_global.fakeip_enabled": "false", "dns_global.strategy": "prefer_ipv4",
		"general.auto_start_core": "true", "general.dnsmasq_takeover_enabled": "true",
		configGeneratorRequestKey: string(requestJSON), simpleSetupStateKey: "prepared",
	} {
		if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
