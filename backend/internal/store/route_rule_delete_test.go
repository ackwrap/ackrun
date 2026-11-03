package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestDeleteRouteRuleRemovesOnlyItsBoundStrategy(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		name := "enabled strategy"
		if !enabled {
			name = "disabled strategy"
		}
		t.Run(name, func(t *testing.T) {
			db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "Shared Region", Type: "selector", FilterInclude: "region", Enabled: true})
			if err != nil {
				t.Fatal(err)
			}
			create := func(name string, enabled bool) (*model.RouteRule, *model.ProxyCollection) {
				t.Helper()
				collection := &model.ProxyCollection{
					Type: "selector", SourceType: "node_groups", NodeUIDs: "[]",
					ReferencedGroupIDs: "[" + formatInt64(group.ID) + "]", Enabled: enabled,
				}
				rule, err := db.CreateRouteRuleWithCollection(&model.RouteRuleRequest{
					Name: name, Enabled: true, RuleType: "domain", Values: []string{"example.com"}, Outbound: "proxy",
				}, collection)
				if err != nil {
					t.Fatal(err)
				}
				return rule, collection
			}
			rule, removed := create("Remove Rule", enabled)
			keptRule, kept := create("Keep Rule", true)
			before, err := db.GetProxyCollection(kept.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.DeleteRouteRule(rule.ID); err != nil {
				t.Fatal(err)
			}
			if stored, err := db.GetRouteRule(rule.ID); err != nil || stored != nil {
				t.Fatalf("deleted rule remains: rule=%+v err=%v", stored, err)
			}
			if _, err := db.GetProxyCollection(removed.ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("deleted rule's strategy remains: err=%v", err)
			}
			after, err := db.GetProxyCollection(kept.ID)
			if err != nil || *after != *before {
				t.Fatalf("unrelated strategy changed: strategy=%+v err=%v", after, err)
			}
			if stored, err := db.GetRouteRule(keptRule.ID); err != nil || stored == nil || stored.Outbound != "proxy" {
				t.Fatalf("unrelated rule changed: rule=%+v err=%v", stored, err)
			}
			if stored, err := db.GetNodeGroup(group.ID); err != nil || stored == nil {
				t.Fatalf("shared node group was deleted: group=%+v err=%v", stored, err)
			}
			collections, err := db.ListProxyCollections()
			if err != nil || len(collections) != 2 || collections[0].Name != systemGlobalDirectName {
				t.Fatalf("expected only the system and retained strategies: count=%d err=%v", len(collections), err)
			}
		})
	}
}
