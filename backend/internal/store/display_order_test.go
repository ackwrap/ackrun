package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestProxyCollectionStoreReorder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	if err := db.ReorderProxyCollections([]int{collections[2].ID, collections[0].ID, collections[1].ID}); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || items[0].ID != collections[2].ID || items[1].ID != collections[0].ID || items[2].ID != collections[1].ID {
		t.Fatalf("unexpected collection order: %+v", items)
	}
}

func TestProxyCollectionStoreReorderRejectsIncompleteOrUnknownIDs(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
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
	if err := db.ReorderProxyCollections([]int{collections[1].ID, 999999}); err == nil {
		t.Fatal("expected unknown collection ID to fail")
	}
}

func TestProxyCollectionStoreKeepsGlobalDirectFirst(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	collections := []*model.ProxyCollection{
		{Name: "全球直连", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: `["direct"]`, Enabled: true},
		{Name: "Google", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true},
	}
	for _, collection := range collections {
		if err := db.CreateProxyCollection(collection); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.ReorderProxyCollections([]int{collections[1].ID, collections[0].ID}); err == nil {
		t.Fatal("expected moving 全球直连 away from first place to fail")
	}
	if _, err := db.db.Exec(`UPDATE proxy_collections SET priority = CASE WHEN name = '全球直连' THEN 99 ELSE 0 END`); err != nil {
		t.Fatal(err)
	}
	items, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Name != "全球直连" {
		t.Fatalf("collection order = %+v, want 全球直连 first", items)
	}
}

func TestDNSServerAndOutboundBindingOrderPersist(t *testing.T) {
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

	want := []string{"proxy", "streaming", "direct"}
	if err := db.SetDNSOutboundBindingOrder(want); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetDNSOutboundBindingOrder()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) {
		t.Fatalf("outbound order = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("outbound order = %v, want %v", got, want)
		}
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
	}{
		{name: "node groups", reorder: db.ReorderNodeGroups},
		{name: "route rules", reorder: db.ReorderRouteRules},
		{name: "DNS rules", reorder: db.ReorderDNSRules},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, ids := range [][]int64{{}, {1}, {1, 1}, {1, 999999}} {
				if err := test.reorder(ids); err == nil {
					t.Fatalf("expected invalid ID set %v to fail", ids)
				}
			}
			if err := test.reorder([]int64{2, 1}); err != nil {
				t.Fatalf("valid reorder failed: %v", err)
			}
		})
	}
}
