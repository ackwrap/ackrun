//go:build linux

package service

import (
	"os"
	"os/exec"
	"syscall"
)

func prepareProcessCommand(*exec.Cmd) error { return nil }

func requestProcessShutdown(process *os.Process) error {
	return process.Signal(syscall.SIGTERM)
}
