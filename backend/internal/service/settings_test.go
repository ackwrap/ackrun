package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type modeConfigGeneratorStub struct {
	calls  int
	result *model.ConfigGenerateResponse
	err    error
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
		{Type: "ip_cidr", Value: "10.9.8.7/8"},
		{Type: "domain_suffix", Value: "Example.COM."},
		{Type: "process_name", Value: "custom-agent"},
	}}
	if err := svc.SetTrafficBypassSettings(settings); err != nil {
		t.Fatal(err)
	}
	if settings.Rules[0].Value != "10.0.0.0/8" || settings.Rules[1].Value != "example.com" {
		t.Fatalf("traffic bypass settings were not normalized: %+v", settings.Rules)
	}
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "ip_cidr", Value: "invalid"}}}); !errors.Is(err, ErrTrafficBypassSettingsInvalid) {
		t.Fatalf("expected invalid CIDR error, got %v", err)
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
