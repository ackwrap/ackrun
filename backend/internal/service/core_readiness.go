package service

import (
	"context"
	"errors"
	"time"
)

const (
	coreStartupReadinessTimeout = 60 * time.Second
	coreReadinessProbeInterval  = 100 * time.Millisecond
)

var errCoreExitedDuringStartup = errors.New("sing-box 在启动就绪前退出")

func coreStartupContext(processExited <-chan struct{}) (context.Context, context.CancelFunc) {
	deadlineContext, cancelDeadline := context.WithTimeout(context.Background(), coreStartupReadinessTimeout)
	ctx, cancelCause := context.WithCancelCause(deadlineContext)
	go func() {
		select {
		case <-processExited:
			cancelCause(errCoreExitedDuringStartup)
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		cancelCause(context.Canceled)
		cancelDeadline()
	}
}

// The runtime API and DNS share one startup deadline. A failed probe must not
// restart the budget, and process exit cancels in-flight network requests.
func waitForCoreReadiness(ctx context.Context, probe func(context.Context) error) error {
	var lastErr error
	for {
		if cause := context.Cause(ctx); cause != nil {
			return errors.Join(cause, lastErr)
		}
		lastErr = probe(ctx)
		if cause := context.Cause(ctx); cause != nil {
			return errors.Join(cause, lastErr)
		}
		if lastErr == nil {
			return nil
		}
		timer := time.NewTimer(coreReadinessProbeInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return errors.Join(context.Cause(ctx), lastErr)
		case <-timer.C:
		}
	}
}
