//go:build linux

package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTransitionDeviceScopeRecoveryAndOffload(t *testing.T) {
	for _, scenario := range []string{"new", "recover", "legacy", "global", "corrupt", "orphan"} {
		t.Run(scenario, func(t *testing.T) {
			g := &nftTransitionGuard{statePath: filepath.Join(t.TempDir(), "guard")}
			present := scenario != "new"
			family := "ip6"
			var script string
			g.run = func(input string, args ...string) ([]byte, error) {
				if strings.Join(args, " ") == "-j list ruleset" {
					entries := []any{map[string]any{"flowtable": map[string]any{"family": family, "table": "other", "name": "ft"}}}
					if present {
						entries = append(entries, map[string]any{"table": map[string]any{"family": "inet", "name": transitionGuardTable, "comment": transitionGuardOwner}})
					}
					return json.Marshal(map[string]any{"nftables": entries})
				}
				if strings.Join(args, " ") != "-f -" {
					t.Fatalf("unexpected command %v", args)
				}
				script = input
				present = true
				return nil, nil
			}
			previous := transitionGuardOwner + "\n" + `{"sources":["fd12:3456::2/128"],"fake_ips":["10.66.0.0/24"]}`
			switch scenario {
			case "legacy":
				previous = legacyTransitionGuardOwner + "\n[]\n"
			case "global":
				previous = transitionGuardOwner + "\n" + `{"sources":[],"fake_ips":[]}`
			case "corrupt":
				previous = transitionGuardOwner + "\n{}"
			}
			if scenario != "new" && scenario != "orphan" {
				if err := os.WriteFile(g.statePath, []byte(previous), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := g.Enable([]string{"192.168.50.2"}, nil)
			if (err == nil) != (scenario == "new") {
				t.Fatalf("unexpected result: %v", err)
			}
			if scenario == "new" {
				if !strings.Contains(script, "ip saddr 192.168.50.2/32 jump protected") || strings.Contains(script, "hook output") || strings.Contains(script, "policy drop") {
					t.Fatal("not scoped to the selected source")
				}
			} else if script != "" {
				t.Fatal("modified firewall before isolation was established")
			}
			if scenario == "corrupt" || scenario == "orphan" {
				return
			}
			// No offload now. Recover the union, never discard old sources/pools
			// or narrow a legacy global transition into device-only protection.
			family = "ip6"
			oldRun := g.run
			g.run = func(input string, args ...string) ([]byte, error) {
				if strings.Join(args, " ") == "-j list ruleset" {
					return []byte(`{"nftables":[]}`), nil
				}
				return oldRun(input, args...)
			}
			if err := g.Enable([]string{"192.168.50.3"}, nil, "10.77.0.0/24"); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(g.statePath)
			if err != nil {
				t.Fatal(err)
			}
			var policy transitionGuardPolicy
			if err := json.Unmarshal([]byte(strings.SplitN(string(data), "\n", 2)[1]), &policy); err != nil {
				t.Fatal(err)
			}
			if scenario == "legacy" || scenario == "global" {
				if len(policy.Sources) != 0 || !strings.Contains(script, "hook output") {
					t.Fatal("pending global guard was narrowed")
				}
			} else {
				old := "192.168.50.2/32"
				if scenario == "recover" {
					old = "fd12:3456::2/128"
					if !slices.Contains(policy.FakeIPs, "10.66.0.0/24") {
						t.Fatal("old FakeIP scope lost")
					}
				}
				if !slices.Contains(policy.Sources, old) || !slices.Contains(policy.Sources, "192.168.50.3/32") {
					t.Fatalf("old/new device scope lost: %v", policy.Sources)
				}
			}
		})
	}
}

func TestDeviceOffloadChecksAgreeAcrossLifecycles(t *testing.T) {
	g, n := testDeviceProtection(t)
	n.flowtables = []map[string]any{{"family": "ip6", "table": "other", "name": "ft"}}
	sources := []string{"192.168.50.2/32"}
	if err := g.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	if err := g.Verify(sources, nil); err != nil {
		t.Fatal(err)
	}
	dns := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "dns")}
	dns.run = func(input string, args ...string) ([]byte, error) {
		if strings.Join(args, " ") == "-f -" {
			return nil, nil
		}
		return n.run(input, args...)
	}
	if err := dns.Activate(sources); err != nil {
		t.Fatal(err)
	}
	n.flowtables[0]["family"] = "inet"
	for _, err := range []error{g.Verify(sources, nil), g.Apply(sources, nil), dns.Activate(sources)} {
		if err == nil || !strings.Contains(err.Error(), "已有卸载连接") {
			t.Fatalf("lifecycle missed a conflicting flowtable: %v", err)
		}
	}
}

func TestTransitionScopeIncludesRunningAndPersistedDevices(t *testing.T) {
	db, generator, _ := devicePolicyFixture(t)
	guard, _ := testDeviceProtection(t)
	mac := "02:00:00:00:00:02"
	// Only the source file is needed to recover MAC identity, even before its
	// kernel rules can be repaired after an interrupted activation.
	rules, _, err := deviceProtectionRules([]string{mac}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard.path, []byte(rules), 0600); err != nil {
		t.Fatal(err)
	}
	core := &SingboxService{store: db, paths: generator.paths, deviceProtection: guard, runningDeviceIPs: []string{"192.168.99.2/32"}}
	sources, err := core.transitionDeviceSources(core.runningDeviceIPs)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{mac, "192.168.99.2/32"} {
		if !slices.Contains(sources, want) {
			t.Fatal("lost old/persistent source", want)
		}
	}
	devices, err := enabledDeviceRules(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range deviceRuleCIDRs(devices) {
		if !slices.Contains(sources, want) {
			t.Fatal("lost configured source", want)
		}
	}
}
