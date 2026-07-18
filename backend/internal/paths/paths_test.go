package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestActiveConfigPathUsesMarker(t *testing.T) {
	dir := t.TempDir()
	p := &Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
	}
	for _, name := range []string{"config.json", "strategy.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(p.ActiveConfigMarkerPath(), []byte("strategy.json"), 0644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := p.ActiveConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != filepath.Join(dir, "strategy.json") {
		t.Fatalf("active path = %q, ok=%t", got, ok)
	}
}

func TestActiveConfigPathFallsBackWhenMarkerIsStale(t *testing.T) {
	dir := t.TempDir()
	p := &Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
	}
	if err := os.WriteFile(p.ConfigPath, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.ActiveConfigMarkerPath(), []byte("missing.json"), 0644); err != nil {
		t.Fatal(err)
	}

	got, ok, err := p.ActiveConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if !ok || got != p.ConfigPath {
		t.Fatalf("active path = %q, ok=%t", got, ok)
	}
}

func TestActiveConfigPathIgnoresRootBackups(t *testing.T) {
	dir := t.TempDir()
	p := &Paths{
		ConfigDir:  dir,
		ConfigPath: filepath.Join(dir, "config.json"),
	}
	for _, name := range []string{
		"config.backup.1784044208996320000.json",
		"config.json.pre-certificate-provider.1.bak.json",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	got, ok, err := p.ActiveConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if ok || got != "" {
		t.Fatalf("active path = %q, ok=%t, want no active config", got, ok)
	}
}

func TestConfigFileNameClassification(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{name: "config.json", want: true},
		{name: "strategy.JSON", want: true},
		{name: "config.backup.123.json", want: false},
		{name: "config.json.123.bak.json", want: false},
		{name: "notes.txt", want: false},
	}
	for _, tt := range tests {
		if got := IsConfigFileName(tt.name); got != tt.want {
			t.Errorf("IsConfigFileName(%q) = %t, want %t", tt.name, got, tt.want)
		}
	}
}
