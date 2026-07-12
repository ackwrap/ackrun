package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestSetProxyModePersistsSupportedMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	if err := svc.SetProxyMode("global"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetProxyMode(); got != "global" {
		t.Fatalf("proxy mode = %q, want global", got)
	}
}

func TestSetExperimentalSettingsRejectsInvalidClashAPIPort(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	err = svc.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090@remote.example"})
	if err == nil {
		t.Fatal("SetExperimentalSettings() error = nil, want invalid port error")
	}
}

func TestSetProxyModeRejectsRunningCore(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	runningCore := &SingboxService{pid: 1, cmd: &exec.Cmd{Process: &os.Process{}}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(runningCore, nil)
	if err := svc.SetProxyMode("direct"); !errors.Is(err, ErrModeChangeWhileRunning) {
		t.Fatalf("error = %v, want ErrModeChangeWhileRunning", err)
	}
	if got := svc.GetProxyMode(); got != "rule" {
		t.Fatalf("proxy mode changed while running: %q", got)
	}
}
