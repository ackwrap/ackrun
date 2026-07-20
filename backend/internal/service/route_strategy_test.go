package service

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestProxyCollectionServiceEnforcesOneToOneRuleBinding(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	routeSvc := newTestRouteRuleService(t, db)
	proxyRule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Proxy Strategy", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	directRule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Direct Strategy", Enabled: true, RuleType: "domain", Values: []string{"direct.example"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	collectionSvc := NewProxyCollectionService(db, nil)
	created, err := collectionSvc.Create(model.ProxyCollectionRequest{Name: "Ignored", RouteRuleID: proxyRule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != proxyRule.Name || created.RouteRuleID != proxyRule.ID || len(created.RouteRuleIDs) != 1 || created.RouteRuleIDs[0] != proxyRule.ID {
		t.Fatalf("forced rule identity not persisted: %+v", created)
	}
	if _, err := collectionSvc.Create(model.ProxyCollectionRequest{RouteRuleID: proxyRule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true}); err == nil {
		t.Fatal("expected duplicate rule binding rejection")
	} else if !errors.Is(err, ErrProxyCollectionRuleBindingConflict) {
		t.Fatalf("duplicate rule binding returned wrong error: %v", err)
	}
	if _, err := collectionSvc.Create(model.ProxyCollectionRequest{RouteRuleIDs: []int64{proxyRule.ID}, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true}); !errors.Is(err, ErrProxyCollectionRuleBindingInvalid) {
		t.Fatalf("legacy-only rule binding was not rejected: %v", err)
	}
	if _, err := collectionSvc.Create(model.ProxyCollectionRequest{RouteRuleID: directRule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true}); err == nil {
		t.Fatal("expected non-proxy rule rejection")
	} else if !errors.Is(err, ErrProxyCollectionRuleBindingInvalid) {
		t.Fatalf("non-proxy rule binding returned wrong error: %v", err)
	}
	if err := collectionSvc.Update(999999, model.ProxyCollectionRequest{RouteRuleID: proxyRule.ID}); !errors.Is(err, ErrProxyCollectionNotFound) {
		t.Fatalf("missing collection update returned wrong error: %v", err)
	}
	if err := collectionSvc.Delete(999999); !errors.Is(err, ErrProxyCollectionNotFound) {
		t.Fatalf("missing collection delete returned wrong error: %v", err)
	}
}

func TestNormalizeProxyCollectionRuleBindingError(t *testing.T) {
	err := normalizeProxyCollectionRuleBindingError(errors.New("constraint failed: UNIQUE constraint failed: proxy_collections.route_rule_id (2067)"))
	if !errors.Is(err, ErrProxyCollectionRuleBindingConflict) {
		t.Fatalf("unique binding error was not normalized: %v", err)
	}
}

func TestRouteRuleNameConflictAndNotFoundErrors(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	rule, err := svc.Create(&model.RouteRuleRequest{Name: "Unique Rule", Enabled: true, RuleType: "domain", Values: []string{"unique.example"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: rule.Name, Enabled: true, RuleType: "domain", Values: []string{"duplicate.example"}, Outbound: "direct"}); !errors.Is(err, ErrRouteRuleNameConflict) {
		t.Fatalf("duplicate rule name returned wrong error: %v", err)
	}
	uniqueErr := normalizeRouteRuleNameConflict(errors.New("constraint failed: UNIQUE constraint failed: route_rules.name (2067)"))
	if !errors.Is(uniqueErr, ErrRouteRuleNameConflict) {
		t.Fatalf("database name conflict was not normalized: %v", uniqueErr)
	}
	request := &model.RouteRuleRequest{Name: "Missing", Enabled: true, RuleType: "domain", Values: []string{"missing.example"}, Outbound: "direct"}
	if _, err := svc.Update(999999, request); !errors.Is(err, ErrRouteRuleNotFound) {
		t.Fatalf("missing rule update returned wrong error: %v", err)
	}
	if _, err := svc.Delete(999999); !errors.Is(err, ErrRouteRuleNotFound) {
		t.Fatalf("missing rule delete returned wrong error: %v", err)
	}
}

func TestRuleModeSkipsOrphanCollectionsAndFailsOnUnconfiguredProxyRule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{Name: "Orphan Strategy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: `["direct"]`, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	generator := NewConfigGeneratorService(db, nil)
	outbounds, _, err := generator.generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range outbounds {
		outbound, _ := raw.(map[string]interface{})
		if outbound["tag"] == "Orphan Strategy" {
			t.Fatalf("orphan collection was generated: %+v", outbounds)
		}
	}

	rule, err := newTestRouteRuleService(t, db).Create(&model.RouteRuleRequest{Name: "Needs Collection", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := generator.generateOutbounds(); err == nil || !strings.Contains(err.Error(), rule.Name) || !strings.Contains(err.Error(), "策略组") {
		t.Fatalf("expected actionable missing collection error, got %v", err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{Name: rule.Name, Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleID: rule.ID, RouteRuleIDs: "[" + fmt.Sprint(rule.ID) + "]", NodeUIDs: `["direct"]`, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := generator.generateOutbounds(); err == nil || !strings.Contains(err.Error(), rule.Name) {
		t.Fatalf("disabled bound collection must not satisfy proxy rule, got %v", err)
	}
	if _, err := generator.generateRoute("direct"); err == nil || !strings.Contains(err.Error(), rule.Name) {
		t.Fatalf("route generation must fail closed for unconfigured proxy rule, got %v", err)
	}
}

func TestRouteRuleOutboundNameValidation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)

	for _, name := range []string{"direct", "proxy", "block", "reject", "DIRECT"} {
		if _, err := svc.Create(&model.RouteRuleRequest{Name: name, Enabled: true, RuleType: "domain", Values: []string{"reserved.example"}, Outbound: "direct"}); err == nil || !strings.Contains(err.Error(), "保留") {
			t.Fatalf("reserved name %q was accepted: %v", name, err)
		}
	}
	first, err := svc.Create(&model.RouteRuleRequest{Name: "Duplicate Strategy", Enabled: true, RuleType: "domain", Values: []string{"first.example"}, Outbound: "block"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: first.Name, Enabled: true, RuleType: "domain", Values: []string{"second.example"}, Outbound: "block"}); err == nil || !strings.Contains(err.Error(), "已存在") {
		t.Fatalf("duplicate route rule name was accepted: %v", err)
	}

	if _, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "Group Collision", Type: "selector", FilterInclude: ".*", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Group Collision", Enabled: true, RuleType: "domain", Values: []string{"group.example"}, Outbound: "direct"}); err == nil || !strings.Contains(err.Error(), "节点组") {
		t.Fatalf("enabled node-group collision was accepted: %v", err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{Name: "Collection Collision", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: `["direct"]`, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(&model.RouteRuleRequest{Name: "Collection Collision", Enabled: true, RuleType: "domain", Values: []string{"collection.example"}, Outbound: "proxy"}); err == nil || !strings.Contains(err.Error(), "策略组") {
		t.Fatalf("enabled collection collision was accepted: %v", err)
	}
}

func TestProxyCollectionUsesForcedRuleNameForCollisionValidation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	collectionSvc := NewProxyCollectionService(db, nil)
	if _, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "Late Group Collision", Type: "selector", FilterInclude: ".*", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	collidingRule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Late Group Collision", Enabled: true, RuleType: "domain", Values: []string{"late.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collectionSvc.Create(model.ProxyCollectionRequest{Name: "Ignored", RouteRuleID: collidingRule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true}); err == nil || !strings.Contains(err.Error(), "节点组") {
		t.Fatalf("forced node-group collision was accepted: %v", err)
	}
}

func TestRouteRuleStrategyViewUsesCanonicalOrder(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	routeSvc := newTestRouteRuleService(t, db)
	proxyRule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Proxy Strategy", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	directRule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Direct Strategy", Enabled: true, RuleType: "domain", Values: []string{"direct.example"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	collection, err := NewProxyCollectionService(db, nil).Create(model.ProxyCollectionRequest{RouteRuleID: proxyRule.ID, Type: "selector", SourceType: "manual", NodeUIDs: []string{"direct"}, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	items, err := routeSvc.Strategies()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 || items[0].Kind != "reject" || items[len(items)-1].Kind != "final" {
		t.Fatalf("unexpected strategy rows: %+v", items)
	}
	if items[len(items)-1].Collection == nil || items[len(items)-1].Collection.Name != SystemGlobalDirectRouteRuleName || !items[len(items)-1].ReadOnly {
		t.Fatalf("global final row lacks system collection: %+v", items[len(items)-1])
	}
	byID := make(map[int64]model.RouteStrategyItem)
	for _, item := range items {
		byID[item.RuleID] = item
	}
	if got := byID[proxyRule.ID]; got.Kind != "proxy" || got.Collection == nil || got.Collection.ID != collection.ID || got.OutboundTag != proxyRule.Name || got.ReadOnly {
		t.Fatalf("unexpected proxy strategy: %+v", got)
	}
	if got := byID[directRule.ID]; got.Kind != "direct" || got.OutboundTag != directRule.Name || !got.ReadOnly {
		t.Fatalf("unexpected direct strategy: %+v", got)
	}
}

func TestRuleModeGeneratesDirectSelectorRejectAndFallbackFinal(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}
	routeSvc := newTestRouteRuleService(t, db)
	directRule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Direct Strategy", Enabled: true, RuleType: "domain_suffix", Values: []string{"direct.example"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Reject Strategy", Enabled: true, RuleType: "domain", Values: []string{"reject.example"}, Outbound: "block"}); err != nil {
		t.Fatal(err)
	}
	generator := NewConfigGeneratorService(db, nil)
	outbounds, _, err := generator.generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	foundSelector := false
	for _, raw := range outbounds {
		outbound, _ := raw.(map[string]interface{})
		if outbound["tag"] == directRule.Name {
			members, _ := outbound["outbounds"].([]string)
			foundSelector = outbound["type"] == "selector" && len(members) == 1 && members[0] == "direct"
		}
	}
	if !foundSelector {
		t.Fatalf("direct selector missing from outbounds: %+v", outbounds)
	}
	route, err := generator.generateRoute("proxy")
	if err != nil {
		t.Fatal(err)
	}
	if route["final"] != SystemGlobalDirectRouteRuleName {
		t.Fatalf("route final = %v", route["final"])
	}
	rules, _ := route["rules"].([]map[string]interface{})
	foundDirect, foundReject := false, false
	for _, rule := range rules {
		if stringListContains(rule["domain_suffix"], "direct.example") {
			foundDirect = rule["action"] == "route" && rule["outbound"] == directRule.Name
		}
		if stringListContains(rule["domain"], "reject.example") {
			_, hasOutbound := rule["outbound"]
			foundReject = rule["action"] == "reject" && !hasOutbound
		}
		if _, exists := rule["fallback"]; exists {
			t.Fatalf("fallback was emitted as an ordinary rule: %+v", rule)
		}
	}
	if !foundDirect || !foundReject {
		t.Fatalf("direct/reject route output missing: %+v", rules)
	}
	if _, err := generator.appendDirectStrategyOutbounds([]interface{}{map[string]interface{}{"tag": directRule.Name}}, nil); err == nil || !strings.Contains(err.Error(), "冲突") {
		t.Fatalf("expected clear direct strategy tag collision, got %v", err)
	}
}

func TestGlobalDirectRuleCannotBeChanged(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	rules, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	global := rules[len(rules)-1]
	if _, err := svc.Update(global.ID, &model.RouteRuleRequest{Name: global.Name, Enabled: false}); !errors.Is(err, ErrSystemRouteRuleProtected) {
		t.Fatalf("expected global direct update protection, got %v", err)
	}
	if _, err := svc.Delete(global.ID); !errors.Is(err, ErrSystemRouteRuleProtected) {
		t.Fatalf("expected global direct delete protection, got %v", err)
	}
}

func TestProxyRuleChangesPreserveCollectionSettings(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	routeSvc := newTestRouteRuleService(t, db)
	rule, err := routeSvc.Create(&model.RouteRuleRequest{Name: "Original Proxy", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	collectionSvc := NewProxyCollectionService(db, nil)
	collection, err := collectionSvc.Create(model.ProxyCollectionRequest{RouteRuleID: rule.ID, Type: "urltest", SourceType: "manual", NodeUIDs: []string{"direct"}, TestURL: "https://example.com/generate_204", TestInterval: 600, Tolerance: 250, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := routeSvc.Update(rule.ID, &model.RouteRuleRequest{Name: "Renamed Proxy", Enabled: true, Priority: rule.Priority, RuleType: rule.RuleType, Values: rule.Values, Outbound: "proxy"}); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Name != "Renamed Proxy" || stored.RouteRuleID != rule.ID || stored.Type != "urltest" || stored.TestInterval != 600 || stored.Tolerance != 250 {
		t.Fatalf("rename lost collection identity or settings: %+v", stored)
	}
	if _, err := routeSvc.Update(rule.ID, &model.RouteRuleRequest{Name: "Renamed Proxy", Enabled: true, Priority: rule.Priority, RuleType: rule.RuleType, Values: rule.Values, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	stored, err = db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RouteRuleID != 0 || stored.RouteRuleIDs != "[]" || stored.Type != "urltest" || stored.TestInterval != 600 || stored.NodeUIDs != `["direct"]` {
		t.Fatalf("unbinding changed persisted collection settings: %+v", stored)
	}
	stored.Enabled = false
	if err := db.UpdateProxyCollection(collection.ID, stored); err != nil {
		t.Fatal(err)
	}
	if _, err := routeSvc.Update(rule.ID, &model.RouteRuleRequest{Name: "Renamed Proxy", Enabled: true, Priority: rule.Priority, RuleType: rule.RuleType, Values: rule.Values, Outbound: "proxy"}); err != nil {
		t.Fatal(err)
	}
	if err := collectionSvc.Update(collection.ID, model.ProxyCollectionRequest{RouteRuleID: rule.ID, Type: "urltest", SourceType: "manual", NodeUIDs: []string{"direct"}, TestURL: "https://example.com/generate_204", TestInterval: 600, Tolerance: 250, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := routeSvc.Delete(rule.ID); err != nil {
		t.Fatal(err)
	}
	stored, err = db.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.RouteRuleID != 0 || stored.Type != "urltest" || stored.TestInterval != 600 || stored.NodeUIDs != `["direct"]` {
		t.Fatalf("rule deletion destroyed collection settings: %+v", stored)
	}
}
