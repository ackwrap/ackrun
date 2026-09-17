package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func geoFallbackTestMirror(t *testing.T, db *store.Store) []byte {
	t.Helper()
	var site, ip []byte
	for _, code := range []string{"CN", "GOOGLE", "CATEGORY-ADS-ALL", "SAGER-ONLY"} {
		entry := geoTestBytes(nil, 1, []byte(code))
		entry = geoTestBytes(entry, 2, geoTestDomain(3, "fallback-only.example", ""))
		site = geoTestBytes(site, 1, entry)
	}
	for _, code := range []string{"CN", "PRIVATE", "TELEGRAM", "SAGER-ONLY"} {
		entry := geoTestBytes(nil, 1, []byte(code))
		cidr := geoTestBytes(nil, 1, []byte{198, 51, 100, 0})
		cidr = protowire.AppendVarint(protowire.AppendTag(cidr, 2, protowire.VarintType), 24)
		entry = geoTestBytes(entry, 2, cidr)
		ip = geoTestBytes(ip, 1, entry)
	}
	srs := testBinaryRuleSet(t, 1)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + model.GeoSourceAssetURL(model.GeoSourceLoyalsoldier, "geosite"):
			_, _ = w.Write(geoTestSiteDAT())
		case "/" + model.GeoSourceAssetURL(model.GeoSourceLoyalsoldier, "geoip"):
			_, _ = w.Write(geoTestIPDAT())
		case "/" + model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geosite"):
			_, _ = w.Write(site)
		case "/" + model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geoip"):
			_, _ = w.Write(ip)
		case "/" + generatedGeoRuleSetURL("geosite-sager-only"),
			"/" + generatedGeoRuleSetURL("geoip-sager-only"),
			"/" + generatedGeoRuleSetURL("geosite-cn"):
			_, _ = w.Write(srs)
		default:
			t.Errorf("unexpected Geo download: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
	return srs
}

func selectGeoFallbackTestSource(t *testing.T, svc *RouteRuleService, db *store.Store) {
	t.Helper()
	settings := NewSettingsService(db)
	settings.SetGeoSourcePreparer(svc)
	source := model.GeoSourceLoyalsoldier
	if err := settings.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source}); err != nil {
		t.Fatal(err)
	}
}

func TestGeoFallbackContentAndCategoryOwnership(t *testing.T) {
	svc, db := geoSourceTestService(t)
	srs := geoFallbackTestMirror(t, db)
	settings, err := db.GetGeneralSettings()
	if err != nil || settings.GeoSource != model.GeoSourceSagerNet {
		t.Fatalf("default source changed: %+v %v", settings, err)
	}
	legacy, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-cn", "", "")
	if err != nil || contentType != "application/octet-stream" || !bytes.Equal(legacy, srs) {
		t.Fatalf("default SagerNet endpoint changed: %q %v", contentType, err)
	}
	selectGeoFallbackTestSource(t, svc, db)
	for _, kind := range []string{"geosite", "geoip"} {
		t.Run(kind, func(t *testing.T) {
			tag := kind + "-sager-only"
			tags, err := svc.GeoTags(kind, "sager-only", 100)
			if err != nil || !reflect.DeepEqual(tags.Tags, []string{"sager-only"}) {
				t.Fatalf("fallback category missing from selector: %+v %v", tags, err)
			}
			data, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), tag, model.GeoSourceLoyalsoldier, "", "binary")
			if err != nil || contentType != "application/octet-stream" || !bytes.Equal(data, srs) {
				t.Fatalf("fallback did not serve native SagerNet SRS: %q %v", contentType, err)
			}
			data, contentType, err = svc.GeneratedGeoRuleSetContentForSource(context.Background(), tag, model.GeoSourceLoyalsoldier, "")
			want := "fallback-only.example"
			primary := "exact.example"
			if kind == "geoip" {
				want, primary = "198.51.100.0/24", "192.0.2.0/24"
			}
			if err != nil || contentType != "application/json; charset=utf-8" || !json.Valid(data) || !bytes.Contains(data, []byte(want)) {
				t.Fatalf("legacy fallback JSON unavailable: %s %q %v", data, contentType, err)
			}
			data, _, err = svc.GeneratedGeoRuleSetContentForSource(context.Background(), kind+"-cn", model.GeoSourceLoyalsoldier, "")
			if err != nil || !bytes.Contains(data, []byte(primary)) || bytes.Contains(data, []byte(want)) {
				t.Fatalf("same-name SagerNet content merged into primary category: %s %v", data, err)
			}
			missing := kind + "-both-missing"
			if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), missing, model.GeoSourceLoyalsoldier, "", "binary"); err == nil || !strings.Contains(err.Error(), missing) {
				t.Fatalf("missing-category error lost the requested tag: %v", err)
			}
		})
	}
	domains, err := svc.GeoDomains("sager-only", 100, 0)
	if err != nil || domains.Total != 1 || len(domains.Items) != 1 || domains.Items[0].Value != "fallback-only.example" || !strings.Contains(domains.Message, "SagerNet") {
		t.Fatalf("fallback domain browser lost content or source message: %+v %v", domains, err)
	}
	domains, err = svc.GeoDomains("cn", 100, 0)
	if err != nil || domains.Total != 4 || strings.Contains(domains.Message, "SagerNet") {
		t.Fatalf("same-name domain browser used fallback: %+v %v", domains, err)
	}
}

func TestGeoFallbackLookupOnlyAddsMissingCategories(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoFallbackTestMirror(t, db)
	selectGeoFallbackTestSource(t, svc, db)
	for _, test := range []struct {
		target   string
		wantIP   []string
		wantSite []string
		fallback bool
	}{
		{"198.51.100.1", []string{"198.51.100.1 => sager-only"}, []string{}, true},
		{"192.0.2.1", []string{"192.0.2.1 => cn, private, telegram"}, []string{}, false},
		{"fallback-only.example", []string{}, []string{"sager-only (domain=fallback-only.example)"}, true},
		{"exact.example", []string{}, []string{"category-ads-all (domain=exact.example)", "cn (domain=exact.example)", "google (domain=exact.example)"}, false},
	} {
		t.Run(test.target, func(t *testing.T) {
			// Invalid local port disables DNS resolution without contacting any server.
			result, err := svc.GeoLookup(test.target, "127.0.0.1:65536")
			if err != nil || !reflect.DeepEqual(result.GeoIPMatches, test.wantIP) || !reflect.DeepEqual(result.GeositeMatches, test.wantSite) {
				t.Fatalf("lookup ignored category ownership: %+v %v", result, err)
			}
			if strings.Contains(result.Message, "SagerNet") != test.fallback {
				t.Fatalf("lookup source message disagrees with matches: %+v", result)
			}
		})
	}
}

func TestGeoFallbackPreparationPreviewAndDNSValidation(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoFallbackTestMirror(t, db)
	if _, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{Name: "Own", Enabled: true, Tag: "geosite-custom", URL: "https://example.invalid/custom.srs", Format: "binary"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Own category", Enabled: true, RuleType: "geosite", Values: []string{"custom"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Fallback IP", Enabled: true, RuleType: "geoip", Values: []string{"sager-only"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, RuleType: "default", Conditions: map[string]any{"geosite": []string{"sager-only"}}, Server: "local"}); err != nil {
		t.Fatal(err)
	}
	// The source switch must validate existing route and DNS rules through fallback.
	selectGeoFallbackTestSource(t, svc, db)
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Fallback site", Enabled: true, RuleType: "geosite", Values: []string{"sager-only"}, Outbound: "direct"}); err != nil {
		t.Fatalf("route save rejected fallback: %v", err)
	}
	dns := NewDNSService(db, svc.paths)
	for _, tag := range []string{"sager-only", "both-missing"} {
		req := &model.DNSRuleRequest{Enabled: true, RuleType: "default", Conditions: map[string]any{"geosite": []string{tag}}, Server: "local"}
		err := dns.validateDNSRuleRequest(req)
		if tag == "sager-only" && err != nil {
			t.Fatalf("DNS rejected fallback: %v", err)
		}
		if tag == "both-missing" && (err == nil || !strings.Contains(err.Error(), "geosite-both-missing")) {
			t.Fatalf("DNS accepted missing category or hid its tag: %v", err)
		}
	}
	if err := db.SetDNSGlobalSettings(&model.DNSGlobalSettings{Enabled: true, Final: "local"}); err != nil {
		t.Fatal(err)
	}
	preview, err := svc.PreviewWithBaseURL("http://127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	route, err := NewConfigGeneratorService(db, svc.paths).generateRoute("")
	if err != nil {
		t.Fatal(err)
	}
	for name, sets := range map[string][]map[string]any{"preview": preview.RuleSets, "config": route["rule_set"].([]map[string]any)} {
		found := map[string]bool{}
		for _, set := range sets {
			tag := set["tag"].(string)
			found[tag] = true
			u, err := url.Parse(set["url"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if tag == "geosite-custom" {
				if set["format"] != "binary" || u.Query().Get("source") != "" || strings.Contains(u.Path, "/geo/rule-sets/") {
					t.Fatalf("%s rewrote independent subscription: %+v", name, set)
				}
				continue
			}
			if set["format"] != "binary" || u.Query().Get("source") != model.GeoSourceLoyalsoldier || u.Query().Get("format") != "binary" || !validGeoAssetVersion(u.Query().Get("version")) {
				t.Fatalf("%s lost fallback source policy: %+v", name, set)
			}
		}
		for _, tag := range []string{"geoip-sager-only", "geosite-sager-only", "geosite-custom"} {
			if !found[tag] {
				t.Fatalf("%s omitted %s", name, tag)
			}
		}
	}
}

func TestGeoFallbackDNSHonorsEnabledSubscription(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoFallbackTestMirror(t, db)
	selectGeoFallbackTestSource(t, svc, db)
	subscription := &model.RouteRuleSubscriptionRequest{Name: "Own", Enabled: true, Tag: "geosite-custom", URL: "https://example.invalid/custom.srs", Format: "binary"}
	saved, err := db.CreateRouteRuleSubscription(subscription)
	if err != nil {
		t.Fatal(err)
	}
	dns := NewDNSService(db, svc.paths)
	rule := &model.DNSRuleRequest{Enabled: true, RuleType: "default", Conditions: map[string]any{"geosite": []string{"custom"}}, Server: "local"}
	if err := dns.validateDNSRuleRequest(rule); err != nil {
		t.Fatalf("enabled independent subscription lost ownership of its DNS tag: %v", err)
	}
	subscription.Enabled = false
	if _, err := db.UpdateRouteRuleSubscription(saved.ID, subscription); err != nil {
		t.Fatal(err)
	}
	if err := dns.validateDNSRuleRequest(rule); err == nil || !strings.Contains(err.Error(), "geosite-custom") {
		t.Fatalf("disabled subscription still bypassed missing-category validation: %v", err)
	}
}

func TestGeoFallbackRefreshPreservesRequiredCategoryRepeatedly(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoFallbackTestMirror(t, db)
	selectGeoFallbackTestSource(t, svc, db)
	initial, err := svc.GeoDomains("sager-only", 100, 0)
	if err != nil || initial.Total != 1 {
		t.Fatalf("initial fallback unavailable: %+v %v", initial, err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Required fallback", Enabled: true, RuleType: "geosite", Values: []string{"sager-only"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	cachePath, err := svc.sagerNetFallbackDatabase("geosite", false)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(svc.paths.GeoDir, model.GeoSourceSagerNet, "fallback", "geosite.db"); cachePath != want {
		t.Fatalf("accepted fallback not isolated from snapshot pruning: %s", cachePath)
	}
	original, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatal(err)
	}
	expired := time.Now().Add(-2 * generatedGeoRuleSetUpdateInterval)
	if err := os.Chtimes(cachePath, expired, expired); err != nil {
		t.Fatal(err)
	}
	var attempts atomic.Int32
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/"+model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geosite") {
			t.Errorf("unexpected refresh download: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		// Distinct rejected snapshots exercise repeated refresh and pruning.
		code := "CHANGED-ONE"
		if attempts.Add(1) > 1 {
			code = "CHANGED-TWO"
		}
		entry := geoTestBytes(nil, 1, []byte(code))
		entry = geoTestBytes(entry, 2, geoTestDomain(3, "changed.example", ""))
		_, _ = w.Write(geoTestBytes(geoTestSiteDAT(), 1, entry))
	}))
	t.Cleanup(mirror.Close)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 2; attempt++ {
		result, err := svc.GeoDomains("sager-only", 100, 0)
		if err != nil || !reflect.DeepEqual(result.Items, initial.Items) || !strings.Contains(result.Message, "SagerNet") {
			t.Fatalf("refresh %d lost the required fallback: %+v %v", attempt, result, err)
		}
		retained, err := os.ReadFile(cachePath)
		if err != nil || !bytes.Equal(retained, original) {
			t.Fatalf("refresh %d replaced the accepted database: %v", attempt, err)
		}
	}
	if attempts.Load() != 2 {
		t.Fatalf("did not exercise both rejected refreshes: %d", attempts.Load())
	}
}

func TestGeoFallbackTagsKeepPrimaryWhenFallbackInvalid(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoFallbackTestMirror(t, db)
	selectGeoFallbackTestSource(t, svc, db)
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geosite"), "/" + model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geoip"):
			_, _ = w.Write([]byte("<html>invalid Geo database</html>"))
		default:
			t.Errorf("unexpected download: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		kind, query string
		want        []string
	}{
		{"geosite", "google", []string{"google", "google@cn"}},
		{"geoip", "telegram", []string{"telegram"}},
	} {
		t.Run(test.kind, func(t *testing.T) {
			result, err := svc.GeoTags(test.kind, test.query, 100)
			if err != nil || !result.Ready || !reflect.DeepEqual(result.Tags, test.want) || !strings.Contains(result.Message, "SagerNet") || !strings.Contains(result.Message, "暂不可用") {
				t.Fatalf("fallback failure hid valid primary tags or warning: %+v %v", result, err)
			}
			tag := test.kind + "-sager-only"
			if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), tag, model.GeoSourceLoyalsoldier, ""); err == nil || !strings.Contains(err.Error(), tag) {
				t.Fatalf("missing primary category silently succeeded without a valid fallback: %v", err)
			}
		})
	}
}
