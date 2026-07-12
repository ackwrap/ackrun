package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLatestConfigBackupUsesModificationTime(t *testing.T) {
	dir := t.TempDir()
	older := filepath.Join(dir, "z-old.bak.json")
	newer := filepath.Join(dir, "a-new.bak.json")
	if err := os.WriteFile(older, []byte("old"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newer, []byte("new"), 0644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(older, now.Add(-time.Minute), now.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newer, now, now); err != nil {
		t.Fatal(err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := latestConfigBackup(dir, entries)
	if !ok || got != newer {
		t.Fatalf("latest backup = %q, ok=%t, want %q", got, ok, newer)
	}
}
