package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
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
	if created != 0 {
		t.Fatalf("created default collections = %d, want migration-seeded collection", created)
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

func TestQuickSetupUpdatesOnlyMatchingBuiltInTemplates(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	custom, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "香港节点", Type: "selector", FilterProtocols: "trojan", FilterSubscriptions: "1",
		FilterInclude: "custom-hk", FilterExclude: "custom-exclude", Enabled: true, Priority: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	manualCollision, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "自动选择", Type: "urltest", FilterProtocols: "trojan", FilterSubscriptions: "1",
		FilterInclude: ".*", FilterExclude: "免费|过期|流量|官网|到期|剩余|套餐|订阅",
		NodeUIDs: []string{"manual-node"}, Enabled: true, Priority: 100, TestInterval: 600, Tolerance: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	settingsCollision, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "台湾节点", Type: "selector", FilterProtocols: "trojan", FilterSubscriptions: "1",
		FilterInclude: "🇹🇼|TW|tw|台湾|台|Taiwan", FilterExclude: "免费|过期",
		Enabled: true, Priority: 1, Tolerance: 999,
	})
	if err != nil {
		t.Fatal(err)
	}
	builtin, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "全部节点", Type: "selector", FilterProtocols: "trojan", FilterSubscriptions: "1",
		FilterInclude: ".*", FilterExclude: "", Enabled: true, Priority: 101,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := NewNodeGroupService(db).QuickSetup(model.NodeGroupQuickSetupRequest{FilterProtocols: "vless", FilterSubscriptions: "2"}); err != nil {
		t.Fatal(err)
	}

	updatedCustom, err := db.GetNodeGroup(custom.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedCustom.FilterProtocols != "trojan" || updatedCustom.FilterSubscriptions != "1" {
		t.Fatalf("custom same-name group was overwritten: %+v", updatedCustom)
	}
	updatedManualCollision, err := db.GetNodeGroup(manualCollision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedManualCollision.FilterProtocols != "trojan" || updatedManualCollision.FilterSubscriptions != "1" {
		t.Fatalf("manual same-name group was overwritten: %+v", updatedManualCollision)
	}
	updatedSettingsCollision, err := db.GetNodeGroup(settingsCollision.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedSettingsCollision.FilterProtocols != "trojan" || updatedSettingsCollision.FilterSubscriptions != "1" {
		t.Fatalf("same-name group with custom settings was overwritten: %+v", updatedSettingsCollision)
	}
	updatedBuiltin, err := db.GetNodeGroup(builtin.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updatedBuiltin.FilterProtocols != "vless" || updatedBuiltin.FilterSubscriptions != "2" {
		t.Fatalf("matching built-in template filters were not updated: %+v", updatedBuiltin)
	}
}
