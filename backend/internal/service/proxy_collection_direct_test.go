package service

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestCollectionDirectOnlySourcesPersistAndRejectAutomaticSelection(t *testing.T) {
	for _, source := range []string{proxyCollectionSourceNodeGroups, proxyCollectionSourceNodeGroupsAndNodes, proxyCollectionSourceManual} {
		t.Run(source, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			ruleID := createHealthTestProxyRule(t, db, "Business")
			svc := NewProxyCollectionService(db, nil)
			req := model.ProxyCollectionRequest{RouteRuleID: ruleID, Type: "selector", SourceType: source, NodeUIDs: []string{"direct"}, Enabled: true}
			collection, err := svc.Create(req)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(collection.NodeUIDs, []string{"direct"}) || len(collection.ReferencedGroups) != 0 {
				t.Fatalf("direct-only source did not round trip: %+v", collection)
			}
			outbound, err := (&ConfigGeneratorService{}).generateCollectionOutbound(collection, nil, nil, nil)
			if err != nil || !reflect.DeepEqual(outbound["outbounds"], []string{"direct"}) {
				t.Fatalf("direct-only outbound = %+v, err=%v", outbound, err)
			}
			if err := svc.Update(collection.ID, req); err != nil {
				t.Fatalf("save direct-only source again: %v", err)
			}
			req.NodeUIDs = nil
			if err := svc.Update(collection.ID, req); err == nil {
				t.Fatal("empty source was accepted")
			}
			req.NodeUIDs = []string{"direct"}
			req.Type = "urltest"
			if err := svc.Update(collection.ID, req); err == nil || !strings.Contains(err.Error(), "自动测速") {
				t.Fatalf("automatic direct update error = %v", err)
			}
			legacy := *collection
			legacy.Type = "urltest"
			if _, err := (&ConfigGeneratorService{}).generateCollectionOutbound(&legacy, nil, nil, nil); err == nil {
				t.Fatal("legacy automatic direct selection was generated")
			}
			req.RouteRuleID = createHealthTestProxyRule(t, db, "Automatic")
			if _, err := svc.Create(req); err == nil || !strings.Contains(err.Error(), "自动测速") {
				t.Fatalf("automatic direct create error = %v", err)
			}
			unchanged, err := svc.Get(collection.ID)
			if err != nil || unchanged.Type != "selector" || !reflect.DeepEqual(unchanged.NodeUIDs, []string{"direct"}) {
				t.Fatalf("failed validation changed saved source: %+v, err=%v", unchanged, err)
			}
			groups, err := db.ListNodeGroups()
			if err != nil || len(groups) != 0 {
				t.Fatalf("built-in direct created a database node group: %+v, err=%v", groups, err)
			}
		})
	}
}

func TestCollectionDirectChoiceDoesNotAffectProxyDNS(t *testing.T) {
	for _, source := range []string{proxyCollectionSourceNodeGroups, proxyCollectionSourceNodeGroupsAndNodes, proxyCollectionSourceManual} {
		t.Run(source, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.SetProxyMode("rule"); err != nil {
				t.Fatal(err)
			}
			sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "Synthetic", URL: "https://subscription.example/list"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.UpsertSubscriptionNodes(sub.ID, []model.ParsedNode{{
				UID: "node-1", Name: "Synthetic Node", Type: "socks", Server: "192.0.2.1", ServerPort: 1080,
				RawJSON: `{"type":"socks","server":"192.0.2.1","server_port":1080}`,
			}}); err != nil {
				t.Fatal(err)
			}
			req := model.ProxyCollectionRequest{RouteRuleID: createHealthTestProxyRule(t, db, "Business"), Type: "selector", SourceType: source, NodeUIDs: []string{"direct"}, Enabled: true}
			if source == proxyCollectionSourceManual {
				req.NodeUIDs = append(req.NodeUIDs, "node-1")
			} else {
				group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "Region", Type: "selector", NodeUIDs: []string{"node-1"}, Enabled: true})
				if err != nil {
					t.Fatal(err)
				}
				req.ReferencedGroupIDs = []int64{group.ID}
			}
			if _, err := NewProxyCollectionService(db, nil).Create(req); err != nil {
				t.Fatal(err)
			}
			if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_remote", Enabled: true, ServerType: "udp", Address: "198.51.100.53"}); err != nil {
				t.Fatal(err)
			}
			generator := NewConfigGeneratorService(db, nil)
			outbounds, endpoints, err := generator.generateOutbounds(true)
			if err != nil {
				t.Fatal(err)
			}
			if !hasDirectOutboundPath(outbounds, endpoints, "Business") {
				t.Fatal("business strategy is missing its explicit direct choice")
			}
			if hasDirectOutboundPath(outbounds, endpoints, "proxy") || !hasUsableNonDirectOutboundPath(outbounds, endpoints, []string{"proxy"}) {
				t.Fatal("proxy DNS can reach a business direct choice or has lost its proxy path")
			}
			for _, raw := range outbounds {
				outbound := raw.(map[string]interface{})
				if outbound["tag"] == "Business" {
					members := outboundMemberTags(outbound["outbounds"])
					if members[0] == "direct" || members[len(members)-1] != "direct" {
						t.Fatalf("direct was not appended as an explicit opt-in: %v", members)
					}
				}
			}
			dns, err := generator.generateDNSFromDatabase("proxy")
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDNSDetourPaths(dns, outbounds, endpoints); err != nil {
				t.Fatalf("default proxy DNS path rejected: %v", err)
			}
			unsafeDNS := map[string]interface{}{"servers": []map[string]interface{}{{"tag": "unsafe", "detour": "Business"}}}
			if err := validateDNSDetourPaths(unsafeDNS, outbounds, endpoints); err == nil || !strings.Contains(err.Error(), "包含直连选项") {
				t.Fatalf("business selector was accepted for proxy DNS: %v", err)
			}
		})
	}
}

func TestHasDirectOutboundPathHandlesAliasesAndCycles(t *testing.T) {
	outbounds := []interface{}{
		map[string]interface{}{"tag": "alias", "type": "direct"},
		map[string]interface{}{"tag": "node", "type": "socks"},
		map[string]interface{}{"tag": "business", "type": "selector", "outbounds": []interface{}{"nested", "node"}},
		map[string]interface{}{"tag": "nested", "type": "selector", "outbounds": []string{"business", "alias"}},
	}
	if !hasDirectOutboundPath(outbounds, nil, "business") {
		t.Fatal("nested direct alias was missed")
	}
	if hasDirectOutboundPath(outbounds, nil, "node") || hasDirectOutboundPath(outbounds, nil, "missing") {
		t.Fatal("non-direct or missing outbound treated as direct")
	}
}

func TestCollectionDirectChoiceDoesNotHideMissingProxyMembers(t *testing.T) {
	for _, source := range []string{proxyCollectionSourceNodeGroups, proxyCollectionSourceNodeGroupsAndNodes, proxyCollectionSourceManual} {
		t.Run(source, func(t *testing.T) {
			collection := &model.ProxyCollectionWithNodes{
				ProxyCollection: model.ProxyCollection{Name: "Business", Type: "selector", SourceType: source},
				NodeUIDs:        []string{"direct"},
			}
			if source == proxyCollectionSourceManual {
				collection.NodeUIDs = append(collection.NodeUIDs, "missing-node")
			} else {
				collection.ReferencedGroups = []model.NodeGroup{{ID: 1, Name: "Empty Region"}}
			}
			if _, err := (&ConfigGeneratorService{}).generateCollectionOutbound(collection, nil, nil, nil); err == nil {
				t.Fatal("missing proxy members silently became direct-only")
			}
		})
	}
}
