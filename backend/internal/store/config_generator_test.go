package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestConfigGenerateRequestRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	request := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		TUNIPv4Address:  "10.254.0.1/30",
		TUNIPv6Address:  "fd12:3456:789a::1/126",
		LogLevel:        "warn",
	}
	if err := db.SetConfigGenerateRequest(request); err != nil {
		t.Fatalf("set request: %v", err)
	}
	got, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if got == nil || *got != *request {
		t.Fatalf("request = %#v, want %#v", got, request)
	}
}

func TestMigrateConfigGenerateRequestTUNAddressesUpdatesOnlyAddressFields(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		TUNIPv4Address:  "172.254.0.1/30",
		TUNIPv6Address:  "fdfe:dcba:9876::1/126",
		LogLevel:        "info",
	}
	if err := db.SetConfigGenerateRequest(request); err != nil {
		t.Fatal(err)
	}
	if err := db.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: true}); err != nil {
		t.Fatal(err)
	}
	migrated, changed, err := db.MigrateConfigGenerateRequestTUNAddresses(
		"172.254.0.1/30", "172.31.255.1/30", "fdfe:dcba:9876::1/126", "fdfe:dcba:9875::1/126",
	)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("old default TUN addresses were not migrated")
	}
	if migrated.TUNIPv4Address != "172.31.255.1/30" || migrated.TUNIPv6Address != "fdfe:dcba:9875::1/126" {
		t.Fatalf("migrated TUN addresses = %q, %q", migrated.TUNIPv4Address, migrated.TUNIPv6Address)
	}
	if migrated.LogLevel != "debug" || migrated.DefaultOutbound != request.DefaultOutbound || migrated.InboundListen != request.InboundListen || migrated.InboundPort != request.InboundPort {
		t.Fatalf("TUN migration changed unrelated settings: %+v", migrated)
	}
	if _, changed, err := db.MigrateConfigGenerateRequestTUNAddresses(
		"172.254.0.1/30", "172.31.255.1/30", "fdfe:dcba:9876::1/126", "fdfe:dcba:9875::1/126",
	); err != nil || changed {
		t.Fatalf("idempotent TUN migration changed=%t err=%v", changed, err)
	}
}
