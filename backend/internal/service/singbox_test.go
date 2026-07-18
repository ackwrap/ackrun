package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestCoreExitStateReportsUnexpectedTUNFailure(t *testing.T) {
	status, runtimeStatus, message := coreExitState(
		errors.New("exit status 1"),
		false,
		"FATAL start inbound/tun[tun-in]: configure tun interface: Access is denied.",
	)
	if status != "error" || runtimeStatus != model.RuntimeError {
		t.Fatalf("unexpected exit state: status=%s runtime=%s", status, runtimeStatus)
	}
	if !strings.Contains(message, "管理员身份") {
		t.Fatalf("unexpected TUN error message: %s", message)
	}
}

func TestCoreExitStateReportsIntentionalStop(t *testing.T) {
	status, runtimeStatus, message := coreExitState(errors.New("signal: killed"), true, "")
	if status != "stopped" || runtimeStatus != model.RuntimeStopped || message != "" {
		t.Fatalf("unexpected intentional stop state: status=%s runtime=%s message=%q", status, runtimeStatus, message)
	}
}

func TestCoreExitStateReportsUnexpectedCleanExit(t *testing.T) {
	status, runtimeStatus, message := coreExitState(nil, false, "")
	if status != "error" || runtimeStatus != model.RuntimeError {
		t.Fatalf("unexpected exit state: status=%s runtime=%s", status, runtimeStatus)
	}
	if !strings.Contains(message, "without an error status") {
		t.Fatalf("unexpected clean-exit message: %s", message)
	}
}
