package service

import (
	"bytes"
	"compress/zlib"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

func newTestRouteRuleService(t *testing.T, db *store.Store) *RouteRuleService {
	t.Helper()
	dir := t.TempDir()
	p := &paths.Paths{DataDir: dir, RulesDir: filepath.Join(dir, "rules"), GeoDir: filepath.Join(dir, "geo")}
	svc := NewRouteRuleService(db, p, nil)
	svc.ruleSetValidator = func(_ context.Context, filePath string) error {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		return validateGeneratedGeoRuleSet(data)
	}
	return svc
}

func testBinaryRuleSet(t *testing.T, version byte) []byte {
	return testCompressedRuleSet(t, version, []byte{0})
}

func testCompressedRuleSet(t *testing.T, version byte, payload []byte) []byte {
	t.Helper()
	var output bytes.Buffer
	_, _ = output.Write(generatedGeoRuleSetMagic[:])
	_ = output.WriteByte(version)
	writer := zlib.NewWriter(&output)
	if _, err := writer.Write(payload); err != nil {
		t.Fatalf("write test rule set: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close test rule set: %v", err)
	}
	return output.Bytes()
}

func TestValidateGeneratedGeoRuleSetRejectsExpandedPayload(t *testing.T) {
	data := testCompressedRuleSet(t, 1, bytes.Repeat([]byte{0}, 65))
	if err := validateGeneratedGeoRuleSetSize(data, 64); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected expanded payload error, got %v", err)
	}
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
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Blocked", Enabled: true, RuleType: "domain", Values: []string{"blocked.example"}, Outbound: "block"}); err != nil {
		t.Fatalf("create block rule: %v", err)
	}
	preview, err := svc.Preview()
	if err != nil {
		t.Fatalf("preview rules: %v", err)
	}
	stripSystemAdBlockPreview(preview)
	if len(preview.Rules) != 2 || preview.Rules[0]["action"] != "route" || preview.Rules[0]["outbound"] != "proxy" {
		t.Fatalf("unexpected preview: %+v", preview)
	}
	if preview.Rules[1]["action"] != "reject" {
		t.Fatalf("block preview must use reject action: %+v", preview.Rules[1])
	}
	if _, exists := preview.Rules[1]["outbound"]; exists {
		t.Fatalf("block preview must not emit outbound: %+v", preview.Rules[1])
	}
}

func TestProcessNameRouteRuleGeneration(t *testing.T) {
	req := &model.RouteRuleRequest{
		Name:     "Browser Proxy",
		Enabled:  true,
		RuleType: "process_name",
		Values:   []string{"chrome.exe", "curl"},
		Outbound: "proxy",
	}
	if err := validateRouteRule(req); err != nil {
		t.Fatalf("validate process_name rule: %v", err)
	}
	rule := singboxRouteRule(req.RuleType, req.Values, req.Outbound, false)
	names, ok := rule["process_name"].([]string)
	if !ok || len(names) != 2 || names[0] != "chrome.exe" || names[1] != "curl" {
		t.Fatalf("unexpected process_name rule: %+v", rule)
	}
	if rule["action"] != "route" || rule["outbound"] != "proxy" {
		t.Fatalf("unexpected process_name action: %+v", rule)
	}

	mixed, err := mixedSingboxRouteRules([]string{"process_name:chrome.exe", "domain_suffix:example.com"}, "direct", false)
	if err != nil {
		t.Fatalf("generate mixed process_name rule: %v", err)
	}
	if len(mixed) != 2 || mixed[0]["process_name"] == nil {
		t.Fatalf("unexpected mixed process_name rules: %+v", mixed)
	}
}

func TestMixedRouteRuleGroupsMultipleConditionTypes(t *testing.T) {
	rules, err := mixedSingboxRouteRules([]string{
		"domain_suffix:example.com",
		"domain_suffix:example.org",
		"geosite:google",
		"geoip:cn",
		"rule_set:custom-sites",
	}, "proxy", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("mixed rules = %+v, want domain_suffix and rule_set groups", rules)
	}
	domains, _ := rules[0]["domain_suffix"].([]string)
	if len(domains) != 2 || domains[0] != "example.com" || domains[1] != "example.org" {
		t.Fatalf("mixed domain suffixes = %v", domains)
	}
	ruleSets, _ := rules[1]["rule_set"].([]string)
	if len(ruleSets) != 3 || ruleSets[0] != "geosite-google" || ruleSets[1] != "geoip-cn" || ruleSets[2] != "custom-sites" {
		t.Fatalf("mixed rule sets = %v", ruleSets)
	}
	for _, rule := range rules {
		if rule["action"] != "route" || rule["outbound"] != "proxy" {
			t.Fatalf("mixed rule action = %+v", rule)
		}
	}

	blocked, err := mixedSingboxRouteRules([]string{
		"domain_suffix:example.com",
		"geoip:cn",
	}, "block", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range blocked {
		if rule["action"] != "reject" || rule["outbound"] != nil || rule["invert"] != true {
			t.Fatalf("mixed block rule action = %+v", rule)
		}
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

	rules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	systemRule := &rules[0]

	if _, err := svc.Update(systemRule.ID, &model.RouteRuleRequest{Name: "Changed", Enabled: false, Priority: 99, RuleType: "domain_suffix", Values: []string{"example.com"}, Outbound: "direct", Invert: true}); !errors.Is(err, ErrSystemRouteRuleProtected) {
		t.Fatalf("expected system update protection, got %v", err)
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
	stripSystemAdBlockPreview(preview)
	if len(preview.RuleSets) != 1 || preview.RuleSets[0]["tag"] != created.Tag {
		t.Fatalf("unexpected rule set preview: %+v", preview)
	}
	if _, exists := preview.RuleSets[0]["download_detour"]; exists {
		t.Fatalf("rule set preview contains deprecated download_detour: %+v", preview.RuleSets[0])
	}
	if len(preview.Rules) != 1 || preview.Rules[0]["outbound"] != "CN Direct" {
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
	stripSystemAdBlockPreview(preview)
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
	created, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{Name: "Other AI", Enabled: true, Tag: "other-ai", URL: server.URL + "/OtherAI.yml", Format: "clash", SyncMode: "off"})
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

func TestGeneratedGeoRuleSetContentCachesDownload(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)

	for i := 0; i < 2; i++ {
		data, contentType, err := svc.generatedGeoRuleSetContent("geosite-test", server.URL)
		if err != nil {
			t.Fatalf("generated geo content: %v", err)
		}
		if !bytes.Equal(data, payload) || contentType != "application/octet-stream" {
			t.Fatalf("unexpected content: type=%s data=%q", contentType, data)
		}
	}
	if requests != 1 {
		t.Fatalf("upstream requests = %d, want 1", requests)
	}
	cachePath := filepath.Join(svc.paths.RulesDir, "geo", "geosite-test.srs")
	if data, err := os.ReadFile(cachePath); err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("unexpected cache: data=%q err=%v", data, err)
	}
}

func TestGeneratedGeoRuleSetContentRefreshesExpiredCache(t *testing.T) {
	payloads := [][]byte{testBinaryRuleSet(t, 1), testBinaryRuleSet(t, 2)}
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write(payloads[requests-1])
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, _, err := svc.generatedGeoRuleSetContent("geosite-test", server.URL); err != nil {
		t.Fatalf("initial generated geo content: %v", err)
	}
	cachePath := filepath.Join(svc.paths.RulesDir, "geo", "geosite-test.srs")
	expiredAt := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
	if err := os.Chtimes(cachePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("expire generated geo cache: %v", err)
	}

	data, _, err := svc.generatedGeoRuleSetContent("geosite-test", server.URL)
	if err != nil {
		t.Fatalf("refresh generated geo content: %v", err)
	}
	if !bytes.Equal(data, payloads[1]) || requests != 2 {
		t.Fatalf("unexpected refreshed content: data=%q requests=%d", data, requests)
	}
}

func TestGeneratedGeoRuleSetContentKeepsExpiredCacheOnRefreshFailure(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, _, err := svc.generatedGeoRuleSetContent("geoip-test", server.URL); err != nil {
		t.Fatalf("initial generated geo content: %v", err)
	}
	cachePath := filepath.Join(svc.paths.RulesDir, "geo", "geoip-test.srs")
	expiredAt := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
	if err := os.Chtimes(cachePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("expire generated geo cache: %v", err)
	}
	fail = true

	data, _, err := svc.generatedGeoRuleSetContent("geoip-test", server.URL)
	if err != nil {
		t.Fatalf("serve stale generated geo content: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("unexpected stale content: %q", data)
	}
}

func TestGeneratedGeoRuleSetContentRejectsInvalidSuccessfulRefresh(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	invalid := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if invalid {
			_, _ = w.Write([]byte("<html>mirror error</html>"))
			return
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, _, err := svc.generatedGeoRuleSetContent("geoip-test", server.URL); err != nil {
		t.Fatalf("initial generated geo content: %v", err)
	}
	cachePath := filepath.Join(svc.paths.RulesDir, "geo", "geoip-test.srs")
	expiredAt := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
	if err := os.Chtimes(cachePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("expire generated geo cache: %v", err)
	}
	invalid = true

	data, _, err := svc.generatedGeoRuleSetContent("geoip-test", server.URL)
	if err != nil {
		t.Fatalf("serve cache after invalid refresh: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("invalid refresh replaced cached content: %q", data)
	}
	cached, err := os.ReadFile(cachePath)
	if err != nil || !bytes.Equal(cached, payload) {
		t.Fatalf("unexpected cache after invalid refresh: data=%q err=%v", cached, err)
	}
}

func TestGeneratedGeoRuleSetContentKeepsCacheWhenCoreRejectsRefresh(t *testing.T) {
	oldPayload := testBinaryRuleSet(t, 1)
	newPayload := testBinaryRuleSet(t, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(newPayload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "geoip-test.srs")
	if err := os.WriteFile(cachePath, oldPayload, 0644); err != nil {
		t.Fatalf("write old cache: %v", err)
	}
	expiredAt := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
	if err := os.Chtimes(cachePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("expire old cache: %v", err)
	}
	svc.ruleSetValidator = func(_ context.Context, filePath string) error {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		if bytes.Equal(data, oldPayload) {
			return nil
		}
		return errors.New("invalid internal rule encoding")
	}

	data, _, err := svc.generatedGeoRuleSetContent("geoip-test", server.URL)
	if err != nil {
		t.Fatalf("serve cache after core validation failure: %v", err)
	}
	if !bytes.Equal(data, oldPayload) {
		t.Fatalf("core-rejected refresh replaced cached content: %q", data)
	}
	cached, err := os.ReadFile(cachePath)
	if err != nil || !bytes.Equal(cached, oldPayload) {
		t.Fatalf("unexpected cache after core validation failure: data=%q err=%v", cached, err)
	}
}

func TestGeneratedGeoRuleSetContentReplacesInvalidCache(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	cachePath := filepath.Join(cacheDir, "geosite-test.srs")
	if err := os.WriteFile(cachePath, []byte("<html>old mirror error</html>"), 0644); err != nil {
		t.Fatalf("write invalid cache: %v", err)
	}

	data, _, err := svc.generatedGeoRuleSetContent("geosite-test", server.URL)
	if err != nil {
		t.Fatalf("replace invalid cache: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("unexpected replacement content: %q", data)
	}
	cached, err := os.ReadFile(cachePath)
	if err != nil || !bytes.Equal(cached, payload) {
		t.Fatalf("unexpected replaced cache: data=%q err=%v", cached, err)
	}
}

func TestGeneratedGeoRuleSetRefreshDoesNotBlockOtherTags(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	refreshedPayload := testBinaryRuleSet(t, 2)
	started := make(chan struct{})
	release := make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write(refreshedPayload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("create cache dir: %v", err)
	}
	stalePath := filepath.Join(cacheDir, "geoip-stale.srs")
	freshPath := filepath.Join(cacheDir, "geosite-fresh.srs")
	if err := os.WriteFile(stalePath, payload, 0644); err != nil {
		t.Fatalf("write stale cache: %v", err)
	}
	if err := os.WriteFile(freshPath, payload, 0644); err != nil {
		t.Fatalf("write fresh cache: %v", err)
	}
	expiredAt := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
	if err := os.Chtimes(stalePath, expiredAt, expiredAt); err != nil {
		t.Fatalf("expire stale cache: %v", err)
	}

	refreshDone := make(chan error, 1)
	go func() {
		_, _, err := svc.generatedGeoRuleSetContent("geoip-stale", server.URL)
		refreshDone <- err
	}()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stale cache refresh did not start")
	}

	start := time.Now()
	data, _, err := svc.generatedGeoRuleSetContent("geosite-fresh", server.URL)
	if err != nil {
		t.Fatalf("read unrelated fresh cache: %v", err)
	}
	if !bytes.Equal(data, payload) {
		t.Fatalf("unexpected unrelated cache: %q", data)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("unrelated cache blocked for %s", elapsed)
	}
	close(release)
	if err := <-refreshDone; err != nil {
		t.Fatalf("refresh stale cache: %v", err)
	}
}

func TestRouteRuleSubscriptionContentRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(bytes.Repeat([]byte{'x'}, int(routeRuleSubscriptionContentMaxSize)+1))
	}))
	defer server.Close()
	client := &http.Client{Timeout: 5 * time.Second}
	if _, err := fetchRouteRuleSubscriptionContentWithClient(client, server.URL); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized response error, got %v", err)
	}
}

func TestBuildGitHubDownloadAttemptsUsesAllAcceleratorsAndOfficialFallback(t *testing.T) {
	upstream := "https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/geoip-cn.srs"
	attempts := buildGitHubDownloadAttempts(&model.UpdateSettingsResponse{Acceleration: "ghproxy_vip"}, upstream)
	wantNames := []string{
		"ghproxy_vip",
		"ghproxy",
		"jsdelivr_fastly",
		"jsdelivr_testingcf",
		"jsdelivr_cdn",
		"official_direct",
	}
	if len(attempts) != len(wantNames) {
		t.Fatalf("attempt count = %d, want %d: %+v", len(attempts), len(wantNames), attempts)
	}
	for index, name := range wantNames {
		if attempts[index].name != name {
			t.Fatalf("attempt %d name = %q, want %q", index, attempts[index].name, name)
		}
		if attempts[index].client.Timeout != generatedGeoRuleSetAttemptTimeout {
			t.Fatalf("attempt %d timeout = %s, want %s", index, attempts[index].client.Timeout, generatedGeoRuleSetAttemptTimeout)
		}
	}
	if attempts[len(attempts)-1].url != upstream {
		t.Fatalf("official fallback url = %q, want %q", attempts[len(attempts)-1].url, upstream)
	}
}

func TestBuildGitHubDownloadAttemptsDoesNotRewriteNonGitHubURL(t *testing.T) {
	upstream := "http://127.0.0.1:18080/rules.srs"
	attempts := buildGitHubDownloadAttempts(&model.UpdateSettingsResponse{Acceleration: "ghproxy"}, upstream)
	if len(attempts) != 1 || attempts[0].name != "official_direct" || attempts[0].url != upstream {
		t.Fatalf("non-GitHub attempts = %+v, want direct only", attempts)
	}
}

func TestGeneratedGeoRuleSetContentUsesConfiguredMirror(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-test.srs" {
			t.Errorf("unexpected mirror path: %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: server.URL}); err != nil {
		t.Fatalf("set update settings: %v", err)
	}
	svc := newTestRouteRuleService(t, db)
	data, _, err := svc.GeneratedGeoRuleSetContent("geosite-test")
	if err != nil {
		t.Fatalf("generated geo content: %v", err)
	}
	if !bytes.Equal(data, payload) || requests != 1 {
		t.Fatalf("unexpected mirror result: data=%q requests=%d", data, requests)
	}
}

func TestGeoAssetSyncUsesConfiguredAcceleration(t *testing.T) {
	payload := []byte("geo-database")
	requests := 0
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/https://github.com/SagerNet/sing-geoip/releases/latest/download/geoip.db" {
			t.Errorf("unexpected mirror path: %s", r.URL.Path)
		}
		_, _ = w.Write(payload)
	}))
	defer mirror.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatalf("set update settings: %v", err)
	}
	assets, err := db.ListGeoAssets()
	if err != nil || len(assets) == 0 {
		t.Fatalf("list geo assets: %v", err)
	}
	svc := newTestRouteRuleService(t, db)
	svc.runGeoAssetSync(assets[0].ID)

	updated, err := db.GetGeoAsset(assets[0].ID)
	if err != nil {
		t.Fatalf("get geo asset: %v", err)
	}
	data, readErr := os.ReadFile(updated.LocalPath)
	if readErr != nil {
		t.Fatalf("read geo database: %v", readErr)
	}
	if updated.SyncStatus != "updated" || requests != 1 || !bytes.Equal(data, payload) {
		t.Fatalf("unexpected geo sync result: status=%s requests=%d data=%q", updated.SyncStatus, requests, data)
	}
}

func TestGeneratedGeoRuleSetContentFallsBackToOfficialURL(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	mirrorRequests := 0
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mirrorRequests++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer mirror.Close()
	officialRequests := 0
	official := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		officialRequests++
		_, _ = w.Write(payload)
	}))
	defer official.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatalf("set update settings: %v", err)
	}
	svc := newTestRouteRuleService(t, db)
	data, _, err := svc.generatedGeoRuleSetContent("geosite-test", official.URL)
	if err != nil {
		t.Fatalf("generated geo content: %v", err)
	}
	if !bytes.Equal(data, payload) || mirrorRequests != 1 || officialRequests != 1 {
		t.Fatalf("unexpected fallback result: data=%q mirror=%d official=%d", data, mirrorRequests, officialRequests)
	}
}

func TestGeneratedGeoRuleSetContentRejectsInvalidTag(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, _, err := svc.generatedGeoRuleSetContent("../geoip-cn", "https://example.com"); err == nil {
		t.Fatal("expected invalid tag error")
	}
}

func TestGeneratedGeoRuleSetContentAllowsLogicalTags(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	for _, tag := range []string{"geosite-geolocation-!cn", "geosite-apple@cn", "geosite-category-ai-!cn"} {
		t.Run(tag, func(t *testing.T) {
			data, _, err := svc.generatedGeoRuleSetContent(tag, upstream.URL)
			if err != nil {
				t.Fatalf("generated geo content: %v", err)
			}
			if !bytes.Equal(data, payload) {
				t.Fatal("unexpected generated geo rule set content")
			}
		})
	}
}

func TestSyncRouteRuleSubscriptionClaimsStateBeforeReturning(t *testing.T) {
	payload := testBinaryRuleSet(t, 1)
	release := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	item, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{
		Name: "Blocking", Enabled: true, Tag: "blocking", URL: upstream.URL, Format: "binary",
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	svc := newTestRouteRuleService(t, db)
	if _, err := svc.SyncSubscription(item.ID); err != nil {
		t.Fatalf("start sync: %v", err)
	}
	updated, err := db.GetRouteRuleSubscription(item.ID)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	if updated.SyncStatus != "syncing" || updated.SyncProgress != 30 {
		t.Fatalf("sync state = %s %.0f, want syncing 30", updated.SyncStatus, updated.SyncProgress)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		updated, err = db.GetRouteRuleSubscription(item.ID)
		if err == nil && updated.SyncStatus == "updated" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("sync did not complete after release: status=%s error=%v", updated.SyncStatus, err)
}

func TestRouteRulePreviewSupportsGeolocationNotCN(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, err := svc.Create(&model.RouteRuleRequest{
		Name: "Non-CN Proxy", Enabled: true, RuleType: "geosite", Values: []string{"geolocation-!cn"}, Outbound: "proxy",
	}); err != nil {
		t.Fatalf("create geosite rule: %v", err)
	}

	preview, err := svc.PreviewWithBaseURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	stripSystemAdBlockPreview(preview)
	const tag = "geosite-geolocation-!cn"
	foundRuleSet := false
	for _, ruleSet := range preview.RuleSets {
		if ruleSet["tag"] == tag {
			foundRuleSet = ruleSet["url"] == "http://127.0.0.1:8080/api/v1/rules/geo/rule-sets/geosite-geolocation-!cn/content"
		}
	}
	if !foundRuleSet {
		t.Fatalf("missing generated geosite rule set: %+v", preview.RuleSets)
	}
	foundRule := false
	for _, rule := range preview.Rules {
		values, _ := rule["rule_set"].([]string)
		for _, value := range values {
			if value == tag {
				foundRule = true
			}
		}
	}
	if !foundRule {
		t.Fatalf("missing generated geosite route rule: %+v", preview.Rules)
	}
}

func TestPreviewUsesGeneratedGeoRuleSetCacheEndpoint(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "CN Direct", Enabled: true, RuleType: "geoip", Values: []string{"cn"}, Outbound: "direct"}); err != nil {
		t.Fatalf("create geo rule: %v", err)
	}
	preview, err := svc.PreviewWithBaseURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	stripSystemAdBlockPreview(preview)
	if len(preview.RuleSets) != 1 || preview.RuleSets[0]["url"] != "http://127.0.0.1:8080/api/v1/rules/geo/rule-sets/geoip-cn/content" || preview.RuleSets[0]["update_interval"] != "24h" {
		t.Fatalf("unexpected generated geo rule set: %+v", preview.RuleSets)
	}
}

func TestInternalRuleSetContentURLsIncludeEscapedAccessToken(t *testing.T) {
	const token = "token with+symbols"
	if got := routeRuleSubscriptionContentURL("http://127.0.0.1:8080", 12, token); got != "http://127.0.0.1:8080/api/v1/rules/subscriptions/12/content?access_token=token+with%2Bsymbols" {
		t.Fatalf("subscription URL = %q", got)
	}
	if got := generatedGeoRuleSetContentURL("http://127.0.0.1:8080", "geosite-google", token); got != "http://127.0.0.1:8080/api/v1/rules/geo/rule-sets/geosite-google/content?access_token=token+with%2Bsymbols" {
		t.Fatalf("Geo URL = %q", got)
	}
}

func TestInternalAPIBaseURLUsesConfiguredListenPort(t *testing.T) {
	t.Setenv("ACKWRAP_LISTEN_ADDR", "0.0.0.0:9090")
	if got := internalAPIBaseURL(); got != "http://127.0.0.1:9090" {
		t.Fatalf("internal API base URL = %q", got)
	}
}

func stripSystemAdBlockPreview(preview *model.RouteRulePreviewResponse) {
	rules := preview.Rules[:0]
	for _, rule := range preview.Rules {
		if rule["action"] == "reject" && stringListContains(rule["rule_set"], "geosite-category-ads-all") {
			continue
		}
		rules = append(rules, rule)
	}
	preview.Rules = rules
	ruleSets := preview.RuleSets[:0]
	for _, ruleSet := range preview.RuleSets {
		if ruleSet["tag"] == "geosite-category-ads-all" {
			continue
		}
		ruleSets = append(ruleSets, ruleSet)
	}
	preview.RuleSets = ruleSets
}
