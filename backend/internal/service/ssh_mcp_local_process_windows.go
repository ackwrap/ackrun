package service

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureMCPLocalCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	command.Cancel = func() error {
		kill := exec.Command("taskkill.exe", "/PID", strconv.Itoa(command.Process.Pid), "/T", "/F")
		kill.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if err := kill.Run(); err != nil {
			return command.Process.Kill()
		}
		return nil
	}
}
