package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestExperimentalSettingsMigratesStoreRDRCToStoreDNS(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if _, err := s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('experimental.cache_file.store_rdrc', 'false', 1)`); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.CacheFileStoreDNS {
		t.Fatal("legacy disabled store_rdrc should migrate to disabled store_dns")
	}

	if err := s.SetExperimentalSettings(&model.ExperimentalSettings{CacheFileEnabled: true, CacheFileStoreDNS: true}); err != nil {
		t.Fatal(err)
	}
	var legacyCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key IN ('experimental.cache_file.store_rdrc', 'experimental.cache_file.rdrc_timeout')`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 {
		t.Fatalf("legacy cache settings remain: %d", legacyCount)
	}
}

func TestLogSettingsPersistLevelAndTimestamp(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	if err := s.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: false}); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetLogSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Level != "debug" || settings.Timestamp {
		t.Fatalf("log settings = %+v, want debug without timestamp", settings)
	}
}

func TestDNSGlobalSettingsFallBackToLegacySettingsWhenUnmigrated(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for key, value := range map[string]string{
		"dns.enabled":        "false",
		"dns.final":          "dns_legacy",
		"dns.strategy":       "ipv4_only",
		"dns.fakeip_enabled": "true",
	} {
		if _, err := s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, 1)`, key, value); err != nil {
			t.Fatal(err)
		}
	}

	settings, err := s.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Enabled || settings.Final != "dns_legacy" || settings.Strategy != "ipv4_only" || !settings.FakeIPEnabled {
		t.Fatalf("legacy DNS fallback = %+v", settings)
	}
	if _, err := s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('dns_global.enabled', 'true', 2)`); err != nil {
		t.Fatal(err)
	}
	settings, err = s.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Enabled {
		t.Fatal("explicit global DNS enabled setting did not override legacy state")
	}
}

func TestUpdateSettingsDefaultsToGHProxyAndPreservesCustomMirror(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	settings, err := s.GetUpdateSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Acceleration != "ghproxy" || settings.CustomMirrorURL != "" {
		t.Fatalf("default update settings = %+v, want ghproxy without custom mirror", settings)
	}
	customMirrorURL := "https://mirror.example"
	if err := s.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: customMirrorURL}); err != nil {
		t.Fatal(err)
	}
	settings, err = s.GetUpdateSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Acceleration != "custom" || settings.CustomMirrorURL != customMirrorURL {
		t.Fatalf("custom update settings = %+v", settings)
	}
}
