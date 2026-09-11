package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const simpleSetupOptionsKey = "simple_setup.options"
const simpleDeviceRemark = "简易模式设备直连"

func (s *Store) SimpleSetupOptions() (*model.SimpleSetupOptions, error) {
	options := model.DefaultSimpleSetupOptions()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, simpleSetupOptionsKey).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		if err := json.Unmarshal([]byte(raw), options); err != nil {
			return nil, err
		}
	}
	state, err := s.SimpleSetupState()
	if err != nil || state == "" {
		return options, err
	}
	rules, err := s.ListRouteRules()
	if err != nil {
		return nil, err
	}
	found := map[string]int{}
	for _, rule := range rules {
		switch {
		case rule.SystemKey == systemRuleAdBlockKey:
			options.AdBlock = rule.Enabled
		case rule.SystemKey == systemRuleGlobalDirectKey:
			options.DefaultOutbound = rule.Outbound
		case rule.Name == "CN" && rule.RuleType == "mixed":
			if !rule.Enabled {
				return nil, errors.New("国内分流已在专业模式禁用，请在专业模式继续管理")
			}
			options.CNOutbound = rule.Outbound
			found[rule.Name]++
		case options.AppRouting[rule.Name] != "" && rule.RuleType == "geosite":
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM proxy_collections WHERE name = ? AND route_rule_id = ? AND enabled = ?`, rule.Name, rule.ID, rule.Outbound == "proxy").Scan(&count); err != nil {
				return nil, err
			}
			if !rule.Enabled || count != 1 {
				return nil, errors.New("应用分流或节点组已在专业模式修改，请在专业模式继续管理")
			}
			options.AppRouting[rule.Name] = rule.Outbound
			found[rule.Name]++
		}
	}
	for _, name := range []string{"CN", "AI", "视频", "Google", "社交", "开发"} {
		if found[name] != 1 {
			return nil, errors.New("内置分流已在专业模式修改，请在专业模式继续管理")
		}
	}
	servers, err := s.ListDNSServers()
	if err != nil {
		return nil, err
	}
	dnsFound := 0
	for _, server := range servers {
		if server.Tag == "dns_local" && server.ServerType == "udp" {
			options.LocalDNS = server.Address
			dnsFound++
		}
		if server.Tag == "dns_proxy" && server.ServerType == "https" {
			options.ProxyDNS = server.Address
			dnsFound++
		}
	}
	if dnsFound != 2 {
		return nil, errors.New("DNS 类型已在专业模式修改，请在专业模式继续管理")
	}
	dns, err := s.GetDNSGlobalSettings()
	if err != nil {
		return nil, err
	}
	options.DNSStrategy = dns.Strategy
	general, err := s.GetGeneralSettings()
	if err != nil {
		return nil, err
	}
	options.AutoStartCore = general.AutoStartCore
	bypass, err := s.GetTrafficBypassSettings()
	if err != nil {
		return nil, err
	}
	options.DirectDevices = []string{}
	for _, rule := range bypass.Rules {
		if rule.Type == "source_ip_cidr" && rule.Remark == simpleDeviceRemark {
			options.DirectDevices = append(options.DirectDevices, rule.Value)
		}
	}
	return options, nil
}

func (s *Store) SaveSimpleSetupOptions(options *model.SimpleSetupOptions) error {
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
		if err := applySimpleSetupOptionsTx(tx, options); err != nil {
			return err
		}
	}
	raw, err := json.Marshal(options)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, simpleSetupOptionsKey, string(raw), time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func applySimpleSetupOptionsTx(tx *sql.Tx, options *model.SimpleSetupOptions) error {
	now := time.Now().UnixMilli()
	for name, outbound := range options.AppRouting {
		var id int64
		var count int
		if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(MIN(id), 0) FROM route_rules WHERE name = ? AND rule_type = 'geosite' AND system_key = ''`, name).Scan(&count, &id); err != nil {
			return err
		}
		if count != 1 {
			return errors.New("内置分流已改变，无法保存简易配置")
		}
		if _, err := tx.Exec(`UPDATE route_rules SET outbound = ?, updated_at = ? WHERE id = ?`, outbound, now, id); err != nil {
			return err
		}
		result, err := tx.Exec(`UPDATE proxy_collections SET enabled = ?, updated_at = ? WHERE name = ? AND route_rule_id = ?`, outbound == "proxy", now, name, id)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return errors.New("应用节点组已改变，无法保存简易配置")
		}
	}
	updates := []struct {
		query string
		args  []interface{}
	}{
		{`UPDATE route_rules SET enabled = ?, updated_at = ? WHERE system_key = ?`, []interface{}{options.AdBlock, now, systemRuleAdBlockKey}},
		{`UPDATE route_rules SET outbound = ?, updated_at = ? WHERE system_key = ?`, []interface{}{options.DefaultOutbound, now, systemRuleGlobalDirectKey}},
		{`UPDATE route_rules SET outbound = ?, updated_at = ? WHERE name = '默认代理' AND rule_type = 'fallback'`, []interface{}{options.DefaultOutbound, now}},
		{`UPDATE route_rules SET outbound = ?, updated_at = ? WHERE name = 'CN' AND rule_type = 'mixed'`, []interface{}{options.CNOutbound, now}},
		{`UPDATE dns_servers SET address = ?, updated_at = ? WHERE tag = 'dns_local' AND server_type = 'udp'`, []interface{}{options.LocalDNS, now}},
		{`UPDATE dns_servers SET address = ?, updated_at = ? WHERE tag = 'dns_proxy' AND server_type = 'https'`, []interface{}{options.ProxyDNS, now}},
	}
	for _, update := range updates {
		result, err := tx.Exec(update.query, update.args...)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil || n != 1 {
			return fmt.Errorf("内置配置已改变，请在专业模式继续管理")
		}
	}
	bypass := defaultTrafficBypassSettings()
	var raw string
	err := tx.QueryRow(`SELECT value FROM app_settings WHERE key = 'traffic_bypass.rules'`).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	if err == nil {
		if err := json.Unmarshal([]byte(raw), &bypass.Rules); err != nil {
			return err
		}
	}
	rules := make([]model.TrafficBypassRule, 0, len(bypass.Rules)+len(options.DirectDevices))
	for _, rule := range bypass.Rules {
		if rule.Type != "source_ip_cidr" || rule.Remark != simpleDeviceRemark {
			rules = append(rules, rule)
		}
	}
	for _, cidr := range options.DirectDevices {
		rules = append(rules, model.TrafficBypassRule{Type: "source_ip_cidr", Value: cidr, Remark: simpleDeviceRemark})
	}
	rulesJSON, err := json.Marshal(rules)
	if err != nil {
		return err
	}
	for key, value := range map[string]string{
		"traffic_bypass.rules": string(rulesJSON), "dns_global.strategy": options.DNSStrategy,
		"general.auto_start_core": strconv.FormatBool(options.AutoStartCore),
	} {
		if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now); err != nil {
			return err
		}
	}
	return nil
}
