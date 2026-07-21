package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestStartIfConfigured(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "sing-box")
	if err := os.WriteFile(binaryPath, nil, 0600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name          string
		isRunning     bool
		binaryPath    string
		configured    bool
		activeErr     error
		startErr      error
		wantErr       bool
		wantStartCall int
	}{
		{name: "already running", isRunning: true, binaryPath: filepath.Join(t.TempDir(), "missing")},
		{name: "binary missing", binaryPath: filepath.Join(t.TempDir(), "missing")},
		{name: "config missing", binaryPath: binaryPath},
		{name: "active config lookup failed", binaryPath: binaryPath, activeErr: errors.New("read config"), wantErr: true},
		{name: "core start failed", binaryPath: binaryPath, configured: true, startErr: errors.New("start core"), wantErr: true, wantStartCall: 1},
		{name: "starts configured core", binaryPath: binaryPath, configured: true, wantStartCall: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			startCalls := 0
			err := startIfConfigured(
				func() bool { return tt.isRunning },
				tt.binaryPath,
				func() (string, bool, error) { return "config.json", tt.configured, tt.activeErr },
				func() error {
					startCalls++
					return tt.startErr
				},
			)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, want error = %t", err, tt.wantErr)
			}
			if startCalls != tt.wantStartCall {
				t.Fatalf("start calls = %d, want %d", startCalls, tt.wantStartCall)
			}
		})
	}
}
