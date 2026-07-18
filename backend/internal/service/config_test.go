package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestListConfigFilesIgnoresRootBackups(t *testing.T) {
	dir := t.TempDir()
	p := &paths.Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
		BinaryPath: filepath.Join(dir, "missing-sing-box"),
	}
	for _, name := range []string{
		"strategy.json",
		"config.backup.1784044208996320000.json",
		"strategy.json.123.bak.json",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	items, err := NewConfigService(p, nil, nil).ListConfigFiles()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Name != "strategy.json" {
		t.Fatalf("config files = %+v, want only strategy.json", items)
	}
}

func TestGenerateDefaultUsesBackendMixedPortAndDedicatedUpdatePort(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	p := &paths.Paths{
		DataDir:    dir,
		ConfigDir:  configDir,
		ConfigPath: filepath.Join(configDir, "config.json"),
	}
	db, err := store.Open(filepath.Join(dir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewConfigService(p, db, NewRealtimeService())
	svc.configValidator = func(string) error { return nil }
	if err := svc.GenerateDefault(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(p.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	var config MinimalConfig
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	ports := make(map[string]int)
	for _, inbound := range config.Inbounds {
		ports[inbound.Tag] = inbound.ListenPort
	}
	if ports["mixed-in"] != model.DefaultMixedInboundPort || ports[updateProxyInboundTag] != updateProxyListenPort {
		t.Fatalf("generated ports = %+v", ports)
	}
	if len(config.Route.Rules) == 0 || !stringListContains(config.Route.Rules[0]["inbound"], updateProxyInboundTag) || config.Route.Rules[0]["outbound"] != "proxy" {
		t.Fatalf("dedicated update route missing: %+v", config.Route.Rules)
	}
}
