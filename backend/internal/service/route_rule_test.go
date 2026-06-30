package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

func newTestRouteRuleService(t *testing.T, db *store.Store) *RouteRuleService {
	t.Helper()
	dir := t.TempDir()
	p := &paths.Paths{DataDir: dir, RulesDir: filepath.Join(dir, "rules"), GeoDir: filepath.Join(dir, "geo")}
	return NewRouteRuleService(db, p, nil)
}

func TestRouteRuleServicePreview(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Proxy Domains", Enabled: true, RuleType: "domain_suffix", Values: []string{"example.com"}, Outbound: "proxy"}); err != nil {
		t.Fatalf("create enabled rule: %v", err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Disabled", Enabled: false, RuleType: "geosite", Values: []string{"cn"}, Outbound: "direct"}); err != nil {
		t.Fatalf("create disabled rule: %v", err)
	}
	preview, err := svc.Preview()
	if err != nil {
		t.Fatalf("preview rules: %v", err)
	}
	if len(preview.Rules) != 1 || preview.Rules[0]["outbound"] != "proxy" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
}

func TestRouteRuleServiceProtectsSystemRule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	if _, err := svc.Create(&model.RouteRuleRequest{Name: SystemAdBlockRouteRuleName, Enabled: true, RuleType: "geosite", Values: []string{"category-ads-all"}, Outbound: "block"}); !errors.Is(err, ErrSystemRouteRuleProtected) {
		t.Fatalf("expected create protection, got %v", err)
	}

	systemRule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: SystemAdBlockRouteRuleName, Enabled: true, Priority: 1, RuleType: "geosite", Values: []string{"category-ads-all"}, Outbound: "block", SystemKey: SystemRuleAdBlockKey})
	if err != nil {
		t.Fatalf("seed system rule: %v", err)
	}

	updated, err := svc.Update(systemRule.ID, &model.RouteRuleRequest{Name: "Changed", Enabled: false, Priority: 99, RuleType: "domain_suffix", Values: []string{"example.com"}, Outbound: "direct", Invert: true})
	if err != nil {
		t.Fatalf("update system rule enabled: %v", err)
	}
	if updated.Enabled || updated.Name != SystemAdBlockRouteRuleName || updated.RuleType != "geosite" || updated.Outbound != "block" || updated.Invert {
		t.Fatalf("system rule fields should be preserved except enabled: %+v", updated)
	}
	if len(updated.Values) != 1 || updated.Values[0] != "category-ads-all" || updated.SystemKey != SystemRuleAdBlockKey || !updated.IsSystem {
		t.Fatalf("system rule metadata should be preserved: %+v", updated)
	}

	if _, err := svc.Delete(systemRule.ID); !errors.Is(err, ErrSystemRouteRuleProtected) {
		t.Fatalf("expected delete protection, got %v", err)
	}
}

func TestNormalizeGeoLookupTargetURL(t *testing.T) {
	cases := map[string]string{
		"https://gemini.google.com/app?hl=en_GB": "gemini.google.com",
		"http://www.youtube.com/watch?v=abc":     "www.youtube.com",
		"chatgpt.com:443":                        "chatgpt.com",
		"[2606:4700:4700::1111]":                 "2606:4700:4700::1111",
	}
	for input, expected := range cases {
		if got := normalizeGeoLookupTarget(input); got != expected {
			t.Fatalf("normalize %q = %q, want %q", input, got, expected)
		}
	}
}

func TestIsQualifiedDomain(t *testing.T) {
	if isQualifiedDomain("serp") {
		t.Fatal("single-label input should not be treated as qualified domain")
	}
	if !isQualifiedDomain("google.com") {
		t.Fatal("google.com should be treated as qualified domain")
	}
}

func TestRouteRuleServicePreviewRuleSubscriptions(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	created, err := svc.CreateSubscription(&model.RouteRuleSubscriptionRequest{Name: "GeoSite CN", Enabled: true, URL: "https://example.com/geosite-cn.srs", Format: "binary", UseProxy: true})
	if err != nil {
		t.Fatalf("create rule subscription: %v", err)
	}
	if created.Tag == "" {
		t.Fatalf("expected generated tag: %+v", created)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "CN Direct", Enabled: true, RuleType: "rule_set", Values: []string{created.Tag}, Outbound: "direct"}); err != nil {
		t.Fatalf("create rule_set route rule: %v", err)
	}

	preview, err := svc.Preview()
	if err != nil {
		t.Fatalf("preview rules: %v", err)
	}
	if len(preview.RuleSets) != 1 || preview.RuleSets[0]["download_detour"] != "direct" || preview.RuleSets[0]["tag"] != created.Tag {
		t.Fatalf("unexpected rule set preview: %+v", preview)
	}
	if len(preview.Rules) != 1 || preview.Rules[0]["outbound"] != "direct" {
		t.Fatalf("unexpected route rule preview: %+v", preview)
	}
}

func TestRouteRuleSubscriptionValidation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	if _, err := svc.CreateSubscription(&model.RouteRuleSubscriptionRequest{Name: "Bad", Enabled: true, URL: "ftp://example.com/rules.srs"}); err == nil {
		t.Fatalf("expected invalid url error")
	}
	if _, err := svc.CreateSubscription(&model.RouteRuleSubscriptionRequest{Name: "Bad", Enabled: true, Tag: "bad tag", URL: "https://example.com/rules.srs"}); err == nil {
		t.Fatalf("expected invalid tag error")
	}
}

func TestRouteRuleSubscriptionAutoDetectsClashYAML(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	created, err := svc.CreateSubscription(&model.RouteRuleSubscriptionRequest{Name: "Other AI", Enabled: true, URL: "https://gh-proxy.com/https://raw.githubusercontent.com/g2x-cmd/rules/refs/heads/main/Providers/OtherAI.yml", Format: "auto"})
	if err != nil {
		t.Fatalf("create rule subscription: %v", err)
	}
	if created.Format != "clash" {
		t.Fatalf("expected clash format, got %+v", created)
	}

	preview, err := svc.PreviewWithBaseURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("preview rules: %v", err)
	}
	if len(preview.RuleSets) != 1 {
		t.Fatalf("expected one rule set: %+v", preview)
	}
	ruleSet := preview.RuleSets[0]
	expectedURL := "http://127.0.0.1:8080/api/v1/rules/subscriptions/" + fmt.Sprint(created.ID) + "/content"
	if ruleSet["format"] != "source" || ruleSet["url"] != expectedURL {
		t.Fatalf("unexpected converted rule set preview: %+v", ruleSet)
	}
}

func TestRouteRuleSubscriptionContentConvertsClashYAML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(`payload:
  - DOMAIN-SUFFIX,cerebras.ai
  - PROCESS-NAME,LM Studio
`))
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := newTestRouteRuleService(t, db)
	created, err := svc.CreateSubscription(&model.RouteRuleSubscriptionRequest{Name: "Other AI", Enabled: true, URL: server.URL + "/OtherAI.yml", Format: "clash"})
	if err != nil {
		t.Fatalf("create rule subscription: %v", err)
	}
	body, contentType, err := svc.SubscriptionContent(created.ID)
	if err != nil {
		t.Fatalf("subscription content: %v", err)
	}
	if !strings.Contains(contentType, "application/json") || !json.Valid(body) {
		t.Fatalf("expected json content, got type=%s body=%s", contentType, string(body))
	}
	if !strings.Contains(string(body), "cerebras.ai") || !strings.Contains(string(body), "LM Studio") {
		t.Fatalf("converted content missing expected values: %s", string(body))
	}
}
