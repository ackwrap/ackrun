package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/parser"
	"github.com/ackwrap/ackrun/internal/store"
)

func chainTestStore(t *testing.T) (*store.Store, int64, []model.ParsedNode) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "Chain-Test", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := parser.ParseSubscriptionNodes([]byte(clashCompatibilitySubscription))
	if err != nil || len(nodes) != 4 {
		t.Fatalf("unexpected parsed test fixture: count=%d err=%v", len(nodes), err)
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, nodes); err != nil {
		t.Fatal(err)
	}
	return db, sub.ID, nodes
}

func TestGenerateOutboundsPreservesSubscriptionChainAfterRename(t *testing.T) {
	db, subID, parsed := chainTestStore(t)
	if err := db.UpdateNodeName(parsed[1].UID, "Renamed Front"); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(subID)
	if err != nil {
		t.Fatal(err)
	}
	tags := buildNodeOutboundTags(nodes)
	outbounds, _, err := NewConfigGeneratorService(db, nil).generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	byTag := map[string]map[string]interface{}{}
	groupCount := 0
	for _, value := range outbounds {
		outbound := value.(map[string]interface{})
		tag, _ := outbound["tag"].(string)
		byTag[tag] = outbound
		if _, leaked := outbound[model.NodeChainMetadataKey]; leaked {
			t.Fatal("internal chain metadata leaked into the core configuration")
		}
		if strings.HasPrefix(tag, "ackwrap-chain-") {
			groupCount++
		}
	}
	exit := byTag[tags[parsed[3].UID]]
	groupTag, _ := exit["detour"].(string)
	group := byTag[groupTag]
	if groupCount != 1 || group == nil || group["type"] != "selector" {
		t.Fatal("the HTTP node must detour through its imported selector")
	}
	members, _ := group["outbounds"].([]string)
	if len(members) != 1 || members[0] != tags[parsed[1].UID] {
		t.Fatal("the selector did not follow the renamed upstream node by UID")
	}
}

func TestGenerateOutboundsRejectsDisabledChainDependencyAcrossSubscriptions(t *testing.T) {
	db, _, parsed := chainTestStore(t)
	other, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "Other", URL: "https://example.com/other"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(other.ID, []model.ParsedNode{parsed[1]}); err != nil {
		t.Fatal(err)
	}
	// Disable only the first subscription's upstream, leaving an identical
	// connection in another subscription. It must not be silently substituted.
	if _, err := db.DB().Exec("UPDATE nodes SET enabled = 0 WHERE subscription_id <> ? AND uid = ?", other.ID, parsed[1].UID); err != nil {
		t.Fatal(err)
	}
	_, _, err = NewConfigGeneratorService(db, nil).generateOutbounds()
	if err == nil || !strings.Contains(err.Error(), "代理链依赖") {
		t.Fatalf("expected a missing/disabled dependency error, got %v", err)
	}
}

func TestChainedSubscriptionSyncPreservesUIDAndNodeState(t *testing.T) {
	db, subID, parsed := chainTestStore(t)
	uid := parsed[3].UID
	if err := db.SetNodeEnabled(uid, false); err != nil {
		t.Fatal(err)
	}
	if err := db.SetNodePreferred(uid, true); err != nil {
		t.Fatal(err)
	}
	if err := db.UpdateNodeName(uid, "Custom Chain"); err != nil {
		t.Fatal(err)
	}
	body := strings.NewReplacer("Valid-SS", "Renamed-SS", "Valid-HTTP-Chain", "Renamed-Chain", "Upstream-Group", "Renamed-Group").Replace(clashCompatibilitySubscription)
	updated, err := parser.ParseSubscriptionNodes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	if updated[3].UID != uid {
		t.Fatal("a subscription rename changed the chain's connection UID")
	}
	if err := db.ReplaceSubscriptionNodes(subID, updated); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(subID)
	if err != nil {
		t.Fatal(err)
	}
	if nodes[3].UID != uid || nodes[3].Enabled || !nodes[3].Preferred || nodes[3].Name != "Custom Chain" {
		t.Fatal("subscription sync did not preserve the chain's saved node state")
	}
}

func TestNodeFiltersAlsoExcludeDependentChains(t *testing.T) {
	db, _, parsed := chainTestStore(t)
	if _, err := db.CreateNodeFilter(&model.NodeFilterRequest{Name: "Remove upstream", Target: "name", Pattern: "^Valid-SS$", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	result := &subscriptionSyncResult{Nodes: parsed, NodeCount: len(parsed)}
	if err := NewSubscriptionService(db, nil).applyNodeFilters(result); err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 2 || len(result.ExcludedNodes) != 2 || !strings.Contains(result.ExcludedNodes[1].Reason, "代理链") {
		t.Fatal("filtering an upstream must also exclude its dependent, with a reason")
	}
	preview, err := NewNodeService(db).ImportPreview(model.NodeImportRequest{Content: clashCompatibilitySubscription})
	if err != nil || preview.Count != 2 {
		t.Fatalf("manual preview must apply the same dependency filtering: err=%v", err)
	}
}

func TestChainedHTTPProbeUsesCoreAndNeverFallsBackToDirectTCP(t *testing.T) {
	db, _, parsed := chainTestStore(t)
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if !strings.Contains(r.URL.Path, "/delay") {
					t.Errorf("unexpected core API path: %s", r.URL.Path)
				}
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"delay":42}`))
			}))
			defer server.Close()
			svc := NewNodeService(db)
			svc.clashBaseURL, svc.httpClient = server.URL, server.Client()
			results, err := svc.TCPing([]string{parsed[3].UID})
			if err != nil || len(results) != 1 || calls != 1 || results[0].Success != (status == http.StatusOK) {
				t.Fatalf("chained HTTP probe must use the core path: calls=%d err=%v", calls, err)
			}
		})
	}
}

func TestGeneratedChainsUseDifferentUIDsForDifferentUpstreams(t *testing.T) {
	first, _ := parser.ParseSubscriptionNodes([]byte(clashCompatibilitySubscription))
	second, _ := parser.ParseSubscriptionNodes([]byte(strings.ReplaceAll(clashCompatibilitySubscription, "test-password", "second-password")))
	if first[3].UID == second[3].UID {
		t.Fatal("identical HTTP credentials with different upstreams must not share a UID")
	}
	var options map[string]any
	_ = json.Unmarshal([]byte(first[3].RawJSON), &options)
	chain, err := model.DecodeNodeChain(options)
	if err != nil || chain == nil || len(chain.Groups) != 1 {
		t.Fatal("expected persisted UID-based chain metadata")
	}
}
