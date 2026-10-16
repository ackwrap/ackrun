package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
)

type transitionGuardStub struct {
	block   func() error
	unblock func() error
}

func (g transitionGuardStub) Enable(...string) error { return g.block() }
func (g transitionGuardStub) Disable() error         { return g.unblock() }

func TestProtectTransitionIncludesPreviousTUNMode(t *testing.T) {
	for _, oldTUN := range []bool{false, true} {
		t.Run(map[bool]string{false: "mixed", true: "tun-to-mixed"}[oldTUN], func(t *testing.T) {
			root := t.TempDir()
			p := &paths.Paths{DataDir: root, ConfigPath: filepath.Join(root, "config.json")}
			if err := os.WriteFile(p.ConfigPath, []byte(`{"inbounds":[{"type":"mixed"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			blocked := false
			svc := &SingboxService{paths: p, runningTUN: oldTUN, transitionGuard: transitionGuardStub{block: func() error { blocked = true; return nil }}}
			if err := svc.protectCoreTransition(); err != nil {
				t.Fatal(err)
			}
			if blocked != oldTUN {
				t.Fatalf("blocked=%t oldTUN=%t", blocked, oldTUN)
			}
		})
	}
}

func TestTransitionGuardReleaseRequiresReadyCoreAndUpdateCommit(t *testing.T) {
	for _, scenario := range []string{"ready", "exited", "stopping", "update-held", "release-failure"} {
		t.Run(scenario, func(t *testing.T) {
			f := newInstallerUpdateFixture(t)
			calls := 0
			svc := &SingboxService{paths: f.svc.paths, store: f.svc.store, realtime: NewRealtimeService(), pid: 123,
				cmd: &exec.Cmd{Process: &os.Process{Pid: 123}}, transitionGuard: transitionGuardStub{
					unblock: func() error {
						calls++
						if scenario == "release-failure" {
							return errors.New("delete failed")
						}
						return nil
					},
				}}
			if scenario == "exited" {
				svc.pid = 0
			}
			svc.stopping = scenario == "stopping"
			svc.transitionHeld = scenario == "update-held"
			_, err := svc.markStarted(&model.ActionResponse{Success: true})
			want := 0
			if scenario == "ready" || scenario == "release-failure" {
				want = 1
			}
			if calls != want || (err != nil) != (scenario == "exited" || scenario == "stopping" || scenario == "release-failure") {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
		})
	}
}

func TestInstallerTransitionGuardCoversSuccessAndRollback(t *testing.T) {
	for _, scenario := range []string{"success", "preflight", "guard-failure", "restart-failure", "rollback-failure", "db-failure"} {
		t.Run(scenario, func(t *testing.T) {
			f := newInstallerUpdateFixture(t)
			blocked := false
			f.svc.coreService = &SingboxService{paths: f.svc.paths, runningTUN: true, transitionGuard: transitionGuardStub{
				block: func() error {
					f.calls = append(f.calls, "block")
					if scenario == "guard-failure" {
						return errors.New("nft unavailable")
					}
					blocked = true
					return nil
				},
				unblock: func() error { f.calls = append(f.calls, "unblock"); blocked = false; return nil },
			}}
			stop, start := f.svc.stopCore, f.svc.startCore
			f.svc.stopCore = func() error {
				if !blocked {
					t.Fatal("core stopped without leak protection")
				}
				return stop()
			}
			f.svc.startCore = func() error {
				if !blocked || !f.svc.coreService.transitionHeld {
					t.Fatal("protection released before update commit")
				}
				binary, _ := os.ReadFile(f.svc.paths.BinaryPath)
				if scenario == "restart-failure" || scenario == "rollback-failure" {
					if string(binary) == "new binary" || scenario == "rollback-failure" {
						f.running = false
						f.calls = append(f.calls, "start-failed")
						return errors.New("startup failed")
					}
				}
				return start()
			}
			if scenario == "preflight" {
				f.svc.candidateValidator = func(string, string, string) error { return errors.New("invalid candidate") }
			}
			if scenario == "db-failure" {
				if _, err := f.svc.store.MigrateDNSIndependentCache(true); err != nil {
					t.Fatal(err)
				}
				if _, err := f.svc.store.DB().Exec(`CREATE TRIGGER reject_guard_dns BEFORE DELETE ON app_settings WHEN OLD.key IN ('dns.independent_cache','dns_global.independent_cache') BEGIN SELECT RAISE(ABORT,'failure'); END`); err != nil {
					t.Fatal(err)
				}
			}
			f.svc.runInstall()
			if blocked != (scenario == "rollback-failure") {
				t.Fatalf("blocked=%t calls=%v", blocked, f.calls)
			}
			if scenario == "preflight" && len(f.calls) != 0 {
				t.Fatalf("preflight changed firewall: %v", f.calls)
			}
			if scenario == "guard-failure" && strings.Join(f.calls, ",") != "validate,block" {
				t.Fatalf("core stopped after guard failure: %v", f.calls)
			}
			if scenario == "db-failure" && strings.Join(f.calls, ",") != "validate,block,stop,start-new,stop,start-old,unblock" {
				t.Fatalf("DB rollback guard lifecycle: %v", f.calls)
			}
		})
	}
}
