package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestListNodesUsesStableSubscriptionAndUIDOrder(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	first, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "first",
		URL:  "https://example.com/first",
	})
	if err != nil {
		t.Fatalf("create first subscription: %v", err)
	}
	second, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "second",
		URL:  "https://example.com/second",
	})
	if err != nil {
		t.Fatalf("create second subscription: %v", err)
	}

	if err := db.ReplaceSubscriptionNodes(first.ID, []model.ParsedNode{
		{UID: "uid-z", Name: "A", Type: "vless", Server: "192.0.2.1", ServerPort: 443},
		{UID: "uid-a", Name: "Z", Type: "ssr", Server: "192.0.2.2", ServerPort: 443},
	}); err != nil {
		t.Fatalf("replace first subscription nodes: %v", err)
	}
	if err := db.ReplaceSubscriptionNodes(second.ID, []model.ParsedNode{
		{UID: "uid-b", Name: "B", Type: "trojan", Server: "192.0.2.3", ServerPort: 443},
	}); err != nil {
		t.Fatalf("replace second subscription nodes: %v", err)
	}

	assertUIDs := func(limit, offset int, want []string) {
		t.Helper()
		response, listErr := db.ListNodes(model.NodeListRequest{Limit: limit, Offset: offset})
		if listErr != nil {
			t.Fatalf("list nodes: %v", listErr)
		}
		got := make([]string, len(response.Items))
		for i, node := range response.Items {
			got[i] = node.UID
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("node order = %v, want %v", got, want)
		}
	}

	assertUIDs(50, 0, []string{"uid-a", "uid-z", "uid-b"})
	if err := db.UpdateNodeTCPing("uid-z", 8, "available"); err != nil {
		t.Fatalf("update node latency: %v", err)
	}
	if err := db.SetNodeEnabled("uid-a", false); err != nil {
		t.Fatalf("update node enabled state: %v", err)
	}
	if err := db.SetNodePreferred("uid-b", true); err != nil {
		t.Fatalf("update node preferred state: %v", err)
	}
	assertUIDs(50, 0, []string{"uid-a", "uid-z", "uid-b"})
	assertUIDs(2, 1, []string{"uid-z", "uid-b"})
}
