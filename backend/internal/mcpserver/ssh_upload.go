package mcpserver

import (
	"context"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

const sshUploadFileDescription = "上传调用端机器上的本地文件到指定 SSH 主机。第一步只传 host_id、远程目标 path、可选 overwrite/sha1（40 位十六进制 SHA-1），返回唯一 upload_url、PUT 方法、临时 headers 和有效期；此时尚未上传。第二步必须在保存文件的调用端机器上使用 curl --upload-file、Bash、PowerShell -InFile 或 Python rb 文件流，将原始二进制请求体 PUT 到 upload_url，原样携带全部返回 headers。不要通过 ssh_exec 上传本地文件，因为命令在远端执行。不要 Base64、JSON 或 multipart，也不需要本地桥接程序。无需取得长期 MCP Token，使用返回的临时 Authorization。15 分钟内开始，地址仅供一次尝试，单次传输最长 1 小时，无文件总大小限制。只有 HTTP 上传响应 success=true 后才算完成，并核对 bytes 和 sha1；失败/过期请重新调用获取地址。"

func prepareSSHUpload(ctx context.Context, uploads *service.SSHMCPService, input model.SSHMCPTransferRequest) (*model.SSHMCPUploadTicket, error) {
	baseURL, _ := ctx.Value(requestBaseURLKey{}).(string)
	if baseURL == "" {
		return nil, &service.SSHServiceError{Code: "SSH_MCP_INVALID", Message: "无法确定上传地址，请通过 LAN HTTP 地址直接连接 SSH MCP"}
	}
	ticket, err := uploads.PrepareUpload(input)
	if err != nil {
		return nil, err
	}
	ticket.UploadURL = baseURL + ticket.EndpointPath
	return ticket, nil
}
