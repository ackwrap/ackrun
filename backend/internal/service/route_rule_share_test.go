package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRouteRuleShareRoundTripMergesAtomically(t *testing.T) {
	sourceDB, err := store.Open(filepath.Join(t.TempDir(), "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sourceDB.Close()
	source := newTestRouteRuleService(t, sourceDB)
	sourceAlpha, err := source.Create(&model.RouteRuleRequest{Name: "Alpha", Enabled: true, RuleType: "domain_suffix", Values: []string{"alpha.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := source.Create(&model.RouteRuleRequest{Name: "Beta", Enabled: false, RuleType: "ip_cidr", Values: []string{"203.0.113.0/24"}, Outbound: "bypass", Invert: true}); err != nil {
		t.Fatal(err)
	}
	setTestFinalStrategy(t, source, "proxy")
	shared, err := source.Share()
	if err != nil {
		t.Fatal(err)
	}
	if shared.Code == "" || shared.RuleCount != 2 {
		t.Fatalf("unexpected share response: %+v", shared)
	}

	targetDB, err := store.Open(filepath.Join(t.TempDir(), "target.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer targetDB.Close()
	target := newTestRouteRuleService(t, targetDB)
	targetAlpha, err := target.Create(&model.RouteRuleRequest{Name: "Alpha", Enabled: false, RuleType: "domain", Values: []string{"old.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	targetExtra, err := target.Create(&model.RouteRuleRequest{Name: "Extra", Enabled: true, RuleType: "domain", Values: []string{"extra.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	routeRuleIDs, _ := json.Marshal([]int64{targetAlpha.ID, targetExtra.ID})
	collection := &model.ProxyCollection{
		Name: "Shared Collection", Type: "selector", SourceType: "manual",
		ReferencedGroupIDs: "[]", RouteRuleID: targetAlpha.ID, RouteRuleIDs: string(routeRuleIDs), NodeUIDs: "[]", Enabled: true,
	}
	if err := targetDB.CreateProxyCollection(collection); err != nil {
		t.Fatal(err)
	}

	result, err := target.ImportShare(&model.RouteRuleImportRequest{Code: shared.Code})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 1 || result.Updated != 1 || result.RuleCount != 2 {
		t.Fatalf("unexpected import result: %+v", result)
	}
	rules, err := target.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 5 {
		t.Fatalf("rule count = %d, want 5: %+v", len(rules), rules)
	}
	if rules[1].Name != "Alpha" || rules[1].Outbound != "proxy" || rules[1].Values[0] != "alpha.example" {
		t.Fatalf("Alpha was not updated from share: %+v", rules[1])
	}
	if rules[2].Name != "Beta" || rules[2].Outbound != "bypass" || rules[2].Enabled || !rules[2].Invert {
		t.Fatalf("Beta was not created from share: %+v", rules[2])
	}
	if rules[3].Name != "Extra" {
		t.Fatalf("unrelated target rule was not preserved: %+v", rules)
	}
	if rules[4].SystemKey != SystemRuleGlobalDirectKey || rules[4].Outbound != "proxy" {
		t.Fatalf("final strategy was not imported: %+v", rules[4])
	}
	storedCollection, err := targetDB.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	var storedRuleIDs []int64
	if err := json.Unmarshal([]byte(storedCollection.RouteRuleIDs), &storedRuleIDs); err != nil {
		t.Fatal(err)
	}
	if len(storedRuleIDs) != 2 || storedRuleIDs[0] != targetAlpha.ID || storedRuleIDs[1] != targetExtra.ID {
		t.Fatalf("proxy collection rule references changed during import: %v", storedRuleIDs)
	}
	if _, err := source.Update(sourceAlpha.ID, &model.RouteRuleRequest{Name: "Alpha", Enabled: true, RuleType: "domain_suffix", Values: []string{"alpha.example"}, Outbound: "direct"}); err != nil {
		t.Fatal(err)
	}
	directShare, err := source.Share()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := target.ImportShare(&model.RouteRuleImportRequest{Code: directShare.Code}); err != nil {
		t.Fatal(err)
	}
	storedCollection, err = targetDB.GetProxyCollection(collection.ID)
	if err != nil {
		t.Fatal(err)
	}
	storedRuleIDs = nil
	if err := json.Unmarshal([]byte(storedCollection.RouteRuleIDs), &storedRuleIDs); err != nil {
		t.Fatal(err)
	}
	if storedCollection.RouteRuleID != 0 || len(storedRuleIDs) != 1 || storedRuleIDs[0] != targetExtra.ID {
		t.Fatalf("non-proxy import removed unrelated collection references: canonical=%d rules=%v", storedCollection.RouteRuleID, storedRuleIDs)
	}
	rules, err = target.List()
	if err != nil {
		t.Fatal(err)
	}

	before := append([]model.RouteRule(nil), rules...)
	if _, err := target.ImportShare(&model.RouteRuleImportRequest{Code: "not-base64"}); err == nil {
		t.Fatal("expected invalid share code error")
	}
	after, err := target.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Fatalf("invalid import changed rules: before=%d after=%d", len(before), len(after))
	}
	for index := range before {
		if before[index].Name != after[index].Name || before[index].UpdatedAt != after[index].UpdatedAt {
			t.Fatalf("invalid import mutated rule %d: before=%+v after=%+v", index, before[index], after[index])
		}
	}
	if _, err := target.ImportShare(&model.RouteRuleImportRequest{Code: strings.Repeat(" ", model.MaxRouteRuleShareCodeSize+1)}); err == nil {
		t.Fatal("expected oversized whitespace share code error")
	}
	afterOversized, err := target.List()
	if err != nil {
		t.Fatal(err)
	}
	for index := range after {
		if after[index].Name != afterOversized[index].Name || after[index].UpdatedAt != afterOversized[index].UpdatedAt {
			t.Fatalf("oversized import mutated rule %d", index)
		}
	}
}

func setTestFinalStrategy(t *testing.T, service *RouteRuleService, outbound string) {
	t.Helper()
	rules, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range rules {
		if rule.SystemKey == SystemRuleGlobalDirectKey {
			if _, err := service.Update(rule.ID, &model.RouteRuleRequest{Outbound: outbound}); err != nil {
				t.Fatal(err)
			}
			return
		}
	}
	t.Fatal("final strategy rule not found")
}
