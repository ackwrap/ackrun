//go:build linux

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
)

func TestStartupRecoveryKeepsProtectionWhenCleanupFails(t *testing.T) {
	root := t.TempDir()
	blocked, stopped := false, false
	svc := &SingboxService{
		paths:      &paths.Paths{DataDir: root, ConfigPath: filepath.Join(root, "absent.json")},
		runningTUN: true,
		transitionGuard: transitionGuardStub{
			block:   func() error { blocked = true; return nil },
			unblock: func() error { t.Fatal("cleanup failure released protection"); return nil },
		},
	}
	marker := []byte("invalid ownership record")
	if err := os.WriteFile(svc.routeTableStatePath(), marker, 0600); err != nil {
		t.Fatal(err)
	}
	err := svc.recoverBeforeStart(func() error {
		if !blocked {
			t.Fatal("core stopped before protection")
		}
		stopped = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "清理 sing-box 网络残留失败") || !blocked || !stopped {
		t.Fatalf("blocked=%t stopped=%t error=%v", blocked, stopped, err)
	}
	if data, err := os.ReadFile(svc.routeTableStatePath()); err != nil || string(data) != string(marker) {
		t.Fatal("failed cleanup discarded recovery state")
	}
}
