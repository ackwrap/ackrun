package service

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestNodeExposureServiceValidatesAuthenticationAndListenerConflicts(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)

	created, err := svc.Create(model.NodeExposureRequest{
		Name:           "local socks",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "socks",
		Listen:         "127.0.0.1",
		ListenPort:     18080,
		Enabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.HasPassword || created.Password != "" {
		t.Fatalf("unexpected password state: %+v", created)
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"password"`) {
		t.Fatalf("API response exposed password metadata: %s", encoded)
	}

	_, err = svc.Create(model.NodeExposureRequest{
		Name:           "unsafe remote",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "http",
		Listen:         "0.0.0.0",
		ListenPort:     18081,
		Enabled:        true,
	})
	if !errors.Is(err, ErrNodeExposureInvalid) || !strings.Contains(err.Error(), "非回环地址") {
		t.Fatalf("remote listener error = %v", err)
	}

	_, err = svc.Create(model.NodeExposureRequest{
		Name:           "blank password",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "socks",
		Listen:         "127.0.0.1",
		ListenPort:     18083,
		Password:       " ",
		Enabled:        true,
	})
	if !errors.Is(err, ErrNodeExposureInvalid) || !strings.Contains(err.Error(), "空白字符") {
		t.Fatalf("blank password error = %v", err)
	}

	_, err = svc.Create(model.NodeExposureRequest{
		Name:           "reserved",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "mixed",
		Listen:         "127.0.0.1",
		ListenPort:     model.DefaultMixedInboundPort,
		Enabled:        true,
	})
	if !errors.Is(err, ErrNodeExposureConflict) {
		t.Fatalf("reserved port error = %v", err)
	}

	_, err = svc.Create(model.NodeExposureRequest{
		Name:           "overlap",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "mixed",
		Listen:         "0.0.0.0",
		ListenPort:     created.ListenPort,
		Username:       "test-user",
		Password:       "test-password",
		Enabled:        true,
	})
	if !errors.Is(err, ErrNodeExposureConflict) {
		t.Fatalf("overlapping listener error = %v", err)
	}
}

func TestNodeExposureServiceSerializesOverlappingListeners(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)
	requests := []model.NodeExposureRequest{
		{
			Name: "wildcard", SubscriptionID: subscriptionID, NodeUID: nodeUID,
			InboundType: "mixed", Listen: "0.0.0.0", ListenPort: 18090,
			Username: "test-user", Password: "test-password", Enabled: true,
		},
		{
			Name: "loopback", SubscriptionID: subscriptionID, NodeUID: nodeUID,
			InboundType: "mixed", Listen: "127.0.0.1", ListenPort: 18090, Enabled: true,
		},
	}
	start := make(chan struct{})
	errorsByRequest := make([]error, len(requests))
	var wg sync.WaitGroup
	for index := range requests {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			_, errorsByRequest[index] = svc.Create(requests[index])
		}(index)
	}
	close(start)
	wg.Wait()

	succeeded, conflicted := 0, 0
	for _, err := range errorsByRequest {
		switch {
		case err == nil:
			succeeded++
		case errors.Is(err, ErrNodeExposureConflict):
			conflicted++
		default:
			t.Fatalf("unexpected create error: %v", err)
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("succeeded = %d, conflicted = %d, errors = %+v", succeeded, conflicted, errorsByRequest)
	}
}

func TestNodeExposureMutationRollsBackWhenRuntimeApplyFails(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)
	created, err := svc.Create(model.NodeExposureRequest{
		Name: "original", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18100, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	runtime := &nodeExposureRuntimeStub{upsertErrors: []error{errors.New("apply failed")}}
	svc.SetRuntimeDependencies(runtime, &nodeExposureCoreStub{running: true})
	_, err = svc.Update(created.ID, model.NodeExposureRequest{
		Name: "changed", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "http", Listen: "127.0.0.1", ListenPort: 18101, Enabled: true,
	})
	if !errors.Is(err, ErrNodeExposureApply) {
		t.Fatalf("update error = %v", err)
	}
	restored, err := db.GetNodeExposure(created.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored.Name != "original" || restored.InboundType != "socks" || restored.ListenPort != 18100 {
		t.Fatalf("update rollback = %+v", restored)
	}
	if len(runtime.upserts) != 2 || runtime.upserts[0].Name != "changed" || runtime.upserts[1].Name != "original" {
		t.Fatalf("runtime was not restored after update failure: %+v", runtime.upserts)
	}

	runtime.deleteErrors = []error{errors.New("delete failed")}
	if err := svc.Delete(created.ID); !errors.Is(err, ErrNodeExposureApply) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := db.GetNodeExposure(created.ID); err != nil {
		t.Fatalf("delete rollback did not restore exposure: %v", err)
	}
	if len(runtime.upserts) != 3 || runtime.upserts[2].Name != "original" {
		t.Fatalf("runtime was not restored after delete failure: %+v", runtime.upserts)
	}
}

func TestNodeExposureCreateRestoresRuntimeAfterApplyFailure(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	runtime := &nodeExposureRuntimeStub{upsertErrors: []error{errors.New("runtime failed")}}
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(runtime, &nodeExposureCoreStub{running: true})

	_, err := svc.Create(model.NodeExposureRequest{
		Name: "restart rollback", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18110, Enabled: true,
	})
	if !errors.Is(err, ErrNodeExposureApply) {
		t.Fatalf("create error = %v", err)
	}
	items, err := db.ListNodeExposures()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("failed create was not rolled back: %+v", items)
	}
	if len(runtime.deletes) != 1 {
		t.Fatalf("partial runtime create was not cleaned up: %+v", runtime.deletes)
	}
}

func TestNodeExposureApplyErrorRedactsPassword(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	const password = "test-password-value"
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(
		&nodeExposureRuntimeStub{upsertErrors: []error{errors.New("runtime rejected " + password)}},
		&nodeExposureCoreStub{running: true},
	)

	_, err := svc.Create(model.NodeExposureRequest{
		Name: "redaction", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18111,
		Username: "test-user", Password: password, Enabled: true,
	})
	if !errors.Is(err, ErrNodeExposureApply) {
		t.Fatalf("create error = %v", err)
	}
	if strings.Contains(err.Error(), password) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("apply error exposed password: %v", err)
	}
}

func TestNodeExposureDefersRuntimeApplyWhileCoreIsStopped(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	runtime := &nodeExposureRuntimeStub{}
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(runtime, &nodeExposureCoreStub{})

	if _, err := svc.Create(model.NodeExposureRequest{
		Name: "stopped core", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18112, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if len(runtime.upserts) != 0 || len(runtime.deletes) != 0 {
		t.Fatalf("stopped core received runtime mutations: upserts=%d deletes=%d", len(runtime.upserts), len(runtime.deletes))
	}
}

func TestNodeExposureSyncRuntimeUsesStoredItems(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)
	_, err := svc.Create(model.NodeExposureRequest{
		Name:           "sync item",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "socks",
		Listen:         "127.0.0.1",
		ListenPort:     18082,
		Enabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime := &nodeExposureRuntimeStub{}
	svc.SetRuntimeDependencies(runtime, &nodeExposureCoreStub{running: true})
	if err := svc.SyncRuntime(); err != nil {
		t.Fatal(err)
	}
	if len(runtime.synced) != 1 || runtime.synced[0].Name != "sync item" {
		t.Fatalf("synced items = %+v", runtime.synced)
	}
}

func TestNodeExposureSyncRuntimeRedactsStoredPassword(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	const password = "sync-password-value"
	svc := NewNodeExposureService(db)
	if _, err := svc.Create(model.NodeExposureRequest{
		Name: "sync redaction", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18113,
		Username: "test-user", Password: password, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	runtime := &nodeExposureRuntimeStub{syncErr: errors.New("runtime rejected " + password)}
	svc.SetRuntimeDependencies(runtime, &nodeExposureCoreStub{running: true})
	err := svc.SyncRuntime()
	if err == nil || strings.Contains(err.Error(), password) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("sync error was not redacted: %v", err)
	}
}

func createNodeExposureTestData(t *testing.T) (*store.Store, int64, string) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "test subscription",
		URL:  "https://subscription.invalid/test",
	})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	node := model.ParsedNode{
		Name:       "test node",
		Type:       "socks",
		Server:     "192.0.2.10",
		ServerPort: 1080,
		RawJSON:    `{"type":"socks","server":"192.0.2.10","server_port":1080}`,
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{node}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		db.Close()
		t.Fatalf("nodes = %+v, err = %v", nodes, err)
	}
	return db, subscription.ID, nodes[0].UID
}

type nodeExposureRuntimeStub struct {
	upserts      []model.NodeExposureWithNode
	deletes      []int64
	synced       []model.NodeExposureWithNode
	upsertErrors []error
	deleteErrors []error
	syncErr      error
}

func (stub *nodeExposureRuntimeStub) Sync(items []model.NodeExposureWithNode) error {
	stub.synced = append([]model.NodeExposureWithNode(nil), items...)
	return stub.syncErr
}

func (stub *nodeExposureRuntimeStub) Upsert(item model.NodeExposureWithNode) error {
	stub.upserts = append(stub.upserts, item)
	if len(stub.upsertErrors) > 0 {
		err := stub.upsertErrors[0]
		stub.upsertErrors = stub.upsertErrors[1:]
		return err
	}
	return nil
}

func (stub *nodeExposureRuntimeStub) Delete(id int64) error {
	stub.deletes = append(stub.deletes, id)
	if len(stub.deleteErrors) > 0 {
		err := stub.deleteErrors[0]
		stub.deleteErrors = stub.deleteErrors[1:]
		return err
	}
	return nil
}

type nodeExposureCoreStub struct {
	running bool
}

func (stub *nodeExposureCoreStub) IsRunning() bool {
	return stub.running
}
