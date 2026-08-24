package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/scrypt"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshHostShareFormat      = "ackwrap-ssh-host"
	sshHostShareCodePrefix  = "ackwrap-ssh-v1."
	sshHostShareVersion     = 1
	sshHostShareSaltSize    = 16
	sshHostShareScryptN     = 32768
	sshHostShareScryptR     = 8
	sshHostShareScryptP     = 1
	sshHostShareKeySize     = 32
	sshHostShareMinPassword = 8
	sshHostShareMaxPassword = 256
	sshHostNameLimit        = 128
)

var sshHostShareAAD = []byte("ackwrap-ssh-host-share:v1")

type sshHostShareEnvelope struct {
	Format     string `json:"format"`
	Version    int    `json:"version"`
	KDF        string `json:"kdf"`
	ScryptN    int    `json:"scrypt_n"`
	ScryptR    int    `json:"scrypt_r"`
	ScryptP    int    `json:"scrypt_p"`
	Salt       []byte `json:"salt"`
	AEAD       string `json:"aead"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

type sshHostSharePayload struct {
	Format     string                 `json:"format"`
	Version    int                    `json:"version"`
	Host       sshHostShareHost       `json:"host"`
	Credential sshHostShareCredential `json:"credential"`
}

type sshHostShareHost struct {
	Name           string   `json:"name"`
	GroupName      string   `json:"group_name"`
	Host           string   `json:"host"`
	Port           int      `json:"port"`
	Username       string   `json:"username"`
	ConnectionMode string   `json:"connection_mode"`
	TerminalType   string   `json:"terminal_type"`
	Enabled        bool     `json:"enabled"`
	Tags           []string `json:"tags"`
	Notes          string   `json:"notes"`
}

type sshHostShareCredential struct {
	Name       string `json:"name"`
	AuthType   string `json:"auth_type"`
	Secret     []byte `json:"secret"`
	Passphrase []byte `json:"passphrase,omitempty"`
}

func (svc *SSHHostService) ShareHost(hostID int64, password string) (*model.SSHHostShareResponse, error) {
	if err := validateSSHHostSharePassword(password); err != nil {
		return nil, err
	}
	svc.shareMu.Lock()
	defer svc.shareMu.Unlock()

	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	credential, err := svc.store.GetSSHCredential(host.CredentialID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	secret, err := svc.cipher.decrypt(credential.SecretContext, "secret", credential.SecretCiphertext, credential.SecretNonce)
	if err != nil {
		return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "解密 SSH 凭据失败", err)
	}
	defer clearBytes(secret)
	var passphrase []byte
	if len(credential.PassphraseCiphertext) > 0 {
		passphrase, err = svc.cipher.decrypt(credential.SecretContext, "passphrase", credential.PassphraseCiphertext, credential.PassphraseNonce)
		if err != nil {
			return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "解密 SSH 私钥口令失败", err)
		}
		defer clearBytes(passphrase)
	}
	payload := sshHostSharePayload{
		Format: sshHostShareFormat, Version: sshHostShareVersion,
		Host: sshHostShareHost{
			Name: host.Name, GroupName: host.GroupName, Host: host.Host, Port: host.Port,
			Username: host.Username, ConnectionMode: host.ConnectionMode, TerminalType: host.TerminalType,
			Enabled: host.Enabled, Tags: append([]string(nil), host.Tags...), Notes: host.Notes,
		},
		Credential: sshHostShareCredential{
			Name: credential.Name, AuthType: credential.AuthType,
			Secret: secret, Passphrase: passphrase,
		},
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "生成 SSH 主机分享内容失败", err)
	}
	defer clearBytes(plaintext)
	code, err := encryptSSHHostShare(plaintext, password)
	if err != nil {
		return nil, err
	}
	logging.Info("ssh_host.share", "生成 SSH 主机加密分享码: host_id=%d", hostID)
	return &model.SSHHostShareResponse{Code: code, HostName: host.Name}, nil
}

func (svc *SSHHostService) ImportHost(request model.SSHHostImportRequest) (*model.SSHHostImportResponse, error) {
	if err := validateSSHHostSharePassword(request.Password); err != nil {
		return nil, err
	}
	svc.shareMu.Lock()
	defer svc.shareMu.Unlock()

	plaintext, err := decryptSSHHostShare(request.Code, request.Password)
	if err != nil {
		return nil, err
	}
	defer clearBytes(plaintext)
	var payload sshHostSharePayload
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码内容无效", nil)
	}
	defer clearBytes(payload.Credential.Secret)
	defer clearBytes(payload.Credential.Passphrase)
	if payload.Format != sshHostShareFormat || payload.Version != sshHostShareVersion {
		return nil, sshError("SSH_SHARE_INVALID", "不支持的 SSH 主机分享码格式或版本", nil)
	}
	if payload.Host.ConnectionMode != "direct" && payload.Host.ConnectionMode != "node_exposure" {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码连接模式无效", nil)
	}

	hosts, err := svc.store.ListSSHHosts()
	if err != nil {
		return nil, err
	}
	credentials, err := svc.store.ListSSHCredentials()
	if err != nil {
		return nil, err
	}
	hostNames := make(map[string]bool, len(hosts))
	for _, item := range hosts {
		hostNames[strings.ToLower(item.Name)] = true
	}
	credentialNames := make(map[string]bool, len(credentials))
	for _, item := range credentials {
		credentialNames[strings.ToLower(item.Name)] = true
	}
	payload.Host.Name = uniqueSSHImportedName(payload.Host.Name, hostNames)
	payload.Credential.Name = uniqueSSHImportedName(payload.Credential.Name, credentialNames)

	credential, err := svc.buildCredential(model.SSHCredentialRequest{
		Name: payload.Credential.Name, AuthType: payload.Credential.AuthType,
		Secret: string(payload.Credential.Secret), Passphrase: string(payload.Credential.Passphrase),
	}, nil)
	if err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码中的凭据无效", err)
	}
	host, err := normalizeSSHHostFields(model.SSHHostRequest{
		Name: payload.Host.Name, GroupName: payload.Host.GroupName, Host: payload.Host.Host,
		Port: payload.Host.Port, Username: payload.Host.Username, ConnectionMode: "direct",
		TerminalType: payload.Host.TerminalType, Enabled: payload.Host.Enabled,
		Tags: payload.Host.Tags, Notes: payload.Host.Notes,
	})
	if err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码中的主机配置无效", err)
	}
	if err := svc.store.CreateSSHHostWithCredential(credential, host); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	created, err := svc.store.GetSSHHost(host.ID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	converted := payload.Host.ConnectionMode != "direct"
	logging.Info("ssh_host.import", "导入 SSH 主机加密分享码: host_id=%d credential_id=%d converted_to_direct=%t", created.ID, created.CredentialID, converted)
	return &model.SSHHostImportResponse{
		Success: true, Message: "SSH 主机已导入", Host: *created, ConvertedToDirect: converted,
	}, nil
}

func encryptSSHHostShare(plaintext []byte, password string) (string, error) {
	salt := make([]byte, sshHostShareSaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", sshError("SSH_SHARE_ENCRYPT_FAILED", "生成 SSH 主机分享盐失败", err)
	}
	key, err := scrypt.Key([]byte(password), salt, sshHostShareScryptN, sshHostShareScryptR, sshHostShareScryptP, sshHostShareKeySize)
	if err != nil {
		return "", sshError("SSH_SHARE_ENCRYPT_FAILED", "派生 SSH 主机分享密钥失败", err)
	}
	defer clearBytes(key)
	aead, err := newSSHHostShareAEAD(key)
	if err != nil {
		return "", sshError("SSH_SHARE_ENCRYPT_FAILED", "初始化 SSH 主机分享加密失败", err)
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", sshError("SSH_SHARE_ENCRYPT_FAILED", "生成 SSH 主机分享随机数失败", err)
	}
	envelope := sshHostShareEnvelope{
		Format: sshHostShareFormat, Version: sshHostShareVersion, KDF: "scrypt",
		ScryptN: sshHostShareScryptN, ScryptR: sshHostShareScryptR, ScryptP: sshHostShareScryptP,
		Salt: salt, AEAD: "aes-256-gcm", Nonce: nonce,
		Ciphertext: aead.Seal(nil, nonce, plaintext, sshHostShareAAD),
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", sshError("SSH_SHARE_ENCRYPT_FAILED", "编码 SSH 主机分享码失败", err)
	}
	defer clearBytes(encoded)
	code := sshHostShareCodePrefix + base64.RawURLEncoding.EncodeToString(encoded)
	if len(code) > model.MaxSSHHostShareCodeSize {
		return "", sshError("SSH_SHARE_TOO_LARGE", "SSH 主机分享码超过大小限制", nil)
	}
	return code, nil
}

func decryptSSHHostShare(code, password string) ([]byte, error) {
	if len(code) > model.MaxSSHHostShareCodeSize {
		return nil, sshError("SSH_SHARE_TOO_LARGE", "SSH 主机分享码超过大小限制", nil)
	}
	code = strings.TrimSpace(code)
	if code == "" || !strings.HasPrefix(code, sshHostShareCodePrefix) {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码无效", nil)
	}
	encoded, err := base64.RawURLEncoding.Strict().DecodeString(strings.TrimPrefix(code, sshHostShareCodePrefix))
	if err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码编码无效", nil)
	}
	var envelope sshHostShareEnvelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码结构无效", nil)
	}
	if envelope.Format != sshHostShareFormat || envelope.Version != sshHostShareVersion ||
		envelope.KDF != "scrypt" || envelope.ScryptN != sshHostShareScryptN ||
		envelope.ScryptR != sshHostShareScryptR || envelope.ScryptP != sshHostShareScryptP ||
		envelope.AEAD != "aes-256-gcm" || len(envelope.Salt) != sshHostShareSaltSize {
		return nil, sshError("SSH_SHARE_INVALID", "不支持的 SSH 主机分享码加密参数", nil)
	}
	key, err := scrypt.Key([]byte(password), envelope.Salt, envelope.ScryptN, envelope.ScryptR, envelope.ScryptP, sshHostShareKeySize)
	if err != nil {
		return nil, sshError("SSH_SHARE_DECRYPT_FAILED", "SSH 主机分享码解密失败，请检查密码", nil)
	}
	defer clearBytes(key)
	aead, err := newSSHHostShareAEAD(key)
	if err != nil || len(envelope.Nonce) != aead.NonceSize() || len(envelope.Ciphertext) < aead.Overhead() {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码加密数据无效", nil)
	}
	plaintext, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, sshHostShareAAD)
	if err != nil {
		return nil, sshError("SSH_SHARE_DECRYPT_FAILED", "SSH 主机分享码解密失败，请检查密码", nil)
	}
	return plaintext, nil
}

func newSSHHostShareAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func validateSSHHostSharePassword(password string) error {
	if utf8.RuneCountInString(password) < sshHostShareMinPassword || len(password) > sshHostShareMaxPassword {
		return sshError("SSH_SHARE_PASSWORD_INVALID", fmt.Sprintf("分享密码至少需要 %d 个字符且不能超过 %d 字节", sshHostShareMinPassword, sshHostShareMaxPassword), nil)
	}
	return nil
}

func uniqueSSHImportedName(name string, used map[string]bool) string {
	name = strings.TrimSpace(name)
	if !used[strings.ToLower(name)] {
		return name
	}
	for index := 1; ; index++ {
		suffix := " (导入)"
		if index > 1 {
			suffix = fmt.Sprintf(" (导入 %d)", index)
		}
		base := []rune(name)
		limit := sshHostNameLimit - utf8.RuneCountInString(suffix)
		if len(base) > limit {
			base = base[:limit]
		}
		candidate := string(base) + suffix
		if !used[strings.ToLower(candidate)] {
			return candidate
		}
	}
}
