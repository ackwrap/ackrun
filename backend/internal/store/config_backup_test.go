package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestReplaceConfigBackups(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	backups := []model.ConfigBackup{
		{ConfigName: "config.json", FileName: "config.json.20260718.bak.json", Path: "backup/config.json.20260718.bak.json", BackupDate: "2026-07-18", SizeBytes: 12, CreatedAt: 100},
		{ConfigName: "home.json", FileName: "home.json.20260717.bak.json", Path: "backup/home.json.20260717.bak.json", BackupDate: "2026-07-17", SizeBytes: 34, CreatedAt: 90},
	}
	if err := db.ReplaceConfigBackups(backups); err != nil {
		t.Fatal(err)
	}
	got, err := db.ListConfigBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ConfigName != "config.json" || got[1].ConfigName != "home.json" {
		t.Fatalf("backups = %+v", got)
	}

	if err := db.ReplaceConfigBackups(backups[:1]); err != nil {
		t.Fatal(err)
	}
	got, err = db.ListConfigBackups()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].FileName != backups[0].FileName {
		t.Fatalf("backups after replace = %+v", got)
	}
}
