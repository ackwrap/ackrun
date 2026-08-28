package store

import (
	"encoding/json"

	"github.com/ackwrap/ackrun/internal/logging"
)

func (s *Store) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS install_state (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			status TEXT NOT NULL,
			version TEXT,
			binary_path TEXT,
			message TEXT,
			error TEXT,
			progress REAL,
			updated_at INTEGER
		)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			user_agent TEXT NOT NULL DEFAULT 'clash-meta/2.4.0',
			sync_interval_minutes INTEGER NOT NULL DEFAULT 0,
			sync_mode TEXT NOT NULL DEFAULT 'off',
			sync_time TEXT NOT NULL DEFAULT '',
			sync_weekday INTEGER NOT NULL DEFAULT 0,
			sync_status TEXT NOT NULL DEFAULT 'updated',
			sync_progress REAL NOT NULL DEFAULT 100,
			sync_timeout_seconds INTEGER NOT NULL DEFAULT 60,
			node_count INTEGER NOT NULL DEFAULT 0,
			traffic_used_bytes INTEGER NOT NULL DEFAULT 0,
			traffic_total_bytes INTEGER NOT NULL DEFAULT 0,
			expire_at INTEGER NOT NULL DEFAULT 0,
			last_sync_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`ALTER TABLE subscriptions ADD COLUMN user_agent TEXT NOT NULL DEFAULT 'clash-meta/2.4.0'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_interval_minutes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscriptions ADD COLUMN sync_mode TEXT NOT NULL DEFAULT 'off'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscriptions ADD COLUMN sync_weekday INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscriptions ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'updated'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_progress REAL NOT NULL DEFAULT 100`,
		`ALTER TABLE subscriptions ADD COLUMN sync_timeout_seconds INTEGER NOT NULL DEFAULT 60`,
		`CREATE TABLE IF NOT EXISTS nodes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			uid TEXT NOT NULL DEFAULT '',
			subscription_id INTEGER NOT NULL,
			name TEXT NOT NULL,
			name_overridden INTEGER NOT NULL DEFAULT 0,
			type TEXT NOT NULL,
			server TEXT NOT NULL,
			server_port INTEGER NOT NULL DEFAULT 0,
			raw TEXT NOT NULL,
			raw_json TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			preferred INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'unknown',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
		)`,
		`ALTER TABLE nodes ADD COLUMN uid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN name_overridden INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE nodes ADD COLUMN preferred INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN last_test_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN test_latency_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN test_success INTEGER NOT NULL DEFAULT 0`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_nodes_subscription_uid ON nodes(subscription_id, uid) WHERE uid <> ''`,
		`CREATE TABLE IF NOT EXISTS node_filters (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			target TEXT NOT NULL,
			pattern TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS route_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			rule_type TEXT NOT NULL,
			values_json TEXT NOT NULL,
			outbound TEXT NOT NULL,
			invert INTEGER NOT NULL DEFAULT 0,
			system_key TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`ALTER TABLE route_rules ADD COLUMN system_key TEXT NOT NULL DEFAULT ''`,
		`UPDATE route_rules SET system_key = 'ad_block' WHERE id = (SELECT id FROM route_rules WHERE name = '广告拦截' AND COALESCE(system_key, '') = '' ORDER BY id ASC LIMIT 1) AND NOT EXISTS (SELECT 1 FROM route_rules WHERE system_key = 'ad_block')`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_route_rules_system_key ON route_rules(system_key) WHERE system_key <> ''`,
		`CREATE TABLE IF NOT EXISTS route_rule_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			tag TEXT NOT NULL UNIQUE,
			url TEXT NOT NULL,
			format TEXT NOT NULL DEFAULT 'binary',
			use_proxy INTEGER NOT NULL DEFAULT 0,
			sync_mode TEXT NOT NULL DEFAULT 'daily',
			sync_time TEXT NOT NULL DEFAULT '04:00:00',
			sync_weekday INTEGER NOT NULL DEFAULT 0,
			sync_status TEXT NOT NULL DEFAULT 'pending',
			sync_progress REAL NOT NULL DEFAULT 0,
			sync_error TEXT NOT NULL DEFAULT '',
			last_sync_at INTEGER NOT NULL DEFAULT 0,
			cached_path TEXT NOT NULL DEFAULT '',
			cached_updated_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_mode TEXT NOT NULL DEFAULT 'daily'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_time TEXT NOT NULL DEFAULT '04:00:00'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_weekday INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_progress REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_error TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN last_sync_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN cached_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN cached_updated_at INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS geo_assets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL UNIQUE,
			url TEXT NOT NULL,
			use_proxy INTEGER NOT NULL DEFAULT 0,
			sync_mode TEXT NOT NULL DEFAULT 'daily',
			sync_time TEXT NOT NULL DEFAULT '03:30:00',
			sync_weekday INTEGER NOT NULL DEFAULT 0,
			sync_status TEXT NOT NULL DEFAULT 'pending',
			sync_error TEXT NOT NULL DEFAULT '',
			last_sync_at INTEGER NOT NULL DEFAULT 0,
			local_path TEXT NOT NULL DEFAULT '',
			cached_updated_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_collections (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			test_url TEXT NOT NULL DEFAULT 'https://www.gstatic.com/generate_204',
			test_interval INTEGER NOT NULL DEFAULT 300,
			tolerance INTEGER NOT NULL DEFAULT 100,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			route_rule_id INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS dns_servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			tag TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			server_type TEXT NOT NULL,
			address TEXT NOT NULL DEFAULT '',
			address_resolver TEXT NOT NULL DEFAULT '',
			address_strategy TEXT NOT NULL DEFAULT '',
			strategy TEXT NOT NULL DEFAULT '',
			detour TEXT NOT NULL DEFAULT '',
			client_subnet TEXT NOT NULL DEFAULT '',
			options_json TEXT NOT NULL DEFAULT '{}',
			priority INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS dns_hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			domain TEXT NOT NULL COLLATE NOCASE UNIQUE,
			addresses_json TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1,
			comment TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS dns_rules (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			rule_type TEXT NOT NULL DEFAULT 'default',
			conditions_json TEXT NOT NULL DEFAULT '{}',
			server TEXT NOT NULL DEFAULT '',
			disable_cache INTEGER NOT NULL DEFAULT 0,
			rewrite_ttl INTEGER NOT NULL DEFAULT 0,
			client_subnet TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS node_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			filter_protocols TEXT NOT NULL DEFAULT '',
			filter_subscriptions TEXT NOT NULL DEFAULT '',
			filter_include TEXT NOT NULL,
			filter_exclude TEXT NOT NULL DEFAULT '',
			test_url TEXT NOT NULL DEFAULT 'https://www.gstatic.com/generate_204',
			test_interval INTEGER NOT NULL DEFAULT 300,
			tolerance INTEGER NOT NULL DEFAULT 100,
			enabled INTEGER NOT NULL DEFAULT 1,
			priority INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`ALTER TABLE proxy_collections ADD COLUMN source_type TEXT NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE proxy_collections ADD COLUMN referenced_group_ids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN route_rule_ids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN route_rule_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_collections ADD COLUMN node_uids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dns_servers ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE node_groups ADD COLUMN node_uids TEXT NOT NULL DEFAULT '[]'`,
		`UPDATE node_groups SET node_uids = '[]' WHERE node_uids = '' OR node_uids = 'null'`,
		`UPDATE node_groups SET filter_exclude = '' WHERE name = '全部节点' AND filter_include = '.*' AND filter_exclude = '免费|过期|流量|官网|到期|剩余'`,
		`CREATE TABLE IF NOT EXISTS geoip_providers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			provider_key TEXT NOT NULL UNIQUE,
			template TEXT NOT NULL DEFAULT 'builtin',
			url TEXT NOT NULL DEFAULT '',
			ip_parameter TEXT NOT NULL DEFAULT '',
			mapping_json TEXT NOT NULL DEFAULT '{}',
			enabled INTEGER NOT NULL DEFAULT 1,
			is_default INTEGER NOT NULL DEFAULT 0,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_geoip_providers_default ON geoip_providers(is_default) WHERE is_default = 1`,
		`INSERT OR IGNORE INTO geoip_providers (name, provider_key, template, enabled, is_default, builtin, created_at, updated_at) VALUES
			('ipapi.is', 'ipapi.is', 'builtin', 1, 1, 1, unixepoch(), unixepoch()),
			('LeoMoeAPI', 'leomoeapi', 'builtin', 1, 0, 1, unixepoch(), unixepoch()),
			('IP.SB', 'ip.sb', 'builtin', 1, 0, 1, unixepoch(), unixepoch()),
			('IPInfo', 'ipinfo', 'builtin', 1, 0, 1, unixepoch(), unixepoch()),
			('IP-API.com', 'ip-api.com', 'builtin', 1, 0, 1, unixepoch(), unixepoch()),
			('百度 IP', 'baidu', 'builtin', 1, 0, 1, unixepoch(), unixepoch())`,
		`UPDATE geoip_providers SET is_default = 0, updated_at = unixepoch() WHERE provider_key = 'songzixian' AND builtin = 1 AND is_default = 1`,
		`DELETE FROM geoip_providers WHERE provider_key = 'songzixian' AND builtin = 1`,
		`UPDATE geoip_providers SET is_default = 1, enabled = 1, updated_at = unixepoch()
			WHERE provider_key = 'ipapi.is'
			AND NOT EXISTS (SELECT 1 FROM geoip_providers WHERE is_default = 1)`,
		`CREATE TABLE IF NOT EXISTS connectivity_targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			builtin INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`INSERT OR IGNORE INTO connectivity_targets (name, url, enabled, builtin, created_at, updated_at) VALUES
			('Google HTTP', 'http://www.gstatic.com/generate_204', 1, 1, unixepoch(), unixepoch()),
			('Cloudflare HTTP', 'http://cp.cloudflare.com/generate_204', 1, 1, unixepoch(), unixepoch()),
			('Apple HTTP', 'http://captive.apple.com/generate_204', 1, 1, unixepoch(), unixepoch()),
			('Google HTTPS', 'https://www.gstatic.com/generate_204', 1, 1, unixepoch(), unixepoch()),
			('Cloudflare HTTPS', 'https://cp.cloudflare.com/generate_204', 1, 1, unixepoch(), unixepoch())`,
		`INSERT OR IGNORE INTO connectivity_targets (name, url, enabled, builtin, created_at, updated_at)
			SELECT '现有连通性地址', value, 1, 0, unixepoch(), unixepoch()
			FROM app_settings WHERE key = 'connectivity.test_url' AND value <> ''`,
		`CREATE TABLE IF NOT EXISTS config_backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			config_name TEXT NOT NULL,
			file_name TEXT NOT NULL,
			path TEXT NOT NULL,
			backup_date TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			UNIQUE(config_name, backup_date)
		)`,
		`CREATE TABLE IF NOT EXISTS node_exposures (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			subscription_id INTEGER NOT NULL,
			node_uid TEXT NOT NULL,
			inbound_type TEXT NOT NULL,
			listen TEXT NOT NULL DEFAULT '127.0.0.1',
			listen_port INTEGER NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			password TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY(subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_exposures_name ON node_exposures(name)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_exposures_active_listener ON node_exposures(listen, listen_port) WHERE enabled = 1`,
		`CREATE TABLE IF NOT EXISTS platform_routes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL COLLATE NOCASE UNIQUE,
				platform TEXT NOT NULL,
				enabled INTEGER NOT NULL DEFAULT 1,
				priority INTEGER NOT NULL DEFAULT 0,
				inbound_exposure_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(inbound_exposure_ids) AND json_type(inbound_exposure_ids) = 'array'),
				source_cidrs TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(source_cidrs) AND json_type(source_cidrs) = 'array'),
				domains TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(domains) AND json_type(domains) = 'array'),
				domain_suffixes TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(domain_suffixes) AND json_type(domain_suffixes) = 'array'),
				domain_keywords TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(domain_keywords) AND json_type(domain_keywords) = 'array'),
				destination_cidrs TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(destination_cidrs) AND json_type(destination_cidrs) = 'array'),
				target_type TEXT NOT NULL CHECK (target_type IN ('node', 'collection', 'direct')),
				target_subscription_id INTEGER,
				target_node_uid TEXT,
				target_collection_id INTEGER,
				fallback_type TEXT NOT NULL DEFAULT 'direct' CHECK (fallback_type IN ('node', 'collection', 'direct')),
				fallback_subscription_id INTEGER,
				fallback_node_uid TEXT,
				fallback_collection_id INTEGER,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				FOREIGN KEY(target_subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL,
				FOREIGN KEY(target_collection_id) REFERENCES proxy_collections(id) ON DELETE SET NULL,
				FOREIGN KEY(fallback_subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL,
				FOREIGN KEY(fallback_collection_id) REFERENCES proxy_collections(id) ON DELETE SET NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_platform_routes_order ON platform_routes(priority, id)`,
		`CREATE INDEX IF NOT EXISTS idx_platform_routes_platform_enabled ON platform_routes(platform, enabled)`,
		`CREATE TABLE IF NOT EXISTS session_leases (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name TEXT NOT NULL,
				enabled INTEGER NOT NULL DEFAULT 1,
				client_cidr TEXT NOT NULL,
				inbound_exposure_ids TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(inbound_exposure_ids) AND json_type(inbound_exposure_ids) = 'array'),
				platform_route_id INTEGER,
				target_type TEXT NOT NULL CHECK (target_type IN ('node', 'collection', 'direct')),
				target_subscription_id INTEGER,
				target_node_uid TEXT,
				target_collection_id INTEGER,
				fallback_type TEXT NOT NULL DEFAULT 'direct' CHECK (fallback_type IN ('node', 'collection', 'direct')),
				fallback_subscription_id INTEGER,
				fallback_node_uid TEXT,
				fallback_collection_id INTEGER,
				expires_at INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL,
				updated_at INTEGER NOT NULL,
				FOREIGN KEY(platform_route_id) REFERENCES platform_routes(id) ON DELETE SET NULL,
				FOREIGN KEY(target_subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL,
				FOREIGN KEY(target_collection_id) REFERENCES proxy_collections(id) ON DELETE SET NULL,
				FOREIGN KEY(fallback_subscription_id) REFERENCES subscriptions(id) ON DELETE SET NULL,
				FOREIGN KEY(fallback_collection_id) REFERENCES proxy_collections(id) ON DELETE SET NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_session_leases_enabled_expires ON session_leases(enabled, expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_session_leases_platform_route ON session_leases(platform_route_id)`,
		`CREATE TABLE IF NOT EXISTS advanced_health_states (
				target_key TEXT PRIMARY KEY,
				target_type TEXT NOT NULL,
				target_ref TEXT NOT NULL DEFAULT '',
				display_name TEXT NOT NULL DEFAULT '',
				status TEXT NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown', 'healthy', 'unhealthy', 'circuit_open')),
				latency_ms INTEGER NOT NULL DEFAULT 0,
				consecutive_failures INTEGER NOT NULL DEFAULT 0,
				consecutive_successes INTEGER NOT NULL DEFAULT 0,
				circuit_open_until INTEGER NOT NULL DEFAULT 0,
				last_error TEXT NOT NULL DEFAULT '',
				last_checked_at INTEGER NOT NULL DEFAULT 0,
				updated_at INTEGER NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_health_states_status ON advanced_health_states(status, updated_at DESC)`,
		`CREATE TABLE IF NOT EXISTS advanced_health_events (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				target_key TEXT NOT NULL,
				target_type TEXT NOT NULL,
				display_name TEXT NOT NULL DEFAULT '',
				event_type TEXT NOT NULL CHECK (event_type IN ('probe_failed', 'circuit_open', 'recovered', 'probe_succeeded')),
				message TEXT NOT NULL DEFAULT '',
				latency_ms INTEGER NOT NULL DEFAULT 0,
				created_at INTEGER NOT NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_health_events_newest ON advanced_health_events(created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_health_events_target ON advanced_health_events(target_key, created_at DESC)`,
		`CREATE TABLE IF NOT EXISTS advanced_access_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				core_event_id TEXT NOT NULL UNIQUE,
				event_time INTEGER NOT NULL,
				network TEXT NOT NULL DEFAULT '',
				inbound TEXT NOT NULL DEFAULT '',
				source_hash TEXT NOT NULL DEFAULT '',
				destination_summary TEXT NOT NULL DEFAULT '',
				domain_summary TEXT NOT NULL DEFAULT '',
				outbound_label TEXT NOT NULL DEFAULT '',
				platform TEXT NOT NULL DEFAULT '',
				platform_route_id INTEGER,
				session_lease_id INTEGER,
				decision TEXT NOT NULL DEFAULT '',
				error_summary TEXT NOT NULL DEFAULT '',
				created_at INTEGER NOT NULL,
				FOREIGN KEY(platform_route_id) REFERENCES platform_routes(id) ON DELETE SET NULL,
				FOREIGN KEY(session_lease_id) REFERENCES session_leases(id) ON DELETE SET NULL
			)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_access_logs_event_time ON advanced_access_logs(event_time DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_access_logs_platform ON advanced_access_logs(platform, event_time DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_advanced_access_logs_decision ON advanced_access_logs(decision, event_time DESC)`,
		`CREATE TABLE IF NOT EXISTS ssh_credentials (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL COLLATE NOCASE UNIQUE,
					auth_type TEXT NOT NULL CHECK (auth_type IN ('password', 'private_key')),
					secret_context TEXT NOT NULL UNIQUE,
					secret_ciphertext BLOB NOT NULL,
					secret_nonce BLOB NOT NULL,
					passphrase_ciphertext BLOB,
					passphrase_nonce BLOB,
					key_fingerprint TEXT NOT NULL DEFAULT '',
					key_version INTEGER NOT NULL DEFAULT 1,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL
				)`,
		`CREATE TABLE IF NOT EXISTS ssh_hosts (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					name TEXT NOT NULL COLLATE NOCASE UNIQUE,
					group_name TEXT NOT NULL DEFAULT '',
					host TEXT NOT NULL,
					port INTEGER NOT NULL DEFAULT 22 CHECK (port BETWEEN 1 AND 65535),
					username TEXT NOT NULL,
					credential_id INTEGER NOT NULL,
					connection_mode TEXT NOT NULL DEFAULT 'direct' CHECK (connection_mode IN ('direct', 'node_exposure')),
					node_exposure_id INTEGER,
					terminal_type TEXT NOT NULL DEFAULT 'xterm-256color',
					enabled INTEGER NOT NULL DEFAULT 1,
					tags_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(tags_json) AND json_type(tags_json) = 'array'),
					notes TEXT NOT NULL DEFAULT '',
					last_status TEXT NOT NULL DEFAULT 'unknown',
					last_latency_ms INTEGER NOT NULL DEFAULT 0,
					last_error_code TEXT NOT NULL DEFAULT '',
					last_error_message TEXT NOT NULL DEFAULT '',
					last_checked_at INTEGER NOT NULL DEFAULT 0,
					created_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					FOREIGN KEY(credential_id) REFERENCES ssh_credentials(id) ON DELETE RESTRICT,
					FOREIGN KEY(node_exposure_id) REFERENCES node_exposures(id) ON DELETE RESTRICT,
					CHECK ((connection_mode = 'direct' AND node_exposure_id IS NULL) OR (connection_mode = 'node_exposure' AND node_exposure_id IS NOT NULL))
				)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_hosts_group_name ON ssh_hosts(group_name, name)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_hosts_credential ON ssh_hosts(credential_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_hosts_node_exposure ON ssh_hosts(node_exposure_id) WHERE node_exposure_id IS NOT NULL`,
		`CREATE TABLE IF NOT EXISTS ssh_host_keys (
					host_id INTEGER PRIMARY KEY,
					key_type TEXT NOT NULL,
					public_key TEXT NOT NULL,
					fingerprint_sha256 TEXT NOT NULL,
					first_seen_at INTEGER NOT NULL,
					last_seen_at INTEGER NOT NULL,
					updated_at INTEGER NOT NULL,
					FOREIGN KEY(host_id) REFERENCES ssh_hosts(id) ON DELETE CASCADE
				)`,
		`CREATE TABLE IF NOT EXISTS ssh_session_audits (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					session_id_hash TEXT NOT NULL DEFAULT '',
					host_id INTEGER,
					host_name TEXT NOT NULL DEFAULT '',
					event_type TEXT NOT NULL,
					connection_mode TEXT NOT NULL DEFAULT '',
					node_exposure_id INTEGER,
					result TEXT NOT NULL,
					error_code TEXT NOT NULL DEFAULT '',
					duration_ms INTEGER NOT NULL DEFAULT 0,
					created_at INTEGER NOT NULL,
					FOREIGN KEY(host_id) REFERENCES ssh_hosts(id) ON DELETE SET NULL,
					FOREIGN KEY(node_exposure_id) REFERENCES node_exposures(id) ON DELETE SET NULL
				)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_session_audits_newest ON ssh_session_audits(created_at DESC, id DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_ssh_session_audits_host ON ssh_session_audits(host_id, created_at DESC)`,
		`DELETE FROM app_settings WHERE key IN ('update.github_token', 'update.proxy_url')`,
		`DELETE FROM app_settings WHERE key = 'update.acceleration' AND value = 'proxy'`,
		`UPDATE app_settings
			SET value = 'fc00::/18', updated_at = unixepoch()
			WHERE key IN ('dns.fakeip_inet6_range', 'dns_global.fakeip_inet6_range')
			AND value = 'fdfe:dcba:9876::/48'`,
	}

	for _, m := range migrations {
		if _, err := s.db.Exec(m); err != nil {
			if isDuplicateColumnMigration(m) {
				continue
			}
			return err
		}
	}

	if err := s.dedupeNodeGroupsByName(); err != nil {
		return err
	}
	if err := s.migrateRouteStrategies(); err != nil {
		return err
	}
	if err := s.EnsureDefaultLocalDNSRule(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_proxy_collections_route_rule_id ON proxy_collections(route_rule_id) WHERE route_rule_id > 0`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_route_rules_name ON route_rules(name)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_node_groups_name ON node_groups(name)`); err != nil {
		return err
	}

	logging.Info("store", "migrations applied")
	return nil
}

func (s *Store) dedupeNodeGroupsByName() error {
	rows, err := s.db.Query(`SELECT id, name FROM node_groups ORDER BY id ASC`)
	if err != nil {
		return err
	}
	defer rows.Close()

	keepByName := make(map[string]int64)
	remap := make(map[int64]int64)
	duplicateIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return err
		}
		if keepID, ok := keepByName[name]; ok {
			remap[id] = keepID
			duplicateIDs = append(duplicateIDs, id)
			continue
		}
		keepByName[name] = id
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(duplicateIDs) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	collectionRows, err := tx.Query(`SELECT id, referenced_group_ids FROM proxy_collections WHERE referenced_group_ids <> '' AND referenced_group_ids <> '[]'`)
	if err != nil {
		return err
	}
	type collectionRef struct {
		id   int64
		refs string
	}
	collections := make([]collectionRef, 0)
	for collectionRows.Next() {
		var item collectionRef
		if err := collectionRows.Scan(&item.id, &item.refs); err != nil {
			collectionRows.Close()
			return err
		}
		collections = append(collections, item)
	}
	if err := collectionRows.Err(); err != nil {
		collectionRows.Close()
		return err
	}
	collectionRows.Close()

	for _, item := range collections {
		var ids []int64
		if err := json.Unmarshal([]byte(item.refs), &ids); err != nil {
			continue
		}
		seen := make(map[int64]bool)
		next := make([]int64, 0, len(ids))
		changed := false
		for _, id := range ids {
			if keepID, ok := remap[id]; ok {
				id = keepID
				changed = true
			}
			if seen[id] {
				changed = true
				continue
			}
			seen[id] = true
			next = append(next, id)
		}
		if !changed {
			continue
		}
		data, _ := json.Marshal(next)
		if _, err := tx.Exec(`UPDATE proxy_collections SET referenced_group_ids = ? WHERE id = ?`, string(data), item.id); err != nil {
			return err
		}
	}

	for _, id := range duplicateIDs {
		if _, err := tx.Exec(`DELETE FROM node_groups WHERE id = ?`, id); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	logging.Info("store", "deduplicated node_groups by name: %d removed", len(duplicateIDs))
	return nil
}

func isDuplicateColumnMigration(m string) bool {
	switch m {
	case `ALTER TABLE subscriptions ADD COLUMN user_agent TEXT NOT NULL DEFAULT 'clash-meta/2.4.0'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_interval_minutes INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscriptions ADD COLUMN sync_mode TEXT NOT NULL DEFAULT 'off'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_time TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE subscriptions ADD COLUMN sync_weekday INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE subscriptions ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'updated'`,
		`ALTER TABLE subscriptions ADD COLUMN sync_progress REAL NOT NULL DEFAULT 100`,
		`ALTER TABLE subscriptions ADD COLUMN sync_timeout_seconds INTEGER NOT NULL DEFAULT 60`,
		`ALTER TABLE nodes ADD COLUMN uid TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE nodes ADD COLUMN name_overridden INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN enabled INTEGER NOT NULL DEFAULT 1`,
		`ALTER TABLE nodes ADD COLUMN preferred INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN last_test_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN test_latency_ms INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE nodes ADD COLUMN test_success INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_mode TEXT NOT NULL DEFAULT 'daily'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_time TEXT NOT NULL DEFAULT '04:00:00'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_weekday INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_status TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_progress REAL NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN sync_error TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN last_sync_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN cached_path TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE route_rule_subscriptions ADD COLUMN cached_updated_at INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_collections ADD COLUMN source_type TEXT NOT NULL DEFAULT 'manual'`,
		`ALTER TABLE proxy_collections ADD COLUMN referenced_group_ids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN route_rule_ids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN route_rule_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE proxy_collections ADD COLUMN node_uids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE proxy_collections ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE dns_servers ADD COLUMN priority INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE node_groups ADD COLUMN node_uids TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE route_rules ADD COLUMN system_key TEXT NOT NULL DEFAULT ''`:
		return true
	default:
		return false
	}
}
