//go:build linux

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

type fakeDNSMasqCommands struct {
	snapshot       dnsmasqOptionsSnapshot
	pendingChanges string
	restartCalls   int
	failRestarts   int
	usedDeltaDir   bool
}

func (fake *fakeDNSMasqCommands) run(_ string, path string, args ...string) ([]byte, error) {
	if strings.Contains(path, "dnsmasq") {
		switch args[0] {
		case "enabled", "running":
			return nil, nil
		case "restart":
			fake.restartCalls++
			if fake.failRestarts > 0 {
				fake.failRestarts--
				return []byte("restart failed"), errors.New("exit status 1")
			}
			return nil, nil
		}
	}
	if !strings.Contains(path, "uci") {
		return nil, fmt.Errorf("unexpected command: %s %v", path, args)
	}
	if slices.Equal(args, []string{"-q", "changes", "dhcp"}) {
		return []byte(fake.pendingChanges), nil
	}
	if slices.Equal(args, []string{"-q", "-X", "show", "dhcp"}) {
		return []byte(formatFakeDNSMasqSnapshot(fake.snapshot)), nil
	}
	if len(args) < 4 || args[0] != "-q" || args[1] != "-t" || args[2] == "" {
		return nil, fmt.Errorf("UCI write did not use a private delta directory: %v", args)
	}
	fake.usedDeltaDir = true
	command := args[3]
	commandArgs := args[4:]
	switch command {
	case "delete":
		if len(commandArgs) != 1 {
			return nil, fmt.Errorf("invalid delete arguments: %v", args)
		}
		if strings.HasSuffix(commandArgs[0], ".server") {
			fake.snapshot.ServersExist = false
			fake.snapshot.Servers = nil
		} else if strings.HasSuffix(commandArgs[0], ".noresolv") {
			fake.snapshot.NoResolvExists = false
			fake.snapshot.NoResolv = ""
		}
	case "add_list":
		if len(commandArgs) != 1 {
			return nil, fmt.Errorf("invalid add_list arguments: %v", args)
		}
		_, value, found := strings.Cut(commandArgs[0], "=")
		if !found {
			return nil, fmt.Errorf("invalid add_list value: %v", args)
		}
		fake.snapshot.ServersExist = true
		fake.snapshot.Servers = append(fake.snapshot.Servers, value)
	case "set":
		if len(commandArgs) != 1 {
			return nil, fmt.Errorf("invalid set arguments: %v", args)
		}
		_, value, found := strings.Cut(commandArgs[0], "=")
		if !found {
			return nil, fmt.Errorf("invalid set value: %v", args)
		}
		fake.snapshot.NoResolvExists = true
		fake.snapshot.NoResolv = value
	case "commit":
		if !slices.Equal(commandArgs, []string{"dhcp"}) {
			return nil, fmt.Errorf("invalid commit arguments: %v", args)
		}
	default:
		return nil, fmt.Errorf("unexpected UCI command: %v", args)
	}
	return nil, nil
}

func formatFakeDNSMasqSnapshot(snapshot dnsmasqOptionsSnapshot) string {
	var lines []string
	lines = append(lines, "dhcp."+snapshot.Section+"=dnsmasq")
	if snapshot.NoResolvExists {
		lines = append(lines, fmt.Sprintf("dhcp.%s.noresolv='%s'", snapshot.Section, snapshot.NoResolv))
	}
	if snapshot.ServersExist {
		quoted := make([]string, len(snapshot.Servers))
		for index, server := range snapshot.Servers {
			quoted[index] = "'" + strings.ReplaceAll(server, "'", "'\\''") + "'"
		}
		lines = append(lines, "dhcp."+snapshot.Section+".server="+strings.Join(quoted, " "))
	}
	return strings.Join(lines, "\n")
}

func newFakeOpenWrtDNSMasqLifecycle(t *testing.T, fake *fakeDNSMasqCommands) *openWrtDNSMasqLifecycle {
	t.Helper()
	directory := t.TempDir()
	releasePath := filepath.Join(directory, "openwrt_release")
	uciPath := filepath.Join(directory, "uci")
	initPath := filepath.Join(directory, "dnsmasq")
	for _, path := range []string{releasePath, uciPath, initPath} {
		if err := os.WriteFile(path, []byte("test"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return &openWrtDNSMasqLifecycle{
		statePath:   filepath.Join(directory, ".dnsmasq-takeover.json"),
		releasePath: releasePath,
		uciPath:     uciPath,
		initPath:    initPath,
		run:         fake.run,
	}
}

func TestOpenWrtDNSMasqLifecycleActivateAndRestore(t *testing.T) {
	original := dnsmasqOptionsSnapshot{
		Section:        "cfg01411c",
		NoResolvExists: true,
		NoResolv:       "0",
		ServersExist:   true,
		Servers:        []string{"192.168.3.1", "/corp.example/172.16.1.1"},
	}
	fake := &fakeDNSMasqCommands{snapshot: original}
	manager := newFakeOpenWrtDNSMasqLifecycle(t, fake)

	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	if !dnsmasqSnapshotsEqual(fake.snapshot, managedDNSMasqSnapshot(original.Section)) {
		t.Fatalf("managed snapshot = %+v", fake.snapshot)
	}
	state, exists, err := readDNSMasqTakeoverState(manager.statePath)
	if err != nil || !exists || state.Phase != "active" {
		t.Fatalf("takeover state = %+v, exists=%t, err=%v", state, exists, err)
	}
	if !fake.usedDeltaDir || fake.restartCalls != 1 {
		t.Fatalf("used delta=%t, restart calls=%d", fake.usedDeltaDir, fake.restartCalls)
	}

	restored, err := manager.Restore()
	if err != nil || !restored {
		t.Fatalf("restore = %t, %v", restored, err)
	}
	if !dnsmasqSnapshotsEqual(fake.snapshot, original) {
		t.Fatalf("restored snapshot = %+v", fake.snapshot)
	}
	if fake.restartCalls != 2 {
		t.Fatalf("restart calls = %d", fake.restartCalls)
	}
	if _, err := os.Stat(manager.statePath); !os.IsNotExist(err) {
		t.Fatalf("takeover state still exists: %v", err)
	}
}

func TestOpenWrtDNSMasqLifecycleRollsBackRestartFailure(t *testing.T) {
	original := dnsmasqOptionsSnapshot{Section: "cfg01411c", ServersExist: true, Servers: []string{"192.168.3.1"}}
	fake := &fakeDNSMasqCommands{snapshot: original, failRestarts: 1}
	manager := newFakeOpenWrtDNSMasqLifecycle(t, fake)

	err := manager.Activate()
	if err == nil || !strings.Contains(err.Error(), "重启 dnsmasq 失败") {
		t.Fatalf("activate error = %v", err)
	}
	if !dnsmasqSnapshotsEqual(fake.snapshot, original) {
		t.Fatalf("rollback snapshot = %+v", fake.snapshot)
	}
	if fake.restartCalls != 2 {
		t.Fatalf("restart calls = %d", fake.restartCalls)
	}
	if _, err := os.Stat(manager.statePath); !os.IsNotExist(err) {
		t.Fatalf("takeover state still exists: %v", err)
	}
}

func TestOpenWrtDNSMasqLifecycleRejectsExternalChanges(t *testing.T) {
	fake := &fakeDNSMasqCommands{snapshot: dnsmasqOptionsSnapshot{Section: "cfg01411c"}}
	manager := newFakeOpenWrtDNSMasqLifecycle(t, fake)
	if err := manager.Activate(); err != nil {
		t.Fatal(err)
	}
	fake.snapshot.Servers = []string{"127.0.0.1#1053", "192.168.9.1"}

	restored, err := manager.Restore()
	if err == nil || restored || !strings.Contains(err.Error(), "外部修改") {
		t.Fatalf("restore = %t, %v", restored, err)
	}
	if _, exists, readErr := readDNSMasqTakeoverState(manager.statePath); readErr != nil || !exists {
		t.Fatalf("takeover state should remain for manual recovery: exists=%t, err=%v", exists, readErr)
	}
}

func TestOpenWrtDNSMasqLifecycleRejectsPendingDHCPChanges(t *testing.T) {
	fake := &fakeDNSMasqCommands{
		snapshot:       dnsmasqOptionsSnapshot{Section: "cfg01411c"},
		pendingChanges: "dhcp.cfg01411c.server='192.168.3.1'",
	}
	manager := newFakeOpenWrtDNSMasqLifecycle(t, fake)

	err := manager.Activate()
	if err == nil || !strings.Contains(err.Error(), "未提交") {
		t.Fatalf("activate error = %v", err)
	}
	if fake.restartCalls != 0 {
		t.Fatalf("restart calls = %d", fake.restartCalls)
	}
}
