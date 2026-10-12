package service

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCoreReadinessPreservesTimeoutAndProbeError(t *testing.T) {
	probeErr := errors.New("core is still initializing")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	err := waitForCoreReadiness(ctx, func(context.Context) error { return probeErr })
	if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, probeErr) {
		t.Fatalf("readiness error = %v, want deadline and last probe failure", err)
	}
}

func TestCoreReadinessRejectsSuccessAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := waitForCoreReadiness(ctx, func(context.Context) error {
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readiness error = %v, want canceled rather than ready", err)
	}
}

func TestCoreReadinessDoesNotProbeAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitForCoreReadiness(ctx, func(context.Context) error {
		t.Fatal("probe called after startup was canceled")
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("readiness error = %v, want canceled", err)
	}
}

func TestCoreReadinessPhasesShareDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	var runtimeDeadline time.Time
	if err := waitForCoreReadiness(ctx, func(ctx context.Context) error {
		runtimeDeadline, _ = ctx.Deadline()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	err := waitForCoreReadiness(ctx, func(ctx context.Context) error {
		dnsDeadline, _ := ctx.Deadline()
		if !dnsDeadline.Equal(runtimeDeadline) {
			t.Fatal("DNS readiness received a new startup budget")
		}
		<-ctx.Done()
		return ctx.Err()
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("DNS readiness error = %v, want shared deadline", err)
	}
}

func TestCoreStartupContextCancelsWhenProcessExits(t *testing.T) {
	processExited := make(chan struct{})
	ctx, cancel := coreStartupContext(processExited)
	defer cancel()
	close(processExited)
	select {
	case <-ctx.Done():
		if !errors.Is(context.Cause(ctx), errCoreExitedDuringStartup) {
			t.Fatalf("startup cancellation = %v, want process exit", context.Cause(ctx))
		}
	case <-time.After(time.Second):
		t.Fatal("startup remained pending after process exit")
	}
}
