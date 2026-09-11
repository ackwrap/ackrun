package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSimpleSetupPreservesProfessionalSettings(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateRouteRule(&model.RouteRuleRequest{Name: "custom", Enabled: true, RuleType: "domain", Values: []string{"example.com"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	available, err := s.SimpleSetupAvailable()
	if err != nil || available {
		t.Fatalf("professional configuration available: %v, %v", available, err)
	}
	if err := s.PrepareSimpleSetup(); err == nil {
		t.Fatal("must refuse to overwrite professional configuration")
	}
	state, err := s.SimpleSetupState()
	if err != nil || state != "" {
		t.Fatalf("unexpected marker %q, %v", state, err)
	}
	groups, err := s.ListNodeGroups()
	if err != nil || len(groups) != 0 {
		t.Fatalf("partially seeded configuration: %v, %v", groups, err)
	}
}

func TestSimpleSetupAtomicAndIdempotent(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`CREATE TRIGGER reject_simple_dns BEFORE INSERT ON dns_servers BEGIN SELECT RAISE(ABORT, 'test failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareSimpleSetup(); err == nil {
		t.Fatal("expected injected database failure")
	}
	available, err := s.SimpleSetupAvailable()
	if err != nil || !available {
		t.Fatalf("failed preparation must roll back all changes: %v, %v", available, err)
	}
	if _, err := s.db.Exec(`DROP TRIGGER reject_simple_dns`); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	rules, err := s.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 10 || rules[0].SystemKey != systemRuleAdBlockKey || !rules[0].Enabled || rules[len(rules)-1].SystemKey != systemRuleGlobalDirectKey || rules[len(rules)-1].Outbound != "proxy" {
		t.Fatalf("unexpected preset rules: %+v", rules)
	}
	for _, rule := range rules {
		if rule.Name == "CN" && (rule.Outbound != "bypass" || rule.RuleType != "mixed") {
			t.Fatalf("CN must bypass in kernel: %+v", rule)
		}
	}
	if _, err := s.db.Exec(`UPDATE route_rules SET enabled = 0 WHERE system_key = ?`, systemRuleAdBlockKey); err != nil {
		t.Fatal(err)
	}
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	rules, err = s.ListRouteRules()
	if err != nil || len(rules) != 10 || rules[0].Enabled {
		t.Fatalf("retry overwrote edits or duplicated preset: %+v, %v", rules, err)
	}
	state, err := s.SimpleSetupState()
	if err != nil || state != "prepared" {
		t.Fatalf("unexpected marker %q, %v", state, err)
	}
	request, err := s.GetConfigGenerateRequest()
	if err != nil || request == nil || request.DefaultOutbound != "默认代理" || s.GetInboundMode() != "tun" || s.GetProxyMode() != "rule" {
		t.Fatalf("unexpected generator defaults: %+v, %v", request, err)
	}
	var server, firstProxy string
	if err := s.db.QueryRow(`SELECT server FROM dns_rules WHERE conditions_json = '{"geosite":["cn"]}'`).Scan(&server); err != nil || server != "dns_local" {
		t.Fatalf("CN must resolve real addresses locally: %q, %v", server, err)
	}
	if err := s.db.QueryRow(`SELECT name FROM proxy_collections WHERE name <> '全球直连' ORDER BY priority ASC, id DESC LIMIT 1`).Scan(&firstProxy); err != nil || firstProxy != "默认代理" {
		t.Fatalf("global proxy must default to common selector: %q, %v", firstProxy, err)
	}
	for _, next := range []string{"applied", "configured"} {
		if err := s.SetSimpleSetupState(next); err != nil {
			t.Fatal(err)
		}
		state, err := s.SimpleSetupState()
		if err != nil || state != next {
			t.Fatalf("unexpected marker %q, %v", state, err)
		}
	}
}

func TestSimpleSetupSubscriptionOwnershipSurvivesNameCollisions(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.PrepareSimpleSetup(); err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSubscription(&model.SubscriptionRequest{Name: "简易订阅", URL: "https://example.com/professional"})
	if err != nil {
		t.Fatal(err)
	}
	request := &model.SubscriptionRequest{Name: "简易订阅", URL: "https://example.com/simple", SyncMode: "daily", SyncTime: "04:00:00", SyncTimeoutSecs: 60}
	owned, err := s.SaveSimpleSetupSubscription(request)
	if err != nil || owned == nil || owned.ID == other.ID {
		t.Fatalf("did not create independently owned subscription: %+v, %v", owned, err)
	}
	request.URL = "https://example.com/retry"
	retry, err := s.SaveSimpleSetupSubscription(request)
	if err != nil || retry == nil || retry.ID != owned.ID {
		t.Fatalf("retry duplicated owned subscription: %+v, %v", retry, err)
	}
	saved, err := s.SimpleSetupSubscription()
	if err != nil || saved == nil || saved.ID != owned.ID || saved.URL != request.URL {
		t.Fatalf("ownership lookup failed: %+v, %v", saved, err)
	}
	if err := s.DeleteSubscription(owned.ID); err != nil {
		t.Fatal(err)
	}
	replacement, err := s.SaveSimpleSetupSubscription(request)
	if err != nil || replacement == nil || replacement.ID == owned.ID || replacement.ID == other.ID {
		t.Fatalf("deleted owned subscription was not recreated: %+v, %v", replacement, err)
	}
	preserved, err := s.GetSubscription(other.ID)
	if err != nil || preserved == nil || preserved.URL != "https://example.com/professional" {
		t.Fatalf("name collision overwrote professional subscription: %+v, %v", preserved, err)
	}
}
