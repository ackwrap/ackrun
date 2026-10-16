package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestDeviceDirectDomainsMigrationAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	req := &model.RouteRuleRequest{Name: "device", Enabled: true, RuleType: "source_ip_cidr", Values: []string{"192.168.1.2/32"}, Outbound: "proxy"}
	device, err := db.CreateRouteRule(req)
	if err != nil {
		t.Fatal(err)
	}
	// Recreate the schema used before this feature, retaining existing rules.
	if _, err := db.db.Exec(`ALTER TABLE route_rules DROP COLUMN direct_domains_json`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := db.GetRouteRule(device.ID)
	if err != nil || loaded.Name != req.Name || len(loaded.DirectDomains) != 0 {
		t.Fatalf("migration: %+v %v", loaded, err)
	}
	req.DirectDomains = []string{"baidu.com", "example.com"}
	updated, err := db.UpdateRouteRule(device.ID, req)
	if err != nil || !reflect.DeepEqual(updated.DirectDomains, req.DirectDomains) {
		t.Fatalf("update: %+v %v", updated, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	loaded, err = db.GetRouteRule(device.ID)
	if err != nil || !reflect.DeepEqual(loaded.DirectDomains, req.DirectDomains) {
		t.Fatalf("reopen: %+v %v", loaded, err)
	}
	req.DirectDomains = nil
	cleared, err := db.UpdateRouteRule(device.ID, req)
	if err != nil || len(cleared.DirectDomains) != 0 {
		t.Fatalf("clear: %+v %v", cleared, err)
	}
}
