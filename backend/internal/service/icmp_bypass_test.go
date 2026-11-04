package service

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func pingBypassRuleIndex(rules []map[string]interface{}) int {
	for index, rule := range rules {
		if rule["action"] == "bypass" && rule["ip_cidr"] != nil && stringListContains(rule["network"], "icmp") {
			return index
		}
	}
	return -1
}

func TestRealIPPingBypassSettingControlsAllProxyModes(t *testing.T) {
	for _, inbound := range []string{"tun", "tun_mixed", "mixed"} {
		for _, proxy := range []string{"rule", "global", "direct"} {
			t.Run(inbound+"/"+proxy, func(t *testing.T) {
				db, g := nodeBypassFixture(t)
				if err := db.SetInboundMode(inbound); err != nil {
					t.Fatal(err)
				}
				if err := db.SetProxyMode(proxy); err != nil {
					t.Fatal(err)
				}
				settings, err := db.GetGeneralSettings()
				if err != nil {
					t.Fatal(err)
				}
				for _, enabled := range []bool{false, true, false} {
					settings.RealIPPingBypassEnabled = enabled
					if err := db.SetGeneralSettings(settings); err != nil {
						t.Fatal(err)
					}
					route, err := g.generateRoute("direct")
					if err != nil {
						t.Fatal(err)
					}
					rules := route["rules"].([]map[string]interface{})
					index := pingBypassRuleIndex(rules)
					if (index >= 0) != (enabled && inbound != "mixed") {
						t.Fatalf("enabled=%t, Ping bypass position=%d", enabled, index)
					}
					if index >= 0 {
						if !reflect.DeepEqual(rules[index]["inbound"], []string{"tun-in"}) || !reflect.DeepEqual(rules[index]["network"], []string{"icmp"}) || rules[index]["outbound"] != "direct" {
							t.Fatal("Ping bypass broadened to other inbounds/networks or lost its fallback")
						}
						for i, rule := range rules {
							if rule["action"] == "sniff" && i <= index {
								t.Fatal("Ping bypass was placed after sniff")
							}
						}
					}
				}
			})
		}
	}
}

func TestRealIPPingBypassExcludesFakeIPAndPreservesDevicePriority(t *testing.T) {
	for _, mac := range []bool{false, true} {
		t.Run(fmt.Sprintf("MAC=%t", mac), func(t *testing.T) {
			db, g, devices := devicePolicyFixture(t)
			if mac {
				if _, err := db.UpdateRouteRule(devices[0].ID, &model.RouteRuleRequest{Name: devices[0].Name, RuleType: "source_ip_cidr", Values: []string{"02:00:00:00:00:25"}, Enabled: true, Outbound: "proxy"}); err != nil {
					t.Fatal(err)
				}
			}
			settings, err := db.GetGeneralSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.RealIPPingBypassEnabled = true
			if err := db.SetGeneralSettings(settings); err != nil {
				t.Fatal(err)
			}
			dns, err := db.GetDNSGlobalSettings()
			if err != nil {
				t.Fatal(err)
			}
			dns.FakeIPInet4Range, dns.FakeIPInet6Range = "10.66.0.0/24", "fd66::/112"
			if err := db.SetDNSGlobalSettings(dns); err != nil {
				t.Fatal(err)
			}
			g.paths.ConfigDir = t.TempDir()
			g.paths.ConfigPath = filepath.Join(g.paths.ConfigDir, "config.json")
			if err := os.WriteFile(g.paths.ConfigPath, []byte(`{"dns":{"servers":[{"type":"fakeip","inet4_range":"10.77.0.0/24","inet6_range":"fd77::/112"}]}}`), 0600); err != nil {
				t.Fatal(err)
			}
			route, err := g.generateRoute("direct")
			if err != nil {
				t.Fatal(err)
			}
			rules := route["rules"].([]map[string]interface{})
			index := pingBypassRuleIndex(rules)
			if index < 0 {
				t.Fatal("missing Ping bypass rule")
			}
			deviceRoutes := 0
			for i, rule := range rules {
				if rule["action"] == "route" && (rule["outbound"] == devices[0].Name || rule["outbound"] == devices[1].Name) {
					deviceRoutes++
					if i >= index || rule["network"] != nil {
						t.Fatal("Ping bypass escaped a device's terminating route")
					}
				}
			}
			if deviceRoutes != len(devices) {
				t.Fatalf("device routes=%d, want %d", deviceRoutes, len(devices))
			}
			for address, want := range map[string]bool{
				"192.0.2.25": true, "2001:db8::25": true, "192.168.50.1": true,
				"198.18.0.1": false, "fc00::1": false,
				"10.66.0.1": false, "fd66::1": false,
				"10.77.0.1": false, "fd77::1": false,
			} {
				matched := false
				for _, cidr := range rules[index]["ip_cidr"].([]string) {
					if netip.MustParsePrefix(cidr).Contains(netip.MustParseAddr(address)) {
						matched = true
					}
				}
				if matched != want {
					t.Fatalf("Ping bypass matches %s=%t, want %t", address, matched, want)
				}
			}
		})
	}
}

func TestRealIPPingBypassWithAvailableCore(t *testing.T) {
	core := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if core == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN for core schema validation")
	}
	_, g := nodeBypassFixture(t)
	rule, err := g.realIPPingBypassRule()
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]interface{}{
		"inbounds":  []map[string]interface{}{{"type": "mixed", "tag": "tun-in", "listen": "127.0.0.1", "listen_port": 17890}},
		"outbounds": []map[string]interface{}{{"type": "direct", "tag": "direct"}},
		"route":     map[string]interface{}{"rules": []map[string]interface{}{rule}, "final": "direct"},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "ping.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(core, "check", "-c", configPath).CombinedOutput(); err != nil {
		t.Fatalf("core rejected Ping bypass: %v: %s", err, out)
	}
}
