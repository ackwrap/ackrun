package service

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

type fakeConfigGenerator struct {
	mu            sync.Mutex
	generateCalls int
	applyCalls    int
	result        *model.ConfigGenerateResponse
	generateErr   error
	applied       chan struct{}
}

func (fake *fakeConfigGenerator) ReconcileCurrent() (*model.ConfigGenerateResponse, error) {
	fake.mu.Lock()
	fake.generateCalls++
	if fake.generateErr != nil || fake.result == nil || !fake.result.Valid {
		defer fake.mu.Unlock()
		return fake.result, fake.generateErr
	}
	fake.applyCalls++
	fake.mu.Unlock()
	select {
	case fake.applied <- struct{}{}:
	default:
	}
	return fake.result, nil
}

func TestConfigReconcileDebouncesTriggers(t *testing.T) {
	fake := &fakeConfigGenerator{
		result:  &model.ConfigGenerateResponse{Valid: true},
		applied: make(chan struct{}, 1),
	}
	svc := newConfigReconcileService(fake, nil, 10*time.Millisecond)
	defer svc.Close()

	svc.Trigger("nodes.update")
	svc.Trigger("rules.update")
	select {
	case <-fake.applied:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for config apply")
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.generateCalls != 1 || fake.applyCalls != 1 {
		t.Fatalf("calls = generate:%d apply:%d, want one debounced run", fake.generateCalls, fake.applyCalls)
	}
}

func TestConfigReconcileDoesNotApplyInvalidConfig(t *testing.T) {
	fake := &fakeConfigGenerator{
		result:  &model.ConfigGenerateResponse{Valid: false, Error: "invalid"},
		applied: make(chan struct{}, 1),
	}
	svc := newConfigReconcileService(fake, nil, time.Millisecond)
	defer svc.Close()

	svc.Trigger("dns.update")
	time.Sleep(30 * time.Millisecond)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.generateCalls != 1 || fake.applyCalls != 0 {
		t.Fatalf("calls = generate:%d apply:%d, want validation failure without apply", fake.generateCalls, fake.applyCalls)
	}
}

func TestConfigReconcileDoesNotApplyGenerateError(t *testing.T) {
	fake := &fakeConfigGenerator{
		generateErr: errors.New("boom"),
		applied:     make(chan struct{}, 1),
	}
	svc := newConfigReconcileService(fake, nil, time.Millisecond)
	defer svc.Close()

	svc.Trigger("collections.update")
	time.Sleep(30 * time.Millisecond)

	fake.mu.Lock()
	defer fake.mu.Unlock()
	if fake.generateCalls != 1 || fake.applyCalls != 0 {
		t.Fatalf("calls = generate:%d apply:%d, want generation failure without apply", fake.generateCalls, fake.applyCalls)
	}
}
