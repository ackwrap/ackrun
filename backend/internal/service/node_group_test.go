package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/store"
)

func TestCreateDefaultProxyCollectionsKeepsAdBlockAsRouteRule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	created, _, err := NewNodeGroupService(db).createDefaultProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if created != 1 {
		t.Fatalf("created default collections = %d, want 1", created)
	}
	collections, err := db.ListProxyCollections()
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 || collections[0].Name != "全球直连" {
		t.Fatalf("default collections = %+v, want only 全球直连", collections)
	}

	rules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	foundAdBlock := false
	for _, rule := range rules {
		if rule.SystemKey == SystemRuleAdBlockKey {
			if !rule.Enabled || rule.Outbound != "block" {
				t.Fatalf("ad block rule = %+v, want enabled block rule", rule)
			}
			foundAdBlock = true
			break
		}
	}
	if !foundAdBlock {
		t.Fatal("system ad block route rule not created")
	}
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}
	route, err := NewConfigGeneratorService(db, nil).generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	generatedRules, ok := route["rules"].([]map[string]interface{})
	if !ok {
		t.Fatalf("route rules type = %T", route["rules"])
	}
	for _, rule := range generatedRules {
		if rule["action"] == "reject" {
			return
		}
	}
	t.Fatal("generated route does not contain ad block reject action")
}

func TestApplicationCleanupIsNotAReservedProxyCollection(t *testing.T) {
	if IsSystemProxyCollectionName("应用净化") {
		t.Fatal("应用净化 must be modeled as a route rule, not a protected proxy collection")
	}
}
