package service

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshMCPUploadTicketTTL      = 15 * time.Minute
	sshMCPMaximumUploadTickets = 128
)

type sshMCPUploadGrant struct {
	request    model.SSHMCPTransferRequest
	secretHash [sha256.Size]byte
	configHash string
	expiresAt  time.Time
}

func (svc *SSHMCPService) PrepareUpload(request model.SSHMCPTransferRequest) (*model.SSHMCPUploadTicket, error) {
	remote, err := validateMCPTransfer(request)
	if err != nil {
		return nil, err
	}
	host, err := svc.store.GetSSHHost(request.HostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	request.Path = remote
	id, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	secret, err := randomHex(32)
	if err != nil {
		return nil, err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cfg, err := svc.store.GetSSHMCPConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled || cfg.TokenHash == "" {
		return nil, sshError("SSH_MCP_DISABLED", "SSH MCP 尚未启用", nil)
	}
	now := svc.now()
	for key, grant := range svc.uploads {
		if !now.Before(grant.expiresAt) || grant.configHash != cfg.TokenHash {
			delete(svc.uploads, key)
		}
	}
	if len(svc.uploads) >= sshMCPMaximumUploadTickets {
		return nil, sshError("SSH_MCP_BUSY", "待使用的上传地址过多，请稍后重试", nil)
	}
	if svc.uploads == nil {
		svc.uploads = make(map[string]sshMCPUploadGrant)
	}
	expiresAt := now.Add(sshMCPUploadTicketTTL)
	svc.uploads[id] = sshMCPUploadGrant{request: request, secretHash: sha256.Sum256([]byte(secret)), configHash: cfg.TokenHash, expiresAt: expiresAt}
	logging.Info("ssh_mcp.upload_ready", "生成单次上传地址: host_id=%d", request.HostID)
	return &model.SSHMCPUploadTicket{
		Status: "ready", EndpointPath: SSHMCPEndpoint + "/uploads/" + id, Method: http.MethodPut,
		Headers:   map[string]string{"Content-Type": "application/octet-stream", "Authorization": "Bearer " + secret},
		ExpiresAt: expiresAt.UnixMilli(), ExpiresInSeconds: int(sshMCPUploadTicketTTL.Seconds()),
		TimeoutSeconds: int(SSHMCPTransferTimeout.Seconds()), SingleUse: true,
		Instructions: "尚未上传文件。下一步必须在保存本地文件的调用端机器上执行 curl、Bash、PowerShell 或 Python HTTP 上传：向 upload_url 发送 PUT，完整使用返回的 headers，请求体为本地文件的原始二进制流。curl 使用 --upload-file，PowerShell 使用 -InFile，Python 以 rb 打开文件并流式发送。不要 Base64、JSON、multipart/form-data，也不要通过 ssh_exec 读取调用端文件；ssh_exec 在远端主机执行。无需取得或发送长期 MCP Token，必须使用返回的临时 Authorization。地址绑定目标主机、远程路径、覆盖选项及 SHA-1 校验值，不要添加或修改 X-SSH-* 请求头。15 分钟内开始上传，单次传输最长 1 小时，无文件总大小限制。地址仅供一次上传尝试；失败或重传须重新调用 upload_file，服务重启或 MCP Token 变更后旧地址失效。只在 HTTP 成功响应且 success=true 后报告上传完成，并核对 bytes 和 sha1（40 位十六进制）；ready 不代表文件已上传。",
	}, nil
}

// ClaimUpload atomically consumes a scoped, single-attempt upload authorization.
// Invalid credentials must not consume a valid ticket.
func (svc *SSHMCPService) ClaimUpload(id, secret string) (*model.SSHMCPTransferRequest, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	cfg, err := svc.store.GetSSHMCPConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, sshError("SSH_MCP_DISABLED", "SSH MCP 尚未启用", nil)
	}
	grant, ok := svc.uploads[id]
	actual := sha256.Sum256([]byte(secret))
	if !ok || secret == "" || subtle.ConstantTimeCompare(actual[:], grant.secretHash[:]) != 1 {
		return nil, sshError("SSH_MCP_UPLOAD_INVALID", "上传地址或临时凭据无效，或地址已使用，请重新调用 upload_file", nil)
	}
	delete(svc.uploads, id)
	if grant.configHash != cfg.TokenHash {
		return nil, sshError("SSH_MCP_UPLOAD_INVALID", "上传凭据已失效，请重新调用 upload_file", nil)
	}
	if !svc.now().Before(grant.expiresAt) {
		return nil, sshError("SSH_MCP_UPLOAD_EXPIRED", "上传地址已过期，请重新调用 upload_file", nil)
	}
	return &grant.request, nil
}
