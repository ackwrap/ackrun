//go:build linux

package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeviceMACRouteOwnershipAndRecovery(t *testing.T) {
	for _, scenario := range []string{"new", "foreign", "partial", "replacement", "missing"} {
		t.Run(scenario, func(t *testing.T) {
			g := &nftDeviceProtection{path: filepath.Join(t.TempDir(), "guard.nft")}
			state := map[string]map[string][]map[string]interface{}{}
			for _, family := range []string{"-4", "-6"} {
				state[family] = map[string][]map[string]interface{}{"rule": {}, "route": {}}
			}
			mutations, fail := 0, scenario == "partial"
			g.ipRun = func(args ...string) ([]byte, error) {
				if args[1] == "-j" {
					return json.Marshal(state[args[0]][args[2]])
				}
				mutations++
				if fail && mutations == 2 {
					return nil, errors.New("synthetic failure")
				}
				if args[2] == "del" {
					state[args[0]][args[1]] = nil
					return nil, nil
				}
				if args[1] == "rule" {
					state[args[0]]["rule"] = []map[string]interface{}{{"priority": 8999, "src": "all", "fwmark": "0xace1", "table": 2024}}
				} else {
					state[args[0]]["route"] = []map[string]interface{}{{"type": "local", "dst": "default", "dev": "lo", "table": 2024, "protocol": "static", "flags": []interface{}{}}}
				}
				return nil, nil
			}
			if scenario == "foreign" {
				state["-4"]["rule"] = []map[string]interface{}{{"priority": 8999, "src": "all", "fwmark": "0xace1", "table": 2024}}
				if g.macRoutes("apply") == nil || mutations != 0 {
					t.Fatal("adopted foreign route without durable ownership")
				}
				return
			}
			err := g.macRoutes("apply")
			if scenario == "partial" {
				if err == nil {
					t.Fatal("partial failure hidden")
				}
				if _, err := os.Stat(g.path + ".mac-routes"); err != nil {
					t.Fatal("lost recovery intent")
				}
				fail = false
				err = g.macRoutes("apply")
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := g.macRoutes("verify"); err != nil {
				t.Fatal(err)
			}
			before := mutations
			if scenario == "replacement" {
				state["-4"]["rule"][0]["dst"] = "192.0.2.0/24"
				if g.macRoutes("verify") == nil || g.macRoutes("remove") == nil || mutations != before {
					t.Fatal("changed rule was trusted/deleted")
				}
				return
			}
			if scenario == "missing" {
				state["-6"]["route"] = nil
				if g.macRoutes("verify") == nil || mutations != before {
					t.Fatal("verification repaired a missing route")
				}
				if err := g.macRoutes("apply"); err != nil {
					t.Fatal(err)
				}
			}
			if err := g.macRoutes("remove"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(g.path + ".mac-routes"); !os.IsNotExist(err) {
				t.Fatal("ownership survived successful cleanup")
			}
		})
	}
}

func TestDeviceMACGuardRejectsBeforeOrdinaryRouting(t *testing.T) {
	rules, _, err := deviceProtectionRules([]string{"02:00:00:00:00:01", "192.0.2.10/32"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"hook prerouting priority -192", "hook prerouting priority -90", "tproxy to :1055", "ct original proto-dst 53 counter drop", "ether saddr 02:00:00:00:00:01 counter drop", "ip saddr 192.0.2.10/32"} {
		if !strings.Contains(rules, required) {
			t.Fatalf("missing %s", required)
		}
	}
	if _, _, err := deviceProtectionRules([]string{"02:00:00:00:00:01"}, []string{"br-lan"}); err == nil {
		t.Fatal("MAC capture silently excluded")
	}
}

func TestDeviceMACCleanupFailureKeepsPacketGuard(t *testing.T) {
	g, kernel := testDeviceProtection(t)
	if err := g.Apply([]string{"192.0.2.10/32"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(g.path+".mac-routes", []byte("Ackwrap device MAC routes v1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	g.ipRun = func(...string) ([]byte, error) { return nil, errors.New("synthetic ip failure") }
	if g.ForceDisable() == nil {
		t.Fatal("route cleanup failure hidden")
	}
	if kernel.owner == "" {
		t.Fatal("packet guard removed before route cleanup completed")
	}
	if _, err := os.Stat(g.path); err != nil {
		t.Fatal("boot guard removed on cleanup failure")
	}
}
