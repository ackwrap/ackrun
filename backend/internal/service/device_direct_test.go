package service

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func setDeviceDirectDomains(t *testing.T, db *store.Store, device *model.RouteRule, domains ...string) {
	t.Helper()
	_, err := db.UpdateRouteRule(device.ID, &model.RouteRuleRequest{
		Name: device.Name, Enabled: device.Enabled, Priority: device.Priority,
		RuleType: device.RuleType, Values: device.Values, Outbound: device.Outbound,
		DirectDomains: domains,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeviceDirectDomainValidation(t *testing.T) {
	domains, err := normalizeDeviceDirectDomains([]string{" Baidu.COM. ", "baidu.com", "xn--fiqs8s.example"})
	if err != nil || !reflect.DeepEqual(domains, []string{"baidu.com", "xn--fiqs8s.example"}) {
		t.Fatalf("normalize: %v %v", domains, err)
	}
	for _, value := range []string{"", ".", "com", "*.example.com", "https://example.com", "example.com/path", "example.com:443", "192.0.2.1", "::1", "bad..example", "-bad.example", strings.Repeat("a", 64) + ".example", "例子.com"} {
		if _, err := normalizeDeviceDirectDomains([]string{value}); err == nil {
			t.Errorf("accepted %q", value)
		}
	}
	if _, err := normalizeDeviceDirectDomains(make([]string, 257)); err == nil {
		t.Fatal("accepted excessive domains")
	}
	if err := validateRouteRule(&model.RouteRuleRequest{Name: "ordinary", RuleType: "domain", Values: []string{"example.com"}, Outbound: "direct", DirectDomains: []string{"baidu.com"}}); err == nil {
		t.Fatal("accepted whitelist outside device rule")
	}
}

func TestDeviceDirectDomainsScopedBeforeFixedNode(t *testing.T) {
	db, g, devices := devicePolicyFixture(t)
	setDeviceDirectDomains(t, db, devices[0], "baidu.com")
	// Prefer the global remote resolver for direct domains, regardless of its saved detour.
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "local-upstream", Enabled: true, ServerType: "udp", Address: "223.5.5.5", Detour: "proxy"}); err != nil {
		t.Fatal(err)
	}
	global, _ := db.GetDNSGlobalSettings()
	global.Final, global.FakeIPEnabled = "local-upstream", true
	if err := db.SetDNSGlobalSettings(global); err != nil {
		t.Fatal(err)
	}
	dns, err := g.generateDNSFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	rules := dns["rules"].([]map[string]interface{})
	if !reflect.DeepEqual(rules[0]["source_ip_cidr"], devices[0].Values) || !reflect.DeepEqual(rules[0]["domain_suffix"], []string{"baidu.com"}) || rules[0]["server"] != deviceDirectDNSServerTag(devices[0].ID) {
		t.Fatalf("unscoped direct DNS: %v", rules[0])
	}
	if rules[1]["server"] != deviceDNSServerTag(devices[0].ID) || rules[2]["server"] != deviceDNSServerTag(devices[1].ID) {
		t.Fatal("device DNS precedence changed")
	}
	if dns["reverse_mapping"] != true {
		t.Fatal("missing DNS domain recovery")
	}
	found := false
	for _, server := range dns["servers"].([]map[string]interface{}) {
		if server["tag"] == deviceDirectDNSServerTag(devices[0].ID) {
			found = true
			if server["detour"] != nil || server["server"] != "223.5.5.5" {
				t.Fatalf("whitelist DNS must directly use global upstream: %v", server)
			}
		}
	}
	if !found {
		t.Fatal("missing direct resolver")
	}
	assertGeneratedDNSServerDetour(t, dns, deviceDNSServerTag(devices[0].ID), devices[0].Name)
	assertGeneratedDNSServerDetour(t, dns, deviceDNSServerTag(devices[1].ID), devices[1].Name)
	route, err := g.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	var sniffed, resolved, rejectedFake, direct, fixed bool
	for _, rule := range route["rules"].([]map[string]interface{}) {
		if _, hasDomains := rule["domain_suffix"]; hasDomains && rule["action"] == "route" && rule["outbound"] == "direct" && !reflect.DeepEqual(rule["source_ip_cidr"], devices[0].Values) {
			t.Fatal("direct domain rule leaked to another source")
		}
		if !reflect.DeepEqual(rule["source_ip_cidr"], devices[0].Values) {
			continue
		}
		switch rule["action"] {
		case "sniff":
			sniffed = true
		case "resolve":
			if rule["server"] == deviceDirectDNSServerTag(devices[0].ID) {
				resolved = sniffed
			}
		case "reject":
			rejectedFake = resolved
		case "route":
			if rule["outbound"] == "direct" {
				if !rejectedFake || !reflect.DeepEqual(rule["domain_suffix"], []string{"baidu.com"}) {
					t.Fatal("direct route lacks domain scoping or FakeIP guard")
				}
				direct = true
			} else if rule["outbound"] == devices[0].Name {
				fixed = direct
			}
		}
	}
	if !fixed {
		t.Fatal("whitelist is not ahead of fixed device fallback")
	}
	// An invalid saved value must stop generation, rather than silently disabling scoping.
	setDeviceDirectDomains(t, db, devices[0], "*")
	if _, err := g.generateRoute("direct"); err == nil {
		t.Fatal("invalid stored whitelist accepted")
	}
}

func TestDeviceDirectDomainShareRoundTrip(t *testing.T) {
	db, _, devices := devicePolicyFixture(t)
	setDeviceDirectDomains(t, db, devices[0], "baidu.com")
	source := newTestRouteRuleService(t, db)
	share, err := source.Share()
	if err != nil {
		t.Fatal(err)
	}
	targetDB, err := store.Open(filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer targetDB.Close()
	target := newTestRouteRuleService(t, targetDB)
	for _, repeat := range []bool{false, true} {
		result, err := target.ImportShare(&model.RouteRuleImportRequest{Code: share.Code})
		if err != nil {
			t.Fatal(err)
		}
		if repeat && result.Updated != 2 {
			t.Fatal("expected merge update")
		}
		rules, err := targetDB.ListRouteRules()
		if err != nil {
			t.Fatal(err)
		}
		for _, rule := range rules {
			if rule.Name == devices[0].Name && !reflect.DeepEqual(rule.DirectDomains, []string{"baidu.com"}) {
				t.Fatal("share lost whitelist")
			}
		}
	}
}
