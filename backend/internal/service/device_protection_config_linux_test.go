//go:build linux

package service

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

func protectedConfigFixture(t *testing.T) (*SingboxService, map[string]interface{}) {
	t.Helper()
	db, g, _ := devicePolicyFixture(t)
	g.paths.ConfigDir = filepath.Join(g.paths.DataDir, "config")
	g.paths.ConfigPath = filepath.Join(g.paths.ConfigDir, "custom.json")
	g.paths.BinaryPath = filepath.Join(g.paths.DataDir, "sing-box")
	if err := os.MkdirAll(g.paths.ConfigDir, 0700); err != nil {
		t.Fatal(err)
	}
	// Check the actual candidate file: stale sections must be repaired before
	// check, and validation failures must never replace the active file.
	script := `#!/bin/sh
case "$1" in
version) echo 'sing-box version 1.14.3'; exit 0 ;;
check)
  if [ -f "$(dirname "$0")/reject-check" ]; then
    echo 'invalid node 192.0.2.10 synthetic-secret' >&2
    exit 1
  fi
  if grep -q 'stale-managed-section' "$3"; then
    echo 'stale managed section' >&2
    exit 1
  fi
  exit 0 ;;
*) exit 1 ;;
esac
`
	if err := os.WriteFile(g.paths.BinaryPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	g.dnsmasqSupported = platformSupportsDNSMasqTakeover
	firewall, _ := testDeviceProtection(t)
	core := NewSingboxService(g.paths, NewRealtimeService(), nil, db)
	core.deviceProtection = firewall
	settings := NewSettingsService(db)
	settings.SetModeDependencies(core, g)
	enabled := true
	if err := settings.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	config, err := g.protectedConfigSections()
	if err != nil {
		t.Fatal(err)
	}
	inbounds, err := g.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	config["inbounds"] = inbounds
	config["log"] = map[string]interface{}{"level": "debug"}
	config["experimental"] = map[string]interface{}{"cache_file": map[string]interface{}{"enabled": true, "path": "custom-cache.db"}}
	config["services"] = []interface{}{map[string]interface{}{"type": "api", "secret": "synthetic-secret"}}
	return core, config
}

func writeProtectedConfig(t *testing.T, core *SingboxService, config map[string]interface{}) []byte {
	t.Helper()
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core.paths.ConfigPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestProtectedConfigStartupRepairsStaleSections(t *testing.T) {
	for _, key := range protectedConfigKeys {
		t.Run(key, func(t *testing.T) {
			core, config := protectedConfigFixture(t)
			if key == "dns" {
				config[key].(map[string]interface{})["final"] = "stale-managed-section"
			} else {
				config[key] = "stale-managed-section"
			}
			before := writeProtectedConfig(t, core, config)
			logging.ClearToolLogs()
			path, err := core.validateActiveConfig()
			if err != nil || path != core.paths.ConfigPath {
				t.Fatalf("startup preflight failed: path=%s error=%v", path, err)
			}
			if err := core.validateProtectedConfig(path); err != nil {
				t.Fatalf("repaired config failed protection checks: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var original, repaired map[string]interface{}
			_ = json.Unmarshal(before, &original)
			_ = json.Unmarshal(after, &repaired)
			for _, untouched := range []string{"log", "experimental", "services", "inbounds"} {
				if !reflect.DeepEqual(original[untouched], repaired[untouched]) {
					t.Fatalf("unrelated %s changed", untouched)
				}
			}
			backups, err := filepath.Glob(filepath.Join(core.paths.ConfigDir, "backup", "*.bak.json"))
			if err != nil || len(backups) != 1 {
				t.Fatalf("missing pre-repair backup: %v %v", backups, err)
			}
			backup, err := os.ReadFile(backups[0])
			if err != nil || !bytes.Equal(backup, before) {
				t.Fatal("backup does not contain the original configuration")
			}
			logged := false
			for _, entry := range logging.ListToolLogs(1000) {
				if entry.Tag == "device_protection.reconcile" && strings.Contains(entry.Message, "已同步受管配置: "+key) {
					logged = true
				}
			}
			if !logged {
				t.Fatal("successful repair did not identify the changed section")
			}
			fixedTime := time.Unix(1700000000, 0)
			if err := os.Chtimes(path, fixedTime, fixedTime); err != nil {
				t.Fatal(err)
			}
			if _, err := core.validateActiveConfig(); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil || !info.ModTime().Equal(fixedTime) {
				t.Fatal("identical config rewritten on subsequent startup")
			}
		})
	}
}

func TestProtectedConfigFailedRepairPreservesRunningCoreAndFile(t *testing.T) {
	for _, failure := range []string{"check", "backup", "policy", "firewall", "inbound"} {
		t.Run(failure, func(t *testing.T) {
			core, config := protectedConfigFixture(t)
			config["route"] = "stale-managed-section"
			before := writeProtectedConfig(t, core, config)
			switch failure {
			case "check":
				if err := os.WriteFile(filepath.Join(core.paths.DataDir, "reject-check"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "backup":
				if err := os.WriteFile(filepath.Join(core.paths.ConfigDir, "backup"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			case "policy":
				dns, err := core.store.GetDNSGlobalSettings()
				if err != nil {
					t.Fatal(err)
				}
				dns.Enabled = false
				if err := core.store.SetDNSGlobalSettings(dns); err != nil {
					t.Fatal(err)
				}
			case "firewall":
				if err := core.deviceProtection.Disable(); err != nil {
					t.Fatal(err)
				}
			case "inbound":
				config["inbounds"] = []interface{}{}
				before = writeProtectedConfig(t, core, config)
			}
			process := exec.Command("sleep", "60")
			if err := process.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = process.Process.Kill(); _ = process.Wait() })
			core.cmd, core.pid = process, process.Process.Pid
			logging.ClearToolLogs()
			if _, err := core.Restart(); err == nil {
				t.Fatal("failed repair was ignored")
			} else if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "192.0.2.10") {
				t.Fatal("repair error exposed configuration values")
			}
			if err := process.Process.Signal(syscall.Signal(0)); err != nil || !core.IsRunning() {
				t.Fatal("failed repair stopped the running core")
			}
			after, err := os.ReadFile(core.paths.ConfigPath)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatal("failed repair changed the active configuration")
			}
			files, err := filepath.Glob(filepath.Join(core.paths.ConfigDir, ".ackwrap-protection-*"))
			if err != nil || len(files) != 0 {
				t.Fatal("failed repair left a staged configuration")
			}
			logged := false
			for _, entry := range logging.ListToolLogs(1000) {
				if entry.Tag == "device_protection.reconcile" && entry.Level == "error" {
					logged = true
				}
			}
			if !logged {
				t.Fatal("repair failure was not logged")
			}
		})
	}
}

func TestProtectedConfigDisabledLeavesCustomConfigUntouched(t *testing.T) {
	core, config := protectedConfigFixture(t)
	settings, err := core.store.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.DeviceProtectionEnabled = false
	if err := core.store.SetGeneralSettings(settings); err != nil {
		t.Fatal(err)
	}
	config["route"] = "stale-managed-section"
	before := writeProtectedConfig(t, core, config)
	configFileMu.Lock()
	err = core.reconcileProtectedConfig(core.paths.ConfigPath)
	configFileMu.Unlock()
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(core.paths.ConfigPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("disabled protection changed a custom config")
	}
}

func TestProtectedConfigRepairUnderCoreUpdateLocks(t *testing.T) {
	core, config := protectedConfigFixture(t)
	config["route"] = "stale-managed-section"
	writeProtectedConfig(t, core, config)
	done := make(chan error, 1)
	go func() {
		release := core.store.HoldConfigSnapshot()
		defer release()
		core.lifecycleMu.Lock()
		defer core.lifecycleMu.Unlock()
		configFileMu.Lock()
		defer configFileMu.Unlock()
		core.networkLifecycleMu.Lock()
		defer core.networkLifecycleMu.Unlock()
		core.mu.Lock()
		defer core.mu.Unlock()
		_, err := core.validateActiveConfigLocked()
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("repair deadlocked under the core update/start lock order")
	}
}
