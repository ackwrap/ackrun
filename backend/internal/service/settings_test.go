package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

type modeConfigGeneratorStub struct {
	calls  int
	result *model.ConfigGenerateResponse
	err    error
}

type blockingConfigGeneratorStub struct {
	mu      sync.Mutex
	calls   int
	started chan struct{}
	release chan struct{}
}

func (stub *blockingConfigGeneratorStub) ReconcileCurrent() (*model.ConfigGenerateResponse, error) {
	stub.mu.Lock()
	stub.calls++
	call := stub.calls
	stub.mu.Unlock()
	if call == 1 {
		close(stub.started)
		<-stub.release
	}
	return &model.ConfigGenerateResponse{Valid: true}, nil
}

func (stub *modeConfigGeneratorStub) ReconcileCurrent() (*model.ConfigGenerateResponse, error) {
	stub.calls++
	return stub.result, stub.err
}

func TestSetProxyModePersistsSupportedMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	if err := svc.SetProxyMode("global"); err != nil {
		t.Fatal(err)
	}
	if got := svc.GetProxyMode(); got != "global" {
		t.Fatalf("proxy mode = %q, want global", got)
	}
}

func TestSetUpdateSettingsRejectsRemovedProxyMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	if err := svc.SetUpdateSettings(&model.UpdateSettings{Acceleration: "proxy"}); err == nil {
		t.Fatal("removed proxy acceleration mode should be rejected")
	}
}

func TestTrafficBypassSettingsDefaultsAndValidation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewSettingsService(db)
	defaults, err := svc.GetTrafficBypassSettings()
	if err != nil {
		t.Fatal(err)
	}
	if len(defaults.Rules) != 5 ||
		defaults.Rules[0].Value != "easytier-core" ||
		defaults.Rules[1].Value != "easytier-tun" ||
		defaults.Rules[2].Value != "10.0.0.0/8" ||
		defaults.Rules[3].Value != "172.16.0.0/12" ||
		defaults.Rules[4].Value != "192.168.0.0/16" {
		t.Fatalf("unexpected traffic bypass defaults: %+v", defaults.Rules)
	}
	settings := &model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{
		{Type: "ip_cidr", Value: "10.9.8.7/8", Remark: "  内网网段  "},
		{Type: "domain_suffix", Value: "Example.COM."},
		{Type: "process_name", Value: "custom-agent"},
	}}
	if err := svc.SetTrafficBypassSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings.Rules[0].Value != "10.0.0.0/8" || settings.Rules[0].Remark != "内网网段" || settings.Rules[1].Value != "example.com" {
		t.Fatalf("traffic bypass settings were not normalized: %+v", settings.Rules)
	}
	persisted, err := svc.GetTrafficBypassSettings()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Rules[0].Remark != "内网网段" {
		t.Fatalf("traffic bypass remark was not persisted: %+v", persisted.Rules)
	}
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "ip_cidr", Value: "invalid"}}}); !errors.Is(err, ErrTrafficBypassSettingsInvalid) {
		t.Fatalf("expected invalid CIDR error, got %v", err)
	}
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "process_name", Value: "test", Remark: "two\nlines"}}}); !errors.Is(err, ErrTrafficBypassSettingsInvalid) {
		t.Fatalf("expected invalid remark error, got %v", err)
	}
}

func TestSetProxyModeReconcilesConfig(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: true}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	if err := svc.SetProxyMode("global"); err != nil {
		t.Fatal(err)
	}
	if generator.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", generator.calls)
	}
	if got := svc.GetProxyMode(); got != "global" {
		t.Fatalf("proxy mode = %q, want global", got)
	}
}

func TestSetInboundModeRollsBackWhenConfigIsInvalid(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: false, Error: "invalid mode config"}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	if err := svc.SetInboundMode("mixed"); err == nil || !strings.Contains(err.Error(), "已回滚") {
		t.Fatalf("error = %v, want rollback error", err)
	}
	if generator.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", generator.calls)
	}
	if got := svc.GetInboundMode(); got != "tun_mixed" {
		t.Fatalf("inbound mode = %q, want rollback to tun_mixed", got)
	}
}

func TestSetMixedInboundSettingsPersistsAndReconciles(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: true}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	if err := svc.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "  proxy-user  ", Password: "short-pass"}); err != nil {
		t.Fatal(err)
	}
	if generator.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", generator.calls)
	}
	settings, err := svc.GetMixedInboundSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Username != "proxy-user" || settings.Password != "short-pass" {
		t.Fatalf("mixed inbound settings were not persisted and normalized")
	}
	if err := svc.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "proxy-user"}); !errors.Is(err, ErrMixedInboundSettingsInvalid) {
		t.Fatalf("partial credentials error = %v, want validation error", err)
	}
}

func TestSetMixedInboundSettingsRollsBackWhenConfigIsInvalid(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "old-user", Password: "old-pass"}); err != nil {
		t.Fatal(err)
	}

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: false, Error: "invalid mixed config"}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	err = svc.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "new-user", Password: "new-pass"})
	if err == nil || !strings.Contains(err.Error(), "已回滚") {
		t.Fatalf("error = %v, want rollback error", err)
	}
	settings, getErr := svc.GetMixedInboundSettings()
	if getErr != nil {
		t.Fatal(getErr)
	}
	if settings.Username != "old-user" || settings.Password != "old-pass" {
		t.Fatal("mixed inbound settings were not rolled back")
	}
}

func TestSetMixedInboundSettingsSerializesConcurrentUpdates(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	generator := &blockingConfigGeneratorStub{started: make(chan struct{}), release: make(chan struct{})}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- svc.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "first-user", Password: "first-pass"})
	}()
	<-generator.started
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- svc.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "second-user", Password: "second-pass"})
	}()
	select {
	case err := <-secondDone:
		t.Fatalf("second update completed before first reconciliation: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(generator.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if err := <-secondDone; err != nil {
		t.Fatal(err)
	}
	settings, err := svc.GetMixedInboundSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.Username != "second-user" || settings.Password != "second-pass" {
		t.Fatal("concurrent mixed inbound update was overwritten")
	}
}

func TestSetExperimentalSettingsRejectsInvalidClashAPIPort(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	err = svc.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090@remote.example"})
	if err == nil {
		t.Fatal("SetExperimentalSettings() error = nil, want invalid port error")
	}
}

func TestSetExperimentalSettingsReconcilesConfig(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: true}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	if err := svc.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9091", CacheFileEnabled: true}); err != nil {
		t.Fatal(err)
	}
	if generator.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", generator.calls)
	}
	settings, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ClashAPIPort != "9091" {
		t.Fatalf("Clash API port = %q, want 9091", settings.ClashAPIPort)
	}
}

func TestSetExperimentalSettingsRollsBackWhenConfigIsInvalid(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIEnabled: true, ClashAPIPort: "9090", CacheFileEnabled: true}); err != nil {
		t.Fatal(err)
	}

	generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: false, Error: "invalid experimental config"}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(nil, generator)
	err = svc.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9091", CacheFileEnabled: true})
	if err == nil || !strings.Contains(err.Error(), "已回滚") {
		t.Fatalf("error = %v, want rollback error", err)
	}
	if generator.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", generator.calls)
	}
	settings, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ClashAPIPort != "9090" {
		t.Fatalf("Clash API port = %q, want rollback to 9090", settings.ClashAPIPort)
	}
}

func TestSetLogSettingsPersistsLevelInGenerationRequest(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	request := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		TUNIPv4Address:  "10.254.0.1/30",
		TUNIPv6Address:  "fd12:3456:789a::1/126",
		LogLevel:        "info",
	}
	if err := db.SetConfigGenerateRequest(request); err != nil {
		t.Fatal(err)
	}

	svc := NewSettingsService(db)
	if err := svc.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: true}); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.LogLevel != "debug" {
		t.Fatalf("generation log level = %+v, want debug", stored)
	}
	want := *request
	want.LogLevel = "debug"
	if *stored != want {
		t.Fatalf("log update changed other generation settings: got %+v, want %+v", stored, want)
	}
}

func TestSetLogSettingsRejectsInvalidLevel(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := NewSettingsService(db).SetLogSettings(&model.LogSettings{Level: "verbose"}); err == nil {
		t.Fatal("SetLogSettings() error = nil, want invalid level error")
	}
}

func TestConnectivitySettingsPersistAndNotifyScheduler(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewSettingsService(db)
	settings, err := svc.GetConnectivitySettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.TestURL != store.DefaultConnectivityTestURL || settings.IntervalSeconds != store.DefaultConnectivityIntervalSeconds {
		t.Fatalf("default connectivity settings = %+v", settings)
	}
	if settings.TestURL != "http://www.gstatic.com/generate_204" {
		t.Fatalf("default connectivity URL = %q, want HTTP", settings.TestURL)
	}
	hookCalled := false
	svc.SetConnectivitySettingsHook(func() { hookCalled = true })
	target, err := svc.CreateConnectivityTarget(&model.ConnectivityTargetRequest{Name: "Test target", URL: "http://connectivity.example/generate_204", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	request := &model.ConnectivitySettings{TestURL: target.URL, IntervalSeconds: 120}
	if err := svc.SetConnectivitySettings(request); err != nil {
		t.Fatal(err)
	}
	stored, err := svc.GetConnectivitySettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.TestURL != request.TestURL || stored.IntervalSeconds != request.IntervalSeconds || !hookCalled {
		t.Fatalf("stored connectivity settings = %+v, hook = %t", stored, hookCalled)
	}
}

func TestConnectivitySettingsRejectInvalidValues(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewSettingsService(db)

	for _, request := range []model.ConnectivitySettings{
		{TestURL: "file:///tmp/check", IntervalSeconds: 300},
		{TestURL: "https://example.com/check", IntervalSeconds: 59},
		{TestURL: "https://example.com/check", IntervalSeconds: 3601},
	} {
		if err := svc.SetConnectivitySettings(&request); !errors.Is(err, ErrConnectivitySettingsInvalid) {
			t.Fatalf("request = %+v, error = %v", request, err)
		}
	}
}

func TestSetProxyModeRejectsRunningCore(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	runningCore := &SingboxService{pid: 1, cmd: &exec.Cmd{Process: &os.Process{}}}
	svc := NewSettingsService(db)
	svc.SetModeDependencies(runningCore, nil)
	if err := svc.SetProxyMode("direct"); !errors.Is(err, ErrModeChangeWhileRunning) {
		t.Fatalf("error = %v, want ErrModeChangeWhileRunning", err)
	}
	if got := svc.GetProxyMode(); got != "rule" {
		t.Fatalf("proxy mode changed while running: %q", got)
	}
}
