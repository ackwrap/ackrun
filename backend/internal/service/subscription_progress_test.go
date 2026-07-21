package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRunSyncRecoversPersistedSyncingState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("socks5://user:pass@127.0.0.1:1080#Synthetic"))
	}))
	defer server.Close()

	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSubscriptionSyncState(subscription.ID, "syncing", 0); err != nil {
		t.Fatal(err)
	}

	NewSubscriptionService(db, nil).runSync(subscription.ID)
	updated, err := db.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SyncStatus != "updated" || updated.SyncProgress != 100 || updated.NodeCount != 1 {
		t.Fatalf("sync result = status %s progress %.0f nodes %d", updated.SyncStatus, updated.SyncProgress, updated.NodeCount)
	}
}

func TestResetInterruptedSubscriptionSyncs(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: "https://example.com/sub"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetSubscriptionSyncState(subscription.ID, "syncing", 30); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetInterruptedSubscriptionSyncs(); err != nil {
		t.Fatal(err)
	}
	updated, err := db.GetSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SyncStatus != "failed" || updated.SyncProgress != 0 {
		t.Fatalf("reset state = %s %.0f", updated.SyncStatus, updated.SyncProgress)
	}
}

func TestResetInterruptedRouteRuleSubscriptionSyncs(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{
		Name: "synthetic", URL: "https://example.com/rules.srs", Tag: "synthetic", Format: "binary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetRouteRuleSubscriptionSyncState(subscription.ID, "syncing", 30, ""); err != nil {
		t.Fatal(err)
	}
	if err := db.ResetInterruptedRouteRuleSubscriptionSyncs(); err != nil {
		t.Fatal(err)
	}
	updated, err := db.GetRouteRuleSubscription(subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.SyncStatus != "failed" || updated.SyncProgress != 0 || updated.SyncError == "" {
		t.Fatalf("reset state = %s %.0f %q", updated.SyncStatus, updated.SyncProgress, updated.SyncError)
	}
}
