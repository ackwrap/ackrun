package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
)

func TestPingBypassSettingRejectsRunningCoreAndMixedProtectionUpdate(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enable", false: "disable"}[enabled], func(t *testing.T) {
			svc, db, previous, _ := geoSettingsTestService(t)
			previous.RealIPPingBypassEnabled = !enabled
			if err := db.SetGeneralSettings(previous); err != nil {
				t.Fatal(err)
			}
			running := &SingboxService{pid: 1, cmd: &exec.Cmd{Process: &os.Process{}}}
			svc.SetModeDependencies(running, nil)
			if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{RealIPPingBypassEnabled: &enabled}); err == nil || !strings.Contains(err.Error(), "请先停止核心") {
				t.Fatalf("running core accepted a Ping policy change: %v", err)
			}
			protection := true
			if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{RealIPPingBypassEnabled: &enabled, DeviceProtectionEnabled: &protection}); err == nil || !strings.Contains(err.Error(), "单独保存") {
				t.Fatalf("combined protection update accepted: %v", err)
			}
			got, err := db.GetGeneralSettings()
			if err != nil || *got != *previous {
				t.Fatalf("rejected request changed settings: %+v, err=%v", got, err)
			}
		})
	}
}

func TestPingBypassSettingReconcilesAndRollsBack(t *testing.T) {
	for _, outcome := range []string{"success", "invalid", "error"} {
		t.Run(outcome, func(t *testing.T) {
			svc, db, previous, _ := geoSettingsTestService(t)
			dir := t.TempDir()
			p := &paths.Paths{BinaryPath: filepath.Join(dir, "sing-box"), ConfigDir: dir, ConfigPath: filepath.Join(dir, "config.json")}
			for _, path := range []string{p.BinaryPath, p.ConfigPath} {
				if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			generator := &modeConfigGeneratorStub{result: &model.ConfigGenerateResponse{Valid: outcome == "success", Error: "synthetic validation failure"}}
			if outcome == "error" {
				generator.err = errors.New("synthetic apply failure")
			}
			svc.SetModeDependencies(&SingboxService{paths: p}, generator)
			enabled := true
			err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{RealIPPingBypassEnabled: &enabled})
			if outcome == "success" && err != nil || outcome != "success" && (err == nil || !strings.Contains(err.Error(), "已回滚")) {
				t.Fatalf("unexpected save result: %v", err)
			}
			got, err := db.GetGeneralSettings()
			if outcome == "success" {
				previous.RealIPPingBypassEnabled = true
			}
			if err != nil || *got != *previous || generator.calls != 1 {
				t.Fatalf("settings/application mismatch: %+v, calls=%d, err=%v", got, generator.calls, err)
			}
		})
	}
}
