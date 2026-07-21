package service

import (
	"errors"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

type scheduledCoreStub struct {
	running bool
}

func (stub *scheduledCoreStub) IsRunning() bool { return stub.running }

type scheduledConfigGeneratorStub struct {
	calls                     int
	restartGeneration         uint64
	observedRestartGeneration uint64
	result                    *model.ConfigGenerateResponse
	err                       error
}

func (stub *scheduledConfigGeneratorStub) CoreRestartGeneration() uint64 {
	return stub.restartGeneration
}

func (stub *scheduledConfigGeneratorStub) ReconcileCurrentForScheduledRestart(observedRestartGeneration uint64) (*model.ConfigGenerateResponse, error) {
	stub.calls++
	stub.observedRestartGeneration = observedRestartGeneration
	return stub.result, stub.err
}

func TestCoreRestartCronSpec(t *testing.T) {
	tests := []struct {
		settings model.CoreRestartSettings
		wantSpec string
		enabled  bool
		wantErr  bool
	}{
		{settings: model.CoreRestartSettings{Mode: "daily", Time: "04:00:00", Weekday: 1}, wantSpec: "0 0 4 * * *", enabled: true},
		{settings: model.CoreRestartSettings{Mode: "weekly", Time: "03:30", Weekday: 0}, wantSpec: "0 30 3 * * 0", enabled: true},
		{settings: model.CoreRestartSettings{Mode: "off", Time: "04:00", Weekday: 1}, enabled: false},
		{settings: model.CoreRestartSettings{Mode: "daily", Time: "25:00", Weekday: 1}, wantErr: true},
	}
	for _, tt := range tests {
		spec, enabled, err := coreRestartCronSpec(&tt.settings)
		if tt.wantErr {
			if !errors.Is(err, ErrInvalidCoreRestartSettings) {
				t.Fatalf("settings %+v error = %v", tt.settings, err)
			}
			continue
		}
		if err != nil || spec != tt.wantSpec || enabled != tt.enabled {
			t.Fatalf("settings %+v = spec %q enabled %t err %v", tt.settings, spec, enabled, err)
		}
	}
}

func TestRunScheduledRestartSkipsStoppedCore(t *testing.T) {
	core := &scheduledCoreStub{}
	config := &scheduledConfigGeneratorStub{}
	svc := &CoreRestartScheduler{core: core, config: config}
	svc.runScheduledRestart()
	if config.calls != 0 {
		t.Fatalf("config reconcile calls = %d, want 0", config.calls)
	}
}

func TestRunScheduledRestartReconcilesConfigAndRestartsRunningCore(t *testing.T) {
	core := &scheduledCoreStub{running: true}
	config := &scheduledConfigGeneratorStub{restartGeneration: 7, result: &model.ConfigGenerateResponse{Valid: true}}
	svc := &CoreRestartScheduler{core: core, config: config}
	svc.runScheduledRestart()
	if config.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", config.calls)
	}
	if config.observedRestartGeneration != config.restartGeneration {
		t.Fatalf("observed restart generation = %d, want %d", config.observedRestartGeneration, config.restartGeneration)
	}
}

func TestRunScheduledRestartStopsWhenGeneratedConfigIsInvalid(t *testing.T) {
	config := &scheduledConfigGeneratorStub{result: &model.ConfigGenerateResponse{Error: "invalid config"}}
	svc := &CoreRestartScheduler{core: &scheduledCoreStub{running: true}, config: config}
	svc.runScheduledRestart()
	if config.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", config.calls)
	}
}

func TestRunScheduledRestartReportsConfigReconcileFailure(t *testing.T) {
	config := &scheduledConfigGeneratorStub{err: errors.New("generate failed")}
	svc := &CoreRestartScheduler{core: &scheduledCoreStub{running: true}, config: config}
	svc.runScheduledRestart()
	if config.calls != 1 {
		t.Fatalf("config reconcile calls = %d, want 1", config.calls)
	}
}
