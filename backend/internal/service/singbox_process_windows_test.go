//go:build windows

package service

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPrepareProcessCommandCreatesDedicatedWindowsProcessGroup(t *testing.T) {
	command := exec.Command("cmd.exe", "/c", "exit", "0")
	if err := prepareProcessCommand(command); err != nil {
		t.Fatal(err)
	}
	if command.SysProcAttr == nil || command.SysProcAttr.CreationFlags&windows.CREATE_NEW_PROCESS_GROUP == 0 {
		t.Fatal("sing-box command must use a dedicated Windows process group")
	}
}
