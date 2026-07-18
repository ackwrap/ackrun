package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestImportManualNodes(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := NewNodeService(db)
	resp, err := svc.Import(model.NodeImportRequest{Content: "ss://aes-128-gcm:pass@example.com:8388#Manual-SS"})
	if err != nil {
		t.Fatalf("import nodes: %v", err)
	}
	if resp.Imported != 1 || resp.SubscriptionID == 0 {
		t.Fatalf("unexpected import response: %+v", resp)
	}
	nodes, err := db.ListNodesBySubscription(resp.SubscriptionID)
	if err != nil {
		t.Fatalf("list imported nodes: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Name != "Manual-SS" || nodes[0].UID == "" {
		t.Fatalf("unexpected imported nodes: %+v", nodes)
	}
	manual, err := db.GetSubscription(resp.SubscriptionID)
	if err != nil {
		t.Fatalf("get manual subscription: %v", err)
	}
	if manual == nil || manual.NodeCount != 1 {
		t.Fatalf("expected manual subscription node_count=1, got %+v", manual)
	}

	resp, err = svc.Import(model.NodeImportRequest{Content: "ss://aes-128-gcm:pass@example.com:8388#Manual-SS-Updated"})
	if err != nil {
		t.Fatalf("import duplicate node: %v", err)
	}
	if resp.Imported != 1 {
		t.Fatalf("unexpected duplicate import response: %+v", resp)
	}
	nodes, err = db.ListNodesBySubscription(resp.SubscriptionID)
	if err != nil {
		t.Fatalf("list duplicate imported nodes: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("expected upsert to keep 1 node, got %+v", nodes)
	}
}

func TestInferNodeEmojiRecognizesDelimitedRegionCode(t *testing.T) {
	if got := inferNodeEmoji(model.Node{Name: "HK Li 香港07 | 倍率:1.5"}); got != "🇭🇰" {
		t.Fatalf("flag = %q, want Hong Kong", got)
	}
	if got := inferNodeEmoji(model.Node{Name: "SHK-node"}); got != "" {
		t.Fatalf("embedded region code flag = %q, want empty", got)
	}
}

func TestManualImportFiltersUnsupportedClashVariants(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	content := `proxies:
  - name: Valid-AnyTLS
    type: anytls
    server: valid.example.com
    port: 443
    password: redacted
  - name: Unsupported-XHTTP
    type: vless
    server: unsupported.example.com
    port: 443
    uuid: 33333333-3333-4333-8333-333333333333
    network: xhttp
`
	svc := NewNodeService(db)
	preview, err := svc.ImportPreview(model.NodeImportRequest{Content: content})
	if err != nil {
		t.Fatalf("preview nodes: %v", err)
	}
	if preview.Count != 1 || len(preview.Items) != 1 || preview.Items[0].Type != "anytls" {
		t.Fatalf("unexpected preview items: %+v", preview.Items)
	}

	response, err := svc.Import(model.NodeImportRequest{Content: content})
	if err != nil {
		t.Fatalf("import nodes: %v", err)
	}
	nodes, err := db.ListNodesBySubscription(response.SubscriptionID)
	if err != nil {
		t.Fatalf("list imported nodes: %v", err)
	}
	if response.Imported != 1 || len(nodes) != 1 || nodes[0].Type != "anytls" {
		t.Fatalf("unsupported variant reached manual subscription: response=%+v nodes=%+v", response, nodes)
	}
}
