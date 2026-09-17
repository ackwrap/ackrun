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
	"testing"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func geoTestBytes(dst []byte, number protowire.Number, value []byte) []byte {
	return protowire.AppendBytes(protowire.AppendTag(dst, number, protowire.BytesType), value)
}

func geoTestDomain(kind uint64, value string, attribute string) []byte {
	domain := protowire.AppendVarint(protowire.AppendTag(nil, 1, protowire.VarintType), kind)
	domain = geoTestBytes(domain, 2, []byte(value))
	if attribute != "" {
		domain = geoTestBytes(domain, 3, geoTestBytes(nil, 1, []byte(attribute)))
	}
	return domain
}

func geoTestSiteDAT() []byte {
	var data []byte
	for _, code := range []string{"CN", "CATEGORY-ADS-ALL", "GOOGLE"} {
		entry := geoTestBytes(nil, 1, []byte(code))
		for _, domain := range [][]byte{
			geoTestDomain(3, "exact.example", ""),
			geoTestDomain(2, "suffix.example", "cn"),
			geoTestDomain(0, "keyword", ""),
			geoTestDomain(1, `^regex\.example$`, ""),
		} {
			entry = geoTestBytes(entry, 2, domain)
		}
		data = geoTestBytes(data, 1, entry)
	}
	return data
}

func geoTestIPDAT() []byte {
	var data []byte
	for _, code := range []string{"CN", "TELEGRAM", "PRIVATE"} {
		entry := geoTestBytes(nil, 1, []byte(code))
		cidr := geoTestBytes(nil, 1, []byte{192, 0, 2, 0})
		cidr = protowire.AppendVarint(protowire.AppendTag(cidr, 2, protowire.VarintType), 24)
		entry = geoTestBytes(entry, 2, cidr)
		data = geoTestBytes(data, 1, entry)
	}
	return data
}

func geoSourceTestService(t *testing.T) (*RouteRuleService, *store.Store) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return newTestRouteRuleService(t, db), db
}

func geoSourceTestMirror(t *testing.T, db *store.Store) {
	t.Helper()
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/https://raw.githubusercontent.com/Loyalsoldier/geoip/release/geoip.dat":
			_, _ = w.Write(geoTestIPDAT())
		case "/https://raw.githubusercontent.com/Loyalsoldier/v2ray-rules-dat/release/geosite.dat":
			_, _ = w.Write(geoTestSiteDAT())
		default:
			t.Errorf("unexpected download: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(mirror.Close)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
}

func selectTestGeoSource(t *testing.T, svc *RouteRuleService, db *store.Store) {
	t.Helper()
	geoSourceTestMirror(t, db)
	settings := NewSettingsService(db)
	settings.SetGeoSourcePreparer(svc)
	source := model.GeoSourceLoyalsoldier
	if err := settings.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source}); err != nil {
		t.Fatal(err)
	}
}

func TestLoyalsoldierGeoSourceConvertsRuleTypesAndAttributes(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	for _, test := range []struct {
		tag  string
		want map[string]any
	}{
		{"geosite-cn", map[string]any{
			"domain": []any{"exact.example"}, "domain_suffix": []any{"suffix.example"},
			"domain_keyword": []any{"keyword"}, "domain_regex": []any{`^regex\.example$`},
		}},
		{"geosite-cn@cn", map[string]any{"domain_suffix": []any{"suffix.example"}}},
		{"geoip-telegram", map[string]any{"ip_cidr": []any{"192.0.2.0/24"}}},
	} {
		t.Run(test.tag, func(t *testing.T) {
			data, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), test.tag, model.GeoSourceLoyalsoldier, "")
			if err != nil || contentType != "application/json; charset=utf-8" {
				t.Fatalf("convert: %s %v", contentType, err)
			}
			var document struct {
				Version int              `json:"version"`
				Rules   []map[string]any `json:"rules"`
			}
			if err := json.Unmarshal(data, &document); err != nil || document.Version != 3 || len(document.Rules) != 1 || !reflect.DeepEqual(document.Rules[0], test.want) {
				t.Fatalf("unexpected rule-set: %s (%v)", data, err)
			}
		})
	}
	tags, err := svc.GeoTags("geoip", "telegram", 100)
	if err != nil || !reflect.DeepEqual(tags.Tags, []string{"telegram"}) {
		t.Fatalf("GeoIP categories: %+v %v", tags, err)
	}
	lookup, err := svc.GeoLookup("192.0.2.1", "")
	if err != nil || len(lookup.GeoIPMatches) != 1 || !strings.Contains(lookup.GeoIPMatches[0], "cn, private, telegram") {
		t.Fatalf("overlapping categories were lost: %+v %v", lookup, err)
	}
}

func TestLoyalsoldierSourcePinsPreviewRouteAndDNSButNotSubscriptions(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "ASN", Enabled: true, RuleType: "geoip", Values: []string{"telegram"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Invalid ASN", Enabled: true, RuleType: "geoip", Values: []string{"missing"}, Outbound: "direct"}); err == nil {
		t.Fatal("unknown source category accepted")
	}
	if _, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{Name: "Own", Enabled: true, Tag: "geosite-custom", URL: "https://example.invalid/custom.srs", Format: "binary"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, RuleType: "default", Conditions: map[string]any{"geosite": []string{"google"}}, Server: "local"}); err != nil {
		t.Fatal(err)
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
	for name, sets := range map[string][]map[string]any{"preview": preview.RuleSets, "route": route["rule_set"].([]map[string]any)} {
		foundIP, foundDNS := false, false
		for _, set := range sets {
			if set["tag"] == "geosite-custom" {
				if set["format"] != "binary" || strings.Contains(set["url"].(string), "source=loyalsoldier") {
					t.Fatalf("custom subscription changed: %+v", set)
				}
				continue
			}
			u, err := url.Parse(set["url"].(string))
			if err != nil || set["format"] != "source" || u.Query().Get("source") != "loyalsoldier" || !validGeoAssetVersion(u.Query().Get("version")) {
				t.Fatalf("un-pinned %s rule set: %+v", name, set)
			}
			foundIP = foundIP || set["tag"] == "geoip-telegram"
			foundDNS = foundDNS || set["tag"] == "geosite-google"
		}
		if !foundIP || (name == "route" && !foundDNS) {
			t.Fatalf("missing generated sets: %s %+v", name, sets)
		}
	}
}

func TestGeoSourcePreparationRejectsMissingCategoriesAndBadData(t *testing.T) {
	svc, db := geoSourceTestService(t)
	geoSourceTestMirror(t, db)
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Missing", Enabled: true, RuleType: "geosite", Values: []string{"not-in-target"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	before, _ := db.ListGeoAssets()
	if _, err := svc.PrepareGeoSource(model.GeoSourceLoyalsoldier); err == nil || !strings.Contains(err.Error(), "not-in-target") {
		t.Fatalf("expected missing category: %v", err)
	}
	after, _ := db.ListGeoAssets()
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed preparation changed selected assets")
	}
	asset := &model.GeoAsset{Source: model.GeoSourceLoyalsoldier, Type: "geoip"}
	if _, err := svc.cacheGeoDatabase(asset, []byte("<html>gateway error</html>")); err == nil {
		t.Fatal("invalid download accepted")
	}
}

func TestGeoSourceCacheIsolationAndPreviousConfig(t *testing.T) {
	svc, db := geoSourceTestService(t)
	originalSettings, _ := db.GetGeneralSettings()
	originalAssets, _ := db.ListGeoAssets()
	selectTestGeoSource(t, svc, db)
	assets, _ := db.ListGeoAssets()
	var version string
	for _, asset := range assets {
		if asset.Type == "geoip" {
			version = geoAssetVersion(asset)
		}
	}
	oldSRS := testBinaryRuleSet(t, 1)
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cacheDir, "geoip-cn.srs"), oldSRS, 0644); err != nil {
		t.Fatal(err)
	}
	data, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", version)
	if err != nil || bytes.Equal(data, oldSRS) || !json.Valid(data) {
		t.Fatalf("source cache contamination: %s %v", data, err)
	}
	if err := db.RestoreGeneralSettingsWithGeoAssets(originalSettings, originalAssets); err != nil {
		t.Fatal(err)
	}
	previous, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", version)
	if err != nil || !bytes.Equal(previous, data) {
		t.Fatalf("previous config lost its source: %v", err)
	}
	legacy, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "", "")
	if err != nil || !bytes.Equal(legacy, oldSRS) {
		t.Fatalf("legacy binary endpoint changed: %v", err)
	}
	for _, source := range []string{"unknown", "../../"} {
		if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", source, version); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", "../../"); err == nil {
		t.Fatal("invalid version accepted")
	}
}
