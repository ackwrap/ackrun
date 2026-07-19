package service

import (
	"errors"
	"os"
	"path/filepath"
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

func TestCoreExitStateReportsOpenWrtNFTablesFailure(t *testing.T) {
	status, runtimeStatus, message := coreExitState(
		errors.New("exit status 1"),
		false,
		"FATAL initialize auto-redirect: create nftables table: operation not permitted",
	)
	if status != "error" || runtimeStatus != model.RuntimeError {
		t.Fatalf("unexpected exit state: status=%s runtime=%s", status, runtimeStatus)
	}
	if !strings.Contains(message, "OpenWrt") || !strings.Contains(message, "CAP_NET_ADMIN") {
		t.Fatalf("unexpected auto_redirect error message: %s", message)
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

func TestReadActiveTUNStateUsesActiveConfigInsteadOfStoredMode(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"inbounds":[{"type":"mixed"},{"type":"tun","address":["172.19.0.1/30","fdfe:dcba:9876::1/126"]}]}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	state, err := readActiveTUNState(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || !state.IPv6 {
		t.Fatalf("active TUN state = %+v", state)
	}
}

func TestReadActiveTUNStateWarnsForIPv4OnlyTUN(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"inbounds":[{"type":"tun","address":["172.19.0.1/30"]}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	state, err := readActiveTUNState(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || state.IPv6 {
		t.Fatalf("active TUN state = %+v", state)
	}
}
