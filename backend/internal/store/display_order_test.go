package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestProxyCollectionWritesReportMissingRows(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	missing := &model.ProxyCollection{Name: "Missing", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true}
	if err := db.UpdateProxyCollection(999999, missing); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing collection update error = %v", err)
	}
	if err := db.DeleteProxyCollection(999999); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("missing collection delete error = %v", err)
	}
}

func TestProxyCollectionStoreReorder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	existing, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	globalID := existing[0].ID
	collections := []*model.ProxyCollection{
		{Name: "first", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
		{Name: "second", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
		{Name: "third", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
	}
	for _, collection := range collections {
		if err := db.CreateProxyCollection(collection); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ReorderProxyCollections([]int{globalID, collections[2].ID, collections[0].ID, collections[1].ID}); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].ID != globalID || items[1].ID != collections[2].ID || items[2].ID != collections[0].ID || items[3].ID != collections[1].ID {
		t.Fatalf("unexpected collection order: %+v", items)
	}
}

func TestProxyCollectionStoreReorderRejectsIncompleteOrUnknownIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	existing, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	globalID := existing[0].ID
	collections := []*model.ProxyCollection{
		{Name: "first", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
		{Name: "second", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
	}
	for _, collection := range collections {
		if err := db.CreateProxyCollection(collection); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ReorderProxyCollections([]int{collections[1].ID}); err == nil {
		t.Fatal("expected incomplete collection order to fail")
	}
	if err := db.ReorderProxyCollections([]int{globalID, collections[1].ID, 999999}); err == nil {
		t.Fatal("expected unknown collection ID to fail")
	}
}

func TestProxyCollectionStoreKeepsGlobalDirectFirst(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	existing, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	direct := existing[0]
	google := &model.ProxyCollection{Name: "Google", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true}
	if err := db.CreateProxyCollection(google); err != nil {
		t.Fatal(err)
	}
	if err := db.ReorderProxyCollections([]int{google.ID, direct.ID}); err == nil {
		t.Fatal("expected moving 全球直连 away from first place to fail")
	}
	if _, err := db.db.Exec(`UPDATE proxy_collections SET priority = CASE WHEN name = '全球直连' THEN 99 ELSE 0 END`); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].ID != direct.ID {
		t.Fatalf("collection order = %+v, want 全球直连 first", items)
	}
}

func TestDNSServerOrderPersists(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "first", Enabled: true, ServerType: "local"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "second", Enabled: true, ServerType: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReorderDNSServers([]int64{second.ID, first.ID}); err != nil {
		t.Fatal(err)
	}
	servers, err := db.ListDNSServers()
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || servers[0].ID != second.ID || servers[1].ID != first.ID {
		t.Fatalf("unexpected DNS server order: %+v", servers)
	}
}

func TestDNSServerReorderRejectsIncompleteOrUnknownIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	first, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "first", Enabled: true, ServerType: "local"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "second", Enabled: true, ServerType: "local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReorderDNSServers([]int64{second.ID}); err == nil {
		t.Fatal("expected incomplete DNS server order to fail")
	}
	if err := db.ReorderDNSServers([]int64{second.ID, first.ID + second.ID + 999999}); err == nil {
		t.Fatal("expected unknown DNS server ID to fail")
	}
}

func TestExistingReorderStoresRejectInvalidIDSets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := []string{
		`INSERT INTO node_groups (name, type, filter_include, created_at, updated_at) VALUES ('one', 'selector', '.*', 1, 1), ('two', 'selector', '.*', 1, 1)`,
		`INSERT INTO route_rules (name, rule_type, values_json, outbound, created_at, updated_at) VALUES ('one', 'domain', '[]', 'direct', 1, 1), ('two', 'domain', '[]', 'direct', 1, 1)`,
		`INSERT INTO dns_rules (rule_type, conditions_json, server, created_at, updated_at) VALUES ('default', '{}', 'local', 1, 1), ('default', '{}', 'local', 1, 1)`,
	}
	for _, statement := range statements {
		if _, err := db.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		name    string
		reorder func([]int64) error
		valid   []int64
	}{
		{name: "node groups", reorder: db.ReorderNodeGroups, valid: []int64{2, 1}},
		{name: "DNS rules", reorder: db.ReorderDNSRules, valid: []int64{2, 1}},
	}
	routeRules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	routeValid := make([]int64, 0, len(routeRules))
	for _, rule := range routeRules {
		if rule.SystemKey == systemRuleAdBlockKey {
			routeValid = append([]int64{rule.ID}, routeValid...)
		} else if rule.SystemKey == systemRuleGlobalDirectKey {
			continue
		} else {
			routeValid = append(routeValid, rule.ID)
		}
	}
	for _, rule := range routeRules {
		if rule.SystemKey == systemRuleGlobalDirectKey {
			routeValid = append(routeValid, rule.ID)
		}
	}
	tests = append(tests, struct {
		name    string
		reorder func([]int64) error
		valid   []int64
	}{name: "route rules", reorder: db.ReorderRouteRules, valid: routeValid})
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			first := test.valid[0]
			for _, ids := range [][]int64{{}, {first}, {first, first}, append(append([]int64{}, test.valid[:len(test.valid)-1]...), 999999)} {
				if err := test.reorder(ids); err == nil {
					t.Fatalf("expected invalid ID set %v to fail", ids)
				}
			}
			if err := test.reorder(test.valid); err != nil {
				t.Fatalf("valid reorder failed: %v", err)
			}
		})
	}
}
