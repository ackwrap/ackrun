package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSimpleSetupRejectsUnsafeStartsWithoutChanges(t *testing.T) {
	for _, tc := range []struct {
		name, url         string
		supported, active bool
	}{
		{name: "unsupported", url: "https://example.com/sub"},
		{name: "invalid-url", url: "file:///etc/passwd", supported: true},
		{name: "existing-config", url: "https://example.com/sub", supported: true, active: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := store.Open(filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			p := &paths.Paths{ConfigDir: filepath.Join(dir, "configs")}
			if tc.active {
				if err := os.MkdirAll(p.ConfigDir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p.ConfigDir, "professional.json"), []byte(`{"log":{"level":"warn"}}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewSimpleSetupService(db, p, nil, nil, nil, nil, nil, nil)
			svc.supported = func() bool { return tc.supported }
			defer svc.Close()
			if _, err := svc.Start(model.SimpleSetupRequest{SubscriptionURL: tc.url}); err == nil {
				t.Fatal("unsafe setup accepted")
			}
			state, err := db.SimpleSetupState()
			if err != nil || state != "" {
				t.Fatalf("rejected start wrote marker: %q, %v", state, err)
			}
			available, err := db.SimpleSetupAvailable()
			if err != nil || !available {
				t.Fatalf("rejected start mutated pristine database: %v, %v", available, err)
			}
			if tc.active {
				content, err := os.ReadFile(filepath.Join(p.ConfigDir, "professional.json"))
				if err != nil || string(content) != `{"log":{"level":"warn"}}` {
					t.Fatalf("active configuration changed: %q, %v", content, err)
				}
			}
		})
	}
}

func TestSimpleSetupStatusProtectsUnmanagedConfigs(t *testing.T) {
	for _, tc := range []struct {
		name, marker, active string
		existing             bool
	}{
		{name: "prepared-retry", marker: "prepared"},
		{name: "prepared-unowned-simple", marker: "prepared", active: "simple.json", existing: true},
		{name: "applied-owned", marker: "applied", active: "simple.json"},
		{name: "applied-professional", marker: "applied", active: "professional.json", existing: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db, err := store.Open(filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.PrepareSimpleSetup(); err != nil {
				t.Fatal(err)
			}
			if err := db.SetSimpleSetupState(tc.marker); err != nil {
				t.Fatal(err)
			}
			p := &paths.Paths{ConfigDir: filepath.Join(dir, "configs")}
			if tc.active != "" {
				if err := os.MkdirAll(p.ConfigDir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(p.ConfigDir, tc.active), []byte(`{}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			svc := NewSimpleSetupService(db, p, nil, nil, nil, nil, nil, nil)
			svc.supported = func() bool { return true }
			defer svc.Close()
			status, err := svc.Status()
			if err != nil {
				t.Fatal(err)
			}
			if status.HasExistingConfig != tc.existing || status.Configured || status.Status != "failed" || status.Error == "" {
				t.Fatalf("unexpected retry status: %+v", status)
			}
		})
	}
}

func TestSimpleSetupMutationGuardRejectsRunningAndClosed(t *testing.T) {
	svc := &SimpleSetupService{state: model.SimpleSetupStatus{Status: "running"}}
	if release, err := svc.HoldMutation(); !errors.Is(err, ErrSimpleSetupBusy) || release != nil {
		t.Fatalf("running mutation accepted: %v", err)
	}
	svc.state.Status = "idle"
	release, err := svc.HoldMutation()
	if err != nil {
		t.Fatal(err)
	}
	release()
	svc.Close()
	if release, err := svc.HoldMutation(); !errors.Is(err, ErrSimpleSetupBusy) || release != nil {
		t.Fatalf("closed mutation accepted: %v", err)
	}
}

func TestSimpleSetupPendingBlocksBackgroundReconciliation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	generator := NewConfigGeneratorService(db, nil)
	for _, marker := range []string{"prepared", "applied"} {
		if err := db.SetSimpleSetupState(marker); err != nil {
			t.Fatal(err)
		}
		if result, err := generator.ReconcileCurrent(); err == nil || result != nil {
			t.Fatalf("background reconciliation accepted %s: %+v, %v", marker, result, err)
		}
		if result, err := generator.ReconcileCurrentForScheduledRestart(0); err == nil || result != nil {
			t.Fatalf("scheduled restart accepted %s: %+v, %v", marker, result, err)
		}
	}
}
