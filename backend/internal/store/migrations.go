package store

import (
	"encoding/json"

	"github.com/ackwrap/ackwrap/internal/logging"
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
		`DELETE FROM app_settings WHERE key IN ('update.github_token', 'update.proxy_url')`,
		`DELETE FROM app_settings WHERE key = 'update.acceleration' AND value = 'proxy'`,
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
