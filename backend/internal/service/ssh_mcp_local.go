package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

// SSHMCPLocalHostID identifies the explicitly enabled, built-in MCP target.
// It is never inferred from a saved SSH address or stored in ssh_hosts.
const SSHMCPLocalHostID int64 = -1

func getSSHMCPHost(db *store.Store, hostID int64) (*model.SSHHost, error) {
	if hostID != SSHMCPLocalHostID {
		host, err := db.GetSSHHost(hostID)
		return host, normalizeSSHStoreError(err)
	}
	cfg, err := db.GetSSHMCPConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || !cfg.LocalEnabled {
		return nil, sshError("SSH_MCP_LOCAL_DISABLED", "本机 MCP 访问未启用，请在 MCP 配置中启用", nil)
	}
	host := &model.SSHHost{ID: SSHMCPLocalHostID, Name: "本机", ConnectionMode: "local", Enabled: true,
		HostKeyStatus: "not_required", Tags: []string{}, Notes: "AckWrap 服务所在设备；以 AckWrap 进程用户执行，无需 SSH"}
	if current, err := user.Current(); err == nil {
		host.Username = current.Username
	}
	return host, nil
}

func (svc *SSHHostService) ListMCPHosts() ([]model.SSHHost, error) {
	hosts, err := svc.ListHosts()
	if err != nil {
		return nil, err
	}
	local, err := getSSHMCPHost(svc.store, SSHMCPLocalHostID)
	var serviceErr *SSHServiceError
	if errors.As(err, &serviceErr) && serviceErr.Code == "SSH_MCP_LOCAL_DISABLED" {
		return hosts, nil
	}
	if err != nil {
		return nil, err
	}
	return append([]model.SSHHost{*local}, hosts...), nil
}

func (svc *SSHHostService) TestMCPHost(ctx context.Context, hostID int64) (*model.SSHConnectionTestResult, error) {
	if hostID != SSHMCPLocalHostID {
		return svc.TestHost(ctx, hostID)
	}
	err := svc.withMCPOperation(ctx, hostID, 15*time.Second, "test", func(context.Context, *model.SSHHost) error { return nil })
	if err != nil {
		return nil, err
	}
	return &model.SSHConnectionTestResult{Success: true, ConnectionMode: "local", ServerVersion: runtime.GOOS}, nil
}

func normalizeMCPPath(hostID int64, value string, allowCurrent bool) (string, error) {
	if hostID != SSHMCPLocalHostID {
		return normalizeSFTPPath(value, allowCurrent)
	}
	value = strings.TrimSpace(value)
	if len(value) > sshMaximumSFTPPathSize || strings.ContainsAny(value, "\x00\r\n") || (value == "" && !allowCurrent) {
		return "", sshError("SSH_SFTP_INVALID_PATH", "本机路径无效或为空", nil)
	}
	if !filepath.IsAbs(value) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", sshError("SSH_SFTP_INVALID_PATH", "无法确定本机主目录，请使用绝对路径", err)
		}
		value = filepath.Join(home, value)
	}
	value = filepath.Clean(value)
	if !allowCurrent && filepath.Dir(value) == value {
		return "", sshError("SSH_SFTP_INVALID_PATH", "目标必须是文件路径", nil)
	}
	return value, nil
}

func (svc *SSHHostService) executeMCPLocal(ctx context.Context, request model.SSHMCPExecRequest) (*model.SSHMCPExecResult, error) {
	result := &model.SSHMCPExecResult{HostID: request.HostID, ExitCode: -1}
	err := svc.withMCPOperation(ctx, request.HostID, time.Duration(request.Timeout)*time.Second, "exec", func(ctx context.Context, _ *model.SSHHost) error {
		command := exec.CommandContext(ctx, "/bin/sh", "-c", request.Command)
		if runtime.GOOS == "windows" {
			command = exec.CommandContext(ctx, "powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding = New-Object System.Text.UTF8Encoding; "+request.Command)
		}
		if home, err := os.UserHomeDir(); err == nil {
			command.Dir = home
		}
		configureMCPLocalCommand(command)
		command.WaitDelay = time.Second
		stdout, stderr := &sshMCPOutput{}, &sshMCPOutput{}
		command.Stdout, command.Stderr = stdout, stderr
		err := command.Run()
		result.Stdout, result.Stderr = stdout.buffer.String(), stderr.buffer.String()
		result.Truncated = stdout.truncated || stderr.truncated
		if err == nil {
			result.ExitCode = 0
			return nil
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitCode()
			return sshError("SSH_MCP_COMMAND_FAILED", "本机命令返回非零退出码", nil)
		}
		return sshError("SSH_MCP_EXEC_FAILED", "本机命令执行失败", err)
	})
	return result, err
}
