package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/pkg/sftp"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshSFTPMaximumTextSize  = 2 << 20
	sshSFTPMaximumCopyFiles = 10000
	sshSFTPMaximumCopyDepth = 64
	sshSFTPMaximumCopyBytes = 2 << 30
)

type sshSFTPCopyState struct {
	entries int
	bytes   int64
}

type sshSFTPTextLock struct {
	mu   sync.Mutex
	refs int
}

func (svc *SSHHostService) CopySFTP(ctx context.Context, sessionID, token, sourcePath, targetPath string) error {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return err
	}
	defer managed.sftpOps.Done()
	sourcePath, err = normalizeSFTPPath(sourcePath, false)
	if err != nil {
		return err
	}
	targetPath, err = normalizeSFTPPath(targetPath, false)
	if err != nil {
		return err
	}
	sourceInfo, err := client.Lstat(sourcePath)
	if err != nil {
		return svc.sftpOperationError(managed, "copy", "读取 SFTP 复制源失败", err)
	}
	if err := validateSFTPCopyInfo(sourceInfo); err != nil {
		return err
	}
	sourcePath, err = client.RealPath(sourcePath)
	if err != nil {
		return svc.sftpOperationError(managed, "copy", "读取 SFTP 复制源失败", err)
	}
	targetParent, err := client.RealPath(path.Dir(targetPath))
	if err != nil {
		return svc.sftpOperationError(managed, "copy", "SFTP 目标目录不存在", err)
	}
	targetPath = path.Join(targetParent, path.Base(targetPath))
	if sourcePath == targetPath {
		return sshError("SSH_SFTP_INVALID_PATH", "SFTP 复制源和目标不能相同", nil)
	}
	info := sourceInfo
	if info.IsDir() && strings.HasPrefix(targetPath, sourcePath+"/") {
		return sshError("SSH_SFTP_INVALID_PATH", "不能将目录复制到其自身内部", nil)
	}
	if _, err := client.Lstat(targetPath); err == nil {
		return sshError("SSH_SFTP_EXISTS", "复制目标已存在", nil)
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return svc.sftpOperationError(managed, "copy", "读取 SFTP 复制目标失败", err)
	}
	tempID, err := randomHex(12)
	if err != nil {
		return sshError("SSH_SFTP_FAILED", "无法创建 SFTP 复制临时路径", err)
	}
	tempPath := path.Join(targetParent, ".ackwrap-copy-"+tempID)
	tempExists := false
	defer func() {
		if !tempExists {
			return
		}
		var cleanupErr error
		if info.IsDir() {
			cleanupErr = client.RemoveAll(tempPath)
		} else {
			cleanupErr = client.Remove(tempPath)
		}
		if cleanupErr != nil {
			logging.Error("ssh_sftp.copy", "清理 SFTP 复制临时资源失败: host_id=%d", managed.host.ID)
		}
	}()
	state := &sshSFTPCopyState{}
	if err := svc.copySFTPEntry(ctx, managed, client, sourcePath, tempPath, info, 0, state); err != nil {
		tempExists = true
		return err
	}
	tempExists = true
	if _, err := client.Lstat(targetPath); err == nil {
		return sshError("SSH_SFTP_EXISTS", "复制目标已存在", nil)
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return svc.sftpOperationError(managed, "copy", "提交 SFTP 复制前检查失败", err)
	}
	if err := client.Rename(tempPath, targetPath); err != nil {
		return svc.sftpOperationError(managed, "copy", "提交 SFTP 复制失败", err)
	}
	tempExists = false
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.copy", "复制 SFTP 资源: host_id=%d entries=%d bytes=%d", managed.host.ID, state.entries, state.bytes)
	return nil
}

func (svc *SSHHostService) copySFTPEntry(
	ctx context.Context,
	managed *managedSSHSession,
	client *sftp.Client,
	sourcePath string,
	targetPath string,
	info os.FileInfo,
	depth int,
	state *sshSFTPCopyState,
) error {
	if err := ctx.Err(); err != nil {
		return sshError("SSH_SFTP_FAILED", "SFTP 复制已取消", err)
	}
	if depth > sshSFTPMaximumCopyDepth || state.entries >= sshSFTPMaximumCopyFiles {
		return sshError("SSH_SFTP_FAILED", "SFTP 复制内容过多或目录层级过深", nil)
	}
	if err := validateSFTPCopyInfo(info); err != nil {
		return err
	}
	state.entries++
	if info.IsDir() {
		if err := client.Mkdir(targetPath); err != nil {
			return svc.sftpOperationError(managed, "copy", "创建 SFTP 目标目录失败", err)
		}
		entries, err := client.ReadDir(sourcePath)
		if err != nil {
			return svc.sftpOperationError(managed, "copy", "读取 SFTP 源目录失败", err)
		}
		for _, entry := range entries {
			if err := svc.copySFTPEntry(
				ctx, managed, client,
				path.Join(sourcePath, entry.Name()), path.Join(targetPath, entry.Name()), entry, depth+1, state,
			); err != nil {
				return err
			}
		}
		if err := client.Chmod(targetPath, info.Mode().Perm()); err != nil {
			return svc.sftpOperationError(managed, "copy", "设置 SFTP 目标目录权限失败", err)
		}
		return nil
	}
	if info.Size() < 0 || info.Size() > sshSFTPMaximumCopyBytes-state.bytes {
		return sshError("SSH_SFTP_FAILED", "SFTP 复制内容超过 2 GiB 限制", nil)
	}
	source, err := client.Open(sourcePath)
	if err != nil {
		return svc.sftpOperationError(managed, "copy", "打开 SFTP 源文件失败", err)
	}
	sourceClosed := false
	defer func() {
		if !sourceClosed {
			_ = source.Close()
		}
	}()
	target, err := client.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return svc.sftpOperationError(managed, "copy", "创建 SFTP 目标文件失败", err)
	}
	buffer := make([]byte, 64<<10)
	for {
		if err := ctx.Err(); err != nil {
			_ = target.Close()
			return sshError("SSH_SFTP_FAILED", "SFTP 复制已取消", err)
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			if int64(count) > sshSFTPMaximumCopyBytes-state.bytes {
				_ = target.Close()
				return sshError("SSH_SFTP_FAILED", "SFTP 复制内容超过 2 GiB 限制", nil)
			}
			written, writeErr := target.Write(buffer[:count])
			if writeErr != nil || written != count {
				_ = target.Close()
				if writeErr == nil {
					writeErr = io.ErrShortWrite
				}
				return svc.sftpOperationError(managed, "copy", "写入 SFTP 目标文件失败", writeErr)
			}
			state.bytes += int64(written)
			managed.lastActive.Store(svc.now().UnixMilli())
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = target.Close()
				return svc.sftpOperationError(managed, "copy", "读取 SFTP 源文件失败", readErr)
			}
			break
		}
	}
	sourceClosed = true
	if err := source.Close(); err != nil {
		_ = target.Close()
		return svc.sftpOperationError(managed, "copy", "关闭 SFTP 源文件失败", err)
	}
	if err := target.Close(); err != nil {
		return svc.sftpOperationError(managed, "copy", "保存 SFTP 目标文件失败", err)
	}
	if err := client.Chmod(targetPath, info.Mode().Perm()); err != nil {
		return svc.sftpOperationError(managed, "copy", "设置 SFTP 目标文件权限失败", err)
	}
	return nil
}

func validateSFTPCopyInfo(info os.FileInfo) error {
	if info.Mode()&os.ModeSymlink != 0 {
		return sshError("SSH_SFTP_UNSUPPORTED", "暂不支持复制符号链接或包含符号链接的目录", nil)
	}
	return nil
}

func (svc *SSHHostService) ReadSFTPText(sessionID, token, requestedPath string) (*model.SSHSFTPTextFile, error) {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return nil, err
	}
	defer managed.sftpOps.Done()
	remotePath, err := normalizeSFTPPath(requestedPath, false)
	if err != nil {
		return nil, err
	}
	result, _, err := svc.readSFTPTextFile(managed, client, remotePath)
	if err != nil {
		return nil, err
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.text.read", "读取 SFTP 文本文件: host_id=%d bytes=%d", managed.host.ID, result.Size)
	return result, nil
}

func (svc *SSHHostService) WriteSFTPText(sessionID, token string, request model.SSHSFTPTextWriteRequest) (*model.SSHSFTPTextFile, error) {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return nil, err
	}
	defer managed.sftpOps.Done()
	remotePath, err := normalizeSFTPPath(request.Path, false)
	if err != nil {
		return nil, err
	}
	releaseTextLock := svc.lockSFTPTextFile(managed.host.ID, remotePath)
	defer releaseTextLock()
	managed.mu.Lock()
	terminated := managed.terminated
	managed.mu.Unlock()
	if terminated {
		return nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话已关闭", nil)
	}
	content := []byte(request.Content)
	if len(content) > sshSFTPMaximumTextSize {
		return nil, sshError("SSH_SFTP_TEXT_TOO_LARGE", "在线编辑仅支持不超过 2 MiB 的文件", nil)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, sshError("SSH_SFTP_BINARY", "在线编辑仅支持 UTF-8 文本文件", nil)
	}
	expectedHash, err := hex.DecodeString(strings.TrimSpace(request.ExpectedSHA256))
	if err != nil || len(expectedHash) != sha256.Size {
		return nil, sshError("SSH_SFTP_INVALID_PATH", "文本文件版本标识无效", err)
	}
	current, mode, err := svc.readSFTPTextFile(managed, client, remotePath)
	if err != nil {
		return nil, err
	}
	currentHash, _ := hex.DecodeString(current.SHA256)
	if subtle.ConstantTimeCompare(expectedHash, currentHash) != 1 {
		return nil, sshError("SSH_SFTP_CONFLICT", "远端文件已被修改，请重新加载后再保存", nil)
	}
	if _, supported := client.HasExtension("posix-rename@openssh.com"); !supported {
		return nil, sshError("SSH_SFTP_UNSUPPORTED", "远端 SFTP 服务不支持安全文本保存", nil)
	}
	tempID, err := randomHex(12)
	if err != nil {
		return nil, sshError("SSH_SFTP_FAILED", "无法创建 SFTP 编辑临时文件", err)
	}
	tempPath := path.Join(path.Dir(remotePath), ".ackwrap-edit-"+tempID)
	file, err := client.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return nil, svc.sftpOperationError(managed, "text.write", "创建 SFTP 编辑临时文件失败", err)
	}
	tempExists := true
	defer func() {
		if tempExists {
			if cleanupErr := client.Remove(tempPath); cleanupErr != nil {
				logging.Error("ssh_sftp.text.write", "清理 SFTP 编辑临时文件失败: host_id=%d", managed.host.ID)
			}
		}
	}()
	if _, err := io.Copy(file, strings.NewReader(request.Content)); err != nil {
		_ = file.Close()
		return nil, svc.sftpOperationError(managed, "text.write", "写入 SFTP 文本文件失败", err)
	}
	if err := file.Close(); err != nil {
		return nil, svc.sftpOperationError(managed, "text.write", "保存 SFTP 文本临时文件失败", err)
	}
	if err := client.Chmod(tempPath, mode.Perm()); err != nil {
		return nil, svc.sftpOperationError(managed, "text.write", "设置 SFTP 文本文件权限失败", err)
	}
	latest, _, err := svc.readSFTPTextFile(managed, client, remotePath)
	if err != nil {
		return nil, err
	}
	latestHash, _ := hex.DecodeString(latest.SHA256)
	if subtle.ConstantTimeCompare(expectedHash, latestHash) != 1 {
		return nil, sshError("SSH_SFTP_CONFLICT", "远端文件已被修改，请重新加载后再保存", nil)
	}
	if err := client.PosixRename(tempPath, remotePath); err != nil {
		return nil, svc.sftpOperationError(managed, "text.write", "提交 SFTP 文本文件失败", err)
	}
	tempExists = false
	info, err := client.Stat(remotePath)
	if err != nil {
		return nil, svc.sftpOperationError(managed, "text.write", "读取已保存 SFTP 文件失败", err)
	}
	hash := sha256.Sum256(content)
	result := &model.SSHSFTPTextFile{
		Path: remotePath, Name: path.Base(remotePath), Content: request.Content,
		SHA256: hex.EncodeToString(hash[:]), Size: int64(len(content)), ModifiedAt: info.ModTime().UnixMilli(),
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.text.write", "保存 SFTP 文本文件: host_id=%d bytes=%d", managed.host.ID, len(content))
	return result, nil
}

func (svc *SSHHostService) lockSFTPTextFile(hostID int64, remotePath string) func() {
	key := fmt.Sprintf("%d\x00%s", hostID, remotePath)
	svc.sftpTextLockMu.Lock()
	if svc.sftpTextLocks == nil {
		svc.sftpTextLocks = make(map[string]*sshSFTPTextLock)
	}
	entry := svc.sftpTextLocks[key]
	if entry == nil {
		entry = &sshSFTPTextLock{}
		svc.sftpTextLocks[key] = entry
	}
	entry.refs++
	svc.sftpTextLockMu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		svc.sftpTextLockMu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(svc.sftpTextLocks, key)
		}
		svc.sftpTextLockMu.Unlock()
	}
}

func (svc *SSHHostService) readSFTPTextFile(
	managed *managedSSHSession,
	client *sftp.Client,
	remotePath string,
) (*model.SSHSFTPTextFile, os.FileMode, error) {
	file, err := client.Open(remotePath)
	if err != nil {
		return nil, 0, svc.sftpOperationError(managed, "text.read", "打开 SFTP 文本文件失败", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, 0, svc.sftpOperationError(managed, "text.read", "读取 SFTP 文本文件信息失败", err)
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, 0, sshError("SSH_SFTP_INVALID_PATH", "目录不能在线编辑", nil)
	}
	if info.Size() < 0 || info.Size() > sshSFTPMaximumTextSize {
		_ = file.Close()
		return nil, 0, sshError("SSH_SFTP_TEXT_TOO_LARGE", "在线编辑仅支持不超过 2 MiB 的文件", nil)
	}
	content, readErr := io.ReadAll(io.LimitReader(file, sshSFTPMaximumTextSize+1))
	closeErr := file.Close()
	if readErr != nil {
		return nil, 0, svc.sftpOperationError(managed, "text.read", "读取 SFTP 文本文件失败", readErr)
	}
	if closeErr != nil {
		return nil, 0, svc.sftpOperationError(managed, "text.read", "关闭 SFTP 文本文件失败", closeErr)
	}
	if len(content) > sshSFTPMaximumTextSize {
		return nil, 0, sshError("SSH_SFTP_TEXT_TOO_LARGE", "在线编辑仅支持不超过 2 MiB 的文件", nil)
	}
	if !utf8.Valid(content) || bytes.IndexByte(content, 0) >= 0 {
		return nil, 0, sshError("SSH_SFTP_BINARY", "在线编辑仅支持 UTF-8 文本文件", nil)
	}
	hash := sha256.Sum256(content)
	return &model.SSHSFTPTextFile{
		Path: remotePath, Name: path.Base(remotePath), Content: string(content),
		SHA256: hex.EncodeToString(hash[:]), Size: int64(len(content)), ModifiedAt: info.ModTime().UnixMilli(),
	}, info.Mode(), nil
}
