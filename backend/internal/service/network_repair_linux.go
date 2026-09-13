//go:build linux

package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func forceCleanupPlatformSingboxState(statePath string) (platformCleanupResult, error) {
	running, err := linuxSingboxProcessRunning()
	if err != nil || running {
		return platformCleanupResult{ProcessRunning: running}, err
	}
	return forceCleanupSingboxState(statePath, singboxOpenWrtNFTInclude, func(name string, args ...string) ([]byte, error) {
		path, err := exec.LookPath(name)
		if err != nil {
			return nil, err
		}
		timeout := linuxNetworkCommandTimeout
		if name == "fw4" {
			timeout = linuxFW4CommandTimeout
		}
		output, err := runLinuxNetworkCommand(timeout, path, args...)
		if err != nil {
			return output, cleanupCommandError(name+" "+strings.Join(args, " "), output, err)
		}
		return output, nil
	})
}

func stopPlatformSingboxForRepair() error {
	for _, signal := range []syscall.Signal{syscall.SIGTERM, syscall.SIGKILL} {
		entries, err := os.ReadDir("/proc")
		if err != nil {
			return err
		}
		for _, entry := range entries {
			pid, err := strconv.Atoi(entry.Name())
			if err != nil || !entry.IsDir() {
				continue
			}
			name, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			if strings.TrimSpace(string(name)) == "sing-box" {
				if err := syscall.Kill(pid, signal); err != nil && !errors.Is(err, syscall.ESRCH) {
					return fmt.Errorf("stop sing-box process %d: %w", pid, err)
				}
			}
		}
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			running, err := linuxSingboxProcessRunning()
			if err != nil || !running {
				return err
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	return fmt.Errorf("sing-box process did not stop after SIGKILL")
}
