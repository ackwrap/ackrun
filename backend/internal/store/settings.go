package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func defaultTrafficBypassSettings() *model.TrafficBypassSettings {
	return &model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{
		{Type: "process_name", Value: "easytier-core"},
		{Type: "interface", Value: "easytier-tun"},
		{Type: "ip_cidr", Value: "10.0.0.0/8"},
		{Type: "ip_cidr", Value: "172.16.0.0/12"},
		{Type: "ip_cidr", Value: "192.168.0.0/16"},
	}}
}

func (s *Store) GetTrafficBypassSettings() (*model.TrafficBypassSettings, error) {
	settings := defaultTrafficBypassSettings()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'traffic_bypass.rules'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &settings.Rules); err != nil {
		return nil, fmt.Errorf("解析流量排除设置失败: %w", err)
	}
	if settings.Rules == nil {
		settings.Rules = []model.TrafficBypassRule{}
	}
	return settings, nil
}

func (s *Store) SetTrafficBypassSettings(settings *model.TrafficBypassSettings) error {
	raw, err := json.Marshal(settings.Rules)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ('traffic_bypass.rules', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, string(raw), time.Now().Unix())
	return err
}

const (
	DefaultConnectivityTestURL         = "http://www.gstatic.com/generate_204"
	DefaultConnectivityIntervalSeconds = 300
)

func (s *Store) GetUpdateSettings() (*model.UpdateSettingsResponse, error) {
	r := &model.UpdateSettingsResponse{Acceleration: "ghproxy"}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key IN ('update.acceleration', 'update.custom_mirror_url')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "update.acceleration":
			r.Acceleration = value
		case "update.custom_mirror_url":
			r.CustomMirrorURL = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) SetUpdateSettings(req *model.UpdateSettings) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"update.acceleration":      req.Acceleration,
		"update.custom_mirror_url": req.CustomMirrorURL,
	}
	for key, value := range settings {
		if value == "" {
			// 允许空值删除设置
			_, err := s.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
			if err != nil {
				return err
			}
			continue
		}
		_, err := s.db.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetLogSettings() (*model.LogSettingsResponse, error) {
	settings := &model.LogSettingsResponse{Level: "info", Timestamp: true}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key IN ('log.level', 'log.timestamp')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "log.level":
			if value != "" {
				settings.Level = value
			}
		case "log.timestamp":
			settings.Timestamp = value == "true"
		}
	}
	return settings, rows.Err()
}

func (s *Store) GetLogLevel() string {
	settings, err := s.GetLogSettings()
	if err != nil || settings == nil || settings.Level == "" {
		return "info"
	}
	return settings.Level
}

func (s *Store) GetLogTimestamp() bool {
	settings, err := s.GetLogSettings()
	if err != nil || settings == nil {
		return true
	}
	return settings.Timestamp
}

func (s *Store) SetLogSettings(req *model.LogSettings) error {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{
		"log.level":     req.Level,
		"log.timestamp": fmt.Sprintf("%t", req.Timestamp),
	} {
		if _, err := tx.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		UPDATE app_settings
		SET value = json_set(value, '$.log_level', ?), updated_at = ?
		WHERE key = ?
	`, req.Level, now, configGeneratorRequestKey); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetConnectivitySettings() (*model.ConnectivitySettings, error) {
	settings := &model.ConnectivitySettings{
		TestURL:         DefaultConnectivityTestURL,
		IntervalSeconds: DefaultConnectivityIntervalSeconds,
	}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key IN ('connectivity.test_url', 'connectivity.interval_seconds')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "connectivity.test_url":
			if value != "" {
				settings.TestURL = value
			}
		case "connectivity.interval_seconds":
			if interval, err := strconv.Atoi(value); err == nil {
				settings.IntervalSeconds = interval
			}
		}
	}
	return settings, rows.Err()
}

func (s *Store) SetConnectivitySettings(req *model.ConnectivitySettings) error {
	now := time.Now().Unix()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for key, value := range map[string]string{
		"connectivity.test_url":         req.TestURL,
		"connectivity.interval_seconds": strconv.Itoa(req.IntervalSeconds),
	} {
		if _, err := tx.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetNTPSettings 获取 NTP 设置
func (s *Store) GetNTPSettings() (*model.NTPSettingsResponse, error) {
	// 默认开启
	r := &model.NTPSettingsResponse{
		Enabled:    true,
		Server:     "time.apple.com",
		ServerPort: 123,
		Interval:   "30m",
		Detour:     "direct",
	}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'ntp.%'`)
	if err != nil {
		return r, nil
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "ntp.enabled":
			r.Enabled = value == "true"
		case "ntp.server":
			if value != "" {
				r.Server = value
			}
		case "ntp.server_port":
			if port, err := strconv.Atoi(value); err == nil {
				r.ServerPort = port
			}
		case "ntp.interval":
			if value != "" {
				r.Interval = value
			}
		case "ntp.detour":
			if value != "" {
				r.Detour = value
			}
		}
	}
	return r, nil
}

// SetNTPSettings 设置 NTP 配置
func (s *Store) SetNTPSettings(req *model.NTPSettings) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"ntp.enabled":     fmt.Sprintf("%t", req.Enabled),
		"ntp.server":      req.Server,
		"ntp.interval":    req.Interval,
		"ntp.detour":      req.Detour,
		"ntp.server_port": fmt.Sprintf("%d", req.ServerPort),
	}
	for key, value := range settings {
		if value == "" {
			_, err := s.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
			if err != nil {
				return err
			}
			continue
		}
		_, err := s.db.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetGeneralSettings() (*model.GeneralSettings, error) {
	settings := &model.GeneralSettings{AutoStartCore: true}
	var value string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'general.auto_start_core'`).Scan(&value)
	if err == sql.ErrNoRows {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	settings.AutoStartCore = value != "false"
	return settings, nil
}

func (s *Store) SetGeneralSettings(settings *model.GeneralSettings) error {
	_, err := s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at)
		VALUES ('general.auto_start_core', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, strconv.FormatBool(settings.AutoStartCore), time.Now().Unix())
	return err
}

func (s *Store) GetMixedInboundSettings() (*model.MixedInboundSettings, error) {
	settings := &model.MixedInboundSettings{}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key IN ('mixed.username', 'mixed.password')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "mixed.username":
			settings.Username = value
		case "mixed.password":
			settings.Password = value
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) SetMixedInboundSettings(settings *model.MixedInboundSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	values := map[string]string{
		"mixed.username": settings.Username,
		"mixed.password": settings.Password,
	}
	for key, value := range values {
		if value == "" {
			if _, err := tx.Exec(`DELETE FROM app_settings WHERE key = ?`, key); err != nil {
				return err
			}
			continue
		}
		if _, err := tx.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetDNSSettings 获取 DNS 设置
func (s *Store) GetDNSSettings() (*model.DNSSettingsResponse, error) {
	r := &model.DNSSettingsResponse{
		Enabled:          true,
		ProxyServer:      "https://1.1.1.1/dns-query",
		DirectServer:     "https://223.5.5.5/dns-query",
		Resolver:         "223.5.5.5",
		Final:            "dns_proxy",
		Strategy:         "prefer_ipv4",
		AddressStrategy:  "prefer_ipv4",
		DisableCache:     false,
		DisableExpire:    false,
		IndependentCache: false,
		ReverseMapping:   false,
		ClientSubnet:     "",
		FakeIPEnabled:    false,
		FakeIPInet4Range: "198.18.0.1/16",
		FakeIPInet6Range: "fdfe:dcba:9876::/48",
		RouteCN:          true,
		RouteNonCN:       true,
		BlockAds:         true,
	}
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'dns.%'`)
	if err != nil {
		return r, nil
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "dns.enabled":
			r.Enabled = value == "true"
		case "dns.proxy_server":
			if value != "" {
				r.ProxyServer = value
			}
		case "dns.direct_server":
			if value != "" {
				r.DirectServer = value
			}
		case "dns.resolver":
			if value != "" {
				r.Resolver = value
			}
		case "dns.final":
			if value != "" {
				r.Final = value
			}
		case "dns.strategy":
			if value != "" {
				r.Strategy = value
			}
		case "dns.address_strategy":
			if value != "" {
				r.AddressStrategy = value
			}
		case "dns.disable_cache":
			r.DisableCache = value == "true"
		case "dns.disable_expire":
			r.DisableExpire = value == "true"
		case "dns.independent_cache":
			r.IndependentCache = value == "true"
		case "dns.reverse_mapping":
			r.ReverseMapping = value == "true"
		case "dns.client_subnet":
			r.ClientSubnet = value
		case "dns.fakeip_enabled":
			r.FakeIPEnabled = value == "true"
		case "dns.fakeip_inet4_range":
			if value != "" {
				r.FakeIPInet4Range = value
			}
		case "dns.fakeip_inet6_range":
			if value != "" {
				r.FakeIPInet6Range = value
			}
		case "dns.route_cn":
			r.RouteCN = value == "true"
		case "dns.route_non_cn":
			r.RouteNonCN = value == "true"
		case "dns.block_ads":
			r.BlockAds = value == "true"
		}
	}
	return r, nil
}

// SetDNSSettings 设置 DNS 配置
func (s *Store) SetDNSSettings(req *model.DNSSettings) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"dns.enabled":                   fmt.Sprintf("%t", req.Enabled),
		"dns_global.enabled":            fmt.Sprintf("%t", req.Enabled),
		"dns.proxy_server":              req.ProxyServer,
		"dns.direct_server":             req.DirectServer,
		"dns.resolver":                  req.Resolver,
		"dns.final":                     req.Final,
		"dns_global.final":              req.Final,
		"dns.strategy":                  req.Strategy,
		"dns_global.strategy":           req.Strategy,
		"dns.address_strategy":          req.AddressStrategy,
		"dns.disable_cache":             fmt.Sprintf("%t", req.DisableCache),
		"dns_global.disable_cache":      fmt.Sprintf("%t", req.DisableCache),
		"dns.disable_expire":            fmt.Sprintf("%t", req.DisableExpire),
		"dns_global.disable_expire":     fmt.Sprintf("%t", req.DisableExpire),
		"dns.independent_cache":         fmt.Sprintf("%t", req.IndependentCache),
		"dns_global.independent_cache":  fmt.Sprintf("%t", req.IndependentCache),
		"dns.reverse_mapping":           fmt.Sprintf("%t", req.ReverseMapping),
		"dns_global.reverse_mapping":    fmt.Sprintf("%t", req.ReverseMapping),
		"dns.client_subnet":             req.ClientSubnet,
		"dns_global.client_subnet":      req.ClientSubnet,
		"dns.fakeip_enabled":            fmt.Sprintf("%t", req.FakeIPEnabled),
		"dns_global.fakeip_enabled":     fmt.Sprintf("%t", req.FakeIPEnabled),
		"dns.fakeip_inet4_range":        req.FakeIPInet4Range,
		"dns_global.fakeip_inet4_range": req.FakeIPInet4Range,
		"dns.fakeip_inet6_range":        req.FakeIPInet6Range,
		"dns_global.fakeip_inet6_range": req.FakeIPInet6Range,
		"dns.route_cn":                  fmt.Sprintf("%t", req.RouteCN),
		"dns.route_non_cn":              fmt.Sprintf("%t", req.RouteNonCN),
		"dns.block_ads":                 fmt.Sprintf("%t", req.BlockAds),
	}
	for key, value := range settings {
		if value == "" {
			_, err := s.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
			if err != nil {
				return err
			}
			continue
		}
		_, err := s.db.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now)
		if err != nil {
			return err
		}
	}
	return nil
}

// GetInboundMode 获取入站运行模式
func (s *Store) GetInboundMode() string {
	var mode string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'inbound.mode'`).Scan(&mode)
	if err != nil {
		return "tun_mixed" // 默认 TUN + Mixed
	}
	return mode
}

// SetInboundMode 设置入站运行模式
func (s *Store) SetInboundMode(mode string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, "inbound.mode", mode, now)
	return err
}

// GetProxyMode 获取代理模式
func (s *Store) GetProxyMode() string {
	var mode string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'proxy.mode'`).Scan(&mode)
	if err != nil {
		return "rule" // 默认规则模式
	}
	return mode
}

// SetProxyMode 设置代理模式
func (s *Store) SetProxyMode(mode string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, "proxy.mode", mode, now)
	return err
}

// GetExperimentalSettings 获取实验性功能设置（始终返回默认值，如果未设置）
func (s *Store) GetExperimentalSettings() (*model.ExperimentalSettingsResponse, error) {
	// 始终返回默认值作为后备
	r := &model.ExperimentalSettingsResponse{
		ClashAPIEnabled:      true,
		ClashAPIPort:         "9090",
		CacheFileEnabled:     true,
		CacheFileStoreFakeIP: true,
		CacheFileStoreDNS:    true,
	}
	storeDNSConfigured := false
	legacyStoreRDRC := false
	legacyStoreRDRCConfigured := false

	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'experimental.%'`)
	if err != nil {
		return r, nil
	}
	defer rows.Close()

	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			continue
		}
		switch key {
		case "experimental.clash_api.enabled":
			r.ClashAPIEnabled = value == "true"
		case "experimental.clash_api.port":
			r.ClashAPIPort = value
		case "experimental.clash_api.secret":
			r.ClashAPISecret = value
		case "experimental.clash_api.external_ui":
			r.ClashAPIExternalUI = value
		case "experimental.clash_api.external_ui_download_url":
			r.ClashAPIExternalUIDownloadURL = value
		case "experimental.clash_api.dashboard":
			r.ClashAPIDashboard = value
		case "experimental.cache_file.enabled":
			r.CacheFileEnabled = value == "true"
		case "experimental.cache_file.store_fakeip":
			r.CacheFileStoreFakeIP = value == "true"
		case "experimental.cache_file.store_dns":
			r.CacheFileStoreDNS = value == "true"
			storeDNSConfigured = true
		case "experimental.cache_file.store_rdrc":
			legacyStoreRDRC = value == "true"
			legacyStoreRDRCConfigured = true
		}
	}
	if !storeDNSConfigured && legacyStoreRDRCConfigured {
		r.CacheFileStoreDNS = legacyStoreRDRC
	}

	// 确保 Clash API 始终启用（强制）
	r.ClashAPIEnabled = true

	return r, nil
}

// SetExperimentalSettings 设置实验性功能设置
func (s *Store) SetExperimentalSettings(req *model.ExperimentalSettings) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"experimental.clash_api.enabled":                  fmt.Sprintf("%t", req.ClashAPIEnabled),
		"experimental.clash_api.port":                     req.ClashAPIPort,
		"experimental.clash_api.secret":                   req.ClashAPISecret,
		"experimental.clash_api.external_ui":              req.ClashAPIExternalUI,
		"experimental.clash_api.external_ui_download_url": req.ClashAPIExternalUIDownloadURL,
		"experimental.clash_api.dashboard":                req.ClashAPIDashboard,
		"experimental.cache_file.enabled":                 fmt.Sprintf("%t", req.CacheFileEnabled),
		"experimental.cache_file.store_fakeip":            fmt.Sprintf("%t", req.CacheFileStoreFakeIP),
		"experimental.cache_file.store_dns":               fmt.Sprintf("%t", req.CacheFileStoreDNS),
	}
	for key, value := range settings {
		if value == "" {
			_, err := s.db.Exec(`DELETE FROM app_settings WHERE key = ?`, key)
			if err != nil {
				return err
			}
			continue
		}
		_, err := s.db.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now)
		if err != nil {
			return err
		}
	}
	if _, err := s.db.Exec(`DELETE FROM app_settings WHERE key IN ('experimental.cache_file.store_rdrc', 'experimental.cache_file.rdrc_timeout')`); err != nil {
		return err
	}
	return nil
}
