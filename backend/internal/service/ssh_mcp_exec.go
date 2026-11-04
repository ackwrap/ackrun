package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const sshMCPMaxContent = 1 << 20

func (svc *SSHHostService) withMCPHost(ctx context.Context, hostID int64, timeout time.Duration, action string, run func(context.Context, *ssh.Client) error) (resultErr error) {
	return svc.withMCPOperation(ctx, hostID, timeout, action, func(ctx context.Context, host *model.SSHHost) error {
		connectCtx, connectCancel := context.WithTimeout(ctx, sshConnectTimeout)
		client, _, _, _, err := svc.connect(connectCtx, host)
		connectCancel()
		if err != nil {
			return err
		}
		defer client.Close()
		stop := context.AfterFunc(ctx, func() { _ = client.Close() })
		defer stop()
		return run(ctx, client)
	})
}

func (svc *SSHHostService) withMCPOperation(ctx context.Context, hostID int64, timeout time.Duration, action string, run func(context.Context, *model.SSHHost) error) (resultErr error) {
	host, err := getSSHMCPHost(svc.store, hostID)
	if err != nil {
		return err
	}
	if !host.Enabled {
		return sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	if err := svc.reserveSessionSlot(hostID); err != nil {
		return err
	}
	defer svc.releaseSessionSlot(hostID)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	svc.sessionMu.Lock()
	if svc.closed {
		svc.sessionMu.Unlock()
		cancel()
		return sshError("SSH_SESSION_CLOSED", "SSH 会话服务已停止", nil)
	}
	if svc.mcpCancels == nil {
		svc.mcpCancels = make(map[context.Context]context.CancelFunc)
	}
	svc.mcpCancels[ctx] = cancel
	svc.sessionMu.Unlock()
	defer func() {
		cancel()
		svc.sessionMu.Lock()
		delete(svc.mcpCancels, ctx)
		svc.sessionMu.Unlock()
	}()
	started := svc.now()
	defer func() {
		result, code := "success", ""
		if resultErr != nil {
			result = "error"
			code, _ = sshErrorInfo(resultErr)
			logging.Error("ssh_mcp.operation", "SSH MCP 操作失败: host_id=%d action=%s code=%s", hostID, action, code)
		} else {
			logging.Info("ssh_mcp.operation", "SSH MCP 操作完成: host_id=%d action=%s", hostID, action)
		}
		svc.audit(host, "mcp_"+action, result, code, svc.now().Sub(started), "")
	}()
	if err := ctx.Err(); err != nil {
		return sshError("SSH_MCP_CANCELLED", "MCP 操作已取消或超时", err)
	}
	err = run(ctx, host)
	if ctx.Err() != nil {
		// Keep the failing connection phase instead of replacing it with a
		// generic operation timeout when both share the same deadline.
		var serviceErr *SSHServiceError
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && errors.As(err, &serviceErr) &&
			(serviceErr.Code == "SSH_CONNECT_TIMEOUT" || serviceErr.Code == "SSH_SESSION_OPEN_TIMEOUT") {
			return err
		}
		message := "SSH MCP 操作已取消"
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = "SSH MCP 操作超时"
			if action == "exec" {
				message = "SSH 命令执行超时"
			}
		}
		return sshError("SSH_MCP_CANCELLED", message, ctx.Err())
	}
	return err
}

type sshMCPOutput struct {
	buffer    bytes.Buffer
	truncated bool
}

func (output *sshMCPOutput) Write(data []byte) (int, error) {
	length := len(data)
	remaining := sshMCPMaxContent - output.buffer.Len()
	if len(data) > remaining {
		data = data[:remaining]
		output.truncated = true
	}
	_, _ = output.buffer.Write(data)
	return length, nil
}

func (svc *SSHHostService) ExecuteMCP(ctx context.Context, request model.SSHMCPExecRequest) (*model.SSHMCPExecResult, error) {
	if strings.TrimSpace(request.Command) == "" || len(request.Command) > 64<<10 || strings.ContainsRune(request.Command, 0) {
		return nil, sshError("SSH_MCP_INVALID", "命令不能为空或超过 64 KiB，且不能包含 NUL", nil)
	}
	if request.Timeout == 0 {
		request.Timeout = 30
	}
	if request.Timeout < 1 || request.Timeout > 600 {
		return nil, sshError("SSH_MCP_INVALID", "超时必须为 1 到 600 秒", nil)
	}
	if request.HostID == SSHMCPLocalHostID {
		return svc.executeMCPLocal(ctx, request)
	}
	result := &model.SSHMCPExecResult{HostID: request.HostID, ExitCode: -1}
	timeout := time.Duration(request.Timeout) * time.Second
	err := svc.withMCPOperation(ctx, request.HostID, timeout, "exec", func(ctx context.Context, host *model.SSHHost) error {
		client, _, _, _, err := svc.connectWithTimeout(ctx, host, timeout)
		if err != nil {
			return err
		}
		defer client.Close()
		stop := context.AfterFunc(ctx, func() { _ = client.Close() })
		defer stop()
		session, err := newSSHSession(ctx, client)
		if err != nil {
			return err
		}
		defer session.Close()
		stdout, stderr := &sshMCPOutput{}, &sshMCPOutput{}
		session.Stdout, session.Stderr = stdout, stderr
		err = session.Run(request.Command)
		result.Stdout, result.Stderr = stdout.buffer.String(), stderr.buffer.String()
		result.Truncated = stdout.truncated || stderr.truncated
		if err == nil {
			result.ExitCode = 0
			return nil
		}
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			result.ExitCode = exit.ExitStatus()
			return sshError("SSH_MCP_COMMAND_FAILED", "远程命令返回非零退出码", nil)
		}
		return sshError("SSH_MCP_EXEC_FAILED", "SSH 命令执行失败", err)
	})
	return result, err
}
