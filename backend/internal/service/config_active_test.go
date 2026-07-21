package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
)

func TestNormalizeConfigFileName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "adds extension", input: "strategy", want: "strategy.json"},
		{name: "normalizes extension", input: "daily.JSON", want: "daily.json"},
		{name: "rejects traversal", input: "../strategy.json", wantErr: true},
		{name: "rejects nested path", input: "folder/strategy.json", wantErr: true},
		{name: "rejects other extension", input: "strategy.txt", wantErr: true},
		{name: "rejects control character", input: "strategy\n.json", wantErr: true},
		{name: "rejects reserved name", input: "CON.json", wantErr: true},
		{name: "rejects reserved name with suffix", input: "COM1.backup.json", wantErr: true},
		{name: "rejects automatic backup name", input: "config.backup.123.json", wantErr: true},
		{name: "rejects migration backup name", input: "strategy.123.bak.json", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeConfigFileName(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidConfigFileName) {
					t.Fatalf("error = %v, want invalid file name", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("normalize = %q, err=%v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestWriteActiveConfigMarkerChangesActiveConfig(t *testing.T) {
	dir := t.TempDir()
	p := &paths.Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
	}
	target := filepath.Join(dir, "strategy.json")
	if err := os.WriteFile(target, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := writeActiveConfigMarker(p, target); err != nil {
		t.Fatal(err)
	}

	got, ok, err := p.ActiveConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != target {
		t.Fatalf("active path = %q, ok=%t, want %q", got, ok, target)
	}
}

func TestSetActiveConfigValidatesBeforeChangingMarker(t *testing.T) {
	dir := t.TempDir()
	p := &paths.Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
	}
	for _, name := range []string{"config.json", "strategy.json", "invalid.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeActiveConfigMarker(p, p.ConfigPath); err != nil {
		t.Fatal(err)
	}
	validatedPath := ""
	svc := &ConfigService{
		paths: p,
		configValidator: func(path string) error {
			validatedPath = path
			if filepath.Base(path) == "invalid.json" {
				return errors.New("invalid config")
			}
			return nil
		},
	}

	status, err := svc.SetActiveConfig("strategy.json")
	if err != nil {
		t.Fatal(err)
	}
	if status.FileName != "strategy.json" || validatedPath != filepath.Join(dir, "strategy.json") {
		t.Fatalf("status = %+v, validated path = %q", status, validatedPath)
	}
	activePath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok || activePath != filepath.Join(dir, "strategy.json") {
		t.Fatalf("active path = %q, ok=%t, err=%v", activePath, ok, err)
	}

	if _, err := svc.SetActiveConfig("invalid.json"); !errors.Is(err, ErrConfigFileInvalid) {
		t.Fatalf("invalid config error = %v", err)
	}
	activePath, ok, err = p.ActiveConfigPath()
	if err != nil || !ok || activePath != filepath.Join(dir, "strategy.json") {
		t.Fatalf("active path changed after invalid selection: %q, ok=%t, err=%v", activePath, ok, err)
	}
}

func TestSetActiveConfigRejectsMissingFile(t *testing.T) {
	dir := t.TempDir()
	svc := &ConfigService{
		paths: &paths.Paths{ConfigDir: dir, ConfigPath: filepath.Join(dir, "config.json")},
		configValidator: func(string) error {
			return nil
		},
	}
	if _, err := svc.SetActiveConfig("missing.json"); !errors.Is(err, ErrConfigFileNotFound) {
		t.Fatalf("error = %v, want config not found", err)
	}
}
