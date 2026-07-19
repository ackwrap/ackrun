package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestGeoAssetAvailabilityTracksLocalFile(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	assets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatalf("list geo assets: %v", err)
	}
	if len(assets) == 0 || assets[0].Type != "geoip" {
		t.Fatalf("unexpected default geo assets: %+v", assets)
	}

	path := filepath.Join(t.TempDir(), "geoip.db")
	item, err := db.UpdateGeoAssetSyncResult(assets[0].ID, path)
	if err != nil {
		t.Fatalf("set geo asset path: %v", err)
	}
	if item.Available {
		t.Fatal("missing geo database must not be available")
	}

	if err := os.WriteFile(path, []byte("geo database"), 0o600); err != nil {
		t.Fatalf("create geo database: %v", err)
	}
	item, err = db.GetGeoAsset(assets[0].ID)
	if err != nil {
		t.Fatalf("get geo asset: %v", err)
	}
	if !item.Available {
		t.Fatal("existing regular geo database must be available")
	}
}

func TestRouteRuleStoreCRUDAndReorder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	first, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Google Proxy", Enabled: true, RuleType: "domain_suffix", Values: []string{"google.com"}, Outbound: "proxy"})
	if err != nil {
		t.Fatalf("create first rule: %v", err)
	}
	second, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "LAN Direct", Enabled: true, RuleType: "ip_cidr", Values: []string{"192.168.0.0/16"}, Outbound: "direct"})
	if err != nil {
		t.Fatalf("create second rule: %v", err)
	}

	items, err := db.ListRouteRules()
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(items) != 2 || items[0].ID != first.ID || items[1].ID != second.ID {
		t.Fatalf("unexpected rule order: %+v", items)
	}

	updated, err := db.UpdateRouteRule(first.ID, &model.RouteRuleRequest{Name: "Google Direct", Enabled: false, Priority: first.Priority, RuleType: "domain_suffix", Values: []string{"google.com", "google.com"}, Outbound: "direct", Invert: true})
	if err != nil {
		t.Fatalf("update rule: %v", err)
	}
	if updated.Enabled || !updated.Invert || updated.Outbound != "direct" {
		t.Fatalf("unexpected updated rule: %+v", updated)
	}

	if err := db.ReorderRouteRules([]int64{second.ID, first.ID}); err != nil {
		t.Fatalf("reorder rules: %v", err)
	}
	items, err = db.ListRouteRules()
	if err != nil {
		t.Fatalf("list reordered rules: %v", err)
	}
	if items[0].ID != second.ID || items[1].ID != first.ID {
		t.Fatalf("unexpected reordered rules: %+v", items)
	}

	if err := db.DeleteRouteRule(first.ID); err != nil {
		t.Fatalf("delete rule: %v", err)
	}
	items, err = db.ListRouteRules()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("unexpected rules after delete: %+v", items)
	}
}

func TestRouteRuleStoreSystemKey(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	created, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "广告拦截", Enabled: true, RuleType: "geosite", Values: []string{"category-ads-all"}, Outbound: "block", SystemKey: "ad_block"})
	if err != nil {
		t.Fatalf("create system rule: %v", err)
	}
	if !created.IsSystem || created.SystemKey != "ad_block" {
		t.Fatalf("expected system rule metadata: %+v", created)
	}

	updated, err := db.UpdateRouteRule(created.ID, &model.RouteRuleRequest{Name: "广告拦截", Enabled: false, Priority: created.Priority, RuleType: "geosite", Values: []string{"category-ads-all"}, Outbound: "block"})
	if err != nil {
		t.Fatalf("update system rule: %v", err)
	}
	if !updated.IsSystem || updated.SystemKey != "ad_block" || updated.Enabled {
		t.Fatalf("system key should survive normal update: %+v", updated)
	}
}

func TestRouteRuleSubscriptionStoreCRUD(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	created, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{Name: "GeoSite CN", Enabled: true, Tag: "geosite-cn", URL: "https://example.com/geosite-cn.srs", Format: "binary", UseProxy: true})
	if err != nil {
		t.Fatalf("create rule subscription: %v", err)
	}
	if !created.Enabled || !created.UseProxy || created.Tag != "geosite-cn" {
		t.Fatalf("unexpected created subscription: %+v", created)
	}

	items, err := db.ListRouteRuleSubscriptions()
	if err != nil {
		t.Fatalf("list rule subscriptions: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("unexpected subscriptions: %+v", items)
	}

	updated, err := db.UpdateRouteRuleSubscription(created.ID, &model.RouteRuleSubscriptionRequest{Name: "GeoIP CN", Enabled: false, Tag: "geoip-cn", URL: "https://example.com/geoip-cn.srs", Format: "source", UseProxy: false})
	if err != nil {
		t.Fatalf("update rule subscription: %v", err)
	}
	if updated.Enabled || updated.UseProxy || updated.Tag != "geoip-cn" || updated.Format != "source" {
		t.Fatalf("unexpected updated subscription: %+v", updated)
	}

	if err := db.DeleteRouteRuleSubscription(created.ID); err != nil {
		t.Fatalf("delete rule subscription: %v", err)
	}
	items, err = db.ListRouteRuleSubscriptions()
	if err != nil {
		t.Fatalf("list after delete: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("unexpected subscriptions after delete: %+v", items)
	}
}
