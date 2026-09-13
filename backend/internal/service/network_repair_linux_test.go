//go:build linux

package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxNetworkRepairIgnoresExitedProcesses(t *testing.T) {
	for _, test := range []struct {
		name, status string
		running      bool
	}{
		{"sing-box", "State:\tS (sleeping)\n", true},
		{"sing-box", "State:\tZ (zombie)\n", false},
		{"sing-box", "State:\tX (dead)\n", false},
		{"easytier-core", "State:\tS (sleeping)\n", false},
	} {
		t.Run(test.name+test.status[:8], func(t *testing.T) {
			root := t.TempDir()
			process := filepath.Join(root, "123")
			if err := os.Mkdir(process, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(process, "comm"), []byte(test.name+"\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(process, "status"), []byte(test.status), 0o600); err != nil {
				t.Fatal(err)
			}
			running, err := linuxSingboxProcessRunningAt(root)
			if err != nil || running != test.running {
				t.Fatalf("process running = %t, want %t: %v", running, test.running, err)
			}
		})
	}
}
