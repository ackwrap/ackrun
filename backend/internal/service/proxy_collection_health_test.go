package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestCollectionHealthCheckUsesClashAPIAndPersistsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/proxies/Node-1-node-1/delay") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("url"); got != "http://connectivity.example/generate_204" {
			t.Fatalf("unexpected test URL: %s", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delay":123}`))
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if err := db.SetConnectivitySettings(&model.ConnectivitySettings{TestURL: "http://connectivity.example/generate_204", IntervalSeconds: 120}); err != nil {
		t.Fatalf("set connectivity settings: %v", err)
	}
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://example.com/sub", SyncMode: "off"})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := db.UpsertSubscriptionNodes(subscription.ID, []model.ParsedNode{{UID: "node-1", Name: "Node 1", Type: "socks", Server: "127.0.0.1", ServerPort: 1080, RawJSON: `{}`}}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	svc := NewProxyCollectionService(db, nil)
	svc.clashBaseURL = server.URL
	routeRuleID := createHealthTestProxyRule(t, db, "Auto")
	collection, err := svc.Create(model.ProxyCollectionRequest{
		Name: "Auto", RouteRuleID: routeRuleID, Type: "urltest", SourceType: "manual", NodeUIDs: []string{"node-1"},
		TestURL: "https://legacy.example/generate_204", TestInterval: 300, Tolerance: 100, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	result, err := svc.Test(collection.ID)
	if err != nil {
		t.Fatalf("test collection: %v", err)
	}
	if result.Tested != 1 || result.Available != 1 || result.FastestUID != "node-1" || result.FastestLatency != 123 {
		t.Fatalf("unexpected result: %+v", result)
	}
	nodes, err := db.ListNodesByUIDs([]string{"node-1"})
	if err != nil || len(nodes) != 1 {
		t.Fatalf("load node result: nodes=%+v err=%v", nodes, err)
	}
	if !nodes[0].TestSuccess || nodes[0].TestLatencyMS != 123 || nodes[0].LastTestAt == 0 {
		t.Fatalf("health result not persisted: %+v", nodes[0])
	}
}

func TestCollectionHealthSettingsValidation(t *testing.T) {
	tests := []model.ProxyCollectionRequest{
		{TestURL: "file:///tmp/test", TestInterval: 300},
		{TestURL: "https://example.com", TestInterval: 59},
		{TestURL: "https://example.com", TestInterval: 3601},
		{TestURL: "https://example.com", TestInterval: 300, Tolerance: -1},
	}
	for _, request := range tests {
		if err := normalizeCollectionHealthSettings(&request); err == nil {
			t.Fatalf("expected validation error for %+v", request)
		}
	}
	valid := model.ProxyCollectionRequest{}
	if err := normalizeCollectionHealthSettings(&valid); err != nil {
		t.Fatalf("normalize defaults: %v", err)
	}
	if valid.TestInterval != 300 || valid.TestURL == "" {
		t.Fatalf("defaults not applied: %+v", valid)
	}
}

func TestCollectionHealthCheckReportsClashFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()
	svc := &ProxyCollectionService{httpClient: server.Client(), clashBaseURL: server.URL}
	result := svc.testNode("node-1", "Node-1-node-1", "https://example.com")
	if result.Success || !strings.Contains(result.Error, "HTTP 503") {
		t.Fatalf("unexpected failure result: %+v", result)
	}
}

func TestCollectionHealthCheckExplainsMissingActiveOutbound(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	svc := &ProxyCollectionService{httpClient: server.Client(), clashBaseURL: server.URL}
	result := svc.testNode("node-1", "Node-1-node-1", "https://example.com")
	if result.Success || !strings.Contains(result.Error, "尚未载入当前运行配置") {
		t.Fatalf("unexpected missing outbound result: %+v", result)
	}
}

func TestCollectionHealthCheckUsesManualNodeGroupUIDs(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://example.com/sub", SyncMode: "off"})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := db.UpsertSubscriptionNodes(subscription.ID, []model.ParsedNode{
		{UID: "node-1", Name: "Node 1", Type: "socks", Server: "127.0.0.1", ServerPort: 1080, RawJSON: `{}`},
		{UID: "node-2", Name: "Node 2", Type: "socks", Server: "127.0.0.2", ServerPort: 1080, RawJSON: `{}`},
	}); err != nil {
		t.Fatalf("seed nodes: %v", err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "Manual", Type: "selector", NodeUIDs: []string{"node-1"}, Enabled: true})
	if err != nil {
		t.Fatalf("create node group: %v", err)
	}
	svc := NewProxyCollectionService(db, nil)
	routeRuleID := createHealthTestProxyRule(t, db, "Combined")
	collection, err := svc.Create(model.ProxyCollectionRequest{
		Name: "Combined", RouteRuleID: routeRuleID, Type: "selector", SourceType: proxyCollectionSourceNodeGroupsAndNodes,
		ReferencedGroupIDs: []int64{group.ID}, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}
	nodes, err := svc.collectionNodes(collection)
	if err != nil {
		t.Fatalf("load collection nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].UID != "node-1" {
		t.Fatalf("unexpected collection nodes: %+v", nodes)
	}
}

func TestCollectionHealthJobRefreshesDoNotLeaveDuplicateEntries(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	svc := NewProxyCollectionService(db, nil)
	routeRuleID := createHealthTestProxyRule(t, db, "Auto")
	collection, err := svc.Create(model.ProxyCollectionRequest{
		Name: "Auto", RouteRuleID: routeRuleID, Type: "urltest", SourceType: "manual", NodeUIDs: []string{"node-1"},
		TestURL: "https://example.com/generate_204", TestInterval: 300, Tolerance: 100, Enabled: true,
	})
	if err != nil {
		t.Fatalf("create collection: %v", err)
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			svc.RefreshHealthCheckJobs()
		}()
		go func() {
			defer wg.Done()
			svc.refreshHealthCheckJob(collection.ID)
		}()
	}
	wg.Wait()

	if got := len(svc.cron.Entries()); got != 1 {
		t.Fatalf("cron entries = %d, want 1", got)
	}
	svc.mu.Lock()
	tracked := len(svc.entries)
	svc.mu.Unlock()
	if tracked != 1 {
		t.Fatalf("tracked entries = %d, want 1", tracked)
	}
}

func createHealthTestProxyRule(t *testing.T, db *store.Store, name string) int64 {
	t.Helper()
	rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: name, Enabled: true, RuleType: "domain", Values: []string{strings.ToLower(name) + ".example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	return rule.ID
}
