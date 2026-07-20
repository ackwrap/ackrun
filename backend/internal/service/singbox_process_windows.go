//go:build windows

package service

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"golang.org/x/sys/windows"
)

var (
	kernel32AllocConsole = windows.NewLazySystemDLL("kernel32.dll").NewProc("AllocConsole")
	processConsoleOnce   sync.Once
	processConsoleErr    error
)

func prepareProcessCommand(command *exec.Cmd) error {
	processConsoleOnce.Do(func() {
		if _, err := windows.GetConsoleCP(); err == nil {
			return
		}
		result, _, callErr := kernel32AllocConsole.Call()
		if result == 0 {
			processConsoleErr = fmt.Errorf("allocate Windows console for graceful core shutdown: %w", callErr)
		}
	})
	if processConsoleErr != nil {
		return processConsoleErr
	}
	if command.SysProcAttr == nil {
		command.SysProcAttr = &syscall.SysProcAttr{}
	}
	command.SysProcAttr.CreationFlags |= windows.CREATE_NEW_PROCESS_GROUP
	return nil
}

func requestProcessShutdown(process *os.Process) error {
	return windows.GenerateConsoleCtrlEvent(windows.CTRL_BREAK_EVENT, uint32(process.Pid))
}
