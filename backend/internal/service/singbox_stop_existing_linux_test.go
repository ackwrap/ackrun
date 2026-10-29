//go:build linux

package service

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The synthetic proc directory contains only children created by this test.
// Their status comes from /proc so exit/zombie handling is exercised for real.
func recoveryProcess(t *testing.T, procRoot, name, mode, appearPath string) (*exec.Cmd, string) {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestSingboxRecoveryHelperProcess$")
	cmd.Env = append(os.Environ(), "ACKWRAP_RECOVERY_HELPER="+mode, "ACKWRAP_RECOVERY_APPEAR="+appearPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || ready != "ready\n" {
		t.Fatalf("helper readiness = %q, %v", ready, err)
	}
	dir := filepath.Join(procRoot, strconv.Itoa(cmd.Process.Pid))
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	comm := filepath.Join(dir, "comm")
	if err := os.WriteFile(comm, []byte(name+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid), "status"), filepath.Join(dir, "status")); err != nil {
		t.Fatal(err)
	}
	return cmd, comm
}

func TestSingboxRecoveryHelperProcess(t *testing.T) {
	mode := os.Getenv("ACKWRAP_RECOVERY_HELPER")
	if mode == "" {
		return
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM)
	fmt.Println("ready")
	for range ch {
		if mode == "ignore" {
			continue
		}
		if path := os.Getenv("ACKWRAP_RECOVERY_APPEAR"); path != "" {
			if err := os.WriteFile(path, []byte("sing-box\n"), 0600); err != nil {
				os.Exit(2)
			}
		}
		os.Exit(0)
	}
}

func TestStopExistingSingboxWaitsForExitAndPreservesOtherProcesses(t *testing.T) {
	for _, mode := range []string{"graceful", "ignore"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			core, _ := recoveryProcess(t, root, "sing-box", mode, "")
			other, _ := recoveryProcess(t, root, "other-vpn", "graceful", "")
			if err := stopLinuxSingboxProcesses(root, 200*time.Millisecond, 2*time.Second); err != nil {
				t.Fatal(err)
			}
			if running, err := linuxSingboxProcessRunningAt(root); err != nil || running {
				t.Fatalf("cleanup allowed before process exit: running=%t error=%v", running, err)
			}
			if err := other.Process.Signal(syscall.Signal(0)); err != nil {
				t.Fatalf("unrelated process stopped: %v", err)
			}
			err := core.Wait()
			if mode == "graceful" && err != nil {
				t.Fatalf("graceful exit failed: %v", err)
			}
			if mode == "ignore" {
				status := core.ProcessState.Sys().(syscall.WaitStatus)
				if !status.Signaled() || status.Signal() != syscall.SIGKILL {
					t.Fatalf("expected forced exit, got %v", core.ProcessState)
				}
			}
		})
	}
}

func TestStopExistingSingboxRejectsNewProcessDuringRecovery(t *testing.T) {
	root := t.TempDir()
	respawned, comm := recoveryProcess(t, root, "not-yet-singbox", "graceful", "")
	recoveryProcess(t, root, "sing-box", "graceful", comm)
	err := stopLinuxSingboxProcesses(root, 200*time.Millisecond, 200*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "重新拉起") {
		t.Fatalf("new process was ignored: %v", err)
	}
	if err := respawned.Process.Signal(syscall.Signal(0)); err != nil {
		t.Fatalf("escalation killed a newly discovered process: %v", err)
	}
}

func TestStopExistingSingboxRejectsUnreadableProcessState(t *testing.T) {
	root := t.TempDir()
	_, comm := recoveryProcess(t, root, "sing-box", "graceful", "")
	status := filepath.Join(filepath.Dir(comm), "status")
	if err := os.Remove(status); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(status, []byte("Name:\tsing-box\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := stopLinuxSingboxProcesses(root, 0, 0); err == nil {
		t.Fatal("missing process state was treated as a stopped core")
	}
}

func TestStartRecoveredCoreRechecksAfterCleanup(t *testing.T) {
	root := t.TempDir()
	verify := func() error { return verifyLinuxSingboxStoppedAt(root) }
	if err := verify(); err != nil {
		t.Fatal(err)
	}
	// Model a supervisor creating a process after the cleanup check passed.
	recoveryProcess(t, root, "sing-box", "graceful", "")
	cmd := exec.Command("true")
	if err := startRecoveredCore(cmd, verify); err == nil || !strings.Contains(err.Error(), "再次运行") {
		t.Fatalf("respawn after cleanup was ignored: %v", err)
	}
	if cmd.Process != nil {
		t.Fatal("launched a second core while another process was running")
	}
	if err := stopLinuxSingboxProcesses(root, time.Second, time.Second); err != nil {
		t.Fatal(err)
	}
	if err := startRecoveredCore(cmd, verify); err != nil {
		t.Fatalf("could not launch replacement after recovery: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
}
