package store

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestRouteStrategyMigrationIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Current Proxy", Enabled: true, RuleType: "domain_suffix", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	collection := &model.ProxyCollection{Name: rule.Name, Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleID: rule.ID, RouteRuleIDs: "[" + formatInt64(rule.ID) + "]", NodeUIDs: `["direct"]`, Enabled: true}
	unboundCollection := &model.ProxyCollection{Name: "Unbound", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: `["direct"]`, Enabled: true}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(unboundCollection); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	for attempt := 0; attempt < 2; attempt++ {
		db, err = Open(path)
		if err != nil {
			t.Fatal(err)
		}
		stored, err := db.GetProxyCollection(collection.ID)
		if err != nil {
			t.Fatal(err)
		}
		if stored.RouteRuleID != rule.ID {
			t.Fatalf("migration attempt %d changed binding to %d, want %d", attempt, stored.RouteRuleID, rule.ID)
		}
		if _, err := db.db.Exec(`UPDATE proxy_collections SET route_rule_id = ? WHERE id = ?`, rule.ID, unboundCollection.ID); err == nil {
			t.Fatal("expected unique route_rule_id index to reject duplicate binding")
		}
		if attempt == 0 {
			if _, err := db.db.Exec(`UPDATE route_rules SET updated_at = 1234`); err != nil {
				t.Fatal(err)
			}
			if _, err := db.db.Exec(`UPDATE proxy_collections SET updated_at = 5678`); err != nil {
				t.Fatal(err)
			}
		} else {
			var changedRules, changedCollections int
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM route_rules WHERE updated_at <> 1234`).Scan(&changedRules); err != nil {
				t.Fatal(err)
			}
			if err := db.db.QueryRow(`SELECT COUNT(*) FROM proxy_collections WHERE updated_at <> 5678`).Scan(&changedCollections); err != nil {
				t.Fatal(err)
			}
			if changedRules != 0 || changedCollections != 0 {
				t.Fatalf("idempotent migration rewrote timestamps: rules=%d collections=%d", changedRules, changedCollections)
			}
		}
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSystemRouteRuleOrderingAndReorderProtection(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ordinary, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Ordinary", Enabled: true, RuleType: "domain", Values: []string{"example.com"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	rules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 3 || rules[0].SystemKey != systemRuleAdBlockKey || rules[1].ID != ordinary.ID || rules[2].SystemKey != systemRuleGlobalDirectKey {
		t.Fatalf("unexpected canonical rule order: %+v", rules)
	}
	valid := []int64{rules[0].ID, ordinary.ID, rules[2].ID}
	if err := db.ReorderRouteRules(valid); err != nil {
		t.Fatalf("valid canonical reorder failed: %v", err)
	}
	if err := db.ReorderRouteRules([]int64{ordinary.ID, rules[0].ID, rules[2].ID}); err == nil {
		t.Fatal("expected ad-block position rejection")
	}
	if err := db.ReorderRouteRules([]int64{rules[0].ID, rules[2].ID, ordinary.ID}); err == nil {
		t.Fatal("expected global-direct position rejection")
	}
	if err := db.ReorderRouteRules(valid[:2]); err == nil {
		t.Fatal("expected incomplete reorder rejection")
	}
}

func TestRouteRuleWritesRollbackWhenOrderNormalizationFails(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	original, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Atomic Original", Enabled: true, RuleType: "domain", Values: []string{"original.example"}, Outbound: "direct"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE route_rules SET system_key = '' WHERE system_key = ?`, systemRuleGlobalDirectKey); err != nil {
		t.Fatal(err)
	}

	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Rolled Back Create", Enabled: true, RuleType: "domain", Values: []string{"create.example"}, Outbound: "direct"}); err == nil {
		t.Fatal("expected create normalization failure")
	}
	var createdCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM route_rules WHERE name = 'Rolled Back Create'`).Scan(&createdCount); err != nil || createdCount != 0 {
		t.Fatalf("failed create was committed: count=%d err=%v", createdCount, err)
	}
	if _, err := db.UpdateRouteRule(original.ID, &model.RouteRuleRequest{Name: "Rolled Back Update", Enabled: true, Priority: original.Priority, RuleType: "domain", Values: []string{"updated.example"}, Outbound: "direct"}); err == nil {
		t.Fatal("expected update normalization failure")
	}
	stored, err := db.GetRouteRule(original.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.Name != original.Name {
		t.Fatalf("failed update was committed: %+v", stored)
	}
	if err := db.DeleteRouteRule(original.ID); err == nil {
		t.Fatal("expected delete normalization failure")
	}
	stored, err = db.GetRouteRule(original.ID)
	if err != nil || stored == nil {
		t.Fatalf("failed delete was committed: item=%+v err=%v", stored, err)
	}
}

func formatInt64(value int64) string {
	return fmt.Sprintf("%d", value)
}
