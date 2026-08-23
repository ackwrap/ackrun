package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"io"
	"os"
	"path"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/sftp"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const sshMaximumSFTPPathSize = 4096

type SSHSFTPDownload struct {
	Reader     io.ReadCloser
	Name       string
	Size       int64
	ModifiedAt time.Time
}

func (svc *SSHHostService) ListSFTP(ctx context.Context, sessionID, token, requestedPath string) (*model.SSHSFTPListResponse, error) {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return nil, err
	}
	defer managed.sftpOps.Done()
	remotePath, err := normalizeSFTPPath(requestedPath, true)
	if err != nil {
		return nil, err
	}
	realPath, err := client.RealPath(remotePath)
	if err != nil {
		return nil, svc.sftpOperationError(managed, "list", "读取 SFTP 目录失败", err)
	}
	entries, err := client.ReadDirContext(ctx, realPath)
	if err != nil {
		return nil, svc.sftpOperationError(managed, "list", "读取 SFTP 目录失败", err)
	}
	home, err := client.RealPath(".")
	if err != nil {
		return nil, svc.sftpOperationError(managed, "list", "读取 SFTP 主目录失败", err)
	}
	sort.Slice(entries, func(left, right int) bool {
		if entries[left].IsDir() != entries[right].IsDir() {
			return entries[left].IsDir()
		}
		return strings.ToLower(entries[left].Name()) < strings.ToLower(entries[right].Name())
	})
	items := make([]model.SSHSFTPEntry, 0, len(entries))
	for _, entry := range entries {
		items = append(items, model.SSHSFTPEntry{
			Name:       entry.Name(),
			Path:       path.Join(realPath, entry.Name()),
			IsDir:      entry.IsDir(),
			IsSymlink:  entry.Mode()&os.ModeSymlink != 0,
			Size:       entry.Size(),
			Mode:       entry.Mode().String(),
			ModifiedAt: entry.ModTime().UnixMilli(),
		})
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.list", "读取 SFTP 目录: host_id=%d entries=%d", managed.host.ID, len(items))
	return &model.SSHSFTPListResponse{
		Path: realPath, Parent: path.Dir(realPath), Home: home, Entries: items,
	}, nil
}

func (svc *SSHHostService) CreateSFTPDirectory(sessionID, token, requestedPath string) error {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return err
	}
	defer managed.sftpOps.Done()
	remotePath, err := normalizeSFTPPath(requestedPath, false)
	if err != nil {
		return err
	}
	if err := client.Mkdir(remotePath); err != nil {
		return svc.sftpOperationError(managed, "mkdir", "创建 SFTP 目录失败", err)
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.mkdir", "创建 SFTP 目录: host_id=%d", managed.host.ID)
	return nil
}

func (svc *SSHHostService) RenameSFTP(sessionID, token, oldPath, newPath string) error {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return err
	}
	defer managed.sftpOps.Done()
	oldPath, err = normalizeSFTPPath(oldPath, false)
	if err != nil {
		return err
	}
	newPath, err = normalizeSFTPPath(newPath, false)
	if err != nil {
		return err
	}
	if oldPath == newPath {
		return nil
	}
	if _, err := client.Lstat(newPath); err == nil {
		return sshError("SSH_SFTP_EXISTS", "远端已存在同名文件", nil)
	} else if !errors.Is(err, os.ErrNotExist) && !errors.Is(err, sftp.ErrSSHFxNoSuchFile) {
		return svc.sftpOperationError(managed, "rename", "读取重命名目标失败", err)
	}
	if err := client.Rename(oldPath, newPath); err != nil {
		return svc.sftpOperationError(managed, "rename", "重命名 SFTP 文件失败", err)
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.rename", "重命名 SFTP 文件: host_id=%d", managed.host.ID)
	return nil
}

func (svc *SSHHostService) DeleteSFTP(sessionID, token, requestedPath string, recursive bool) error {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return err
	}
	defer managed.sftpOps.Done()
	remotePath, err := normalizeSFTPPath(requestedPath, false)
	if err != nil {
		return err
	}
	info, err := client.Lstat(remotePath)
	if err != nil {
		return svc.sftpOperationError(managed, "delete", "读取待删除 SFTP 文件失败", err)
	}
	if info.IsDir() {
		if recursive {
			err = client.RemoveAll(remotePath)
		} else {
			err = client.RemoveDirectory(remotePath)
		}
	} else {
		err = client.Remove(remotePath)
	}
	if err != nil {
		return svc.sftpOperationError(managed, "delete", "删除 SFTP 文件失败", err)
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.delete", "删除 SFTP 文件: host_id=%d recursive=%t", managed.host.ID, recursive)
	return nil
}

func (svc *SSHHostService) UploadSFTP(sessionID, token, requestedPath string, overwrite bool, content io.ReadCloser) (written int64, resultErr error) {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		_ = content.Close()
		return 0, err
	}
	uploadID, registered := managed.registerSFTPUpload(content)
	if !registered {
		_ = content.Close()
		managed.sftpOps.Done()
		return 0, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话已关闭", nil)
	}
	defer func() {
		_ = content.Close()
		managed.unregisterSFTPUpload(uploadID)
		managed.sftpOps.Done()
	}()
	remotePath, err := normalizeSFTPPath(requestedPath, false)
	if err != nil {
		return 0, err
	}
	_, statErr := client.Lstat(remotePath)
	targetExists := statErr == nil
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) && !errors.Is(statErr, sftp.ErrSSHFxNoSuchFile) {
		return 0, svc.sftpOperationError(managed, "upload", "读取远端 SFTP 文件信息失败", statErr)
	}
	if targetExists && !overwrite {
		return 0, sshError("SSH_SFTP_EXISTS", "远端已存在同名文件，请确认覆盖", nil)
	}
	if targetExists && overwrite {
		if _, supported := client.HasExtension("posix-rename@openssh.com"); !supported {
			return 0, sshError("SSH_SFTP_UNSUPPORTED", "远端 SFTP 服务不支持安全覆盖，请先重命名或删除原文件", nil)
		}
	}
	tempID, err := randomHex(12)
	if err != nil {
		return 0, sshError("SSH_SFTP_FAILED", "无法创建 SFTP 上传临时文件", err)
	}
	tempPath := path.Join(path.Dir(remotePath), ".ackwrap-upload-"+tempID)
	file, err := client.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return 0, svc.sftpOperationError(managed, "upload", "创建远端 SFTP 文件失败", err)
	}
	tempExists := true
	defer func() {
		if tempExists {
			if cleanupErr := client.Remove(tempPath); cleanupErr != nil {
				logging.Error("ssh_sftp.upload", "清理 SFTP 上传临时文件失败: host_id=%d", managed.host.ID)
				cleanupErr = sshError("SSH_SFTP_FAILED", "清理 SFTP 上传临时文件失败", cleanupErr)
				if resultErr != nil {
					resultErr = errors.Join(resultErr, cleanupErr)
				} else {
					resultErr = cleanupErr
				}
			}
		}
	}()
	written, copyErr := io.Copy(file, &sshSFTPActivityReader{reader: content, active: &managed.lastActive, now: svc.now})
	closeErr := file.Close()
	if copyErr != nil {
		return written, svc.sftpOperationError(managed, "upload", "上传 SFTP 文件失败", copyErr)
	}
	if closeErr != nil {
		return written, svc.sftpOperationError(managed, "upload", "保存远端 SFTP 文件失败", closeErr)
	}
	if targetExists {
		err = client.PosixRename(tempPath, remotePath)
	} else {
		err = client.Rename(tempPath, remotePath)
	}
	if err != nil {
		return written, svc.sftpOperationError(managed, "upload", "提交远端 SFTP 文件失败", err)
	}
	tempExists = false
	logging.Info("ssh_sftp.upload", "上传 SFTP 文件: host_id=%d bytes=%d", managed.host.ID, written)
	return written, nil
}

func (managed *managedSSHSession) registerSFTPUpload(content io.ReadCloser) (uint64, bool) {
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if managed.terminated {
		return 0, false
	}
	managed.nextUpload++
	if managed.uploads == nil {
		managed.uploads = make(map[uint64]io.ReadCloser)
	}
	managed.uploads[managed.nextUpload] = content
	return managed.nextUpload, true
}

func (managed *managedSSHSession) unregisterSFTPUpload(id uint64) {
	managed.mu.Lock()
	delete(managed.uploads, id)
	managed.mu.Unlock()
}

func (svc *SSHHostService) OpenSFTPDownload(sessionID, token, requestedPath string) (*SSHSFTPDownload, error) {
	managed, client, err := svc.sftpClient(sessionID, token)
	if err != nil {
		return nil, err
	}
	releaseOperation := true
	defer func() {
		if releaseOperation {
			managed.sftpOps.Done()
		}
	}()
	remotePath, err := normalizeSFTPPath(requestedPath, false)
	if err != nil {
		return nil, err
	}
	file, err := client.Open(remotePath)
	if err != nil {
		return nil, svc.sftpOperationError(managed, "download", "打开 SFTP 文件失败", err)
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, svc.sftpOperationError(managed, "download", "读取 SFTP 文件信息失败", err)
	}
	if info.IsDir() {
		_ = file.Close()
		return nil, sshError("SSH_SFTP_INVALID_PATH", "目录不能直接下载", nil)
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	logging.Info("ssh_sftp.download", "下载 SFTP 文件: host_id=%d bytes=%d", managed.host.ID, info.Size())
	releaseOperation = false
	return &SSHSFTPDownload{
		Reader: &sshSFTPActivityReadCloser{
			ReadCloser: file, active: &managed.lastActive, now: svc.now, done: managed.sftpOps.Done,
		},
		Name: info.Name(), Size: info.Size(), ModifiedAt: info.ModTime(),
	}, nil
}

func (svc *SSHHostService) sftpClient(sessionID, token string) (*managedSSHSession, *sftp.Client, error) {
	if sessionID == "" || token == "" {
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	svc.sessionMu.Lock()
	managed := svc.sessions[sessionID]
	svc.sessionMu.Unlock()
	if managed == nil {
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	tokenHash := sha256.Sum256([]byte(token))
	managed.mu.Lock()
	if managed.terminated || subtle.ConstantTimeCompare(managed.sftpToken[:], tokenHash[:]) != 1 {
		managed.mu.Unlock()
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在或授权无效", nil)
	}
	if managed.sftp != nil {
		client := managed.sftp
		managed.sftpOps.Add(1)
		managed.mu.Unlock()
		return managed, client, nil
	}
	sshClient := managed.client
	managed.mu.Unlock()

	client, err := sftp.NewClient(sshClient)
	if err != nil {
		return nil, nil, svc.sftpOperationError(managed, "connect", "远端主机不支持 SFTP", err)
	}
	managed.mu.Lock()
	if managed.terminated {
		managed.mu.Unlock()
		_ = client.Close()
		return nil, nil, sshError("SSH_SESSION_NOT_FOUND", "SSH 会话已关闭", nil)
	}
	if managed.sftp == nil {
		managed.sftp = client
	} else {
		extraClient := client
		client = managed.sftp
		managed.sftpOps.Add(1)
		managed.mu.Unlock()
		_ = extraClient.Close()
		managed.lastActive.Store(svc.now().UnixMilli())
		return managed, client, nil
	}
	managed.sftpOps.Add(1)
	managed.mu.Unlock()
	managed.lastActive.Store(svc.now().UnixMilli())
	return managed, client, nil
}

func (svc *SSHHostService) sftpOperationError(managed *managedSSHSession, operation, message string, err error) error {
	code := "SSH_SFTP_FAILED"
	if operation == "connect" {
		code = "SSH_SFTP_UNAVAILABLE"
	}
	switch {
	case errors.Is(err, os.ErrNotExist), errors.Is(err, sftp.ErrSSHFxNoSuchFile):
		code, message = "SSH_SFTP_NOT_FOUND", "SFTP 文件或目录不存在"
	case errors.Is(err, os.ErrPermission), errors.Is(err, sftp.ErrSSHFxPermissionDenied):
		code, message = "SSH_SFTP_PERMISSION_DENIED", "SFTP 操作被远端拒绝"
	case errors.Is(err, sftp.ErrSSHFxOpUnsupported):
		code, message = "SSH_SFTP_UNSUPPORTED", "远端 SFTP 服务不支持此操作"
	}
	logging.Error("ssh_sftp."+operation, "SFTP 操作失败: host_id=%d code=%s", managed.host.ID, code)
	return sshError(code, message, err)
}

func normalizeSFTPPath(value string, allowCurrent bool) (string, error) {
	if value == "" {
		if allowCurrent {
			return ".", nil
		}
		return "", sshError("SSH_SFTP_INVALID_PATH", "SFTP 路径不能为空", nil)
	}
	if len(value) > sshMaximumSFTPPathSize || strings.ContainsAny(value, "\x00\r\n") {
		return "", sshError("SSH_SFTP_INVALID_PATH", "SFTP 路径无效或过长", nil)
	}
	cleaned := path.Clean(value)
	if !allowCurrent && (cleaned == "." || cleaned == "/" || cleaned == ".." || strings.HasPrefix(cleaned, "../")) {
		return "", sshError("SSH_SFTP_INVALID_PATH", "禁止修改 SFTP 根目录或当前目录", nil)
	}
	return cleaned, nil
}

type sshSFTPActivityReader struct {
	reader io.Reader
	active *atomic.Int64
	now    func() time.Time
}

func (reader *sshSFTPActivityReader) Read(content []byte) (int, error) {
	count, err := reader.reader.Read(content)
	if count > 0 {
		reader.active.Store(reader.now().UnixMilli())
	}
	return count, err
}

type sshSFTPActivityReadCloser struct {
	io.ReadCloser
	active *atomic.Int64
	now    func() time.Time
	done   func()
	once   sync.Once
}

func (reader *sshSFTPActivityReadCloser) Read(content []byte) (int, error) {
	count, err := reader.ReadCloser.Read(content)
	if count > 0 {
		reader.active.Store(reader.now().UnixMilli())
	}
	if err != nil {
		reader.once.Do(reader.done)
	}
	return count, err
}

func (reader *sshSFTPActivityReadCloser) Close() error {
	err := reader.ReadCloser.Close()
	reader.once.Do(reader.done)
	return err
}
