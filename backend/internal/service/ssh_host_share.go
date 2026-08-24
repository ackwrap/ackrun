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
	sshHostShareFormat        = "ackwrap-ssh-host"
	sshHostShareBundleFormat  = "ackwrap-ssh-host-bundle"
	sshHostShareBundleVersion = 1
	sshHostShareCodePrefix    = "ackwrap-ssh-v1."
	sshHostShareVersion       = 1
	sshHostShareSaltSize      = 16
	sshHostShareScryptN       = 32768
	sshHostShareScryptR       = 8
	sshHostShareScryptP       = 1
	sshHostShareKeySize       = 32
	sshHostShareMinPassword   = 8
	sshHostShareMaxPassword   = 256
	sshHostNameLimit          = 128
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

type sshHostShareBundlePayload struct {
	Format      string                         `json:"format"`
	Version     int                            `json:"version"`
	Credentials []sshHostShareBundleCredential `json:"credentials"`
	Hosts       []sshHostShareBundleHost       `json:"hosts"`
}

type sshHostShareBundleCredential struct {
	Ref        string                 `json:"ref"`
	Credential sshHostShareCredential `json:"credential"`
}

type sshHostShareBundleHost struct {
	CredentialRef string           `json:"credential_ref"`
	Host          sshHostShareHost `json:"host"`
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
	return &model.SSHHostShareResponse{Code: code, HostName: host.Name, HostCount: 1}, nil
}

func (svc *SSHHostService) ShareHosts(hostIDs []int64, password string) (*model.SSHHostShareResponse, error) {
	if err := validateSSHHostSharePassword(password); err != nil {
		return nil, err
	}
	hostIDs, err := normalizeSSHHostShareIDs(hostIDs)
	if err != nil {
		return nil, err
	}
	svc.shareMu.Lock()
	defer svc.shareMu.Unlock()

	payload := sshHostShareBundlePayload{
		Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion,
		Credentials: make([]sshHostShareBundleCredential, 0),
		Hosts:       make([]sshHostShareBundleHost, 0, len(hostIDs)),
	}
	defer clearSSHHostShareBundle(&payload)
	credentialRefs := make(map[int64]string)
	for _, hostID := range hostIDs {
		host, err := svc.store.GetSSHHost(hostID)
		if err != nil {
			return nil, normalizeSSHStoreError(err)
		}
		credentialRef, exists := credentialRefs[host.CredentialID]
		if !exists {
			credential, err := svc.store.GetSSHCredential(host.CredentialID)
			if err != nil {
				return nil, normalizeSSHStoreError(err)
			}
			secret, err := svc.cipher.decrypt(credential.SecretContext, "secret", credential.SecretCiphertext, credential.SecretNonce)
			if err != nil {
				return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "解密 SSH 凭据失败", err)
			}
			var passphrase []byte
			if len(credential.PassphraseCiphertext) > 0 {
				passphrase, err = svc.cipher.decrypt(credential.SecretContext, "passphrase", credential.PassphraseCiphertext, credential.PassphraseNonce)
				if err != nil {
					clearBytes(secret)
					return nil, sshError("SSH_SECRET_DECRYPT_FAILED", "解密 SSH 私钥口令失败", err)
				}
			}
			credentialRef = fmt.Sprintf("credential-%d", len(payload.Credentials)+1)
			credentialRefs[host.CredentialID] = credentialRef
			payload.Credentials = append(payload.Credentials, sshHostShareBundleCredential{
				Ref: credentialRef,
				Credential: sshHostShareCredential{
					Name: credential.Name, AuthType: credential.AuthType, Secret: secret, Passphrase: passphrase,
				},
			})
		}
		payload.Hosts = append(payload.Hosts, sshHostShareBundleHost{
			CredentialRef: credentialRef,
			Host: sshHostShareHost{
				Name: host.Name, GroupName: host.GroupName, Host: host.Host, Port: host.Port,
				Username: host.Username, ConnectionMode: host.ConnectionMode, TerminalType: host.TerminalType,
				Enabled: host.Enabled, Tags: append([]string(nil), host.Tags...), Notes: host.Notes,
			},
		})
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "生成 SSH 主机批量分享内容失败", err)
	}
	defer clearBytes(plaintext)
	code, err := encryptSSHHostShare(plaintext, password)
	if err != nil {
		return nil, err
	}
	logging.Info("ssh_host.share", "生成 SSH 主机批量加密分享码: host_count=%d credential_count=%d", len(payload.Hosts), len(payload.Credentials))
	return &model.SSHHostShareResponse{Code: code, HostCount: len(payload.Hosts)}, nil
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
	payload, err := decodeSSHHostSharePayload(plaintext)
	if err != nil {
		return nil, err
	}
	defer clearSSHHostShareBundle(payload)

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

	builtCredentials := make([]*model.SSHCredential, 0, len(payload.Credentials))
	credentialIndexes := make(map[string]int, len(payload.Credentials))
	for index := range payload.Credentials {
		item := &payload.Credentials[index]
		item.Ref = strings.TrimSpace(item.Ref)
		if item.Ref == "" || utf8.RuneCountInString(item.Ref) > 64 || strings.ContainsAny(item.Ref, "\x00\r\n") {
			return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码包含无效凭据引用", nil)
		}
		if _, exists := credentialIndexes[item.Ref]; exists {
			return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码包含重复凭据引用", nil)
		}
		item.Credential.Name = uniqueSSHImportedName(item.Credential.Name, credentialNames)
		credentialNames[strings.ToLower(item.Credential.Name)] = true
		credential, err := svc.buildCredential(model.SSHCredentialRequest{
			Name: item.Credential.Name, AuthType: item.Credential.AuthType,
			Secret: string(item.Credential.Secret), Passphrase: string(item.Credential.Passphrase),
		}, nil)
		if err != nil {
			return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("SSH 主机分享码中的第 %d 个凭据无效", index+1), err)
		}
		credentialIndexes[item.Ref] = len(builtCredentials)
		builtCredentials = append(builtCredentials, credential)
	}

	builtHosts := make([]*model.SSHHost, 0, len(payload.Hosts))
	hostCredentialIndexes := make([]int, 0, len(payload.Hosts))
	usedCredentialRefs := make(map[string]bool, len(payload.Credentials))
	convertedCount := 0
	for index := range payload.Hosts {
		item := &payload.Hosts[index]
		credentialIndex, exists := credentialIndexes[strings.TrimSpace(item.CredentialRef)]
		if !exists {
			return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("SSH 主机分享码中的第 %d 台主机引用了不存在的凭据", index+1), nil)
		}
		usedCredentialRefs[strings.TrimSpace(item.CredentialRef)] = true
		if item.Host.ConnectionMode != "direct" && item.Host.ConnectionMode != "node_exposure" {
			return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("SSH 主机分享码中的第 %d 台主机连接模式无效", index+1), nil)
		}
		if item.Host.ConnectionMode != "direct" {
			convertedCount++
		}
		item.Host.Name = uniqueSSHImportedName(item.Host.Name, hostNames)
		hostNames[strings.ToLower(item.Host.Name)] = true
		host, err := normalizeSSHHostFields(model.SSHHostRequest{
			Name: item.Host.Name, GroupName: item.Host.GroupName, Host: item.Host.Host,
			Port: item.Host.Port, Username: item.Host.Username, ConnectionMode: "direct",
			TerminalType: item.Host.TerminalType, Enabled: item.Host.Enabled,
			Tags: item.Host.Tags, Notes: item.Host.Notes,
		})
		if err != nil {
			return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("SSH 主机分享码中的第 %d 台主机配置无效", index+1), err)
		}
		builtHosts = append(builtHosts, host)
		hostCredentialIndexes = append(hostCredentialIndexes, credentialIndex)
	}
	if len(usedCredentialRefs) != len(builtCredentials) {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码包含未使用的凭据", nil)
	}
	if err := svc.store.CreateSSHHostsWithCredentials(builtCredentials, builtHosts, hostCredentialIndexes); err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	created := make([]model.SSHHost, 0, len(builtHosts))
	for _, host := range builtHosts {
		item, err := svc.store.GetSSHHost(host.ID)
		if err != nil {
			return nil, normalizeSSHStoreError(err)
		}
		created = append(created, *item)
	}
	logging.Info("ssh_host.import", "导入 SSH 主机加密分享码: host_count=%d credential_count=%d converted_to_direct=%d", len(created), len(builtCredentials), convertedCount)
	return &model.SSHHostImportResponse{
		Success: true, Message: fmt.Sprintf("已导入 %d 台 SSH 主机", len(created)),
		Host: created[0], Hosts: created, HostCount: len(created), CredentialCount: len(builtCredentials),
		ConvertedToDirect: convertedCount > 0, ConvertedToDirectCount: convertedCount,
	}, nil
}

func decodeSSHHostSharePayload(plaintext []byte) (*sshHostShareBundlePayload, error) {
	var metadata struct {
		Format  string `json:"format"`
		Version int    `json:"version"`
	}
	if err := json.Unmarshal(plaintext, &metadata); err != nil {
		return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码内容无效", nil)
	}
	switch metadata.Format {
	case sshHostShareFormat:
		var legacy sshHostSharePayload
		if metadata.Version != sshHostShareVersion {
			return nil, sshError("SSH_SHARE_INVALID", "不支持的 SSH 主机分享码格式或版本", nil)
		}
		if err := json.Unmarshal(plaintext, &legacy); err != nil {
			clearBytes(legacy.Credential.Secret)
			clearBytes(legacy.Credential.Passphrase)
			return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享码内容无效", nil)
		}
		return &sshHostShareBundlePayload{
			Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion,
			Credentials: []sshHostShareBundleCredential{{Ref: "credential-1", Credential: legacy.Credential}},
			Hosts:       []sshHostShareBundleHost{{CredentialRef: "credential-1", Host: legacy.Host}},
		}, nil
	case sshHostShareBundleFormat:
		var payload sshHostShareBundlePayload
		if metadata.Version != sshHostShareBundleVersion {
			return nil, sshError("SSH_SHARE_INVALID", "不支持的 SSH 主机批量分享码格式或版本", nil)
		}
		if err := json.Unmarshal(plaintext, &payload); err != nil {
			clearSSHHostShareBundle(&payload)
			return nil, sshError("SSH_SHARE_INVALID", "SSH 主机批量分享码内容无效", nil)
		}
		if len(payload.Hosts) == 0 || len(payload.Hosts) > model.MaxSSHHostShareHosts ||
			len(payload.Credentials) == 0 || len(payload.Credentials) > len(payload.Hosts) {
			clearSSHHostShareBundle(&payload)
			return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("SSH 主机批量分享码必须包含 1 到 %d 台主机", model.MaxSSHHostShareHosts), nil)
		}
		return &payload, nil
	default:
		return nil, sshError("SSH_SHARE_INVALID", "不支持的 SSH 主机分享码格式或版本", nil)
	}
}

func normalizeSSHHostShareIDs(values []int64) ([]int64, error) {
	if len(values) == 0 || len(values) > model.MaxSSHHostShareHosts {
		return nil, sshError("SSH_SHARE_INVALID", fmt.Sprintf("请选择 1 到 %d 台 SSH 主机", model.MaxSSHHostShareHosts), nil)
	}
	result := make([]int64, 0, len(values))
	seen := make(map[int64]bool, len(values))
	for _, id := range values {
		if id <= 0 {
			return nil, sshError("SSH_SHARE_INVALID", "SSH 主机分享列表包含无效 ID", nil)
		}
		if !seen[id] {
			seen[id] = true
			result = append(result, id)
		}
	}
	return result, nil
}

func clearSSHHostShareBundle(payload *sshHostShareBundlePayload) {
	if payload == nil {
		return
	}
	for index := range payload.Credentials {
		clearBytes(payload.Credentials[index].Credential.Secret)
		clearBytes(payload.Credentials[index].Credential.Passphrase)
	}
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
