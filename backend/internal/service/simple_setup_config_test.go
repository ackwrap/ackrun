package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSimpleSetupGeneratesUsableRoutingAndLocalCNDNS(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: "https://example.com/sub"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSubscriptionNodes(sub.ID, []model.ParsedNode{{UID: "setup-node", Name: "Test Node", Type: "socks", Server: "203.0.113.2", ServerPort: 1080, RawJSON: `{"type":"socks","server":"203.0.113.2","server_port":1080}`}}); err != nil {
		t.Fatal(err)
	}
	generator := NewConfigGeneratorService(db, nil)
	outbounds, endpoints, err := generator.generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	if !hasUsableNonDirectOutboundPath(outbounds, endpoints, []string{"proxy"}) {
		t.Fatal("simple setup must have a usable proxy path")
	}
	foundProxy := false
	for _, item := range outbounds {
		outbound := item.(map[string]interface{})
		if outbound["tag"] == "proxy" {
			foundProxy = true
			members, ok := outbound["outbounds"].([]string)
			if !ok || len(members) == 0 || members[0] != "默认代理" {
				t.Fatalf("global proxy does not default to common selector: %+v", outbound)
			}
		}
	}
	if !foundProxy {
		t.Fatal("missing proxy selector")
	}
	route, err := generator.generateRoute("默认代理")
	if err != nil {
		t.Fatal(err)
	}
	if route["final"] != "proxy" {
		t.Fatalf("unexpected final route: %v", route["final"])
	}
	rules := route["rules"].([]map[string]interface{})
	positions := map[string]int{}
	cnBypass, ads := false, false
	cnIndex, adsIndex := -1, -1
	for i, rule := range rules {
		if outbound, ok := rule["outbound"].(string); ok {
			if _, exists := positions[outbound]; !exists {
				positions[outbound] = i
			}
		}
		if stringListContains(rule["rule_set"], "geoip-cn") && rule["action"] == "bypass" {
			cnBypass = true
			cnIndex = i
		}
		if stringListContains(rule["rule_set"], "geosite-category-ads-all") && rule["action"] == "reject" {
			ads = true
			adsIndex = i
		}
	}
	if !cnBypass || !ads {
		t.Fatalf("missing default CN bypass or ads reject: %+v", rules)
	}
	if adsIndex >= cnIndex {
		t.Fatal("advertising reject must precede CN kernel bypass")
	}
	for _, name := range []string{"AI", "视频", "Google", "社交", "开发"} {
		if _, ok := positions[name]; !ok {
			t.Fatalf("missing category proxy binding %q", name)
		}
	}
	if positions["AI"] >= positions["Google"] || positions["视频"] >= positions["Google"] {
		t.Fatalf("specialized rules must precede Google: %+v", positions)
	}
	dns := mustGenerateDNS(t, generator, "proxy")
	dnsRules := dns["rules"].([]map[string]interface{})
	cnDNSIndex, adsDNSIndex := -1, -1
	for i, rule := range dnsRules {
		if stringListContains(rule["rule_set"], "geosite-cn") && rule["server"] == "dns_local" {
			cnDNSIndex = i
		}
		if stringListContains(rule["rule_set"], "geosite-category-ads-all") && rule["action"] == "reject" {
			adsDNSIndex = i
		}
	}
	if cnDNSIndex < 0 || adsDNSIndex < 0 || adsDNSIndex >= cnDNSIndex {
		t.Fatalf("DNS ads reject must precede CN real-address resolution: %+v", dnsRules)
	}
	storedRules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range storedRules {
		if rule.SystemKey == SystemRuleAdBlockKey {
			if _, err := db.UpdateRouteRule(rule.ID, &model.RouteRuleRequest{Name: rule.Name, Enabled: false, Priority: rule.Priority, RuleType: rule.RuleType, Values: rule.Values, Outbound: rule.Outbound}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, rule := range mustGenerateDNS(t, generator, "proxy")["rules"].([]map[string]interface{}) {
		if stringListContains(rule["rule_set"], "geosite-category-ads-all") && rule["action"] == "reject" {
			t.Fatalf("disabling ads retained DNS blocking: %+v", rule)
		}
	}
}
