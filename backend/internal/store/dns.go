package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const defaultLocalDNSRuleMarker = "dns.defaults.local_real_ip_v1"

var defaultLocalDNSRuleSuffixes = []string{
	"localhost",
	"lan",
	"local",
	"internal",
	"intranet",
	"corp",
	"routerlogin.com",
	"router.asus.com",
	"miwifi.com",
	"mi.wifi",
	"tplogin.cn",
	"tendawifi.com",
	"my.router",
	"wifi.cmcc",
}

func SupportedDNSServerType(serverType string) bool {
	switch serverType {
	case "udp", "tcp", "https", "tls", "quic", "h3", "local", "hosts", "dhcp", "rcode", "fakeip":
		return true
	default:
		return false
	}
}

func RemoteDNSServerType(serverType string) bool {
	switch serverType {
	case "udp", "tcp", "https", "tls", "quic", "h3":
		return true
	default:
		return false
	}
}

func UsableRealDNSServer(serverType, address string) bool {
	switch serverType {
	case "local", "dhcp", "hosts":
		return true
	case "udp", "tcp", "https", "tls", "quic", "h3":
		return validRemoteDNSAddress(address)
	default:
		return false
	}
}

func validRemoteDNSAddress(address string) bool {
	address = strings.TrimSpace(address)
	if address == "" {
		return false
	}
	if parsed, err := netip.ParseAddr(strings.Trim(address, "[]")); err == nil && parsed.IsValid() {
		return true
	}
	var parsed *url.URL
	var err error
	if strings.Contains(address, "://") {
		parsed, err = url.Parse(address)
	} else {
		parsed, err = url.Parse("//" + address)
		if err == nil && (parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "") {
			return false
		}
	}
	if err != nil || parsed.User != nil || parsed.Hostname() == "" {
		return false
	}
	host := strings.TrimSuffix(parsed.Hostname(), ".")
	if parsedIP := net.ParseIP(host); parsedIP != nil {
		return true
	}
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < 'A' || char > 'Z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}

type dnsStoreExecutor interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	Query(query string, args ...interface{}) (*sql.Rows, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

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
	return getDNSServer(s.db, id)
}

func getDNSServer(executor dnsStoreExecutor, id int64) (*model.DNSServer, error) {
	var srv model.DNSServer
	var enabled int
	err := executor.QueryRow(`SELECT id, tag, enabled, server_type, address, address_resolver, address_strategy, strategy, detour, client_subnet, options_json, priority, created_at, updated_at FROM dns_servers WHERE id = ?`, id).Scan(&srv.ID, &srv.Tag, &enabled, &srv.ServerType, &srv.Address, &srv.AddressResolver, &srv.AddressStrategy, &srv.Strategy, &srv.Detour, &srv.ClientSubnet, &srv.OptionsJSON, &srv.Priority, &srv.CreatedAt, &srv.UpdatedAt)
	if err != nil {
		return nil, err
	}
	srv.Enabled = enabled == 1
	return &srv, nil
}

func (s *Store) CreateDNSServer(req *model.DNSServerRequest) (*model.DNSServer, error) {
	id, err := createDNSServer(s.db, req)
	if err != nil {
		return nil, err
	}
	return s.GetDNSServer(id)
}

func createDNSServer(executor dnsStoreExecutor, req *model.DNSServerRequest) (int64, error) {
	now := time.Now().Unix()
	var priority int
	if err := executor.QueryRow(`SELECT COALESCE(MAX(priority), -1) + 1 FROM dns_servers`).Scan(&priority); err != nil {
		return 0, err
	}
	optionsJSON, err := json.Marshal(req.Options)
	if err != nil {
		return 0, err
	}
	if req.Options == nil {
		optionsJSON = []byte("{}")
	}

	result, err := executor.Exec(`INSERT INTO dns_servers (tag, enabled, server_type, address, address_resolver, address_strategy, strategy, detour, client_subnet, options_json, priority, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		req.Tag, req.Enabled, req.ServerType, req.Address, req.AddressResolver, req.AddressStrategy, req.Strategy, req.Detour, req.ClientSubnet, string(optionsJSON), priority, now, now)
	if err != nil {
		return 0, err
	}

	id, err := result.LastInsertId()
	return id, err
}

func (s *Store) UpdateDNSServer(id int64, req *model.DNSServerRequest) error {
	return updateDNSServer(s.db, id, req)
}

func updateDNSServer(executor dnsStoreExecutor, id int64, req *model.DNSServerRequest) error {
	optionsJSON, err := json.Marshal(req.Options)
	if err != nil {
		return err
	}
	if req.Options == nil {
		optionsJSON = []byte("{}")
	}

	_, err = executor.Exec(`UPDATE dns_servers SET tag = ?, enabled = ?, server_type = ?, address = ?, address_resolver = ?, address_strategy = ?, strategy = ?, detour = ?, client_subnet = ?, options_json = ?, updated_at = ? WHERE id = ?`,
		req.Tag, req.Enabled, req.ServerType, req.Address, req.AddressResolver, req.AddressStrategy, req.Strategy, req.Detour, req.ClientSubnet, string(optionsJSON), time.Now().Unix(), id)
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

// DNS Hosts

func (s *Store) ListDNSHosts() ([]model.DNSHost, error) {
	rows, err := s.db.Query(`SELECT id, domain, addresses_json, enabled, comment, created_at, updated_at FROM dns_hosts ORDER BY domain COLLATE NOCASE ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.DNSHost, 0)
	for rows.Next() {
		item, err := scanDNSHost(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetDNSHost(id int64) (*model.DNSHost, error) {
	item, err := scanDNSHost(s.db.QueryRow(`SELECT id, domain, addresses_json, enabled, comment, created_at, updated_at FROM dns_hosts WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return item, err
}

func (s *Store) CreateDNSHost(req *model.DNSHostRequest) (*model.DNSHost, error) {
	addressesJSON, err := json.Marshal(req.Addresses)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`INSERT INTO dns_hosts (domain, addresses_json, enabled, comment, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		req.Domain, string(addressesJSON), req.Enabled, req.Comment, now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetDNSHost(id)
}

func (s *Store) UpdateDNSHost(id int64, req *model.DNSHostRequest) (*model.DNSHost, error) {
	addressesJSON, err := json.Marshal(req.Addresses)
	if err != nil {
		return nil, err
	}
	result, err := s.db.Exec(`UPDATE dns_hosts SET domain = ?, addresses_json = ?, enabled = ?, comment = ?, updated_at = ? WHERE id = ?`,
		req.Domain, string(addressesJSON), req.Enabled, req.Comment, time.Now().UnixMilli(), id)
	if err != nil {
		return nil, err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if updated == 0 {
		return nil, sql.ErrNoRows
	}
	return s.GetDNSHost(id)
}

func (s *Store) DeleteDNSHost(id int64) error {
	result, err := s.db.Exec(`DELETE FROM dns_hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if deleted == 0 {
		return sql.ErrNoRows
	}
	return nil
}

type dnsHostScanner interface {
	Scan(dest ...any) error
}

func scanDNSHost(scanner dnsHostScanner) (*model.DNSHost, error) {
	var item model.DNSHost
	var addressesJSON string
	var enabled int
	if err := scanner.Scan(&item.ID, &item.Domain, &addressesJSON, &enabled, &item.Comment, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(addressesJSON), &item.Addresses); err != nil {
		return nil, fmt.Errorf("decode DNS host %d addresses: %w", item.ID, err)
	}
	if item.Addresses == nil {
		item.Addresses = []string{}
	}
	item.Enabled = enabled != 0
	return &item, nil
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

// EnsureDefaultLocalDNSRule creates the editable real-IP exception after the
// first usable DNS server exists. The marker preserves an intentional delete.
func (s *Store) EnsureDefaultLocalDNSRule() error {
	s.dnsDefaultsMu.Lock()
	defer s.dnsDefaultsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	createdCount, serverTag, err := ensureDefaultLocalDNSRuleTx(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logDefaultLocalDNSRule(createdCount, serverTag)
	return nil
}

// CreateDNSServerWithDefaultLocalRule atomically creates the DNS server and
// the one-time editable local-domain exception.
func (s *Store) CreateDNSServerWithDefaultLocalRule(req *model.DNSServerRequest) (*model.DNSServer, error) {
	s.dnsDefaultsMu.Lock()
	defer s.dnsDefaultsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	id, err := createDNSServer(tx, req)
	if err != nil {
		return nil, err
	}
	createdCount, serverTag, err := ensureDefaultLocalDNSRuleTx(tx)
	if err != nil {
		return nil, err
	}
	server, err := getDNSServer(tx, id)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	logDefaultLocalDNSRule(createdCount, serverTag)
	return server, nil
}

// UpdateDNSServerWithDefaultLocalRule atomically handles a disabled server
// becoming the first usable real DNS server.
func (s *Store) UpdateDNSServerWithDefaultLocalRule(id int64, req *model.DNSServerRequest) error {
	s.dnsDefaultsMu.Lock()
	defer s.dnsDefaultsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := updateDNSServer(tx, id, req); err != nil {
		return err
	}
	createdCount, serverTag, err := ensureDefaultLocalDNSRuleTx(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	logDefaultLocalDNSRule(createdCount, serverTag)
	return nil
}

func ensureDefaultLocalDNSRuleTx(tx *sql.Tx) (int, string, error) {
	var marker string
	err := tx.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, defaultLocalDNSRuleMarker).Scan(&marker)
	if err == nil {
		return 0, "", nil
	}
	if err != sql.ErrNoRows {
		return 0, "", err
	}

	serverRows, err := tx.Query(`SELECT tag, server_type, address FROM dns_servers
		WHERE enabled = 1
		ORDER BY CASE WHEN server_type IN ('local', 'dhcp', 'hosts') THEN 0 ELSE 1 END, priority ASC, id ASC
	`)
	if err != nil {
		return 0, "", err
	}
	var serverTag string
	for serverRows.Next() {
		var tag, serverType, address string
		if err := serverRows.Scan(&tag, &serverType, &address); err != nil {
			serverRows.Close()
			return 0, "", err
		}
		if UsableRealDNSServer(serverType, address) {
			serverTag = tag
			break
		}
	}
	if err := serverRows.Err(); err != nil {
		serverRows.Close()
		return 0, "", err
	}
	serverRows.Close()
	if serverTag == "" {
		return 0, "", nil
	}

	rows, err := tx.Query(`SELECT r.conditions_json, s.server_type, s.address
		FROM dns_rules r
		JOIN dns_servers s ON s.tag = r.server
		WHERE r.enabled = 1 AND s.enabled = 1`)
	if err != nil {
		return 0, "", err
	}
	equivalentRuleExists := false
	for rows.Next() {
		var raw, serverType, address string
		if err := rows.Scan(&raw, &serverType, &address); err != nil {
			rows.Close()
			return 0, "", err
		}
		if UsableRealDNSServer(serverType, address) && dnsRuleCoversDefaultLocalSuffixes(raw) {
			equivalentRuleExists = true
			break
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, "", err
	}
	rows.Close()

	createdCount := 0
	if !equivalentRuleExists {
		conditionsJSON, err := json.Marshal(map[string]interface{}{"domain_suffix": defaultLocalDNSRuleSuffixes})
		if err != nil {
			return 0, "", err
		}
		var priority int
		if err := tx.QueryRow(`SELECT COALESCE(MAX(priority), -1) + 1 FROM dns_rules`).Scan(&priority); err != nil {
			return 0, "", err
		}
		now := time.Now().Unix()
		if _, err := tx.Exec(`INSERT INTO dns_rules (enabled, priority, rule_type, conditions_json, server, disable_cache, rewrite_ttl, client_subnet, created_at, updated_at)
			VALUES (1, ?, 'default', ?, ?, 0, 0, '', ?, ?)`, priority, string(conditionsJSON), serverTag, now, now); err != nil {
			return 0, "", err
		}
		createdCount = len(defaultLocalDNSRuleSuffixes)
	}

	if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, '1', unixepoch())
		ON CONFLICT(key) DO NOTHING`, defaultLocalDNSRuleMarker); err != nil {
		return 0, "", err
	}
	return createdCount, serverTag, nil
}

func dnsRuleCoversDefaultLocalSuffixes(raw string) bool {
	var conditions map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil || len(conditions) != 1 {
		return false
	}
	rawSuffixes, exists := conditions["domain_suffix"]
	if !exists {
		return false
	}
	var suffixes []string
	if err := json.Unmarshal(rawSuffixes, &suffixes); err != nil {
		return false
	}
	covered := make(map[string]bool, len(suffixes))
	for _, suffix := range suffixes {
		normalized := strings.ToLower(strings.TrimSpace(suffix))
		if suffix != normalized {
			return false
		}
		covered[normalized] = true
	}
	for _, suffix := range defaultLocalDNSRuleSuffixes {
		if !covered[suffix] {
			return false
		}
	}
	return true
}

func logDefaultLocalDNSRule(createdCount int, serverTag string) {
	if createdCount > 0 {
		logging.Info("dns.rule.default", "已生成本地域名真实 IP 例外: %d 项, server=%s", createdCount, serverTag)
	}
}

// DNS Global Settings (复用 app_settings 表)

func (s *Store) GetDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	r := &model.DNSGlobalSettings{
		Enabled:          true,
		Final:            "dns_proxy",
		ProxyFinal:       "",
		Strategy:         "prefer_ipv4",
		DisableCache:     false,
		DisableExpire:    false,
		IndependentCache: false,
		ReverseMapping:   false,
		CacheCapacity:    4096,
		ClientSubnet:     "",
		FakeIPEnabled:    false,
		FakeIPInet4Range: "198.18.0.1/16",
		FakeIPInet6Range: "fc00::/18",
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
		case "dns_global.proxy_final":
			r.ProxyFinal = value
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
	return s.setDNSGlobalSettings(req, true)
}

func (s *Store) SetDNSGlobalSettingsForCore(req *model.DNSGlobalSettings, independentCacheSupported bool) error {
	return s.setDNSGlobalSettings(req, independentCacheSupported)
}

func (s *Store) setDNSGlobalSettings(req *model.DNSGlobalSettings, independentCacheSupported bool) error {
	now := time.Now().Unix()
	settings := map[string]string{
		"dns.enabled":                   fmt.Sprintf("%t", req.Enabled),
		"dns_global.enabled":            fmt.Sprintf("%t", req.Enabled),
		"dns.final":                     req.Final,
		"dns_global.final":              req.Final,
		"dns_global.proxy_final":        req.ProxyFinal,
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
	if !independentCacheSupported {
		settings["dns.independent_cache"] = ""
		settings["dns_global.independent_cache"] = ""
	} else {
		settings["dns.independent_cache_initialized"] = "true"
		settings["dns.independent_cache_preference"] = fmt.Sprintf("%t", req.IndependentCache)
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

func (s *Store) MigrateDNSIndependentCache(supported bool) (bool, error) {
	const initializedKey = "dns.independent_cache_initialized"
	const preferenceKey = "dns.independent_cache_preference"

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	readSetting := func(key string) (string, bool, error) {
		var value string
		err := tx.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, key).Scan(&value)
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return value, err == nil, err
	}

	initializedValue, initialized, err := readSetting(initializedKey)
	if err != nil {
		return false, err
	}
	initialized = initialized && initializedValue == "true"
	preferenceValue, hasPreference, err := readSetting(preferenceKey)
	if err != nil {
		return false, err
	}
	preference := preferenceValue == "true"
	changed := false
	now := time.Now().Unix()
	if !initialized || !hasPreference {
		preference = true
		if !supported {
			if value, exists, readErr := readSetting("dns_global.independent_cache"); readErr != nil {
				return false, readErr
			} else if exists {
				preference = value == "true"
			} else if value, exists, readErr = readSetting("dns.independent_cache"); readErr != nil {
				return false, readErr
			} else if exists {
				preference = value == "true"
			}
		}
		for key, value := range map[string]string{
			initializedKey: "true",
			preferenceKey:  fmt.Sprintf("%t", preference),
		} {
			if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now); err != nil {
				return false, err
			}
		}
		changed = true
	}
	if supported {
		value := fmt.Sprintf("%t", preference)
		for _, key := range []string{"dns.independent_cache", "dns_global.independent_cache"} {
			currentValue, exists, err := readSetting(key)
			if err != nil {
				return false, err
			}
			if !exists || currentValue != value {
				changed = true
			}
			if _, err := tx.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now); err != nil {
				return false, err
			}
		}
	} else {
		result, err := tx.Exec(`DELETE FROM app_settings WHERE key IN ('dns.independent_cache', 'dns_global.independent_cache')`)
		if err != nil {
			return false, err
		}
		if affected, err := result.RowsAffected(); err != nil {
			return false, err
		} else if affected > 0 {
			changed = true
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return changed, nil
}
