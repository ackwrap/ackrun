package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

const clashCompatibilitySubscription = `proxies:
  - name: Valid-Hysteria
    type: hysteria
    server: hy.example.com
    port: 443
    password: redundant-password
    auth-str: test-auth
    up: 10
    down: 50
  - name: Valid-SS
    type: ss
    server: ss.example.com
    port: 8388
    cipher: aes-256-gcm
    password: test-password
    tls: false
    skip-cert-verify: true
  - name: Valid-HTTP
    type: http
    server: http.example.com
    port: 8080
  - name: Unsupported-HTTP-Chain
    type: http
    server: chained.example.com
    port: 8080
    dialer-proxy: Upstream-Group
proxy-groups:
  - name: Upstream-Group
    type: select
    proxies: [Valid-SS]
`

func TestFetchAndParseClashCompatibility(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(clashCompatibilitySubscription))
	}))
	defer server.Close()
	result, err := NewSubscriptionService(nil, nil).fetchAndParse(server.URL, "clash-meta/2.4.0", 5)
	if err != nil {
		t.Fatal(err)
	}
	if result.NodeCount != 3 || len(result.Nodes) != 3 {
		t.Fatalf("expected three standalone nodes, got %d", result.NodeCount)
	}
	if result.UnsupportedCount["http"] != 1 || len(result.ExcludedNodes) != 1 || !strings.Contains(result.ExcludedNodes[0].Reason, "dialer-proxy") {
		t.Fatalf("expected an observable chain dependency rejection, got %+v", result.ExcludedNodes)
	}
	for index, wantType := range []string{"hysteria", "shadowsocks", "http"} {
		node := result.Nodes[index]
		if node.Type != wantType {
			t.Fatalf("node %d type = %q, want %q", index, node.Type, wantType)
		}
		outbound, err := (&ConfigGeneratorService{}).generateNodeOutbound(&model.Node{Type: node.Type, RawJSON: node.RawJSON}, "test-outbound", nil)
		if err != nil {
			t.Fatal(err)
		}
		if node.Type == "hysteria" && (outbound["password"] != nil || outbound["auth_str"] != "test-auth") {
			t.Fatal("Hysteria v1 generated incompatible authentication fields")
		}
	}
}

func TestManualImportAndPreviewClashCompatibility(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewNodeService(db)
	request := model.NodeImportRequest{Content: clashCompatibilitySubscription}
	preview, err := svc.ImportPreview(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := svc.Import(request)
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(response.SubscriptionID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Count != 3 || len(preview.Items) != 3 || response.Imported != 3 || len(nodes) != 3 {
		t.Fatalf("preview/import counts differ: preview=%d imported=%d stored=%d", preview.Count, response.Imported, len(nodes))
	}
	for index, wantName := range []string{"Valid-Hysteria", "Valid-SS", "Valid-HTTP"} {
		if preview.Items[index].Name != wantName || nodes[index].Name != wantName || preview.Items[index].UID != nodes[index].UID {
			t.Fatalf("preview and import differ for node %d", index)
		}
	}
}

func TestFetchAndParseChainedOnlySubscriptionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`proxies:
  - name: Unsupported-HTTP-Chain
    type: http
    server: chained.example.com
    port: 8080
    dialer-proxy: Upstream-Group
`))
	}))
	defer server.Close()
	_, err := NewSubscriptionService(nil, nil).fetchAndParse(server.URL, "clash-meta/2.4.0", 5)
	if err == nil || !strings.Contains(err.Error(), "no supported nodes after filtering") {
		t.Fatalf("expected a fully filtered subscription to fail, got %v", err)
	}
}
