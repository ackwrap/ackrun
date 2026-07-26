package service

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
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

	generator := &nodeExposureGeneratorStub{err: errors.New("apply failed")}
	svc.SetRuntimeDependencies(generator, nil)
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

	err = svc.Delete(created.ID)
	if !errors.Is(err, ErrNodeExposureApply) {
		t.Fatalf("delete error = %v", err)
	}
	if _, err := db.GetNodeExposure(created.ID); err != nil {
		t.Fatalf("delete rollback did not restore exposure: %v", err)
	}
}

func TestNodeExposureCreateRestoresRuntimeAfterRestartFailure(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	generator := &nodeExposureGeneratorStub{}
	core := &nodeExposureCoreStub{running: true, restartErr: errors.New("restart failed")}
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(generator, core)

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
	if generator.calls != 2 || core.restartCalls != 1 || core.startCalls != 1 || !core.running {
		t.Fatalf("runtime restore: generator=%d restart=%d start=%d running=%t", generator.calls, core.restartCalls, core.startCalls, core.running)
	}
}

func TestNodeExposureApplyErrorRedactsPassword(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	const password = "test-password-value"
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(&nodeExposureGeneratorStub{err: errors.New("config rejected " + password)}, nil)

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

func TestNodeExposureReloadsCoreThatStartsDuringConfigApply(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	core := &nodeExposureCoreStub{}
	generator := &nodeExposureGeneratorStub{beforeCallback: func() {
		core.running = true
	}}
	svc := NewNodeExposureService(db)
	svc.SetRuntimeDependencies(generator, core)

	if _, err := svc.Create(model.NodeExposureRequest{
		Name: "start race", SubscriptionID: subscriptionID, NodeUID: nodeUID,
		InboundType: "socks", Listen: "127.0.0.1", ListenPort: 18112, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if core.restartCalls != 1 || !core.running {
		t.Fatalf("core was not reloaded after concurrent start: restart=%d running=%t", core.restartCalls, core.running)
	}
}

func TestGenerateNodeExposureConfigRoutesInboundToSelectedNode(t *testing.T) {
	db, subscriptionID, nodeUID := createNodeExposureTestData(t)
	defer db.Close()
	svc := NewNodeExposureService(db)
	created, err := svc.Create(model.NodeExposureRequest{
		Name:           "authenticated mixed",
		SubscriptionID: subscriptionID,
		NodeUID:        nodeUID,
		InboundType:    "mixed",
		Listen:         "127.0.0.1",
		ListenPort:     18082,
		Username:       "test-user",
		Password:       "test-password",
		Enabled:        true,
	})
	if err != nil {
		t.Fatal(err)
	}

	generator := NewConfigGeneratorService(db, &paths.Paths{})
	outbounds, endpoints, err := generator.generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	inbounds, rules, err := generator.generateNodeExposureConfig(&model.ConfigGenerateRequest{InboundPort: model.DefaultMixedInboundPort}, outbounds, endpoints)
	if err != nil {
		t.Fatal(err)
	}
	if len(inbounds) != 1 || len(rules) != 1 {
		t.Fatalf("inbounds = %d, rules = %d", len(inbounds), len(rules))
	}
	inbound := inbounds[0].(map[string]interface{})
	if inbound["tag"] != "node-exposure-in-"+jsonNumber(created.ID) || inbound["type"] != "mixed" {
		t.Fatalf("generated inbound = %+v", inbound)
	}
	users := inbound["users"].([]map[string]string)
	if len(users) != 1 || users[0]["password"] != "test-password" {
		t.Fatalf("generated users = %+v", users)
	}
	inboundTags := rules[0]["inbound"].([]string)
	if len(inboundTags) != 1 || inboundTags[0] != inbound["tag"] || rules[0]["action"] != "route" || rules[0]["outbound"] == "" {
		t.Fatalf("generated route rule = %+v", rules[0])
	}
	if !generatedConfigHasTag(outbounds, endpoints, rules[0]["outbound"].(string)) {
		t.Fatalf("route target is not a generated node tag: %+v", rules[0])
	}

	config := map[string]interface{}{"inbounds": inbounds}
	redacted := redactConfigAccessTokens(config).(map[string]interface{})
	redactedInbound := redacted["inbounds"].([]interface{})[0].(map[string]interface{})
	redactedUsers := redactedInbound["users"].([]map[string]string)
	if redactedUsers[0]["password"] != "[REDACTED]" {
		t.Fatalf("redacted users = %+v", redactedUsers)
	}
	if users[0]["password"] != "test-password" {
		t.Fatal("redaction mutated the generated configuration")
	}
	encoded, err := json.Marshal(created)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "test-password") || strings.Contains(string(encoded), `"password"`) {
		t.Fatalf("API response exposed password: %s", encoded)
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

func jsonNumber(value int64) string {
	data, _ := json.Marshal(value)
	return string(data)
}

type nodeExposureGeneratorStub struct {
	calls          int
	err            error
	beforeCallback func()
}

func (stub *nodeExposureGeneratorStub) ReconcileCurrentWithCallback(afterApply func() error) (*model.ConfigGenerateResponse, error) {
	stub.calls++
	if stub.err != nil {
		return nil, stub.err
	}
	result := &model.ConfigGenerateResponse{Valid: true}
	if afterApply != nil {
		if stub.beforeCallback != nil {
			stub.beforeCallback()
		}
		if err := afterApply(); err != nil {
			return result, err
		}
	}
	return result, nil
}

type nodeExposureCoreStub struct {
	running      bool
	restartCalls int
	startCalls   int
	restartErr   error
}

func (stub *nodeExposureCoreStub) ApplyConfigRuntime(startIfStopped bool) (bool, error) {
	wasRunning := stub.running
	if stub.running {
		stub.restartCalls++
		if stub.restartErr != nil {
			stub.running = false
			err := stub.restartErr
			stub.restartErr = nil
			return true, err
		}
		return true, nil
	}
	if startIfStopped {
		stub.startCalls++
		stub.running = true
	}
	return wasRunning, nil
}
