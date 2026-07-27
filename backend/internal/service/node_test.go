package service

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
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

func TestNodeShareUsesImportedURI(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewNodeService(db)
	original := "vless://55555555-5555-4555-8555-555555555555@proxy.example:443?security=tls#Share-Test"
	imported, err := svc.Import(model.NodeImportRequest{Content: original})
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(imported.SubscriptionID)
	if err != nil || len(nodes) != 1 {
		t.Fatalf("unexpected imported nodes: count=%d err=%v", len(nodes), err)
	}
	shared, err := svc.Share(nodes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if shared.URI != original {
		t.Fatal("share response did not preserve the original URI")
	}
	secondSubscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "Second", URL: "https://subscription.example/second"})
	if err != nil {
		t.Fatal(err)
	}
	secondURI := "vless://55555555-5555-4555-8555-555555555555@proxy.example:443?security=tls#Second-Share"
	if err := db.ReplaceSubscriptionNodes(secondSubscription.ID, []model.ParsedNode{{
		UID: nodes[0].UID, Name: "Second-Share", Type: "vless", Server: "proxy.example",
		ServerPort: 443, Raw: secondURI, RawJSON: nodes[0].RawJSON,
	}}); err != nil {
		t.Fatal(err)
	}
	secondNodes, err := db.ListNodesBySubscription(secondSubscription.ID)
	if err != nil || len(secondNodes) != 1 {
		t.Fatalf("unexpected second subscription nodes: count=%d err=%v", len(secondNodes), err)
	}
	secondShared, err := svc.Share(secondNodes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondShared.URI != secondURI {
		t.Fatal("node ID share returned another subscription's URI")
	}
	if _, err := svc.Share(nodes[0].ID + 1000); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("missing node error = %v, want ErrNodeNotFound", err)
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
