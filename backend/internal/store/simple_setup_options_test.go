package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSimpleSetupOptionsDraftAndPreparation(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	options := model.DefaultSimpleSetupOptions()
	options.AdBlock = false
	options.AppRouting["AI"] = "direct"
	options.CNOutbound = "direct"
	options.DefaultOutbound = "direct"
	options.LocalDNS = "119.29.29.29"
	options.ProxyDNS = "8.8.8.8"
	options.DNSStrategy = "ipv4_only"
	options.AutoStartCore = false
	options.DirectDevices = []string{"192.168.1.50/32", "2001:db8::50/128"}
	if err := s.SaveSimpleSetupOptions(options); err != nil {
		t.Fatal(err)
	}
	if available, err := s.SimpleSetupAvailable(); err != nil || !available {
		t.Fatalf("saving preferences blocks first setup: %t, %v", available, err)
	}
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	actual, err := s.SimpleSetupOptions()
	if err != nil || !reflect.DeepEqual(actual, options) {
		t.Fatalf("preferences not applied: %+v, %v", actual, err)
	}
	var enabled bool
	if err := s.db.QueryRow(`SELECT enabled FROM proxy_collections WHERE name = 'AI'`).Scan(&enabled); err != nil || enabled {
		t.Fatalf("direct category retains conflicting selector: %t, %v", enabled, err)
	}
	bypass, err := s.GetTrafficBypassSettings()
	if err != nil {
		t.Fatal(err)
	}
	bypass.Rules = append(bypass.Rules, model.TrafficBypassRule{Type: "source_ip_cidr", Value: "192.168.2.50/32", Remark: "custom"})
	if err := s.SetTrafficBypassSettings(bypass); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveSimpleSetupOptions(model.DefaultSimpleSetupOptions()); err != nil {
		t.Fatal(err)
	}
	bypass, err = s.GetTrafficBypassSettings()
	if err != nil {
		t.Fatal(err)
	}
	want := append(defaultTrafficBypassSettings().Rules, model.TrafficBypassRule{Type: "source_ip_cidr", Value: "192.168.2.50/32", Remark: "custom"})
	if !reflect.DeepEqual(bypass.Rules, want) {
		t.Fatalf("device edits changed unrelated exclusions: %+v", bypass.Rules)
	}
	if err := s.db.QueryRow(`SELECT enabled FROM proxy_collections WHERE name = 'AI'`).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("proxy category did not restore its selector: %t, %v", enabled, err)
	}
}

func TestSimpleSetupOptionsRollbackOnDatabaseFailure(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	before, err := s.SimpleSetupOptions()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER reject_simple_options BEFORE UPDATE ON dns_servers BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	options := model.DefaultSimpleSetupOptions()
	options.AdBlock = false
	options.AppRouting["AI"] = "direct"
	if err := s.SaveSimpleSetupOptions(options); err == nil {
		t.Fatal("expected injected failure")
	}
	after, err := s.SimpleSetupOptions()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("partial preferences survived failed transaction: %+v, %v", after, err)
	}
	var enabled bool
	if err := s.db.QueryRow(`SELECT enabled FROM proxy_collections WHERE name = 'AI'`).Scan(&enabled); err != nil || !enabled {
		t.Fatalf("collection not rolled back: %t, %v", enabled, err)
	}
}

func TestSimpleSetupOptionsRejectsDisabledExpertCollection(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE proxy_collections SET enabled = 0 WHERE name = 'AI'`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SimpleSetupOptions(); err == nil {
		t.Fatal("expert changes would be silently re-enabled by a simple settings save")
	}
}
