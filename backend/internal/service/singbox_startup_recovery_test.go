package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
)

func TestStartupRecoveryKeepsProtectionOnStopFailure(t *testing.T) {
	for _, guardFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "stop-failure", true: "guard-failure"}[guardFailure], func(t *testing.T) {
			root := t.TempDir()
			blocked, stopped := false, false
			svc := &SingboxService{
				paths:      &paths.Paths{DataDir: root, ConfigPath: filepath.Join(root, "absent.json")},
				runningTUN: true,
				transitionGuard: transitionGuardStub{
					block: func() error {
						if guardFailure {
							return errors.New("synthetic guard failure")
						}
						blocked = true
						return nil
					},
					unblock: func() error { t.Fatal("failure released protection"); return nil },
				},
			}
			// Cleanup would reject this record. A stop failure must be returned
			// first, leaving the record available for a later recovery attempt.
			marker := []byte("invalid ownership record")
			if err := os.WriteFile(svc.routeTableStatePath(), marker, 0600); err != nil {
				t.Fatal(err)
			}
			err := svc.recoverBeforeStart(func() error {
				if !blocked {
					t.Fatal("stopped the existing core before enabling protection")
				}
				stopped = true
				return errors.New("synthetic stop failure")
			})
			want := "synthetic stop failure"
			if guardFailure {
				want = "synthetic guard failure"
			}
			if err == nil || !strings.Contains(err.Error(), want) || stopped == guardFailure || blocked == guardFailure {
				t.Fatalf("blocked=%t stopped=%t error=%v", blocked, stopped, err)
			}
			if data, err := os.ReadFile(svc.routeTableStatePath()); err != nil || string(data) != string(marker) {
				t.Fatal("failed stop changed network ownership")
			}
		})
	}
}

func TestStartupValidatesReplacementBeforeRecovery(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "core")
	if err := os.WriteFile(binary, nil, 0600); err != nil {
		t.Fatal(err)
	}
	svc := &SingboxService{
		paths:      &paths.Paths{DataDir: root, BinaryPath: binary},
		realtime:   NewRealtimeService(),
		runningTUN: true,
		transitionGuard: transitionGuardStub{
			block: func() error { t.Fatal("invalid replacement entered recovery"); return nil },
		},
	}
	_, err := svc.start(func() (string, error) { return "", errors.New("synthetic invalid config") })
	if err == nil || !strings.Contains(err.Error(), "synthetic invalid config") {
		t.Fatalf("replacement validation failure lost: %v", err)
	}
}
