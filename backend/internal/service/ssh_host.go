package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

const (
	sshConnectTimeout   = 10 * time.Second
	sshChallengeTimeout = 5 * time.Minute
)

type sshCore interface {
	IsRunning() bool
}

type SSHServiceError struct {
	Code    string
	Message string
	Details any
	Cause   error
}

func (err *SSHServiceError) Error() string { return err.Message }
func (err *SSHServiceError) Unwrap() error { return err.Cause }

type sshHostKeyChallenge struct {
	model.SSHHostKeyChallenge
	HostID         int64
	HostAddress    string
	PublicKey      string
	ConnectionMode string
}

type SSHHostService struct {
	store  *store.Store
	cipher *sshSecretCipher
	core   sshCore
	now    func() time.Time

	challengeMu sync.Mutex
	challenges  map[string]sshHostKeyChallenge

	sessionMu       sync.Mutex
	sessions        map[string]*managedSSHSession
	pendingSessions int
	pendingByHost   map[int64]int
	realtime        *RealtimeService
	closed          bool

	sftpTextLockMu sync.Mutex
	sftpTextLocks  map[string]*sshSFTPTextLock

	shareMu sync.Mutex
}

func NewSSHHostService(db *store.Store, p *paths.Paths, core sshCore) (*SSHHostService, error) {
	credentials, err := db.ListSSHCredentials()
	if err != nil {
		return nil, fmt.Errorf("inspect SSH credentials: %w", err)
	}
	cipher, err := loadOrCreateSSHSecretCipher(p, len(credentials) == 0)
	if err != nil {
		return nil, err
	}
	return &SSHHostService{
		store: db, cipher: cipher, core: core, now: time.Now,
		challenges: make(map[string]sshHostKeyChallenge), sessions: make(map[string]*managedSSHSession),
		pendingByHost: make(map[int64]int),
	}, nil
}

func (svc *SSHHostService) ListCredentials() ([]model.SSHCredential, error) {
	logging.Info("ssh_credential.list", "读取 SSH 凭据")
	return svc.store.ListSSHCredentials()
}

func (svc *SSHHostService) CreateCredential(request model.SSHCredentialRequest) (*model.SSHCredential, error) {
	item, err := svc.buildCredential(request, nil)
	if err != nil {
		return nil, err
	}
	if err := svc.store.CreateSSHCredential(item); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	logging.Info("ssh_credential.create", "创建 SSH 凭据: %d", item.ID)
	return svc.store.GetSSHCredential(item.ID)
}

func (svc *SSHHostService) UpdateCredential(id int64, request model.SSHCredentialRequest) (*model.SSHCredential, error) {
	existing, err := svc.store.GetSSHCredential(id)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	item, err := svc.buildCredential(request, existing)
	if err != nil {
		return nil, err
	}
	if err := svc.store.UpdateSSHCredential(item); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	logging.Info("ssh_credential.update", "更新 SSH 凭据: %d", id)
	return svc.store.GetSSHCredential(id)
}

func (svc *SSHHostService) DeleteCredential(id int64) error {
	if err := svc.store.DeleteSSHCredential(id); err != nil {
		return normalizeSSHStoreError(err)
	}
	logging.Info("ssh_credential.delete", "删除 SSH 凭据: %d", id)
	return nil
}

func (svc *SSHHostService) buildCredential(request model.SSHCredentialRequest, existing *model.SSHCredential) (*model.SSHCredential, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.AuthType = strings.TrimSpace(request.AuthType)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 128 {
		return nil, sshError("SSH_CREDENTIAL_INVALID", "凭据名称不能为空且不能超过 128 个字符", nil)
	}
	if request.AuthType != "password" && request.AuthType != "private_key" {
		return nil, sshError("SSH_CREDENTIAL_INVALID", "凭据类型必须是 password 或 private_key", nil)
	}
	if existing != nil && request.Secret == "" {
		if request.AuthType != existing.AuthType {
			return nil, sshError("SSH_CREDENTIAL_INVALID", "更改凭据类型时必须提供新的秘密", nil)
		}
		copy := *existing
		copy.Name = request.Name
		return &copy, nil
	}
	if request.Secret == "" {
		return nil, sshError("SSH_CREDENTIAL_INVALID", "凭据秘密不能为空", nil)
	}

	contextID, err := randomHex(16)
	if err != nil {
		return nil, sshError("SSH_CREDENTIAL_INVALID", "无法生成凭据加密上下文", err)
	}
	item := &model.SSHCredential{Name: request.Name, AuthType: request.AuthType, SecretContext: contextID, KeyVersion: 1}
	if existing != nil {
		item.ID, item.CreatedAt = existing.ID, existing.CreatedAt
	}
	if request.AuthType == "private_key" {
		signer, parseErr := parseSSHPrivateKey([]byte(request.Secret), []byte(request.Passphrase))
		if parseErr != nil {
			return nil, sshError("SSH_CREDENTIAL_INVALID", "私钥或私钥口令无效", nil)
		}
		item.KeyFingerprint = ssh.FingerprintSHA256(signer.PublicKey())
	}
	item.SecretCiphertext, item.SecretNonce, err = svc.cipher.encrypt(contextID, "secret", []byte(request.Secret))
	if err != nil {
		return nil, sshError("SSH_SECRET_ENCRYPT_FAILED", "加密 SSH 凭据失败", err)
	}
	if request.AuthType == "private_key" && request.Passphrase != "" {
		item.PassphraseCiphertext, item.PassphraseNonce, err = svc.cipher.encrypt(contextID, "passphrase", []byte(request.Passphrase))
		if err != nil {
			return nil, sshError("SSH_SECRET_ENCRYPT_FAILED", "加密私钥口令失败", err)
		}
	}
	return item, nil
}

func (svc *SSHHostService) ListHosts() ([]model.SSHHost, error) {
	logging.Info("ssh_host.list", "读取 SSH 主机")
	return svc.store.ListSSHHosts()
}

func (svc *SSHHostService) GetHost(id int64) (*model.SSHHost, error) {
	item, err := svc.store.GetSSHHost(id)
	return item, normalizeSSHStoreError(err)
}

func (svc *SSHHostService) CreateHost(request model.SSHHostRequest) (*model.SSHHost, error) {
	item, err := svc.normalizeHost(request)
	if err != nil {
		return nil, err
	}
	if err := svc.store.CreateSSHHost(item); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	logging.Info("ssh_host.create", "创建 SSH 主机: %d", item.ID)
	return svc.store.GetSSHHost(item.ID)
}

func (svc *SSHHostService) UpdateHost(id int64, request model.SSHHostRequest) (*model.SSHHost, error) {
	existing, err := svc.store.GetSSHHost(id)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	item, err := svc.normalizeHost(request)
	if err != nil {
		return nil, err
	}
	item.ID, item.CreatedAt = id, existing.CreatedAt
	if err := svc.store.UpdateSSHHost(item); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !strings.EqualFold(existing.Host, item.Host) || existing.Port != item.Port {
		if err := svc.store.DeleteSSHHostKey(id); err != nil {
			return nil, err
		}
	}
	logging.Info("ssh_host.update", "更新 SSH 主机: %d", id)
	return svc.store.GetSSHHost(id)
}

func (svc *SSHHostService) DeleteHost(id int64) error {
	if err := svc.store.DeleteSSHHost(id); err != nil {
		return normalizeSSHStoreError(err)
	}
	logging.Info("ssh_host.delete", "删除 SSH 主机: %d", id)
	return nil
}

func (svc *SSHHostService) normalizeHost(request model.SSHHostRequest) (*model.SSHHost, error) {
	item, err := normalizeSSHHostFields(request)
	if err != nil {
		return nil, err
	}
	if _, err := svc.store.GetSSHCredential(item.CredentialID); err != nil {
		if errors.Is(err, store.ErrSSHCredentialNotFound) {
			return nil, sshError("SSH_CREDENTIAL_NOT_FOUND", "SSH 凭据不存在", err)
		}
		return nil, err
	}
	if item.ConnectionMode == "node_exposure" {
		if _, err := svc.store.GetNodeExposure(*item.NodeExposureID); err != nil {
			return nil, sshError("SSH_NODE_EXPOSURE_NOT_FOUND", "节点入口不存在", err)
		}
	}
	return item, nil
}

func normalizeSSHHostFields(request model.SSHHostRequest) (*model.SSHHost, error) {
	request.Name = strings.TrimSpace(request.Name)
	request.GroupName = strings.TrimSpace(request.GroupName)
	request.Host = strings.TrimSpace(request.Host)
	request.Username = strings.TrimSpace(request.Username)
	request.ConnectionMode = strings.TrimSpace(request.ConnectionMode)
	request.TerminalType = strings.TrimSpace(request.TerminalType)
	request.Notes = strings.TrimSpace(request.Notes)
	if request.Name == "" || utf8.RuneCountInString(request.Name) > 128 {
		return nil, sshError("SSH_HOST_INVALID", "主机名称不能为空且不能超过 128 个字符", nil)
	}
	if utf8.RuneCountInString(request.GroupName) > 128 || utf8.RuneCountInString(request.Notes) > 4096 {
		return nil, sshError("SSH_HOST_INVALID", "分组不能超过 128 个字符，备注不能超过 4096 个字符", nil)
	}
	if request.Host == "" || strings.ContainsAny(request.Host, "\x00\r\n\t /\\") || utf8.RuneCountInString(request.Host) > 253 {
		return nil, sshError("SSH_HOST_INVALID", "SSH 主机地址无效", nil)
	}
	if request.Port < 1 || request.Port > 65535 {
		return nil, sshError("SSH_HOST_INVALID", "SSH 端口必须在 1 到 65535 之间", nil)
	}
	if request.Username == "" || strings.ContainsAny(request.Username, "\x00\r\n") || utf8.RuneCountInString(request.Username) > 128 {
		return nil, sshError("SSH_HOST_INVALID", "SSH 用户名无效", nil)
	}
	if request.ConnectionMode == "" {
		request.ConnectionMode = "direct"
	}
	if request.ConnectionMode != "direct" && request.ConnectionMode != "node_exposure" {
		return nil, sshError("SSH_HOST_INVALID", "连接模式必须是 direct 或 node_exposure", nil)
	}
	if request.ConnectionMode == "direct" {
		request.NodeExposureID = nil
	} else {
		if request.NodeExposureID == nil || *request.NodeExposureID <= 0 {
			return nil, sshError("SSH_HOST_INVALID", "通过节点入口连接时必须选择入口", nil)
		}
	}
	if request.TerminalType == "" {
		request.TerminalType = "xterm-256color"
	}
	if strings.ContainsAny(request.TerminalType, "\x00\r\n ") || utf8.RuneCountInString(request.TerminalType) > 64 {
		return nil, sshError("SSH_HOST_INVALID", "终端类型无效", nil)
	}
	for _, tag := range request.Tags {
		if utf8.RuneCountInString(strings.TrimSpace(tag)) > 64 {
			return nil, sshError("SSH_HOST_INVALID", "单个 SSH 主机标签不能超过 64 个字符", nil)
		}
	}
	tags := normalizeSSHTags(request.Tags)
	if len(tags) > 32 {
		return nil, sshError("SSH_HOST_INVALID", "SSH 主机标签不能超过 32 个", nil)
	}
	return &model.SSHHost{
		Name: request.Name, GroupName: request.GroupName, Host: request.Host, Port: request.Port,
		Username: request.Username, CredentialID: request.CredentialID, ConnectionMode: request.ConnectionMode,
		NodeExposureID: request.NodeExposureID, TerminalType: request.TerminalType, Enabled: request.Enabled,
		Tags: tags, Notes: request.Notes,
	}, nil
}

func (svc *SSHHostService) GetHostKey(hostID int64) (*model.SSHHostKey, error) {
	if _, err := svc.store.GetSSHHost(hostID); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	return svc.store.GetSSHHostKey(hostID)
}

func (svc *SSHHostService) DeleteHostKey(hostID int64) error {
	if _, err := svc.store.GetSSHHost(hostID); err != nil {
		return normalizeSSHStoreError(err)
	}
	if err := svc.store.DeleteSSHHostKey(hostID); err != nil {
		return err
	}
	logging.Info("ssh_host_key.delete", "删除 SSH Host Key 信任: %d", hostID)
	return nil
}

func (svc *SSHHostService) TrustHostKey(hostID int64, request model.SSHHostKeyTrustRequest, rotate bool) (*model.SSHHostKey, error) {
	svc.challengeMu.Lock()
	svc.pruneChallengesLocked()
	challenge, exists := svc.challenges[request.ChallengeID]
	if exists {
		delete(svc.challenges, request.ChallengeID)
	}
	svc.challengeMu.Unlock()
	if !exists || challenge.HostID != hostID || challenge.FingerprintSHA256 != request.FingerprintSHA256 {
		return nil, sshError("SSH_HOST_KEY_CHALLENGE_INVALID", "Host Key 确认已失效，请重新测试连接", nil)
	}
	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if net.JoinHostPort(host.Host, strconv.Itoa(host.Port)) != challenge.HostAddress {
		return nil, sshError("SSH_HOST_KEY_CHALLENGE_INVALID", "主机地址已变化，请重新测试连接", nil)
	}
	trusted, err := svc.store.GetSSHHostKey(hostID)
	if err != nil {
		return nil, err
	}
	if rotate && trusted == nil {
		return nil, sshError("SSH_HOST_KEY_NOT_TRUSTED", "主机尚无可轮换的 Host Key", nil)
	}
	if rotate && (challenge.TrustedFingerprint == "" || trusted.FingerprintSHA256 != challenge.TrustedFingerprint) {
		return nil, sshError("SSH_HOST_KEY_CHALLENGE_INVALID", "Host Key 信任已变化，请重新测试连接", nil)
	}
	if !rotate && trusted != nil {
		return nil, sshError("SSH_HOST_KEY_ALREADY_TRUSTED", "主机已有 Host Key；如需替换请执行轮换", nil)
	}
	item := &model.SSHHostKey{
		HostID: hostID, KeyType: challenge.KeyType, PublicKey: challenge.PublicKey,
		FingerprintSHA256: challenge.FingerprintSHA256,
	}
	if trusted != nil {
		item.FirstSeenAt = trusted.FirstSeenAt
	}
	if err := svc.store.UpsertSSHHostKey(item); err != nil {
		return nil, err
	}
	action := "ssh_host_key.trust"
	if rotate {
		action = "ssh_host_key.rotate"
	}
	logging.Info(action, "更新 SSH Host Key 信任: %d", hostID)
	return svc.store.GetSSHHostKey(hostID)
}

func (svc *SSHHostService) TestHost(ctx context.Context, hostID int64) (*model.SSHConnectionTestResult, error) {
	started := svc.now()
	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	client, metadata, observed, handshakeDuration, err := svc.connect(ctx, host)
	duration := svc.now().Sub(started)
	if err != nil {
		code, message := sshErrorInfo(err)
		status := "unavailable"
		if code == "SSH_HOST_KEY_UNKNOWN" || code == "SSH_HOST_KEY_CHANGED" {
			status = "host_key_pending"
		}
		_ = svc.store.UpdateSSHHostTestResult(hostID, status, 0, code, message)
		svc.audit(host, "test", "error", code, duration, "")
		logging.Error("ssh_host.test", "SSH 主机测试失败: id=%d code=%s", hostID, code)
		return nil, err
	}
	defer client.Close()
	session, err := newSSHSessionWithTimeout(ctx, client)
	if err != nil {
		wrapped := err
		var serviceErr *SSHServiceError
		if !errors.As(err, &serviceErr) {
			wrapped = sshError("SSH_SESSION_OPEN_FAILED", "SSH 认证成功但无法创建会话", err)
		}
		code, message := sshErrorInfo(wrapped)
		_ = svc.store.UpdateSSHHostTestResult(hostID, "unavailable", 0, code, message)
		svc.audit(host, "test", "error", code, duration, "")
		return nil, wrapped
	}
	_ = session.Close()
	latency := duration.Milliseconds()
	_ = svc.store.UpdateSSHHostTestResult(hostID, "available", latency, "", "")
	_ = svc.store.TouchSSHHostKey(hostID)
	svc.audit(host, "test", "success", "", duration, "")
	logging.Info("ssh_host.test", "SSH 主机测试成功: id=%d mode=%s latency_ms=%d", hostID, host.ConnectionMode, latency)
	return &model.SSHConnectionTestResult{
		Success: true, LatencyMS: latency, HandshakeMS: handshakeDuration.Milliseconds(),
		ServerVersion: string(metadata.ServerVersion()), KeyType: observed.Type(),
		FingerprintSHA256: ssh.FingerprintSHA256(observed), ConnectionMode: host.ConnectionMode,
	}, nil
}

type sshSessionOpenResult struct {
	session *ssh.Session
	err     error
}

func newSSHSessionWithTimeout(ctx context.Context, client *ssh.Client) (*ssh.Session, error) {
	operationCtx, cancel := context.WithTimeout(ctx, sshConnectTimeout)
	defer cancel()
	completed := make(chan sshSessionOpenResult, 1)
	go func() {
		session, err := client.NewSession()
		completed <- sshSessionOpenResult{session: session, err: err}
	}()
	select {
	case result := <-completed:
		return result.session, result.err
	case <-operationCtx.Done():
		_ = client.Close()
		if errors.Is(operationCtx.Err(), context.DeadlineExceeded) {
			return nil, sshError("SSH_SESSION_OPEN_TIMEOUT", "SSH 认证成功但创建会话超时", operationCtx.Err())
		}
		return nil, sshError("SSH_CONNECT_CANCELLED", "SSH 连接已取消", operationCtx.Err())
	}
}

func (svc *SSHHostService) connect(ctx context.Context, host *model.SSHHost) (*ssh.Client, ssh.ConnMetadata, ssh.PublicKey, time.Duration, error) {
	credential, err := svc.store.GetSSHCredential(host.CredentialID)
	if err != nil {
		return nil, nil, nil, 0, sshError("SSH_CREDENTIAL_NOT_FOUND", "SSH 凭据不存在", err)
	}
	auth, err := svc.authMethod(credential)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	address := net.JoinHostPort(host.Host, strconv.Itoa(host.Port))
	conn, err := svc.dial(ctx, host, address)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	defer func() {
		if conn != nil {
			_ = conn.Close()
		}
	}()
	activeConn := conn
	stopCancellation := context.AfterFunc(ctx, func() { _ = activeConn.Close() })
	defer stopCancellation()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(svc.now().Add(sshConnectTimeout))
	}
	trusted, err := svc.store.GetSSHHostKey(host.ID)
	if err != nil {
		return nil, nil, nil, 0, err
	}
	var observed ssh.PublicKey
	config := &ssh.ClientConfig{
		User: host.Username, Auth: []ssh.AuthMethod{auth}, Timeout: sshConnectTimeout,
		HostKeyCallback: func(_ string, _ net.Addr, key ssh.PublicKey) error {
			observed = key
			if trusted == nil {
				return errors.New("host key is not trusted")
			}
			trustedKey, _, _, _, parseErr := ssh.ParseAuthorizedKey([]byte(trusted.PublicKey))
			if parseErr != nil || !sshKeysEqual(trustedKey, key) {
				return errors.New("host key changed")
			}
			return nil
		},
	}
	handshakeStarted := svc.now()
	clientConn, channels, requests, err := ssh.NewClientConn(conn, address, config)
	handshakeDuration := svc.now().Sub(handshakeStarted)
	if err != nil {
		if observed != nil {
			challenge, challengeErr := svc.createHostKeyChallenge(host, address, observed, trusted)
			if challengeErr != nil {
				return nil, nil, nil, 0, challengeErr
			}
			code, message := "SSH_HOST_KEY_UNKNOWN", "SSH Host Key 尚未信任"
			if trusted != nil {
				code, message = "SSH_HOST_KEY_CHANGED", "SSH Host Key 已变化，连接已阻止"
			}
			return nil, nil, nil, 0, &SSHServiceError{Code: code, Message: message, Details: challenge, Cause: err}
		}
		return nil, nil, nil, 0, classifySSHConnectError(ctx, err)
	}
	_ = conn.SetDeadline(time.Time{})
	conn = nil
	return ssh.NewClient(clientConn, channels, requests), clientConn, observed, handshakeDuration, nil
}

func (svc *SSHHostService) authMethod(credential *model.SSHCredential) (ssh.AuthMethod, error) {
	secret, err := svc.cipher.decrypt(credential.SecretContext, "secret", credential.SecretCiphertext, credential.SecretNonce)
	if err != nil {
		return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "无法解密 SSH 凭据", err)
	}
	defer clearBytes(secret)
	if credential.AuthType == "password" {
		return ssh.Password(string(secret)), nil
	}
	var passphrase []byte
	if len(credential.PassphraseCiphertext) > 0 {
		passphrase, err = svc.cipher.decrypt(credential.SecretContext, "passphrase", credential.PassphraseCiphertext, credential.PassphraseNonce)
		if err != nil {
			return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "无法解密 SSH 私钥口令", err)
		}
		defer clearBytes(passphrase)
	}
	signer, err := parseSSHPrivateKey(secret, passphrase)
	if err != nil {
		return nil, sshError("SSH_CREDENTIAL_INVALID", "已保存的 SSH 私钥无效", nil)
	}
	return ssh.PublicKeys(signer), nil
}

func (svc *SSHHostService) createHostKeyChallenge(host *model.SSHHost, address string, key ssh.PublicKey, trusted *model.SSHHostKey) (*model.SSHHostKeyChallenge, error) {
	id, err := randomHex(24)
	if err != nil {
		return nil, sshError("SSH_HOST_KEY_CHALLENGE_FAILED", "无法创建 Host Key 确认", err)
	}
	expiresAt := svc.now().Add(sshChallengeTimeout).UnixMilli()
	challenge := sshHostKeyChallenge{
		SSHHostKeyChallenge: model.SSHHostKeyChallenge{
			ChallengeID: id, KeyType: key.Type(), FingerprintSHA256: ssh.FingerprintSHA256(key), ExpiresAt: expiresAt,
		},
		HostID: host.ID, HostAddress: address, PublicKey: strings.TrimSpace(string(ssh.MarshalAuthorizedKey(key))),
		ConnectionMode: host.ConnectionMode,
	}
	if trusted != nil {
		challenge.TrustedFingerprint = trusted.FingerprintSHA256
	}
	svc.challengeMu.Lock()
	svc.pruneChallengesLocked()
	svc.challenges[id] = challenge
	svc.challengeMu.Unlock()
	logging.Info("ssh_host_key.discover", "发现 SSH Host Key: host_id=%d changed=%t", host.ID, trusted != nil)
	result := challenge.SSHHostKeyChallenge
	return &result, nil
}

func (svc *SSHHostService) pruneChallengesLocked() {
	now := svc.now().UnixMilli()
	for id, challenge := range svc.challenges {
		if challenge.ExpiresAt <= now {
			delete(svc.challenges, id)
		}
	}
}

func (svc *SSHHostService) audit(host *model.SSHHost, eventType, result, errorCode string, duration time.Duration, sessionHash string) {
	if err := svc.store.CreateSSHSessionAudit(&model.SSHSessionAudit{
		SessionIDHash: sessionHash, HostID: host.ID, HostName: host.Name, EventType: eventType,
		ConnectionMode: host.ConnectionMode, NodeExposureID: host.NodeExposureID,
		Result: result, ErrorCode: errorCode, DurationMS: duration.Milliseconds(),
	}); err != nil {
		logging.Error("ssh_audit.create", "写入 SSH 审计失败: host_id=%d", host.ID)
	}
}

func parseSSHPrivateKey(privateKey, passphrase []byte) (ssh.Signer, error) {
	if len(passphrase) > 0 {
		return ssh.ParsePrivateKeyWithPassphrase(privateKey, passphrase)
	}
	return ssh.ParsePrivateKey(privateKey)
}

func sshKeysEqual(first, second ssh.PublicKey) bool {
	return first != nil && second != nil && first.Type() == second.Type() && string(first.Marshal()) == string(second.Marshal())
}

func normalizeSSHTags(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] || utf8.RuneCountInString(value) > 64 {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func randomHex(size int) (string, error) {
	content := make([]byte, size)
	if _, err := rand.Read(content); err != nil {
		return "", err
	}
	return hex.EncodeToString(content), nil
}

func clearBytes(content []byte) {
	for index := range content {
		content[index] = 0
	}
}

func sshError(code, message string, cause error) error {
	return &SSHServiceError{Code: code, Message: message, Cause: cause}
}

func sshErrorInfo(err error) (string, string) {
	var serviceErr *SSHServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code, serviceErr.Message
	}
	return "SSH_CONNECT_FAILED", "SSH 连接失败"
}

func normalizeSSHStoreError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, store.ErrSSHCredentialNotFound):
		return sshError("SSH_CREDENTIAL_NOT_FOUND", "SSH 凭据不存在", err)
	case errors.Is(err, store.ErrSSHHostNotFound):
		return sshError("SSH_HOST_NOT_FOUND", "SSH 主机不存在", err)
	case errors.Is(err, store.ErrSSHReferenceInUse):
		return sshError("SSH_RESOURCE_IN_USE", "资源仍被 SSH 主机引用", err)
	case strings.Contains(strings.ToLower(err.Error()), "unique constraint"):
		return sshError("SSH_NAME_CONFLICT", "名称已存在", err)
	case strings.Contains(strings.ToLower(err.Error()), "foreign key constraint"):
		return sshError("SSH_REFERENCE_INVALID", "SSH 配置引用无效", err)
	default:
		return err
	}
}

func classifySSHConnectError(ctx context.Context, err error) error {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return sshError("SSH_CONNECT_TIMEOUT", "SSH 连接超时", err)
	}
	if errors.Is(ctx.Err(), context.Canceled) {
		return sshError("SSH_CONNECT_CANCELLED", "SSH 连接已取消", err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return sshError("SSH_CONNECT_TIMEOUT", "SSH 连接超时", err)
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "unable to authenticate") || strings.Contains(message, "no supported methods remain") {
		return sshError("SSH_AUTH_FAILED", "SSH 用户认证失败", err)
	}
	return sshError("SSH_HANDSHAKE_FAILED", "SSH 握手失败", err)
}
