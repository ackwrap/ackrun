package store

import (
	"path/filepath"
	"testing"
)

func TestMigrateRemovesSongziAndPromotesIPAPIIS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE geoip_providers SET is_default = 0 WHERE provider_key = 'ipapi.is'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO geoip_providers (name, provider_key, template, enabled, is_default, builtin, created_at, updated_at) VALUES ('legacy', 'songzixian', 'builtin', 1, 1, 1, unixepoch(), unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var songziCount, ipapiDefault int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM geoip_providers WHERE provider_key = 'songzixian'`).Scan(&songziCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT is_default FROM geoip_providers WHERE provider_key = 'ipapi.is'`).Scan(&ipapiDefault); err != nil {
		t.Fatal(err)
	}
	if songziCount != 0 || ipapiDefault != 1 {
		t.Fatalf("songzi count = %d, ipapi.is default = %d", songziCount, ipapiDefault)
	}
}

func TestMigrateRemovesLegacyUpdateCredentialsAndProxyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"update.github_token":      "legacy-token",
		"update.proxy_url":         "http://127.0.0.1:9901",
		"update.acceleration":      "proxy",
		"update.custom_mirror_url": "https://mirror.example",
	} {
		if _, err := db.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, unixepoch())`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var legacyCount, mirrorCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key IN ('update.github_token', 'update.proxy_url') OR (key = 'update.acceleration' AND value = 'proxy')`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key = 'update.custom_mirror_url' AND value = 'https://mirror.example'`).Scan(&mirrorCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 || mirrorCount != 1 {
		t.Fatalf("legacy update settings = %d, preserved mirrors = %d", legacyCount, mirrorCount)
	}
}
