package service

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshAttachTimeout         = 30 * time.Second
	sshIdleTimeout           = 30 * time.Minute
	sshMaximumSessionAge     = 8 * time.Hour
	sshKeepaliveInterval     = 30 * time.Second
	sshMaximumSessions       = 4
	sshMaximumHostSessions   = 2
	sshMaximumInputSize      = 64 << 10
	sshOutputChunkSize       = 32 << 10
	sshSessionInputQueueSize = 32
	sshSFTPCloseGrace        = 5 * time.Second
)

type managedSSHSession struct {
	id          string
	attachToken string
	sftpToken   [sha256.Size]byte
	host        *model.SSHHost
	client      *ssh.Client
	sftp        *sftp.Client
	createdAt   time.Time
	expiresAt   time.Time
	columns     int
	rows        int

	mu         sync.Mutex
	owner      *websocket.Conn
	session    *ssh.Session
	stdin      io.WriteCloser
	input      chan []byte
	done       chan struct{}
	lastActive atomic.Int64
	closeOnce  sync.Once
	sftpOps    sync.WaitGroup
	monitorMu  sync.Mutex
	uploads    map[uint64]io.ReadCloser
	nextUpload uint64
	terminated bool
}

type sshSessionAttachCommand struct {
	SessionID   string `json:"session_id"`
	AttachToken string `json:"attach_token"`
	Columns     int    `json:"columns"`
	Rows        int    `json:"rows"`
}

type sshSessionInputCommand struct {
	SessionID string `json:"session_id"`
	Content   string `json:"content"`
}

type sshSessionResizeCommand struct {
	SessionID string `json:"session_id"`
	Columns   int    `json:"columns"`
	Rows      int    `json:"rows"`
}

type sshSessionCloseCommand struct {
	SessionID string `json:"session_id"`
}

func (svc *SSHHostService) SetRealtimeService(realtime *RealtimeService) {
	svc.sessionMu.Lock()
	svc.realtime = realtime
	svc.sessionMu.Unlock()
	if realtime != nil {
		realtime.SetCommandHandlers(svc.HandleRealtimeCommand, svc.CloseRealtimeClientSessions)
	}
}

func (svc *SSHHostService) CreateSession(ctx context.Context, hostID int64, request model.SSHSessionCreateRequest) (*model.SSHSessionCreateResponse, error) {
	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	request.Columns, request.Rows = normalizeSSHWindow(request.Columns, request.Rows)
	if err := svc.reserveSessionSlot(hostID); err != nil {
		return nil, err
	}
	reserved := true
	defer func() {
		if reserved {
			svc.releaseSessionSlot(hostID)
		}
	}()
	started := svc.now()
	client, _, _, _, err := svc.connect(ctx, host)
	if err != nil {
		code, _ := sshErrorInfo(err)
		svc.audit(host, "session_failed", "error", code, svc.now().Sub(started), "")
		return nil, err
	}
	id, err := randomHex(24)
	if err != nil {
		client.Close()
		return nil, sshError("SSH_SESSION_CREATE_FAILED", "无法创建 SSH 会话标识", err)
	}
	token, err := randomHex(32)
	if err != nil {
		client.Close()
		return nil, sshError("SSH_SESSION_CREATE_FAILED", "无法创建 SSH 会话授权", err)
	}
	sftpToken, err := randomHex(32)
	if err != nil {
		client.Close()
		return nil, sshError("SSH_SESSION_CREATE_FAILED", "无法创建 SFTP 会话授权", err)
	}
	now := svc.now()
	managed := &managedSSHSession{
		id: id, attachToken: token, host: host, client: client, createdAt: now,
		expiresAt: now.Add(sshAttachTimeout), columns: request.Columns, rows: request.Rows,
		input: make(chan []byte, sshSessionInputQueueSize),
		done:  make(chan struct{}),
	}
	managed.sftpToken = sha256.Sum256([]byte(sftpToken))
	managed.lastActive.Store(now.UnixMilli())
	svc.sessionMu.Lock()
	if svc.closed {
		svc.releaseSessionSlotLocked(hostID)
		svc.sessionMu.Unlock()
		client.Close()
		reserved = false
		return nil, sshError("SSH_SESSION_CLOSED", "SSH 会话服务已停止", nil)
	}
	if _, exists := svc.sessions[id]; exists {
		svc.releaseSessionSlotLocked(hostID)
		svc.sessionMu.Unlock()
		client.Close()
		reserved = false
		return nil, sshError("SSH_SESSION_CREATE_FAILED", "SSH 会话标识冲突，请重试", nil)
	}
	svc.sessions[id] = managed
	svc.releaseSessionSlotLocked(hostID)
	svc.sessionMu.Unlock()
	reserved = false
	go svc.expireUnattachedSession(managed)
	logging.Info("ssh_session.create", "创建 SSH 会话: host_id=%d", hostID)
	return &model.SSHSessionCreateResponse{
		SessionID: id, AttachToken: token, SFTPToken: sftpToken, ExpiresAt: managed.expiresAt.UnixMilli(),
	}, nil
}

func (svc *SSHHostService) CloseSession(sessionID string) error {
	if !svc.closeSession(sessionID, "cancelled", "") {
		return sshError("SSH_SESSION_NOT_FOUND", "SSH 会话不存在", nil)
	}
	return nil
}

func (svc *SSHHostService) HandleRealtimeCommand(conn *websocket.Conn, command model.WSCommand) {
	switch command.Type {
	case "ssh.session.attach":
		var payload sshSessionAttachCommand
		if json.Unmarshal(command.Data, &payload) != nil {
			svc.sendSessionError(conn, "", "SSH_SESSION_COMMAND_INVALID", "SSH 会话命令无效")
			return
		}
		svc.attachSession(conn, payload)
	case "ssh.session.input":
		var payload sshSessionInputCommand
		if json.Unmarshal(command.Data, &payload) != nil {
			svc.sendSessionError(conn, "", "SSH_SESSION_COMMAND_INVALID", "SSH 会话输入无效")
			return
		}
		svc.writeSessionInput(conn, payload)
	case "ssh.session.resize":
		var payload sshSessionResizeCommand
		if json.Unmarshal(command.Data, &payload) != nil {
			svc.sendSessionError(conn, "", "SSH_SESSION_COMMAND_INVALID", "SSH 终端尺寸无效")
			return
		}
		svc.resizeSession(conn, payload)
	case "ssh.session.close":
		var payload sshSessionCloseCommand
		if json.Unmarshal(command.Data, &payload) != nil {
			return
		}
		if managed := svc.ownedSession(conn, payload.SessionID); managed != nil {
			svc.closeSession(payload.SessionID, "cancelled", "")
		}
	}
}

func (svc *SSHHostService) CloseRealtimeClientSessions(conn *websocket.Conn) {
	svc.sessionMu.Lock()
	ids := make([]string, 0)
	for id, managed := range svc.sessions {
		managed.mu.Lock()
		owned := managed.owner == conn
		managed.mu.Unlock()
		if owned {
			ids = append(ids, id)
		}
	}
	svc.sessionMu.Unlock()
	for _, id := range ids {
		svc.closeSession(id, "cancelled", "SSH_WEBSOCKET_CLOSED")
	}
}

func (svc *SSHHostService) Close() {
	svc.sessionMu.Lock()
	ids := make([]string, 0, len(svc.sessions))
	for id := range svc.sessions {
		ids = append(ids, id)
	}
	svc.closed = true
	svc.sessionMu.Unlock()
	for _, id := range ids {
		svc.closeSession(id, "cancelled", "SSH_SERVICE_STOPPED")
	}
}

func (svc *SSHHostService) attachSession(conn *websocket.Conn, payload sshSessionAttachCommand) {
	svc.sessionMu.Lock()
	managed := svc.sessions[payload.SessionID]
	svc.sessionMu.Unlock()
	if managed == nil {
		svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_NOT_FOUND", "SSH 会话不存在")
		return
	}
	managed.mu.Lock()
	if managed.terminated || managed.owner != nil || svc.now().After(managed.expiresAt) || subtle.ConstantTimeCompare([]byte(managed.attachToken), []byte(payload.AttachToken)) != 1 {
		managed.mu.Unlock()
		svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_ATTACH_EXPIRED", "SSH 会话授权无效或已过期")
		return
	}
	managed.owner = conn
	managed.attachToken = ""
	managed.columns, managed.rows = normalizeSSHWindow(payload.Columns, payload.Rows)
	managed.mu.Unlock()
	if !svc.hasRealtimeClient(conn) {
		svc.closeSession(managed.id, "cancelled", "SSH_WEBSOCKET_CLOSED")
		return
	}
	attachTimedOut := atomic.Bool{}
	attachTimer := time.AfterFunc(sshConnectTimeout, func() {
		attachTimedOut.Store(true)
		if managed.client != nil {
			_ = managed.client.Close()
		}
	})
	defer attachTimer.Stop()

	session, err := managed.client.NewSession()
	if err != nil {
		code, message := "SSH_SESSION_OPEN_FAILED", "无法创建 SSH 终端会话"
		if attachTimedOut.Load() {
			code, message = "SSH_SESSION_ATTACH_TIMEOUT", "创建 SSH 终端会话超时"
		}
		svc.failSessionStart(conn, managed, code, message)
		return
	}
	stdin, err := session.StdinPipe()
	if err != nil {
		session.Close()
		svc.failSessionStart(conn, managed, "SSH_SESSION_OPEN_FAILED", "无法创建 SSH 终端输入")
		return
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		session.Close()
		svc.failSessionStart(conn, managed, "SSH_SESSION_OPEN_FAILED", "无法创建 SSH 终端输出")
		return
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		session.Close()
		svc.failSessionStart(conn, managed, "SSH_SESSION_OPEN_FAILED", "无法创建 SSH 终端错误输出")
		return
	}
	modes := ssh.TerminalModes{ssh.ECHO: 1, ssh.TTY_OP_ISPEED: 14400, ssh.TTY_OP_OSPEED: 14400}
	if err := session.RequestPty(managed.host.TerminalType, managed.rows, managed.columns, modes); err != nil {
		session.Close()
		svc.failSessionStart(conn, managed, "SSH_PTY_FAILED", "远端拒绝创建 PTY")
		return
	}
	if err := session.Shell(); err != nil {
		session.Close()
		svc.failSessionStart(conn, managed, "SSH_SHELL_FAILED", "远端拒绝启动 shell")
		return
	}
	if !managed.commitAttach(session, stdin) {
		_ = stdin.Close()
		_ = session.Close()
		return
	}
	managed.lastActive.Store(svc.now().UnixMilli())

	var outputs sync.WaitGroup
	outputs.Add(2)
	go svc.pumpSessionOutput(managed, stdout, &outputs)
	go svc.pumpSessionOutput(managed, stderr, &outputs)
	go svc.pumpSessionInput(managed)
	go svc.monitorSession(managed)
	go func() {
		err := session.Wait()
		outputs.Wait()
		code := ""
		result := "success"
		if err != nil && !errors.Is(err, io.EOF) {
			code, result = "SSH_SESSION_REMOTE_CLOSED", "error"
		}
		svc.closeSession(managed.id, result, code)
	}()
	svc.sendTo(conn, "ssh.session.status", map[string]any{"session_id": managed.id, "status": "attached"})
	svc.audit(managed.host, "session_started", "success", "", svc.now().Sub(managed.createdAt), sessionIDHash(managed.id))
	logging.Info("ssh_session.attach", "SSH 会话已连接: host_id=%d", managed.host.ID)
}

func (managed *managedSSHSession) commitAttach(session *ssh.Session, stdin io.WriteCloser) bool {
	managed.mu.Lock()
	defer managed.mu.Unlock()
	if managed.terminated {
		return false
	}
	managed.session, managed.stdin = session, stdin
	return true
}

func (svc *SSHHostService) writeSessionInput(conn *websocket.Conn, payload sshSessionInputCommand) {
	managed := svc.ownedSession(conn, payload.SessionID)
	if managed == nil {
		svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_NOT_FOUND", "SSH 会话不存在或不属于当前连接")
		return
	}
	content, err := base64.StdEncoding.DecodeString(payload.Content)
	if err != nil || len(content) > sshMaximumInputSize {
		svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_INPUT_INVALID", "SSH 会话输入无效或过大")
		return
	}
	managed.lastActive.Store(svc.now().UnixMilli())
	select {
	case <-managed.done:
		return
	case managed.input <- content:
	default:
		svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_BACKPRESSURE", "SSH 会话输入积压，连接已关闭")
		svc.closeSession(payload.SessionID, "error", "SSH_SESSION_BACKPRESSURE")
	}
}

func (svc *SSHHostService) resizeSession(conn *websocket.Conn, payload sshSessionResizeCommand) {
	managed := svc.ownedSession(conn, payload.SessionID)
	if managed == nil {
		return
	}
	columns, rows := normalizeSSHWindow(payload.Columns, payload.Rows)
	managed.mu.Lock()
	session := managed.session
	managed.columns, managed.rows = columns, rows
	managed.mu.Unlock()
	if session != nil {
		if err := session.WindowChange(rows, columns); err != nil {
			svc.sendSessionError(conn, payload.SessionID, "SSH_SESSION_RESIZE_FAILED", "调整 SSH 终端尺寸失败")
		}
	}
}

func (svc *SSHHostService) pumpSessionInput(managed *managedSSHSession) {
	for {
		var content []byte
		select {
		case <-managed.done:
			return
		case content = <-managed.input:
		}
		managed.mu.Lock()
		stdin := managed.stdin
		managed.mu.Unlock()
		if stdin == nil {
			return
		}
		if _, err := stdin.Write(content); err != nil {
			svc.closeSession(managed.id, "error", "SSH_SESSION_WRITE_FAILED")
			return
		}
	}
}

func (svc *SSHHostService) pumpSessionOutput(managed *managedSSHSession, reader io.Reader, completed *sync.WaitGroup) {
	defer completed.Done()
	buffer := make([]byte, sshOutputChunkSize)
	for {
		count, err := reader.Read(buffer)
		if count > 0 {
			managed.lastActive.Store(svc.now().UnixMilli())
			managed.mu.Lock()
			owner := managed.owner
			managed.mu.Unlock()
			if owner == nil || !svc.sendTo(owner, "ssh.session.output", map[string]any{
				"session_id": managed.id,
				"content":    base64.StdEncoding.EncodeToString(buffer[:count]),
			}) {
				svc.closeSession(managed.id, "error", "SSH_SESSION_BACKPRESSURE")
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (svc *SSHHostService) monitorSession(managed *managedSSHSession) {
	ticker := time.NewTicker(sshKeepaliveInterval)
	defer ticker.Stop()
	for {
		select {
		case <-managed.done:
			return
		case <-ticker.C:
		}
		if svc.now().Sub(managed.createdAt) >= sshMaximumSessionAge {
			svc.closeSession(managed.id, "cancelled", "SSH_SESSION_MAX_AGE")
			return
		}
		lastActive := time.UnixMilli(managed.lastActive.Load())
		if svc.now().Sub(lastActive) >= sshIdleTimeout {
			svc.closeSession(managed.id, "cancelled", "SSH_SESSION_IDLE_TIMEOUT")
			return
		}
		if _, _, err := managed.client.SendRequest("keepalive@openssh.com", true, nil); err != nil {
			svc.closeSession(managed.id, "error", "SSH_SESSION_KEEPALIVE_FAILED")
			return
		}
	}
}

func (svc *SSHHostService) expireUnattachedSession(managed *managedSSHSession) {
	timer := time.NewTimer(time.Until(managed.expiresAt))
	defer timer.Stop()
	<-timer.C
	managed.mu.Lock()
	unattached := managed.owner == nil
	managed.mu.Unlock()
	if unattached {
		svc.closeSession(managed.id, "cancelled", "SSH_SESSION_ATTACH_EXPIRED")
	}
}

func (svc *SSHHostService) closeSession(sessionID, result, errorCode string) bool {
	svc.sessionMu.Lock()
	managed := svc.sessions[sessionID]
	if managed != nil {
		delete(svc.sessions, sessionID)
	}
	svc.sessionMu.Unlock()
	if managed == nil {
		return false
	}
	managed.closeOnce.Do(func() {
		managed.mu.Lock()
		managed.terminated = true
		owner, session, stdin, sftpClient := managed.owner, managed.session, managed.stdin, managed.sftp
		uploads := make([]io.ReadCloser, 0, len(managed.uploads))
		for _, upload := range managed.uploads {
			uploads = append(uploads, upload)
		}
		managed.owner, managed.session, managed.stdin, managed.sftp = nil, nil, nil, nil
		managed.uploads = nil
		managed.sftpToken = [sha256.Size]byte{}
		close(managed.done)
		managed.mu.Unlock()
		for _, upload := range uploads {
			_ = upload.Close()
		}
		if stdin != nil {
			_ = stdin.Close()
		}
		if session != nil {
			_ = session.Close()
		}
		sftpDone := make(chan struct{})
		go func() {
			managed.sftpOps.Wait()
			close(sftpDone)
		}()
		select {
		case <-sftpDone:
		case <-time.After(sshSFTPCloseGrace):
			logging.Error("ssh_sftp.close", "等待 SFTP 操作结束超时: host_id=%d", managed.host.ID)
		}
		if sftpClient != nil {
			_ = sftpClient.Close()
		}
		if managed.client != nil {
			_ = managed.client.Close()
		}
		if owner != nil {
			svc.sendTo(owner, "ssh.session.closed", map[string]any{
				"session_id": sessionID, "result": result, "error_code": errorCode,
			})
		}
		svc.audit(managed.host, "session_closed", result, errorCode, svc.now().Sub(managed.createdAt), sessionIDHash(sessionID))
		logging.Info("ssh_session.close", "关闭 SSH 会话: host_id=%d result=%s code=%s", managed.host.ID, result, errorCode)
	})
	return true
}

func (svc *SSHHostService) failSessionStart(conn *websocket.Conn, managed *managedSSHSession, code, message string) {
	svc.sendSessionError(conn, managed.id, code, message)
	svc.closeSession(managed.id, "error", code)
}

func (svc *SSHHostService) ownedSession(conn *websocket.Conn, sessionID string) *managedSSHSession {
	svc.sessionMu.Lock()
	managed := svc.sessions[sessionID]
	svc.sessionMu.Unlock()
	if managed == nil {
		return nil
	}
	managed.mu.Lock()
	owned := managed.owner == conn
	managed.mu.Unlock()
	if !owned {
		return nil
	}
	return managed
}

func (svc *SSHHostService) reserveSessionSlot(hostID int64) error {
	svc.sessionMu.Lock()
	defer svc.sessionMu.Unlock()
	if err := svc.checkSessionLimitLocked(hostID); err != nil {
		return err
	}
	svc.pendingSessions++
	svc.pendingByHost[hostID]++
	return nil
}

func (svc *SSHHostService) checkSessionLimitLocked(hostID int64) error {
	if svc.closed {
		return sshError("SSH_SESSION_CLOSED", "SSH 会话服务已停止", nil)
	}
	if len(svc.sessions)+svc.pendingSessions >= sshMaximumSessions {
		return sshError("SSH_SESSION_LIMIT", "SSH 会话已达到全局并发上限", nil)
	}
	hostCount := 0
	for _, managed := range svc.sessions {
		if managed.host.ID == hostID {
			hostCount++
		}
	}
	if hostCount+svc.pendingByHost[hostID] >= sshMaximumHostSessions {
		return sshError("SSH_SESSION_LIMIT", "该主机的 SSH 会话已达到并发上限", nil)
	}
	return nil
}

func (svc *SSHHostService) releaseSessionSlot(hostID int64) {
	svc.sessionMu.Lock()
	svc.releaseSessionSlotLocked(hostID)
	svc.sessionMu.Unlock()
}

func (svc *SSHHostService) releaseSessionSlotLocked(hostID int64) {
	if svc.pendingSessions > 0 {
		svc.pendingSessions--
	}
	if svc.pendingByHost[hostID] <= 1 {
		delete(svc.pendingByHost, hostID)
	} else {
		svc.pendingByHost[hostID]--
	}
}

func (svc *SSHHostService) sendSessionError(conn *websocket.Conn, sessionID, code, message string) {
	svc.sendTo(conn, "ssh.session.error", map[string]any{"session_id": sessionID, "code": code, "message": message})
}

func (svc *SSHHostService) sendTo(conn *websocket.Conn, eventType string, data any) bool {
	svc.sessionMu.Lock()
	realtime := svc.realtime
	svc.sessionMu.Unlock()
	return realtime != nil && realtime.SendTo(conn, eventType, data)
}

func (svc *SSHHostService) hasRealtimeClient(conn *websocket.Conn) bool {
	svc.sessionMu.Lock()
	realtime := svc.realtime
	svc.sessionMu.Unlock()
	return realtime != nil && realtime.HasClient(conn)
}

func normalizeSSHWindow(columns, rows int) (int, int) {
	if columns < 20 || columns > 500 {
		columns = 80
	}
	if rows < 2 || rows > 200 {
		rows = 24
	}
	return columns, rows
}

func sessionIDHash(sessionID string) string {
	hash := sha256.Sum256([]byte(sessionID))
	return hex.EncodeToString(hash[:16])
}
