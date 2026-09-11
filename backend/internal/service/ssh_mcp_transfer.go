package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const SSHMCPTransferTimeout = time.Hour

var (
	errMCPSourceAddressDenied = errors.New("MCP source address is not public")
	mcpSourceCGNATPrefix      = netip.MustParsePrefix("100.64.0.0/10")
)

func (svc *SSHHostService) UploadMCP(ctx context.Context, request model.SSHMCPUploadRequest) (any, error) {
	if request.Content != nil && request.SourceURL != "" {
		return nil, sshError("SSH_MCP_INVALID", "content 和 source_url 不能同时提供", nil)
	}
	if request.Content != nil {
		if len(*request.Content) > base64.StdEncoding.EncodedLen(sshMCPMaxContent) {
			return nil, sshError("SSH_MCP_FILE_LIMIT", "内联文件超过 1 MiB，请使用流式 HTTP 上传或 source_url", nil)
		}
		content, err := base64.StdEncoding.DecodeString(*request.Content)
		if err != nil {
			return nil, sshError("SSH_MCP_INVALID", "content 必须为有效 Base64", nil)
		}
		if len(content) > sshMCPMaxContent {
			return nil, sshError("SSH_MCP_FILE_LIMIT", "内联文件超过 1 MiB，请使用流式 HTTP 上传或 source_url", nil)
		}
		return svc.UploadMCPStream(ctx, request.SSHMCPTransferRequest, io.NopCloser(bytes.NewReader(content)), int64(len(content)))
	}
	if request.SourceURL != "" {
		return svc.UploadMCPFromURL(ctx, request.SSHMCPTransferRequest, request.SourceURL)
	}
	return svc.PrepareMCPTransfer(request.SSHMCPTransferRequest, true)
}

func validateMCPTransfer(request model.SSHMCPTransferRequest) (string, error) {
	remote, err := normalizeSFTPPath(request.Path, false)
	if err != nil {
		return "", err
	}
	if request.SHA256 != "" {
		decoded, err := hex.DecodeString(request.SHA256)
		if err != nil || len(decoded) != sha256.Size {
			return "", sshError("SSH_MCP_INVALID", "SHA-256 必须为 64 位十六进制字符串", nil)
		}
	}
	return remote, nil
}

func (svc *SSHHostService) PrepareMCPTransfer(request model.SSHMCPTransferRequest, upload bool) (any, error) {
	remote, err := validateMCPTransfer(request)
	if err != nil {
		return nil, err
	}
	host, err := svc.GetHost(request.HostID)
	if err != nil {
		return nil, err
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	method := http.MethodGet
	headers := map[string]string{"X-SSH-Path": remote}
	if upload {
		method = http.MethodPut
		headers["Content-Type"] = "application/octet-stream"
		headers["X-SSH-Overwrite"] = strconv.FormatBool(request.Overwrite)
		if request.SHA256 != "" {
			headers["X-SSH-SHA256"] = request.SHA256
		}
	}
	return map[string]any{
		"status": "ready", "method": method,
		"endpoint_path": SSHMCPEndpoint + "/files/" + strconv.FormatInt(request.HostID, 10),
		"headers":       headers,
		"instructions":  "No file has been transferred yet. Resolve endpoint_path against the same origin as the MCP URL and send Authorization: Bearer <your MCP token>. Stream raw file bytes with HTTP PUT for upload, or save the HTTP GET response to a local file for download. No file-size cap or Base64 conversion applies to this endpoint. The MCP server cannot read files from the client's local filesystem.",
	}, nil
}

func (svc *SSHHostService) UploadMCPStream(ctx context.Context, request model.SSHMCPTransferRequest, content io.ReadCloser, expectedSize int64) (*model.SSHMCPTransferResult, error) {
	defer content.Close()
	return svc.uploadMCPStream(ctx, request, func(context.Context) (io.ReadCloser, int64, error) {
		return content, expectedSize, nil
	})
}

func (svc *SSHHostService) UploadMCPFromURL(ctx context.Context, request model.SSHMCPTransferRequest, source string) (*model.SSHMCPTransferResult, error) {
	return svc.uploadMCPFromURL(ctx, request, source, newMCPSourceHTTPClient())
}

func validMCPSourceURL(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == ""
}

func newMCPSourceHTTPClient() *http.Client {
	dialer := &net.Dialer{}
	return newMCPSourceHTTPClientWithDial(net.DefaultResolver.LookupNetIP, dialer.DialContext)
}

func newMCPSourceHTTPClientWithDial(
	lookup func(context.Context, string, string) ([]netip.Addr, error),
	dial func(context.Context, string, string) (net.Conn, error),
) *http.Client {
	transport := directHTTPTransport()
	transport.DisableKeepAlives = true
	transport.DisableCompression = true
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errors.New("invalid source address")
		}
		addresses, err := lookup(ctx, "ip", host)
		if err != nil {
			return nil, errors.New("source hostname resolution failed")
		}
		var lastErr error
		for _, candidate := range addresses {
			if isDisallowedMCPSourceAddress(candidate) {
				continue
			}
			connection, err := dial(ctx, network, net.JoinHostPort(candidate.String(), port))
			if err == nil {
				return connection, nil
			}
			lastErr = err
		}
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, errMCPSourceAddressDenied
	}
	return &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !validMCPSourceURL(req.URL.String()) {
			return errors.New("invalid source redirect")
		}
		return nil
	}}
}

func isDisallowedMCPSourceAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsValid() || !address.IsGlobalUnicast() || address.IsPrivate() || address.IsLoopback() || address.IsLinkLocalUnicast() || address.IsUnspecified() || address.IsMulticast() {
		return true
	}
	return mcpSourceCGNATPrefix.Contains(address)
}

func (svc *SSHHostService) uploadMCPFromURL(ctx context.Context, request model.SSHMCPTransferRequest, source string, client *http.Client) (*model.SSHMCPTransferResult, error) {
	if !validMCPSourceURL(source) {
		return nil, sshError("SSH_MCP_INVALID", "下载来源必须是 HTTP(S) 地址，不能包含用户信息或 fragment", nil)
	}
	return svc.uploadMCPStream(ctx, request, func(ctx context.Context) (io.ReadCloser, int64, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
		if err != nil {
			return nil, 0, sshError("SSH_MCP_SOURCE_FAILED", "无法创建文件下载请求", nil)
		}
		response, err := client.Do(req)
		if err != nil {
			if errors.Is(err, errMCPSourceAddressDenied) {
				return nil, 0, sshError("SSH_MCP_INVALID", "source_url 只能解析到公网 IP 地址", nil)
			}
			return nil, 0, sshError("SSH_MCP_SOURCE_FAILED", "无法下载源文件", nil)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return nil, 0, sshError("SSH_MCP_SOURCE_FAILED", fmt.Sprintf("文件下载返回 HTTP %d", response.StatusCode), nil)
		}
		return response.Body, response.ContentLength, nil
	})
}

func (svc *SSHHostService) uploadMCPStream(ctx context.Context, request model.SSHMCPTransferRequest, openSource func(context.Context) (io.ReadCloser, int64, error)) (*model.SSHMCPTransferResult, error) {
	remote, err := validateMCPTransfer(request)
	if err != nil {
		return nil, err
	}
	result := &model.SSHMCPTransferResult{}
	err = svc.withMCPHost(ctx, request.HostID, SSHMCPTransferTimeout, "upload", func(ctx context.Context, client *ssh.Client) error {
		files, err := sftp.NewClient(client)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法启动 SFTP", err)
		}
		defer files.Close()
		info, err := files.Lstat(remote)
		if err != nil && !os.IsNotExist(err) {
			return sshError("SSH_MCP_SFTP_FAILED", "无法检查远程文件", err)
		}
		if info != nil && (!request.Overwrite || !info.Mode().IsRegular()) {
			return sshError("SSH_MCP_FILE_EXISTS", "目标已存在；仅允许显式覆盖普通文件", nil)
		}
		extension := "hardlink@openssh.com"
		if request.Overwrite {
			extension = "posix-rename@openssh.com"
		}
		if _, supported := files.HasExtension(extension); !supported {
			return sshError("SSH_MCP_SFTP_UNSUPPORTED", "远端 SFTP 不支持安全提交文件所需的 "+extension, nil)
		}
		content, expectedSize, err := openSource(ctx)
		if err != nil {
			return err
		}
		defer content.Close()
		stopRead := context.AfterFunc(ctx, func() { _ = content.Close() })
		defer stopRead()
		suffix, err := randomHex(16)
		if err != nil {
			return err
		}
		temporary := path.Join(path.Dir(remote), ".ackwrap-mcp-"+suffix)
		file, err := files.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法创建远程临时文件", err)
		}
		committed := false
		defer func() {
			_ = file.Close()
			if !committed || !request.Overwrite {
				if err := files.Remove(temporary); err != nil {
					logging.Error("ssh_mcp.upload_cleanup", "远程临时文件清理失败: host_id=%d", request.HostID)
				}
			}
		}()
		mode := os.FileMode(0o600)
		if info != nil {
			mode = info.Mode().Perm()
		}
		if err := file.Chmod(mode); err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法设置远程文件权限", err)
		}
		hash := sha256.New()
		result.Bytes, err = io.CopyBuffer(io.MultiWriter(file, hash), struct{ io.Reader }{content}, make([]byte, 64<<10))
		closeErr := file.Close()
		if err != nil || closeErr != nil {
			return sshError("SSH_MCP_TRANSFER_FAILED", "流式上传中断，原文件未替换", errors.Join(err, closeErr))
		}
		if expectedSize >= 0 && result.Bytes != expectedSize {
			return sshError("SSH_MCP_TRANSFER_INCOMPLETE", "上传长度不符，原文件未替换", nil)
		}
		result.SHA256 = hex.EncodeToString(hash.Sum(nil))
		if request.SHA256 != "" && !strings.EqualFold(request.SHA256, result.SHA256) {
			return sshError("SSH_MCP_CHECKSUM_MISMATCH", "SHA-256 校验失败，原文件未替换", nil)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if request.Overwrite {
			err = files.PosixRename(temporary, remote)
		} else {
			err = files.Link(temporary, remote)
		}
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "提交远程文件失败，目标可能已存在", err)
		}
		committed, result.Success = true, true
		logging.Info("ssh_mcp.upload", "流式上传完成: host_id=%d bytes=%d", request.HostID, result.Bytes)
		return nil
	})
	return result, err
}

func (svc *SSHHostService) DownloadMCPStream(ctx context.Context, request model.SSHMCPFileRequest, consume func(context.Context, io.Reader, os.FileInfo) error) error {
	remote, err := normalizeSFTPPath(request.Path, false)
	if err != nil {
		return err
	}
	return svc.withMCPHost(ctx, request.HostID, SSHMCPTransferTimeout, "download", func(ctx context.Context, client *ssh.Client) error {
		files, err := sftp.NewClient(client)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法启动 SFTP", err)
		}
		defer files.Close()
		file, err := files.Open(remote)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法打开远程文件", err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Size() < 0 {
			return sshError("SSH_MCP_SFTP_FAILED", "只能下载普通文件", err)
		}
		if err := consume(ctx, io.LimitReader(file, info.Size()), info); err != nil {
			return sshError("SSH_MCP_TRANSFER_FAILED", "流式下载中断", err)
		}
		logging.Info("ssh_mcp.download", "流式下载完成: host_id=%d bytes=%d", request.HostID, info.Size())
		return nil
	})
}
