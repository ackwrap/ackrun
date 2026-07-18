package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestSyncConfigBackupsMovesAndDeduplicatesLegacyFiles(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := &paths.Paths{ConfigDir: configDir, ConfigPath: filepath.Join(configDir, "config.json")}
	older := filepath.Join(configDir, "config.backup.100.json")
	newer := filepath.Join(configDir, "config.backup.200.json")
	if err := os.WriteFile(older, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 18, 9, 0, 0, 0, time.Local)
	if err := os.Chtimes(older, day, day); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, day.Add(time.Hour), day.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	backups, err := syncConfigBackups(p, db)
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || backups[0].BackupDate != "2026-07-18" {
		t.Fatalf("backups = %+v", backups)
	}
	wantPath := filepath.Join(configDir, "backup", "config.json.20260718.bak.json")
	if backups[0].Path != wantPath {
		t.Fatalf("backup path = %q, want %q", backups[0].Path, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "old" {
		t.Fatalf("kept backup = %q, want oldest daily snapshot", data)
	}
	for _, legacyPath := range []string{older, newer} {
		if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
			t.Fatalf("legacy backup still exists: %s", legacyPath)
		}
	}
	indexed, err := db.ListConfigBackups()
	if err != nil || len(indexed) != 1 || indexed[0].Path != wantPath {
		t.Fatalf("indexed backups = %+v, err=%v", indexed, err)
	}
}

func TestEnsureDailyConfigBackupKeepsOneSnapshotPerDay(t *testing.T) {
	root := t.TempDir()
	configDir := filepath.Join(root, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p := &paths.Paths{ConfigDir: configDir, ConfigPath: filepath.Join(configDir, "config.json")}
	if err := os.WriteFile(p.ConfigPath, []byte("version-1"), 0644); err != nil {
		t.Fatal(err)
	}
	day := time.Date(2026, 7, 18, 8, 0, 0, 0, time.Local)
	first, created, err := ensureDailyConfigBackup(p, db, p.ConfigPath, day)
	if err != nil || !created {
		t.Fatalf("first backup = %+v, created=%t, err=%v", first, created, err)
	}
	if err := os.WriteFile(p.ConfigPath, []byte("version-2"), 0644); err != nil {
		t.Fatal(err)
	}
	second, created, err := ensureDailyConfigBackup(p, db, p.ConfigPath, day.Add(4*time.Hour))
	if err != nil || created || second.Path != first.Path {
		t.Fatalf("second backup = %+v, created=%t, err=%v", second, created, err)
	}
	data, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "version-1" {
		t.Fatalf("daily backup content = %q, want version-1", data)
	}
	third, created, err := ensureDailyConfigBackup(p, db, p.ConfigPath, day.Add(24*time.Hour))
	if err != nil || !created || third.Path == first.Path {
		t.Fatalf("next-day backup = %+v, created=%t, err=%v", third, created, err)
	}
	indexed, err := db.ListConfigBackups()
	if err != nil || len(indexed) != 2 {
		t.Fatalf("indexed backups = %+v, err=%v", indexed, err)
	}
}
