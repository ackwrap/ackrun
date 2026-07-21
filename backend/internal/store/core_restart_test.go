package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestCoreRestartSettingsDefaultsAndPersistence(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settings, err := db.GetCoreRestartSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Mode != "daily" || settings.Time != "04:00:00" || settings.Weekday != 1 {
		t.Fatalf("defaults = %+v", settings)
	}

	want := &model.CoreRestartSettings{Mode: "weekly", Time: "03:30:00", Weekday: 0}
	if err := db.SetCoreRestartSettings(want); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetCoreRestartSettings()
	if err != nil {
		t.Fatal(err)
	}
	if *got != *want {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
}
