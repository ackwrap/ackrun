package service

import (
	"encoding/json"
	"errors"
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

func TestGenerateDefaultUsesOnlyBackendMixedPort(t *testing.T) {
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
	listens := make(map[string]string)
	for _, inbound := range config.Inbounds {
		ports[inbound.Tag] = inbound.ListenPort
		listens[inbound.Tag] = inbound.Listen
	}
	if len(config.Inbounds) != 1 || ports["mixed-in"] != model.DefaultMixedInboundPort {
		t.Fatalf("generated ports = %+v", ports)
	}
	if listens["mixed-in"] != "0.0.0.0" {
		t.Fatalf("generated listen addresses = %+v", listens)
	}
	if len(config.Route.Rules) != 0 {
		t.Fatalf("default config contains unexpected route rules: %+v", config.Route.Rules)
	}
}

func TestConfigStatusCachesValidationUntilFileChanges(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService(&paths.Paths{
		ConfigDir:  dir,
		ConfigPath: configPath,
	}, nil, nil)
	validationCount := 0
	svc.configValidator = func(string) error {
		validationCount++
		return nil
	}

	if _, err := svc.GetConfigStatus(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetConfigStatus(); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListConfigFiles(); err != nil {
		t.Fatal(err)
	}
	if validationCount != 1 {
		t.Fatalf("unchanged config validated %d times, want 1", validationCount)
	}
	if err := svc.Validate(); err != nil {
		t.Fatal(err)
	}
	if validationCount != 2 {
		t.Fatalf("explicit validation count = %d, want 2", validationCount)
	}

	if err := os.WriteFile(configPath, []byte("{\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetConfigStatus(); err != nil {
		t.Fatal(err)
	}
	if validationCount != 3 {
		t.Fatalf("changed config validation count = %d, want 3", validationCount)
	}
}

func TestConfigMetadataReadsSkipValidation(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService(&paths.Paths{
		ConfigDir:  dir,
		ConfigPath: configPath,
	}, nil, nil)
	validationCount := 0
	svc.configValidator = func(string) error {
		validationCount++
		return nil
	}

	status, err := svc.GetConfigStatusMetadata()
	if err != nil {
		t.Fatal(err)
	}
	items, err := svc.ListConfigFilesMetadata()
	if err != nil {
		t.Fatal(err)
	}
	if validationCount != 0 {
		t.Fatalf("metadata reads validated config %d times, want 0", validationCount)
	}
	if status.Validated || len(items) != 1 || items[0].Validated {
		t.Fatalf("metadata unexpectedly marked validated: status=%+v items=%+v", status, items)
	}

	status, err = svc.GetConfigStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Validated || !status.Valid || validationCount != 1 {
		t.Fatalf("validated status=%+v count=%d, want valid with one validation", status, validationCount)
	}
}

func TestExplicitConfigValidationRefreshesFailedCache(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(configPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewConfigService(&paths.Paths{
		ConfigDir:  dir,
		ConfigPath: configPath,
	}, nil, nil)
	validationErr := errors.New("invalid")
	validationCount := 0
	svc.configValidator = func(string) error {
		validationCount++
		return validationErr
	}

	status, err := svc.GetConfigStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.Valid || validationCount != 1 {
		t.Fatalf("initial status = %+v, validation count = %d", status, validationCount)
	}
	validationErr = nil
	if err := svc.Validate(); err != nil {
		t.Fatal(err)
	}
	status, err = svc.GetConfigStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Valid || validationCount != 2 {
		t.Fatalf("refreshed status = %+v, validation count = %d", status, validationCount)
	}
}
