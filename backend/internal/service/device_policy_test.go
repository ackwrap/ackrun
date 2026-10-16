package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestDeviceRuleValidation(t *testing.T) {
	for _, value := range []string{"192.168.1.100", "192.168.1.100/32", "fd12:3456::100/128"} {
		req := &model.RouteRuleRequest{Name: "device", RuleType: "source_ip_cidr", Values: []string{value}, Outbound: "proxy"}
		if err := validateRouteRule(req); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(req.Values[0], "/") {
			t.Fatal("host address was not normalized")
		}
	}
	for _, value := range []string{"example.com", "0.0.0.0/0", "::/0", "127.0.0.1", "ff02::1", "fe80::1", "::ffff:192.168.1.1", "192.168.1.1/33", "192.168.1.1; drop"} {
		if _, err := normalizeDeviceCIDRs([]string{value}); err == nil {
			t.Fatalf("accepted invalid device source %q", value)
		}
	}
	for _, req := range []*model.RouteRuleRequest{
		{Name: "device", RuleType: "source_ip_cidr", Values: []string{"192.168.1.100"}, Outbound: "direct"},
		{Name: "device", RuleType: "source_ip_cidr", Values: []string{"192.168.1.100"}, Outbound: "proxy", Invert: true},
		{Name: "device", RuleType: "mixed", Values: []string{"source_ip_cidr:192.168.1.100/32"}, Outbound: "proxy"},
	} {
		if err := validateRouteRule(req); err == nil {
			t.Fatal("accepted ambiguous device rule")
		}
	}
}

func devicePolicyFixture(t *testing.T) (*store.Store, *ConfigGeneratorService, []*model.RouteRule) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "remote", Enabled: true, ServerType: "https", Address: "https://1.1.1.1/dns-query"}); err != nil {
		t.Fatal(err)
	}
	global, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	global.Enabled, global.Final, global.ProxyFinal = true, "remote", "remote"
	if err := db.SetDNSGlobalSettings(global); err != nil {
		t.Fatal(err)
	}
	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: "https://example.invalid/sub"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, []model.ParsedNode{
		{UID: "node-a", Name: "Node A", Type: "socks", Server: "192.0.2.10", ServerPort: 1080, RawJSON: `{"type":"socks","server":"192.0.2.10","server_port":1080}`},
		{UID: "node-b", Name: "Node B", Type: "socks", Server: "192.0.2.11", ServerPort: 1080, RawJSON: `{"type":"socks","server":"192.0.2.11","server_port":1080}`},
	}); err != nil {
		t.Fatal(err)
	}
	var rules []*model.RouteRule
	for i, name := range []string{"Device A", "Device B"} {
		cidrs := []string{"192.168.50.10/32", "fd12:3456::10/128"}
		uid := "node-a"
		if i == 1 {
			cidrs, uid = []string{"192.168.50.20/32"}, "node-b"
		}
		rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: name, RuleType: "source_ip_cidr", Values: cidrs, Enabled: true, Outbound: "proxy"})
		if err != nil {
			t.Fatal(err)
		}
		_, err = NewProxyCollectionService(db, nil).Create(model.ProxyCollectionRequest{
			RouteRuleID: rule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{uid}, Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		rules = append(rules, rule)
	}
	g := NewConfigGeneratorService(db, &paths.Paths{DataDir: dir})
	g.dnsmasqSupported = func() bool { return true }
	return db, g, rules
}

func TestDeviceTrafficAndDNSUseSameFixedNode(t *testing.T) {
	db, g, devices := devicePolicyFixture(t)
	for _, outbound := range []string{"direct", "bypass", "block"} {
		if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
			Name: "Ordinary " + outbound, Enabled: true, Priority: 1,
			RuleType: "ip_cidr", Values: []string{"0.0.0.0/0"}, Outbound: outbound,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if runtime.GOOS == "linux" {
		if err := g.validateDevicePolicies(); err != nil {
			t.Fatal(err)
		}
	}
	outbounds, endpoints, err := g.generateOutbounds(true)
	if err != nil {
		t.Fatal(err)
	}
	dns, err := g.generateDNSFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDNSDetourPaths(dns, outbounds, endpoints); err != nil {
		t.Fatal(err)
	}
	dnsRules := dns["rules"].([]map[string]interface{})
	route, err := g.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	routes := route["rules"].([]map[string]interface{})
	fakeIPs, err := g.deviceFakeIPCIDRs()
	if err != nil {
		t.Fatal(err)
	}
	local, err := excludeDeviceFakeIPCIDRs(deviceLocalDestinationCIDRs(), fakeIPs)
	if err != nil {
		t.Fatal(err)
	}
	for i, device := range devices {
		tag := deviceDNSServerTag(device.ID)
		if dnsRules[i]["server"] != tag || !reflect.DeepEqual(dnsRules[i]["source_ip_cidr"], device.Values) {
			t.Fatal("device DNS is not ahead of normal DNS/FakeIP rules")
		}
		assertGeneratedDNSServerDetour(t, dns, tag, device.Name)
		found := false
		localBeforeResolve, localAfterResolve, resolved := false, false, false
		for _, rule := range routes {
			if rule["action"] == "sniff" || reflect.DeepEqual(rule["ip_cidr"], []string{"0.0.0.0/0"}) {
				break
			}
			if rule["action"] == "route" && rule["outbound"] == device.Name {
				found = reflect.DeepEqual(rule["source_ip_cidr"], device.Values)
				if !localBeforeResolve || !localAfterResolve {
					t.Fatal("device local traffic must bypass before resolution and before the fixed outbound")
				}
			}
			if !reflect.DeepEqual(rule["source_ip_cidr"], device.Values) {
				continue
			}
			if rule["action"] == "resolve" {
				resolved = true
			}
			if rule["action"] == "bypass" && rule["outbound"] == "direct" && reflect.DeepEqual(rule["ip_cidr"], local) {
				if resolved {
					localAfterResolve = true
				} else {
					localBeforeResolve = true
				}
			}
		}
		if !found {
			t.Fatal("device route not ahead of business rules")
		}
	}
	inbounds, err := g.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]interface{}{"dns": dns, "route": route, "inbounds": inbounds}
	data, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "linux" {
		state, err := readActiveTUNState(path)
		if err != nil {
			t.Fatal(err)
		}
		if !state.DNSMasqTakeover || len(state.DeviceDNSCIDRs) != 3 {
			t.Fatalf("missing DNS lifecycle state: %+v", state)
		}
	}
}

func TestDevicePolicyAllowsOnlyLocalTargetExclusions(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux device policy validation")
	}
	for _, test := range []struct {
		name    string
		rule    model.TrafficBypassRule
		allowed bool
	}{
		{"private-10", model.TrafficBypassRule{Type: "ip_cidr", Value: "10.0.0.0/8"}, true},
		{"private-172", model.TrafficBypassRule{Type: "ip_cidr", Value: "172.16.0.0/12"}, true},
		{"private-192", model.TrafficBypassRule{Type: "ip_cidr", Value: "192.168.0.0/16"}, true},
		{"private-subnet", model.TrafficBypassRule{Type: "ip_cidr", Value: "10.20.30.0/24"}, true},
		{"ula", model.TrafficBypassRule{Type: "ip_cidr", Value: "fc00::/7"}, true},
		{"link-local", model.TrafficBypassRule{Type: "ip_cidr", Value: "fe80::/10"}, true},
		{"easytier-process", model.TrafficBypassRule{Type: "process_name", Value: "easytier-core"}, true},
		{"unrelated-source", model.TrafficBypassRule{Type: "source_ip_cidr", Value: "192.168.60.0/24"}, true},
		{"overlapping-source", model.TrafficBypassRule{Type: "source_ip_cidr", Value: "192.168.50.0/24"}, false},
		{"interface", model.TrafficBypassRule{Type: "interface", Value: "easytier-tun"}, true},
		{"domain", model.TrafficBypassRule{Type: "domain_suffix", Value: "example.com"}, false},
		{"public", model.TrafficBypassRule{Type: "ip_cidr", Value: "8.8.8.8/32"}, false},
		{"spans-public", model.TrafficBypassRule{Type: "ip_cidr", Value: "10.0.0.0/7"}, false},
		{"spans-public-172", model.TrafficBypassRule{Type: "ip_cidr", Value: "172.16.0.0/11"}, false},
		{"default", model.TrafficBypassRule{Type: "ip_cidr", Value: "0.0.0.0/0"}, false},
		{"public-ipv6", model.TrafficBypassRule{Type: "ip_cidr", Value: "2000::/3"}, false},
		{"mapped", model.TrafficBypassRule{Type: "ip_cidr", Value: "::ffff:192.168.1.1/128"}, false},
		{"fakeip", model.TrafficBypassRule{Type: "ip_cidr", Value: "198.18.0.0/15"}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, g, _ := devicePolicyFixture(t)
			if err := db.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{test.rule}}); err != nil {
				t.Fatal(err)
			}
			if err := g.validateDevicePolicies(); (err == nil) != test.allowed {
				t.Fatalf("allowed=%t, err=%v", test.allowed, err)
			}
		})
	}
}

func TestDevicePoliciesRejectConflictsAndPreserveActiveConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux device policy validation")
	}
	for _, scenario := range []string{"overlap", "bypass", "dns-off", "mixed", "missing-binding", "direct-node", "disabled-node", "advanced-route"} {
		t.Run(scenario, func(t *testing.T) {
			db, g, rules := devicePolicyFixture(t)
			switch scenario {
			case "advanced-route":
				settings, err := db.GetAdvancedSettings()
				if err != nil {
					t.Fatal(err)
				}
				settings.RoutingEnabled = true
				if err := db.SetAdvancedSettings(settings); err != nil {
					t.Fatal(err)
				}
				if err := db.CreatePlatformRoute(&model.PlatformRoute{Name: "Catch all", Platform: "test", Enabled: true, Priority: 10, TargetType: "direct", FallbackType: "direct"}); err != nil {
					t.Fatal(err)
				}
			case "overlap":
				_, err := db.UpdateRouteRule(rules[1].ID, &model.RouteRuleRequest{Name: rules[1].Name, Enabled: true, RuleType: "source_ip_cidr", Values: []string{"192.168.50.0/24"}, Outbound: "proxy"})
				if err != nil {
					t.Fatal(err)
				}
			case "bypass":
				if err := db.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "source_ip_cidr", Value: "192.168.50.0/24"}}}); err != nil {
					t.Fatal(err)
				}
			case "dns-off":
				settings, _ := db.GetDNSGlobalSettings()
				settings.Enabled = false
				if err := db.SetDNSGlobalSettings(settings); err != nil {
					t.Fatal(err)
				}
			case "mixed":
				if err := db.SetInboundMode("mixed"); err != nil {
					t.Fatal(err)
				}
			case "missing-binding":
				if _, err := db.DB().Exec("UPDATE proxy_collections SET enabled = 0 WHERE route_rule_id = ?", rules[0].ID); err != nil {
					t.Fatal(err)
				}
			case "direct-node":
				if _, err := db.DB().Exec("UPDATE proxy_collections SET node_uids = '[\"direct\"]' WHERE route_rule_id = ?", rules[0].ID); err != nil {
					t.Fatal(err)
				}
			case "disabled-node":
				if _, err := db.DB().Exec("UPDATE nodes SET enabled = 0 WHERE uid = 'node-a'"); err != nil {
					t.Fatal(err)
				}
			}
			active := filepath.Join(t.TempDir(), "active.json")
			if err := os.WriteFile(active, []byte("working configuration"), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := g.generateLockedTo(&model.ConfigGenerateRequest{DefaultOutbound: "direct", TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address}, active)
			if err == nil {
				t.Fatal("invalid device policy accepted")
			}
			data, _ := os.ReadFile(active)
			if string(data) != "working configuration" {
				t.Fatal("failed generation replaced working config")
			}
		})
	}
}

func TestDevicePolicyRejectsRuntimeOverrideAndRollsBack(t *testing.T) {
	for _, test := range []struct {
		name    string
		sources []string
		blocked bool
	}{
		{name: "catch-all", blocked: true},
		{name: "overlapping", sources: []string{"192.168.50.0/24"}, blocked: true},
		{name: "ipv6-overlapping", sources: []string{"fd12:3456::/64"}, blocked: true},
		{name: "other-device", sources: []string{"192.168.60.0/24"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, _, _ := devicePolicyFixture(t)
			settings, err := db.GetAdvancedSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.RoutingEnabled = true
			if err := db.SetAdvancedSettings(settings); err != nil {
				t.Fatal(err)
			}
			runtime := &advancedRuntimeStub{}
			svc := NewAdvancedRoutingService(db, runtime, &advancedCoreStub{running: true}, nil, "test")
			_, err = svc.CreatePlatformRoute(model.PlatformRoute{
				Name: "Platform route", Platform: "test", Enabled: true, Priority: 10,
				SourceCIDRs: test.sources, TargetType: "direct", FallbackType: "direct",
			})
			if test.blocked {
				if err == nil || !strings.Contains(err.Error(), "设备规则") {
					t.Fatalf("missing device conflict error: %v", err)
				}
				routes, err := db.ListPlatformRoutes()
				if err != nil || len(routes) != 0 || len(runtime.puts) != 0 {
					t.Fatalf("conflicting mutation was not rolled back: routes=%d puts=%d err=%v", len(routes), len(runtime.puts), err)
				}
			} else if err != nil || len(runtime.puts) != 1 {
				t.Fatalf("unrelated platform route rejected: %v", err)
			}
		})
	}
}

func TestDevicePolicyInactiveOutsideRuleMode(t *testing.T) {
	db, g, devices := devicePolicyFixture(t)
	setDeviceDirectDomains(t, db, devices[0], "example.com")
	for _, mode := range []string{"direct", "global"} {
		if err := db.SetProxyMode(mode); err != nil {
			t.Fatal(err)
		}
		rules, err := g.deviceRules()
		if err != nil || len(rules) != 0 {
			t.Fatalf("mode %s: %v", mode, err)
		}
	}
}

func TestDeviceConfigurationPassesAvailableCoreCheck(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux device DNS interception")
	}
	binary := strings.TrimSpace(os.Getenv("ACKWRAP_SINGBOX_114_BIN"))
	if binary == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN for real core validation")
	}
	db, g, devices := devicePolicyFixture(t)
	if err := db.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "interface", Value: "easytier-tun"}}}); err != nil {
		t.Fatal(err)
	}
	g.paths.BinaryPath = binary
	result, err := g.generateLockedTo(&model.ConfigGenerateRequest{
		DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: 7890,
		TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
	}, filepath.Join(t.TempDir(), "generated.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("device configuration rejected by core: %s", result.Error)
	}
	setDeviceDirectDomains(t, db, devices[0], "baidu.com", "example.com")
	global, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	global.FakeIPEnabled = true
	if err := db.SetDNSGlobalSettings(global); err != nil {
		t.Fatal(err)
	}
	result, err = g.generateLockedTo(&model.ConfigGenerateRequest{
		DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: 7890,
		TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
	}, filepath.Join(t.TempDir(), "whitelist.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("whitelist configuration rejected by core: %s", result.Error)
	}
}
