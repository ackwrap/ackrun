package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

type configGeneratorCoreStub struct {
	mu             sync.Mutex
	reloadCalls    int
	scheduledCalls int
	reloadStarted  chan struct{}
	releaseReload  chan struct{}
}

func (stub *configGeneratorCoreStub) ReloadConfig() (*model.ActionResponse, error) {
	stub.mu.Lock()
	stub.reloadCalls++
	started := stub.reloadStarted
	release := stub.releaseReload
	stub.mu.Unlock()
	if started != nil {
		select {
		case started <- struct{}{}:
		default:
		}
	}
	if release != nil {
		<-release
	}
	return &model.ActionResponse{Success: true}, nil
}

func (stub *configGeneratorCoreStub) ScheduledRestart() (*model.ActionResponse, error) {
	stub.mu.Lock()
	stub.scheduledCalls++
	stub.mu.Unlock()
	return &model.ActionResponse{Success: true}, nil
}

func (stub *configGeneratorCoreStub) calls() (int, int) {
	stub.mu.Lock()
	defer stub.mu.Unlock()
	return stub.reloadCalls, stub.scheduledCalls
}

func TestScheduledRestartRegeneratesAndAppliesLatestConfig(t *testing.T) {
	core := &configGeneratorCoreStub{}
	svc, db, p, _ := newScheduledRestartConfigGenerator(t, core)
	beforePath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok {
		t.Fatalf("initial active config = %q, ok=%t, err=%v", beforePath, ok, err)
	}
	before, err := os.ReadFile(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: true}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.ReconcileCurrentForScheduledRestart(svc.CoreRestartGeneration())
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("generated config is invalid: %s", result.Error)
	}
	afterPath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok {
		t.Fatalf("updated active config = %q, ok=%t, err=%v", afterPath, ok, err)
	}
	after, err := os.ReadFile(afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) == string(before) {
		t.Fatal("active config did not change after database settings changed")
	}
	reloads, scheduled := core.calls()
	if reloads != 0 || scheduled != 1 {
		t.Fatalf("core calls = reload:%d scheduled:%d, want reload:0 scheduled:1", reloads, scheduled)
	}
}

func TestScheduledRestartKeepsActiveConfigWhenGenerationIsInvalid(t *testing.T) {
	core := &configGeneratorCoreStub{}
	svc, _, p, binaryPath := newScheduledRestartConfigGenerator(t, core)
	beforePath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok {
		t.Fatalf("initial active config = %q, ok=%t, err=%v", beforePath, ok, err)
	}
	before, err := os.ReadFile(beforePath)
	if err != nil {
		t.Fatal(err)
	}
	writeConfigCheckBinary(t, binaryPath, false)

	result, err := svc.ReconcileCurrentForScheduledRestart(svc.CoreRestartGeneration())
	if err != nil {
		t.Fatal(err)
	}
	if result.Valid {
		t.Fatal("generated config unexpectedly passed validation")
	}
	afterPath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok {
		t.Fatalf("active config after failure = %q, ok=%t, err=%v", afterPath, ok, err)
	}
	after, err := os.ReadFile(afterPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("active config changed after validation failure")
	}
	reloads, scheduled := core.calls()
	if reloads != 0 || scheduled != 0 {
		t.Fatalf("core calls = reload:%d scheduled:%d, want no restart", reloads, scheduled)
	}
}

func TestScheduledRestartCoalescesConcurrentConfigReload(t *testing.T) {
	core := &configGeneratorCoreStub{
		reloadStarted: make(chan struct{}, 1),
		releaseReload: make(chan struct{}),
	}
	svc, db, _, _ := newScheduledRestartConfigGenerator(t, core)
	if err := db.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: true}); err != nil {
		t.Fatal(err)
	}
	reconcileDone := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileCurrent()
		reconcileDone <- err
	}()
	select {
	case <-core.reloadStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for concurrent config reload")
	}

	observedRestartGeneration := svc.CoreRestartGeneration()
	close(core.releaseReload)
	if err := <-reconcileDone; err != nil {
		t.Fatal(err)
	}
	if svc.CoreRestartGeneration() <= observedRestartGeneration {
		t.Fatalf("restart generation = %d, want greater than %d", svc.CoreRestartGeneration(), observedRestartGeneration)
	}
	if _, err := svc.ReconcileCurrentForScheduledRestart(observedRestartGeneration); err != nil {
		t.Fatal(err)
	}
	reloads, scheduled := core.calls()
	if reloads != 1 || scheduled != 0 {
		t.Fatalf("core calls = reload:%d scheduled:%d, want one coalesced reload", reloads, scheduled)
	}
}

func TestScheduledRestartWaitsForNodeUpdateAndUsesAddedAndRemovedNodes(t *testing.T) {
	core := &configGeneratorCoreStub{}
	svc, db, p, _ := newScheduledRestartConfigGenerator(t, core)
	releaseUpdate := db.HoldConfigUpdate()
	firstDone := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileCurrentForScheduledRestart(svc.CoreRestartGeneration())
		firstDone <- err
	}()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "node-sync", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{
		{Name: "Node A", Type: "socks", Server: "node-a.example.com", ServerPort: 1080, RawJSON: `{"type":"socks","server":"node-a.example.com","server_port":1080}`},
		{Name: "Node B", Type: "socks", Server: "node-b.example.com", ServerPort: 1080, RawJSON: `{"type":"socks","server":"node-b.example.com","server_port":1080}`},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "All Nodes", Type: "selector", FilterInclude: ".*", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	releaseUpdate()
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	firstConfig := readActiveConfigForRestartTest(t, p)
	if !strings.Contains(firstConfig, "node-a.example.com") || !strings.Contains(firstConfig, "node-b.example.com") {
		t.Fatal("active config does not contain all nodes added by the completed update")
	}

	releaseUpdate = db.HoldConfigUpdate()
	secondDone := make(chan error, 1)
	go func() {
		_, err := svc.ReconcileCurrentForScheduledRestart(svc.CoreRestartGeneration())
		secondDone <- err
	}()
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{
		{Name: "Node B", Type: "socks", Server: "node-b.example.com", ServerPort: 1080, RawJSON: `{"type":"socks","server":"node-b.example.com","server_port":1080}`},
		{Name: "Node C", Type: "socks", Server: "node-c.example.com", ServerPort: 1080, RawJSON: `{"type":"socks","server":"node-c.example.com","server_port":1080}`},
	}); err != nil {
		t.Fatal(err)
	}
	releaseUpdate()
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	secondConfig := readActiveConfigForRestartTest(t, p)
	if strings.Contains(secondConfig, "node-a.example.com") {
		t.Fatal("active config still contains the removed node")
	}
	if !strings.Contains(secondConfig, "node-b.example.com") || !strings.Contains(secondConfig, "node-c.example.com") {
		t.Fatal("active config does not contain the latest retained and added nodes")
	}
	reloads, scheduled := core.calls()
	if reloads != 0 || scheduled != 2 {
		t.Fatalf("core calls = reload:%d scheduled:%d, want one restart per completed node update", reloads, scheduled)
	}
}

func TestSubscriptionSyncAndScheduledRestartShareOneCoreLifecycle(t *testing.T) {
	core := &configGeneratorCoreStub{}
	configGenerator, db, p, _ := newScheduledRestartConfigGenerator(t, core)
	fetchStarted := make(chan struct{})
	releaseFetch := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(fetchStarted)
		<-releaseFetch
		_, _ = w.Write([]byte("socks5://node-sync.example.com:1080#Node-Sync\n"))
	}))
	defer server.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "concurrent-sync", URL: server.URL, SyncTimeoutSecs: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateNodeGroup(&model.NodeGroupRequest{Name: "All Nodes", Type: "selector", FilterInclude: ".*", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	reconciler := newConfigReconcileService(configGenerator, nil, 5*time.Millisecond)
	subscriptions := NewSubscriptionService(db, nil)
	subscriptions.SetConfigReconciler(reconciler)
	syncDone := make(chan struct{})
	go func() {
		subscriptions.runSync(subscription.ID)
		close(syncDone)
	}()
	select {
	case <-fetchStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription fetch")
	}

	scheduledDone := make(chan error, 1)
	observedRestartGeneration := configGenerator.CoreRestartGeneration()
	go func() {
		_, err := configGenerator.ReconcileCurrentForScheduledRestart(observedRestartGeneration)
		scheduledDone <- err
	}()
	close(releaseFetch)
	select {
	case <-syncDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for subscription sync")
	}
	if err := <-scheduledDone; err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	reconciler.Close()

	activeConfig := readActiveConfigForRestartTest(t, p)
	if !strings.Contains(activeConfig, "node-sync.example.com") {
		t.Fatal("scheduled restart config does not contain the node from the completed subscription sync")
	}
	reloads, scheduled := core.calls()
	if reloads+scheduled != 1 {
		t.Fatalf("core lifecycle calls = reload:%d scheduled:%d, want exactly one", reloads, scheduled)
	}
}

func newScheduledRestartConfigGenerator(t *testing.T, core configGeneratorCore) (*ConfigGeneratorService, *store.Store, *paths.Paths, string) {
	t.Helper()
	root := t.TempDir()
	p := &paths.Paths{
		DataDir:      root,
		BinaryDir:    filepath.Join(root, "bin"),
		BinaryPath:   filepath.Join(root, "bin", configCheckBinaryName()),
		ConfigDir:    filepath.Join(root, "config"),
		ConfigPath:   filepath.Join(root, "config", "config.json"),
		RulesDir:     filepath.Join(root, "rules"),
		GeoDir:       filepath.Join(root, "geo"),
		DownloadsDir: filepath.Join(root, "downloads"),
		DBPath:       filepath.Join(root, "ackwrap.db"),
	}
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	writeConfigCheckBinary(t, p.BinaryPath, true)
	db, err := store.Open(p.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc := NewConfigGeneratorService(db, p)
	svc.singbox = core
	result, err := svc.GenerateCurrent()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid {
		t.Fatalf("initial config is invalid: %s", result.Error)
	}
	if err := svc.Apply("config.json", false); err != nil {
		t.Fatal(err)
	}
	return svc, db, p, p.BinaryPath
}

func configCheckBinaryName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.cmd"
	}
	return "sing-box"
}

func writeConfigCheckBinary(t *testing.T, path string, valid bool) {
	t.Helper()
	exitCode := "0"
	content := "#!/bin/sh\nexit " + exitCode + "\n"
	if !valid {
		exitCode = "1"
		content = "#!/bin/sh\necho invalid config\nexit " + exitCode + "\n"
	}
	if runtime.GOOS == "windows" {
		content = "@echo off\r\nexit /b " + exitCode + "\r\n"
	}
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatal(err)
	}
}

func readActiveConfigForRestartTest(t *testing.T, p *paths.Paths) string {
	t.Helper()
	activePath, ok, err := p.ActiveConfigPath()
	if err != nil || !ok {
		t.Fatalf("active config = %q, ok=%t, err=%v", activePath, ok, err)
	}
	content, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
