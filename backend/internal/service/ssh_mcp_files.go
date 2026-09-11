package service

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
)

func (svc *SSHHostService) ReadMCPFile(ctx context.Context, request model.SSHMCPFileRequest, action string) (any, error) {
	remote, err := normalizeSFTPPath(request.Path, action == "list_dir" || action == "stat")
	if err != nil {
		return nil, err
	}
	var result any
	err = svc.withMCPHost(ctx, request.HostID, 60*time.Second, action, func(ctx context.Context, client *ssh.Client) error {
		files, err := sftp.NewClient(client)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法启动 SFTP", err)
		}
		defer files.Close()
		switch action {
		case "list_dir":
			entries, err := files.ReadDirContext(ctx, remote)
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法列出远程目录", err)
			}
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].IsDir() != entries[j].IsDir() {
					return entries[i].IsDir()
				}
				return entries[i].Name() < entries[j].Name()
			})
			truncated := len(entries) > 1000
			if truncated {
				entries = entries[:1000]
			}
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				items = append(items, mcpFileInfo(entry))
			}
			result = map[string]any{"entries": items, "truncated": truncated}
		case "stat":
			info, err := files.Lstat(remote)
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法读取远程文件信息", err)
			}
			result = mcpFileInfo(info)
		case "read_file", "download":
			file, err := files.Open(remote)
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法打开远程文件", err)
			}
			defer file.Close()
			content, err := io.ReadAll(io.LimitReader(file, sshMCPMaxContent+1))
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "读取远程文件失败", err)
			}
			if len(content) > sshMCPMaxContent {
				return sshError("SSH_MCP_FILE_LIMIT", "MCP 单次文件传输最大为 1 MiB", nil)
			}
			encoding, text := "utf-8", string(content)
			if action == "download" {
				encoding, text = "base64", base64.StdEncoding.EncodeToString(content)
			} else if !utf8.Valid(content) {
				return sshError("SSH_MCP_FILE_ENCODING", "文件不是 UTF-8 文本，请使用 ssh_download", nil)
			}
			result = map[string]any{"content": text, "encoding": encoding, "size": len(content)}
		default:
			return sshError("SSH_MCP_INVALID", "不支持的文件操作", nil)
		}
		return nil
	})
	return result, err
}

func mcpFileInfo(info os.FileInfo) map[string]any {
	return map[string]any{"name": info.Name(), "size": info.Size(), "is_dir": info.IsDir(), "mode": info.Mode().String(), "modified_at": info.ModTime().Unix()}
}

func (svc *SSHHostService) WriteMCPFile(ctx context.Context, request model.SSHMCPWriteRequest, binary bool) error {
	remote, err := normalizeSFTPPath(request.Path, false)
	if err != nil {
		return err
	}
	if len(request.Content) > base64.StdEncoding.EncodedLen(sshMCPMaxContent) {
		return sshError("SSH_MCP_FILE_LIMIT", "MCP 单次文件传输最大为 1 MiB", nil)
	}
	content := []byte(request.Content)
	if binary {
		content, err = base64.StdEncoding.DecodeString(request.Content)
		if err != nil {
			return sshError("SSH_MCP_INVALID", "上传内容必须是标准 Base64 编码", nil)
		}
	}
	if len(content) > sshMCPMaxContent {
		return sshError("SSH_MCP_FILE_LIMIT", "MCP 单次文件传输最大为 1 MiB", nil)
	}
	return svc.withMCPHost(ctx, request.HostID, 60*time.Second, "write_file", func(_ context.Context, client *ssh.Client) error {
		files, err := sftp.NewClient(client)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法启动 SFTP", err)
		}
		defer files.Close()
		info, err := files.Lstat(remote)
		if err != nil && !os.IsNotExist(err) {
			return sshError("SSH_MCP_SFTP_FAILED", "无法检查远程文件", err)
		}
		if err == nil && (!request.Overwrite || !info.Mode().IsRegular()) {
			return sshError("SSH_MCP_FILE_EXISTS", "目标已存在；仅允许显式覆盖普通文件", nil)
		}
		suffix, err := randomHex(16)
		if err != nil {
			return err
		}
		temporary := path.Join(path.Dir(remote), ".ackwrap-mcp-"+suffix)
		file, err := files.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法创建远程临时文件", err)
		}
		defer files.Remove(temporary)
		mode := os.FileMode(0o600)
		if info != nil {
			mode = info.Mode().Perm()
		}
		if err := file.Chmod(mode); err != nil {
			_ = file.Close()
			return sshError("SSH_MCP_SFTP_FAILED", "无法设置远程文件权限", err)
		}
		_, writeErr := io.Copy(file, strings.NewReader(string(content)))
		closeErr := file.Close()
		if err := errors.Join(writeErr, closeErr); err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "写入远程临时文件失败，原文件未替换", err)
		}
		if request.Overwrite {
			err = files.PosixRename(temporary, remote)
		} else {
			err = files.Rename(temporary, remote)
		}
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "提交远程文件失败，覆盖操作需要服务器支持原子替换", err)
		}
		return nil
	})
}
