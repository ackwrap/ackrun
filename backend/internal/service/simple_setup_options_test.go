package service

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSimpleSetupOptionsValidation(t *testing.T) {
	for name, mutate := range map[string]func(*model.SimpleSetupOptions){
		"unknown-category": func(o *model.SimpleSetupOptions) { o.AppRouting["unexpected"] = "proxy" },
		"invalid-policy":   func(o *model.SimpleSetupOptions) { o.AppRouting["AI"] = "bypass" },
		"missing-category": func(o *model.SimpleSetupOptions) { delete(o.AppRouting, "AI") },
		"invalid-cn":       func(o *model.SimpleSetupOptions) { o.CNOutbound = "proxy" },
		"invalid-final":    func(o *model.SimpleSetupOptions) { o.DefaultOutbound = "bypass" },
		"dns-url":          func(o *model.SimpleSetupOptions) { o.LocalDNS = "https://example.com" },
		"dns-loopback":     func(o *model.SimpleSetupOptions) { o.ProxyDNS = "127.0.0.1" },
		"invalid-strategy": func(o *model.SimpleSetupOptions) { o.DNSStrategy = "off" },
		"all-devices":      func(o *model.SimpleSetupOptions) { o.DirectDevices = []string{"0.0.0.0/0"} },
		"invalid-device":   func(o *model.SimpleSetupOptions) { o.DirectDevices = []string{"device.example.com"} },
	} {
		t.Run(name, func(t *testing.T) {
			options := model.DefaultSimpleSetupOptions()
			mutate(options)
			if err := validateSimpleSetupOptions(options); err == nil {
				t.Fatal("invalid options accepted")
			}
		})
	}
	options := model.DefaultSimpleSetupOptions()
	options.DirectDevices = []string{"192.168.1.50", "192.168.1.50/32", "2001:db8::50", "192.168.2.99/24"}
	if err := validateSimpleSetupOptions(options); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(options.DirectDevices, []string{"192.168.1.50/32", "2001:db8::50/128", "192.168.2.0/24"}) {
		t.Fatalf("unexpected normalized devices: %v", options.DirectDevices)
	}
}

func TestSimpleSetupOptionsRestoreAfterConfigValidationFailure(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSimpleSetupState("configured"); err != nil {
		t.Fatal(err)
	}
	p := &paths.Paths{DataDir: dir, ConfigDir: filepath.Join(dir, "config"), BinaryPath: filepath.Join(dir, "missing-core")}
	if err := os.MkdirAll(p.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(p.ConfigDir, "simple.json")
	original := []byte(`{"log":{"level":"warn"}}`)
	if err := os.WriteFile(active, original, 0600); err != nil {
		t.Fatal(err)
	}
	generator := NewConfigGeneratorService(db, p)
	svc := NewSimpleSetupService(db, p, nil, nil, nil, generator, nil, nil)
	svc.supported = func() bool { return true }
	defer svc.Close()
	options := model.DefaultSimpleSetupOptions()
	options.AdBlock = false
	if err := svc.UpdateOptions(options); err == nil {
		t.Fatal("invalid generated configuration accepted")
	}
	actual, err := os.ReadFile(active)
	if err != nil || string(actual) != string(original) {
		t.Fatalf("original config changed: %s, %v", actual, err)
	}
	stored, err := svc.Options()
	if err != nil || !stored.AdBlock {
		t.Fatalf("options not restored: %+v, %v", stored, err)
	}
}

func TestSimpleSetupOptionsSaveDraftAndRejectAppliedChanges(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := &paths.Paths{ConfigDir: filepath.Join(dir, "config")}
	svc := NewSimpleSetupService(db, p, nil, nil, nil, nil, nil, nil)
	svc.supported = func() bool { return true }
	defer svc.Close()
	options := model.DefaultSimpleSetupOptions()
	options.AdBlock = false
	if err := svc.UpdateOptions(options); err != nil {
		t.Fatal(err)
	}
	status, err := svc.Status()
	if err != nil || status.HasExistingConfig {
		t.Fatalf("draft blocked onboarding: %+v, %v", status, err)
	}
	if err := db.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	if err := db.SetSimpleSetupState("applied"); err != nil {
		t.Fatal(err)
	}
	options.AdBlock = true
	if err := svc.UpdateOptions(options); err == nil {
		t.Fatal("applied setup preferences changed before startup retry")
	}
	stored, err := svc.Options()
	if err != nil || stored.AdBlock {
		t.Fatalf("locked options changed: %+v, %v", stored, err)
	}
}
