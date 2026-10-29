//go:build linux

package service

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
)

func stopPlatformSingboxForStart() error {
	return stopLinuxSingboxProcesses("/proc", 10*time.Second, 2*time.Second)
}

func stopLinuxSingboxProcesses(procRoot string, gracefulTimeout, killTimeout time.Duration) error {
	pids, err := linuxSingboxProcessIDsAt(procRoot)
	if err != nil || len(pids) == 0 {
		return err
	}
	var processes []*os.Process
	defer func() {
		for _, process := range processes {
			_ = process.Release()
		}
	}()
	for _, pid := range pids {
		process, err := os.FindProcess(pid)
		if err != nil {
			return fmt.Errorf("open sing-box process %d: %w", pid, err)
		}
		processes = append(processes, process)
		// Go 1.26 uses pidfd handles on supported Linux kernels. Require one
		// explicitly: FindProcess may otherwise fall back to a recyclable PID.
		if err := process.WithHandle(func(uintptr) {}); err != nil {
			if errors.Is(err, os.ErrProcessDone) {
				continue
			}
			return fmt.Errorf("无法安全停止 sing-box (pid=%d)，需要可用的 pidfd 进程句柄: %w", pid, err)
		}
	}
	for _, process := range processes {
		pid := process.Pid
		// Recheck after opening the process handle. In particular, do not signal
		// another program that reused a PID from the directory snapshot.
		running, err := linuxSingboxProcessAliveAt(procRoot, pid)
		if err != nil {
			return err
		}
		if !running {
			continue
		}
		logging.Info("core.recover", "停止已有 sing-box: pid=%d signal=SIGTERM", pid)
		if err := process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("stop sing-box process %d: %w", pid, err)
		}
	}
	if stopped, err := waitLinuxSingboxStopped(procRoot, gracefulTimeout); err != nil || stopped {
		return err
	}
	// Escalate only the original process handles, never a fresh list of PIDs.
	// A competing supervisor that respawns sing-box must cause startup to fail.
	for _, process := range processes {
		running, err := linuxSingboxProcessAliveAt(procRoot, process.Pid)
		if err != nil {
			return err
		}
		if !running {
			continue
		}
		logging.Info("core.recover", "已有 sing-box 未按时退出，强制停止: pid=%d", process.Pid)
		if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
			return fmt.Errorf("kill sing-box process %d: %w", process.Pid, err)
		}
	}
	if stopped, err := waitLinuxSingboxStopped(procRoot, killTimeout); err != nil || stopped {
		return err
	}
	return errors.New("sing-box 进程仍在运行或被其他服务重新拉起")
}

func verifyPlatformSingboxStopped() error {
	return verifyLinuxSingboxStoppedAt("/proc")
}

func verifyLinuxSingboxStoppedAt(procRoot string) error {
	running, err := linuxSingboxProcessRunningAt(procRoot)
	if err != nil {
		return err
	}
	if running {
		return errors.New("检测到 sing-box 再次运行，可能被其他服务自动拉起，取消恢复")
	}
	return nil
}

func waitLinuxSingboxStopped(procRoot string, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for {
		running, err := linuxSingboxProcessRunningAt(procRoot)
		if err != nil || !running {
			return !running, err
		}
		if !time.Now().Before(deadline) {
			return false, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}
