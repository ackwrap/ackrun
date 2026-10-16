package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestDisableDeviceProtectionForRepairPreservesOtherData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "minimal.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// A deliberately minimal old schema proves repair performs no migrations.
	if _, err := db.Exec(`CREATE TABLE app_settings (key TEXT PRIMARY KEY, value TEXT, updated_at INTEGER);
		INSERT INTO app_settings VALUES ('general.device_protection_enabled', 'true', 1), ('unrelated', 'keep', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := DisableDeviceProtectionForRepair(path); err != nil {
		t.Fatal(err)
	}
	var enabled, unrelated string
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key = 'general.device_protection_enabled'`).Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT value FROM app_settings WHERE key = 'unrelated'`).Scan(&unrelated); err != nil {
		t.Fatal(err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table'`).Scan(&tables); err != nil {
		t.Fatal(err)
	}
	if enabled != "false" || unrelated != "keep" || tables != 1 {
		t.Fatalf("repair modified unrelated data: flag=%s other=%s tables=%d", enabled, unrelated, tables)
	}
	if err := DisableDeviceProtectionForRepair(path); err != nil {
		t.Fatal("repeat reset failed", err)
	}
}

func TestDisableDeviceProtectionForRepairMissingOrCorruptDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.db")
	if err := DisableDeviceProtectionForRepair(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("repair created an empty database")
	}
	if err := os.WriteFile(path, []byte("damaged database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := DisableDeviceProtectionForRepair(path); err == nil {
		t.Fatal("damaged database reported reset success")
	}
}
