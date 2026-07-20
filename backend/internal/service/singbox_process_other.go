//go:build !linux && !windows

package service

import (
	"os"
	"os/exec"
)

func prepareProcessCommand(*exec.Cmd) error { return nil }

func requestProcessShutdown(process *os.Process) error {
	return process.Signal(os.Interrupt)
}
