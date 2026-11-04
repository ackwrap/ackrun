package service

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

type mcpLocalReader struct {
	ctx context.Context
	io.Reader
}

func (reader mcpLocalReader) Read(data []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.Reader.Read(data)
}

func openMCPLocalFile(name string) (*os.File, os.FileInfo, error) {
	info, err := os.Stat(name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, nil, sshError("SSH_MCP_SFTP_FAILED", "只能读取普通文件", err)
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, sshError("SSH_MCP_SFTP_FAILED", "无法打开本机文件", err)
	}
	info, err = file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, sshError("SSH_MCP_SFTP_FAILED", "只能读取普通文件", err)
	}
	return file, info, nil
}

func (svc *SSHHostService) readMCPLocalFile(ctx context.Context, request model.SSHMCPFileRequest, name, action string) (any, error) {
	var result any
	err := svc.withMCPOperation(ctx, request.HostID, time.Minute, action, func(ctx context.Context, _ *model.SSHHost) error {
		switch action {
		case "list_dir":
			entries, err := os.ReadDir(name)
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法列出本机目录", err)
			}
			sort.SliceStable(entries, func(i, j int) bool { return entries[i].IsDir() && !entries[j].IsDir() })
			truncated := len(entries) > 1000
			if truncated {
				entries = entries[:1000]
			}
			items := make([]map[string]any, 0, len(entries))
			for _, entry := range entries {
				if err := ctx.Err(); err != nil {
					return err
				}
				info, err := entry.Info()
				if err != nil {
					return sshError("SSH_MCP_SFTP_FAILED", "无法读取本机目录项", err)
				}
				items = append(items, mcpFileInfo(info))
			}
			result = map[string]any{"entries": items, "truncated": truncated}
		case "stat":
			info, err := os.Lstat(name)
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法读取本机文件信息", err)
			}
			result = mcpFileInfo(info)
		case "read_file", "download":
			file, _, err := openMCPLocalFile(name)
			if err != nil {
				return err
			}
			defer file.Close()
			content, err := io.ReadAll(io.LimitReader(mcpLocalReader{ctx, file}, sshMCPMaxContent+1))
			if err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "读取本机文件失败", err)
			}
			if len(content) > sshMCPMaxContent {
				return sshError("SSH_MCP_FILE_LIMIT", "内联文件最大为 1 MiB，请使用流式下载", nil)
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

func (svc *SSHHostService) uploadMCPLocal(ctx context.Context, request model.SSHMCPTransferRequest, name string, openSource func(context.Context) (io.ReadCloser, int64, error)) (*model.SSHMCPTransferResult, error) {
	result := &model.SSHMCPTransferResult{}
	err := svc.withMCPOperation(ctx, request.HostID, SSHMCPTransferTimeout, "upload", func(ctx context.Context, _ *model.SSHHost) error {
		info, err := os.Lstat(name)
		if err != nil && !os.IsNotExist(err) {
			return sshError("SSH_MCP_SFTP_FAILED", "无法检查本机目标文件", err)
		}
		if info != nil && (!request.Overwrite || !info.Mode().IsRegular()) {
			return sshError("SSH_MCP_FILE_EXISTS", "目标已存在；仅允许显式覆盖普通文件", nil)
		}
		content, size, err := openSource(ctx)
		if err != nil {
			return err
		}
		defer content.Close()
		stop := context.AfterFunc(ctx, func() { _ = content.Close() })
		defer stop()
		file, err := os.CreateTemp(filepath.Dir(name), ".ackwrap-mcp-*")
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "无法创建本机临时文件", err)
		}
		temporary := file.Name()
		defer func() {
			_ = file.Close()
			if err := os.Remove(temporary); err != nil && !os.IsNotExist(err) {
				logging.Error("ssh_mcp.upload_cleanup", "本机临时文件清理失败")
			}
		}()
		if info != nil {
			if err := file.Chmod(info.Mode().Perm()); err != nil {
				return sshError("SSH_MCP_SFTP_FAILED", "无法设置本机文件权限", err)
			}
		}
		hash := sha1.New()
		result.Bytes, err = io.CopyBuffer(file, io.TeeReader(mcpLocalReader{ctx, content}, hash), make([]byte, 64<<10))
		if err := errors.Join(err, file.Close()); err != nil {
			return sshError("SSH_MCP_TRANSFER_FAILED", "流式上传中断，原文件未替换", err)
		}
		if size >= 0 && result.Bytes != size {
			return sshError("SSH_MCP_TRANSFER_INCOMPLETE", "上传长度不符，原文件未替换", nil)
		}
		result.SHA1 = hex.EncodeToString(hash.Sum(nil))
		if request.SHA1 != "" && !strings.EqualFold(request.SHA1, result.SHA1) {
			return sshError("SSH_MCP_CHECKSUM_MISMATCH", "SHA-1 校验失败，原文件未替换", nil)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if request.Overwrite {
			current, checkErr := os.Lstat(name)
			if checkErr != nil && !os.IsNotExist(checkErr) {
				return sshError("SSH_MCP_SFTP_FAILED", "无法检查本机目标文件", checkErr)
			}
			if current != nil && !current.Mode().IsRegular() {
				return sshError("SSH_MCP_FILE_EXISTS", "只允许覆盖普通文件", nil)
			}
			err = os.Rename(temporary, name)
		} else {
			err = os.Link(temporary, name)
		}
		if err != nil {
			return sshError("SSH_MCP_SFTP_FAILED", "提交本机文件失败，原文件未替换", err)
		}
		result.Success = true
		logging.Info("ssh_mcp.upload", "本机流式上传完成: bytes=%d", result.Bytes)
		return nil
	})
	return result, err
}

func (svc *SSHHostService) downloadMCPLocal(ctx context.Context, request model.SSHMCPFileRequest, name string, consume func(context.Context, io.Reader, os.FileInfo) error) error {
	return svc.withMCPOperation(ctx, request.HostID, SSHMCPTransferTimeout, "download", func(ctx context.Context, _ *model.SSHHost) error {
		file, info, err := openMCPLocalFile(name)
		if err != nil {
			return err
		}
		defer file.Close()
		if err := consume(ctx, io.LimitReader(mcpLocalReader{ctx, file}, info.Size()), info); err != nil {
			return sshError("SSH_MCP_TRANSFER_FAILED", "本机流式下载中断", err)
		}
		logging.Info("ssh_mcp.download", "本机流式下载完成: bytes=%d", info.Size())
		return nil
	})
}
