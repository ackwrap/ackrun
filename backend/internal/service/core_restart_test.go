package service

import (
	"errors"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

type scheduledCoreStub struct {
	running bool
	calls   int
	err     error
}

func (stub *scheduledCoreStub) IsRunning() bool { return stub.running }
func (stub *scheduledCoreStub) ScheduledRestart() (*model.ActionResponse, error) {
	stub.calls++
	return &model.ActionResponse{Success: stub.err == nil}, stub.err
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
	svc := &CoreRestartScheduler{core: core}
	svc.runScheduledRestart()
	if core.calls != 0 {
		t.Fatalf("restart calls = %d, want 0", core.calls)
	}
}

func TestRunScheduledRestartRestartsRunningCore(t *testing.T) {
	core := &scheduledCoreStub{running: true}
	svc := &CoreRestartScheduler{core: core}
	svc.runScheduledRestart()
	if core.calls != 1 {
		t.Fatalf("restart calls = %d, want 1", core.calls)
	}
}
