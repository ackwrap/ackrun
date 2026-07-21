package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
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
	if !state.Enabled || !state.IPv6 || state.ManagesRoutes {
		t.Fatalf("active TUN state = %+v", state)
	}
}

func TestReadActiveTUNStateDetectsManagedRoutes(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"inbounds":[{"type":"tun","auto_route":false,"auto_redirect":true}]}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	state, err := readActiveTUNState(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Enabled || !state.ManagesRoutes {
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

func TestReadActiveTUNStateAcceptsDefaultCleanupIdentity(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"inbounds":[{"type":"tun","address":["172.254.0.1/30","fdfe:dcba:9876::1/126"],"auto_route":true,"auto_redirect":true,"iproute2_table_index":2022,"iproute2_rule_index":9000,"auto_redirect_iproute2_fallback_rule_index":32768,"auto_redirect_input_mark":"0x2023","auto_redirect_output_mark":"0x2024"}]}`)
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatal(err)
	}
	state, err := readActiveTUNState(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateLinuxTUNCompatibility(state); err != nil {
		t.Fatalf("default lifecycle identity rejected: %v", err)
	}
	if state.RouteManagingInbounds != 1 || !state.ExpectedIPv4 || !state.ExpectedIPv6 {
		t.Fatalf("active TUN state = %+v", state)
	}
}

func TestValidateLinuxTUNCompatibilityRejectsUnsafeShapes(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "auto route without redirect", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_route":true}]}`, want: "without auto_redirect"},
		{name: "multiple route managers", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true},{"type":"tun","address":["fdfe:dcba:9876::1/126"],"auto_redirect":true}]}`, want: "exactly one"},
		{name: "no parseable family", config: `{"inbounds":[{"type":"tun","address":["not-a-prefix"],"auto_redirect":true}]}`, want: "parseable"},
		{name: "custom table", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true,"iproute2_table_index":2023}]}`, want: "cleanup identity mismatch"},
		{name: "custom rule", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true,"iproute2_rule_index":9100}]}`, want: "cleanup identity mismatch"},
		{name: "custom fallback", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true,"auto_redirect_iproute2_fallback_rule_index":32769}]}`, want: "cleanup identity mismatch"},
		{name: "custom input mark", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true,"auto_redirect_input_mark":"0x3030"}]}`, want: "cleanup identity mismatch"},
		{name: "custom output mark", config: `{"inbounds":[{"type":"tun","address":["172.254.0.1/30"],"auto_redirect":true,"auto_redirect_output_mark":"0x3031"}]}`, want: "cleanup identity mismatch"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(configPath, []byte(test.config), 0644); err != nil {
				t.Fatal(err)
			}
			state, err := readActiveTUNState(configPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateLinuxTUNCompatibility(state); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("compatibility error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestParseOptionalTUNUintRejectsInvalidValue(t *testing.T) {
	if _, _, err := parseOptionalTUNUint([]byte(`"not-a-mark"`)); err == nil {
		t.Fatal("invalid mark must be rejected")
	}
}
