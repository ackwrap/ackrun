package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackwrap/internal/paths"
)

func TestDiagnosticsDoesNotExposeCoreLogContent(t *testing.T) {
	root := t.TempDir()
	p := &paths.Paths{
		DataDir:    root,
		BinaryDir:  filepath.Join(root, "bin"),
		BinaryPath: filepath.Join(root, "bin", "sing-box"),
		ConfigDir:  filepath.Join(root, "config"),
		ConfigPath: filepath.Join(root, "config", "config.json"),
	}
	logs := NewCoreLogService()
	logs.Append("stderr", time.Now().UnixMilli(), "FATAL sensitive-value-must-not-leak")
	svc := NewSingboxService(p, nil, logs, nil)

	report, err := svc.Diagnostics()
	if err != nil {
		t.Fatalf("build diagnostics: %v", err)
	}
	if report.Logs.Total != 1 || report.Logs.Stderr != 1 || report.Logs.ErrorLines != 1 {
		t.Fatalf("unexpected log summary: %+v", report.Logs)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	if strings.Contains(string(data), "sensitive-value-must-not-leak") {
		t.Fatal("diagnostics exposed core log content")
	}
}
