package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

type forcedCleanupCommands struct {
	calls    []string
	failFW4  bool
	failIP   bool
	foreign  bool
	baseline bool
}

func (commands *forcedCleanupCommands) run(name string, args ...string) ([]byte, error) {
	call := name + " " + strings.Join(args, " ")
	commands.calls = append(commands.calls, call)
	if call == "nft list tables" {
		return []byte("table inet fw4\ntable inet sing-box\ntable inet other-vpn\n"), nil
	}
	if call == "fw4 reload" && commands.failFW4 {
		return nil, errors.New("synthetic fw4 reload failed")
	}
	if name == "ip" && commands.failIP {
		return nil, errors.New("synthetic ip command failed")
	}
	if len(args) > 0 && args[0] == "-6" {
		args = args[1:]
	}
	action := strings.Join(args, " ")
	if name == "ip" && action == "rule show" {
		if commands.foreign {
			return []byte("0: from all lookup local\n1: from all lookup 640805858\n100: from all lookup 88\n32766: from all lookup main\n32767: from all lookup default\n"), nil
		}
		return []byte("0: from all lookup local\n1: from all lookup 300\n1: from all lookup 640805858\n100: from all lookup 88\n9000: from all fwmark 0x2024/0x2027 goto 9002\n9000: from all lookup 2022\n9001: from all fwmark 0x2023/0x2027 lookup 2022\n9002: from all nop\n9005: from all lookup 2022\n32766: from all lookup main\n32767: from all lookup default\n32768: not from all fwmark 0x2024/0x2027 lookup 2022\n"), nil
	}
	if name == "ip" && action == "-o route show table all" {
		return []byte("default dev tun0 table 2022\nlocal 127.0.0.1 dev br-lan table 300 scope host\ndefault dev eth0 table 640805858\n"), nil
	}
	if name == "ip" && action == "route show table 300" {
		if commands.baseline {
			return nil, fmt.Errorf("baseline table must not be inspected")
		}
		if len(strings.Fields(call)) > 1 && strings.Contains(call, "-6") {
			return []byte("local ::1 dev br-lan metric 1024 pref medium\n"), nil
		}
		return []byte("local 127.0.0.1 dev br-lan scope host\n"), nil
	}
	if name == "ip" && action == "route show table 640805858" {
		return []byte("local 192.0.2.1 dev eth0 scope host\ndefault dev easytier-tun\n"), nil
	}
	if call == "nft delete table inet sing-box" || call == "fw4 reload" || name == "ip" && (strings.HasPrefix(action, "rule del ") || strings.HasPrefix(action, "route flush table ")) {
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected command: %s", call)
}

func TestForceCleanupIgnoresUnusableOwnership(t *testing.T) {
	for _, state := range []string{
		"",
		`{"version":2,"phase":"pending","expect_ipv4":true,"expect_ipv6":true,"nft_table_absent":true}`,
		`{"version":2,"phase":`,
		`{"version":1,"nft_include_identity":"wrong","nft_table_handle":"999"}`,
	} {
		t.Run(fmt.Sprintf("state-%d", len(state)), func(t *testing.T) {
			root := t.TempDir()
			statePath, include := filepath.Join(root, "state.json"), filepath.Join(root, "include.nft")
			if state != "" {
				if err := os.WriteFile(statePath, []byte(state), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			for _, path := range []string{include, include + ".ackwrap-stale"} {
				if err := os.WriteFile(path, []byte("unrecorded include"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			commands := &forcedCleanupCommands{}
			result, err := forceCleanupSingboxState(statePath, include, commands.run)
			if err != nil || !result.Cleaned {
				t.Fatalf("forced cleanup = %+v, %v", result, err)
			}
			for _, path := range []string{statePath, include, include + ".ackwrap-stale"} {
				if _, err := os.Lstat(path); !os.IsNotExist(err) {
					t.Fatalf("residual state remains at %s: %v", path, err)
				}
			}
			for _, call := range []string{"nft delete table inet sing-box", "fw4 reload", "ip rule del priority 9000", "ip -6 rule del priority 32768", "ip route flush table 2022", "ip -6 route flush table 300", "ip rule del priority 1 from all lookup 300"} {
				if !slices.Contains(commands.calls, call) {
					t.Errorf("missing cleanup command: %s", call)
				}
			}
			for _, call := range commands.calls {
				if strings.Contains(call, "del priority 100") || strings.Contains(call, "del priority 32766") || strings.Contains(call, "del priority 32767") || strings.Contains(call, "flush table 640805858") || strings.Contains(call, "del priority 1 from all lookup 640805858") || strings.Contains(call, "nft delete table inet fw4") || strings.Contains(call, "nft flush ruleset") {
					t.Errorf("unrelated network state touched: %s", call)
				}
			}
		})
	}
}

func TestForceCleanupContinuesAfterFailureAndCanRetry(t *testing.T) {
	root := t.TempDir()
	state, include := filepath.Join(root, "state.json"), filepath.Join(root, "include.nft")
	if err := os.WriteFile(state, []byte("damaged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(include, []byte("unrecorded include"), 0o600); err != nil {
		t.Fatal(err)
	}
	commands := &forcedCleanupCommands{failFW4: true}
	_, err := forceCleanupSingboxState(state, include, commands.run)
	if err == nil || !strings.Contains(err.Error(), "synthetic fw4 reload failed") {
		t.Fatalf("command failure was hidden: %v", err)
	}
	if !slices.Contains(commands.calls, "ip -6 route flush table 300") {
		t.Fatal("fw4 failure blocked independent route cleanup")
	}
	for _, path := range []string{state, include + ".ackwrap-stale"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retry state was lost: %v", err)
		}
	}
	commands.failFW4 = false
	if _, err := forceCleanupSingboxState(state, include, commands.run); err != nil {
		t.Fatalf("retry failed: %v", err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("successful retry did not remove ownership state")
	}
}

func TestForceCleanupPreservesBaselineRedirectTable(t *testing.T) {
	root := t.TempDir()
	statePath := filepath.Join(root, "state.json")
	state, err := pendingSingboxRouteTableState(platformNetworkBaseline{Required: true, ExpectIPv4: true, ExpectIPv6: true, NFTTableAbsent: true, IPv4Tables: []string{"300"}, IPv6Tables: []string{"300"}})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSingboxRouteTableState(statePath, state); err != nil {
		t.Fatal(err)
	}
	commands := &forcedCleanupCommands{baseline: true}
	if _, err := forceCleanupSingboxState(statePath, filepath.Join(root, "absent.nft"), commands.run); err != nil {
		t.Fatal(err)
	}
	for _, call := range commands.calls {
		if strings.Contains(call, "flush table 300") || strings.Contains(call, "del priority 1 from all lookup 300") {
			t.Fatalf("baseline table deleted: %s", call)
		}
	}
}

func TestForceCleanupReportsIPFailureAfterFirewallCleanup(t *testing.T) {
	root := t.TempDir()
	commands := &forcedCleanupCommands{failIP: true}
	_, err := forceCleanupSingboxState(filepath.Join(root, "state.json"), filepath.Join(root, "absent.nft"), commands.run)
	if err == nil || !strings.Contains(err.Error(), "synthetic ip command failed") {
		t.Fatalf("IP failure was hidden: %v", err)
	}
	if !slices.Contains(commands.calls, "nft delete table inet sing-box") || !slices.Contains(commands.calls, "fw4 reload") || !slices.Contains(commands.calls, "ip -6 rule show") {
		t.Fatal("independent cleanup steps were not attempted")
	}
}
