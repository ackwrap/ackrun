//go:build linux

package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNFTTransitionGuardOwnershipOffloadAndAtomicRules(t *testing.T) {
	for _, scenario := range []string{"new", "existing", "foreign", "offload", "apply-failure", "remove-failure"} {
		t.Run(scenario, func(t *testing.T) {
			var calls []string
			var script string
			present := scenario == "existing" || scenario == "foreign" || scenario == "remove-failure"
			g := &nftTransitionGuard{statePath: filepath.Join(t.TempDir(), "guard")}
			if present {
				if err := os.WriteFile(g.statePath, []byte(legacyTransitionGuardOwner+"\n[]\n"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			g.run = func(input string, args ...string) ([]byte, error) {
				call := strings.Join(args, " ")
				calls = append(calls, call)
				if call == "-j list ruleset" {
					owner := transitionGuardOwner
					if scenario == "foreign" {
						owner = "someone else"
					}
					if present {
						return []byte(`{"nftables":[{"table":{"family":"inet","name":"ackwrap_transition","comment":"` + owner + `"}}]}`), nil
					}
					if scenario == "offload" {
						return []byte(`{"nftables":[{"flowtable":{"family":"inet","table":"fw4","name":"ft"}}]}`), nil
					}
					return []byte(`{"nftables":[]}`), nil
				}
				if call == "-f -" {
					script = input
					if scenario == "apply-failure" {
						return nil, errors.New("apply failed")
					}
					present = true
					return nil, nil
				}
				if call == "delete table inet ackwrap_transition" {
					if scenario == "remove-failure" {
						return nil, errors.New("delete failed")
					}
					present = false
					return nil, nil
				}
				t.Fatalf("unexpected command: %s", call)
				return nil, nil
			}
			err := g.Enable(nil, nil)
			fails := scenario == "foreign" || scenario == "offload" || scenario == "apply-failure"
			if (err != nil) != fails {
				t.Fatalf("Enable error=%v", err)
			}
			if scenario == "foreign" || scenario == "offload" {
				if len(calls) != 1 {
					t.Fatal("modified foreign/offloaded firewall")
				}
				return
			}
			for _, want := range []string{"hook forward priority -1; policy drop", "hook output priority 300; policy drop", "oifname \"lo\" accept", "meta mark 0x2024 accept", "ct direction reply accept"} {
				if !strings.Contains(script, want) {
					t.Fatalf("missing %q", want)
				}
			}
			if strings.Contains(script, "ct state established") || strings.Contains(script, "flush ruleset") || strings.Contains(script, "delete table inet fw4") {
				t.Fatal("unsafe firewall exemption or cleanup")
			}
			if _, err := os.Stat(g.statePath); err != nil {
				t.Fatal("missing durable guard intent")
			}
			if scenario == "apply-failure" {
				return
			}
			// Simulate a fresh backend: protection must be removable after readiness.
			restarted := &nftTransitionGuard{run: g.run, statePath: g.statePath}
			err = restarted.Disable()
			if scenario == "remove-failure" {
				if err == nil || !present {
					t.Fatal("removal failure did not retain guard")
				}
				if _, err := os.Stat(g.statePath); err != nil {
					t.Fatal("lost recovery marker")
				}
			} else if err != nil || present {
				t.Fatalf("Disable error=%v present=%t", err, present)
			}
		})
	}
}

func TestNFTTransitionGuardMixedModeDoesNotInspectFirewall(t *testing.T) {
	g := &nftTransitionGuard{statePath: filepath.Join(t.TempDir(), "absent"), run: func(string, ...string) ([]byte, error) {
		t.Fatal("mixed-only startup queried firewall")
		return nil, nil
	}}
	if err := g.Disable(); err != nil {
		t.Fatal(err)
	}
}
