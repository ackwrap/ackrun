package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestCollectionHealthCheckUsesClashAPIAndPersistsResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/proxies/Node-1-node-1/delay") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("url"); got != "https://example.com/generate_204" {
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
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "test", URL: "https://example.com/sub", SyncMode: "off"})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	if err := db.UpsertSubscriptionNodes(subscription.ID, []model.ParsedNode{{UID: "node-1", Name: "Node 1", Type: "socks", Server: "127.0.0.1", ServerPort: 1080, RawJSON: `{}`}}); err != nil {
		t.Fatalf("seed node: %v", err)
	}

	svc := NewProxyCollectionService(db, nil)
	svc.clashBaseURL = server.URL
	collection, err := svc.Create(model.ProxyCollectionRequest{
		Name: "Auto", Type: "urltest", SourceType: "manual", NodeUIDs: []string{"node-1"},
		TestURL: "https://example.com/generate_204", TestInterval: 300, Tolerance: 100, Enabled: true,
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
