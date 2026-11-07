package store

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDNSModeMigrationPreservesEffectiveModeOnce(t *testing.T) {
	for _, inbound := range []string{"tun", "tun_mixed", "mixed"} {
		t.Run(inbound, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ackwrap.db")
			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.SetInboundMode(inbound); err != nil {
				t.Fatal(err)
			}
			// Simulate an old release's ignored, contradictory persisted flag.
			if _, err := s.db.Exec(`DELETE FROM app_settings WHERE key = 'dns.mode_initialized'`); err != nil {
				t.Fatal(err)
			}
			settings, err := s.GetDNSGlobalSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.FakeIPEnabled = inbound == "mixed"
			if err := s.SetDNSGlobalSettings(settings); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			settings, err = s.GetDNSGlobalSettings()
			if err != nil || settings.FakeIPEnabled != (inbound != "mixed") {
				t.Fatalf("upgrade changed effective DNS mode: %+v, %v", settings, err)
			}
			// An explicit choice after upgrading must survive migration and inbound changes.
			settings.FakeIPEnabled = !settings.FakeIPEnabled
			if err := s.SetDNSGlobalSettings(settings); err != nil {
				t.Fatal(err)
			}
			if err := s.SetInboundMode("mixed"); err != nil {
				t.Fatal(err)
			}
			if err := s.migrate(); err != nil {
				t.Fatal(err)
			}
			stored, err := s.GetDNSGlobalSettings()
			if err != nil || stored.FakeIPEnabled != settings.FakeIPEnabled {
				t.Fatalf("migration reset explicit DNS choice: %+v, %v", stored, err)
			}
		})
	}
}

func TestDNSModeSaveIsAtomic(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	before, err := s.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	legacyBefore, err := s.GetDNSSettings()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_dns_mode BEFORE INSERT ON app_settings
		WHEN NEW.key = 'dns_global.fakeip_enabled' BEGIN SELECT RAISE(ABORT, 'mode write failed'); END`); err != nil {
		t.Fatal(err)
	}
	req := *before
	req.FakeIPEnabled = false
	req.ReverseMapping = true
	req.Final = "changed"
	if err := s.SetDNSGlobalSettings(&req); err == nil {
		t.Fatal("mode save ignored database failure")
	}
	after, err := s.GetDNSGlobalSettings()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("failed mode save partially changed global settings: %+v, %v", after, err)
	}
	legacyAfter, err := s.GetDNSSettings()
	if err != nil || !reflect.DeepEqual(legacyBefore, legacyAfter) {
		t.Fatalf("failed mode save partially changed legacy settings: %+v, %v", legacyAfter, err)
	}
}

func TestDNSModeMigrationFailureCanBeRetried(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`DELETE FROM app_settings WHERE key = 'dns.mode_initialized';
		UPDATE app_settings SET value = 'false' WHERE key IN ('dns.fakeip_enabled', 'dns_global.fakeip_enabled');
		CREATE TRIGGER fail_dns_mode_migration BEFORE INSERT ON app_settings
		WHEN NEW.key = 'dns_global.fakeip_enabled' BEGIN SELECT RAISE(ABORT, 'mode migration failed'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateDNSMode(); err == nil {
		t.Fatal("migration ignored database failure")
	}
	var changed int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key = 'dns.mode_initialized'
		OR (key IN ('dns.fakeip_enabled', 'dns_global.fakeip_enabled') AND value != 'false')`).Scan(&changed); err != nil || changed != 0 {
		t.Fatalf("failed migration left partial state: count=%d, %v", changed, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_dns_mode_migration`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrateDNSMode(); err != nil {
		t.Fatalf("migration could not be retried: %v", err)
	}
	settings, err := s.GetDNSGlobalSettings()
	if err != nil || !settings.FakeIPEnabled {
		t.Fatalf("retried migration did not preserve TUN mode: %+v, %v", settings, err)
	}
}
