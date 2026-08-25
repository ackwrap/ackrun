package service

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	sshSingboxDeployTimeout        = 10 * time.Minute
	sshSingboxRollbackGrace        = 15 * time.Second
	sshSingboxMaximumOutput        = 8 << 10
	sshSingboxConfigPath           = "/etc/sing-box/config.json"
	sshSingboxManagedMarkerPath    = "/etc/sing-box/.ackwrap-managed"
	sshSingboxRealityDefaultPort   = 443
	sshSingboxShadowsocksPort      = 8388
	sshSingboxDefaultRealityServer = "www.microsoft.com"
)

const sshSingboxPrivilegeCommand = `if test "$(id -u)" = "0"; then
  printf 'ACKWRAP_PRIVILEGE\troot\n'
elif command -v sudo >/dev/null 2>&1 && sudo -n sh -c 'exit 0' >/dev/null 2>&1; then
  printf 'ACKWRAP_PRIVILEGE\tsudo\n'
else
  printf 'ACKWRAP_PRIVILEGE\tunsupported\n'
fi`

type sshSingboxDeployment struct {
	serverConfig []byte
	clientConfig map[string]interface{}
	nodes        []model.SSHSingboxNode
	protocols    []string
}

type sshSingboxCommandOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (output *sshSingboxCommandOutput) Write(content []byte) (int, error) {
	written := len(content)
	remaining := sshSingboxMaximumOutput - output.buffer.Len()
	if remaining <= 0 {
		output.overflow = true
		return written, nil
	}
	if len(content) > remaining {
		content = content[:remaining]
		output.overflow = true
	}
	_, _ = output.buffer.Write(content)
	return written, nil
}

func (svc *SSHHostService) DeploySingbox(ctx context.Context, hostID int64, request model.SSHSingboxDeployRequest) (*model.SSHSingboxDeployResponse, error) {
	started := svc.now()
	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	deployCtx, cancel := context.WithTimeout(ctx, sshSingboxDeployTimeout)
	if !svc.beginSingboxDeploy(hostID, cancel) {
		cancel()
		return nil, sshError("SSH_SINGBOX_DEPLOY_BUSY", "该主机正在执行 sing-box 部署", nil)
	}
	defer svc.endSingboxDeploy(hostID)
	defer cancel()

	deployment, err := buildSSHSingboxDeployment(host, request)
	if err != nil {
		return nil, err
	}
	logging.Info("ssh_singbox.deploy", "开始部署远端 sing-box: host_id=%d protocols=%s", hostID, strings.Join(deployment.protocols, ","))
	svc.broadcastSingboxDeploy(hostID, "deploying", "正在连接远端主机", "")

	client, _, _, _, err := svc.connect(deployCtx, host)
	if err != nil {
		svc.finishSingboxDeployAudit(host, started, err)
		return nil, err
	}
	defer client.Close()

	privilegeOutput, privilegeErr := runSSHSingboxCommand(deployCtx, client, sshSingboxPrivilegeCommand, nil)
	privilege := parseSSHSingboxPrivilege(privilegeOutput)
	if errors.Is(privilegeErr, context.DeadlineExceeded) {
		err = sshError("SSH_SINGBOX_DEPLOY_TIMEOUT", "远端 sing-box 部署超时", privilegeErr)
	} else if privilegeErr != nil || privilege == "" {
		err = sshError("SSH_SINGBOX_PREFLIGHT_FAILED", "无法检查远端安装权限", privilegeErr)
	} else if privilege == "unsupported" {
		err = sshError("SSH_SINGBOX_ROOT_REQUIRED", "一键部署需要 root 用户或免密 sudo 权限", nil)
	}
	if err != nil {
		svc.finishSingboxDeployAudit(host, started, err)
		return nil, err
	}

	clientConfig, err := json.MarshalIndent(deployment.clientConfig, "", "  ")
	if err != nil {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成客户端配置失败", err)
	}
	script := buildSSHSingboxDeployScript(deployment.serverConfig, clientConfig, request.ReplaceExistingConfig)
	command := "sh -s"
	if privilege == "sudo" {
		command = "sudo -n sh -s"
	}
	svc.broadcastSingboxDeploy(hostID, "installing", "正在安装并校验 sing-box", "")
	output, runErr := runSSHSingboxCommand(deployCtx, client, command, strings.NewReader(script))
	version, backupPath, resultErr := parseSSHSingboxDeployOutput(output, runErr)
	if resultErr != nil {
		svc.finishSingboxDeployAudit(host, started, resultErr)
		return nil, resultErr
	}

	_ = svc.store.TouchSSHHostKey(hostID)
	svc.audit(host, "singbox_deploy", "success", "", svc.now().Sub(started), "")
	logging.Info("ssh_singbox.deploy", "远端 sing-box 部署完成: host_id=%d", hostID)
	svc.broadcastSingboxDeploy(hostID, "success", "sing-box 部署完成", "")
	return &model.SSHSingboxDeployResponse{
		Success: true, Message: "sing-box 已安装、校验并启动", Version: version,
		ConfigPath: sshSingboxConfigPath, BackupPath: backupPath,
		Nodes: deployment.nodes, ClientConfig: deployment.clientConfig,
	}, nil
}

func (svc *SSHHostService) beginSingboxDeploy(hostID int64, cancel context.CancelFunc) bool {
	svc.sessionMu.Lock()
	defer svc.sessionMu.Unlock()
	if svc.closed {
		return false
	}
	svc.singboxDeployMu.Lock()
	defer svc.singboxDeployMu.Unlock()
	if svc.singboxDeploying[hostID] {
		return false
	}
	svc.singboxDeploying[hostID] = true
	svc.singboxDeployCancels[hostID] = cancel
	return true
}

func (svc *SSHHostService) endSingboxDeploy(hostID int64) {
	svc.singboxDeployMu.Lock()
	delete(svc.singboxDeploying, hostID)
	delete(svc.singboxDeployCancels, hostID)
	svc.singboxDeployMu.Unlock()
}

func (svc *SSHHostService) finishSingboxDeployAudit(host *model.SSHHost, started time.Time, err error) {
	code, _ := sshErrorInfo(err)
	svc.audit(host, "singbox_deploy", "error", code, svc.now().Sub(started), "")
	logging.Error("ssh_singbox.deploy", "远端 sing-box 部署失败: host_id=%d code=%s", host.ID, code)
	svc.broadcastSingboxDeploy(host.ID, "error", "sing-box 部署失败", code)
}

func (svc *SSHHostService) broadcastSingboxDeploy(hostID int64, status, message, errorCode string) {
	svc.sessionMu.Lock()
	realtime := svc.realtime
	svc.sessionMu.Unlock()
	if realtime == nil {
		return
	}
	payload := map[string]interface{}{"host_id": hostID, "status": status, "message": message}
	if errorCode != "" {
		payload["error_code"] = errorCode
	}
	realtime.Broadcast("ssh.singbox.deploy", payload)
}

func buildSSHSingboxDeployment(host *model.SSHHost, request model.SSHSingboxDeployRequest) (*sshSingboxDeployment, error) {
	serverAddress := strings.TrimSpace(request.ServerAddress)
	if !validSSHDeploymentAddress(serverAddress) {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "服务器地址必须是有效的 IP 或域名，且不能包含协议和端口", nil)
	}
	if !request.VLESSRealityEnabled && !request.ShadowsocksEnabled {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "至少启用一种 sing-box 入站协议", nil)
	}
	if request.VLESSRealityPort == 0 {
		request.VLESSRealityPort = sshSingboxRealityDefaultPort
	}
	if request.ShadowsocksPort == 0 {
		request.ShadowsocksPort = sshSingboxShadowsocksPort
	}
	if request.RealityServerName == "" {
		request.RealityServerName = sshSingboxDefaultRealityServer
	}
	request.RealityServerName = strings.TrimSpace(request.RealityServerName)
	if request.VLESSRealityEnabled && !validSSHDeploymentDomain(request.RealityServerName) {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "Reality 伪装域名必须是有效域名", nil)
	}
	if request.VLESSRealityEnabled && !validSSHDeploymentPort(request.VLESSRealityPort, host.Port) {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "VLESS + REALITY 端口无效或与 SSH 端口冲突", nil)
	}
	if request.ShadowsocksEnabled && !validSSHDeploymentPort(request.ShadowsocksPort, host.Port) {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "Shadowsocks 端口无效或与 SSH 端口冲突", nil)
	}
	if request.VLESSRealityEnabled && request.ShadowsocksEnabled && request.VLESSRealityPort == request.ShadowsocksPort {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "不同协议不能监听同一个端口", nil)
	}

	inbounds := make([]interface{}, 0, 2)
	clientOutbounds := make([]interface{}, 0, 2)
	nodes := make([]model.SSHSingboxNode, 0, 2)
	protocols := make([]string, 0, 2)
	if request.VLESSRealityEnabled {
		privateKey, publicKey, keyErr := generateSSHX25519KeyPair()
		if keyErr != nil {
			return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成 Reality 密钥失败", keyErr)
		}
		uuid, uuidErr := generateSSHUUID()
		if uuidErr != nil {
			return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成 VLESS UUID 失败", uuidErr)
		}
		shortID, shortIDErr := randomHex(8)
		if shortIDErr != nil {
			return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成 Reality Short ID 失败", shortIDErr)
		}
		inbounds = append(inbounds, map[string]interface{}{
			"type": "vless", "tag": "vless-reality-in", "listen": "::", "listen_port": request.VLESSRealityPort,
			"users": []interface{}{map[string]interface{}{"name": "ackwrap", "uuid": uuid, "flow": "xtls-rprx-vision"}},
			"tls": map[string]interface{}{
				"enabled": true,
				"reality": map[string]interface{}{
					"enabled":     true,
					"handshake":   map[string]interface{}{"server": request.RealityServerName, "server_port": 443},
					"private_key": privateKey, "short_id": []string{shortID},
				},
			},
		})
		outbound := map[string]interface{}{
			"type": "vless", "tag": "vless-reality", "server": serverAddress, "server_port": request.VLESSRealityPort,
			"uuid": uuid, "flow": "xtls-rprx-vision",
			"tls": map[string]interface{}{
				"enabled": true, "server_name": request.RealityServerName,
				"utls":    map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
				"reality": map[string]interface{}{"enabled": true, "public_key": publicKey, "short_id": shortID},
			},
		}
		query := url.Values{
			"encryption": {"none"}, "flow": {"xtls-rprx-vision"}, "security": {"reality"},
			"sni": {request.RealityServerName}, "fp": {"chrome"}, "pbk": {publicKey}, "sid": {shortID}, "type": {"tcp"},
		}
		shareURL := &url.URL{Scheme: "vless", User: url.User(uuid), Host: net.JoinHostPort(serverAddress, strconv.Itoa(request.VLESSRealityPort)), RawQuery: query.Encode(), Fragment: host.Name + " VLESS Reality"}
		clientOutbounds = append(clientOutbounds, outbound)
		nodes = append(nodes, model.SSHSingboxNode{Type: "vless-reality", Name: host.Name + " VLESS Reality", ListenPort: request.VLESSRealityPort, Network: "TCP", ShareURI: shareURL.String(), ClientOutbound: outbound})
		protocols = append(protocols, "vless-reality")
	}
	if request.ShadowsocksEnabled {
		passwordBytes := make([]byte, 16)
		if _, err := rand.Read(passwordBytes); err != nil {
			return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成 Shadowsocks 密码失败", err)
		}
		password := base64.StdEncoding.EncodeToString(passwordBytes)
		clearBytes(passwordBytes)
		const method = "2022-blake3-aes-128-gcm"
		inbounds = append(inbounds, map[string]interface{}{
			"type": "shadowsocks", "tag": "shadowsocks-in", "listen": "::", "listen_port": request.ShadowsocksPort,
			"method": method, "password": password,
		})
		outbound := map[string]interface{}{
			"type": "shadowsocks", "tag": "shadowsocks-2022", "server": serverAddress,
			"server_port": request.ShadowsocksPort, "method": method, "password": password,
		}
		userInfo := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
		shareURL := &url.URL{Scheme: "ss", User: url.User(userInfo), Host: net.JoinHostPort(serverAddress, strconv.Itoa(request.ShadowsocksPort)), Fragment: host.Name + " Shadowsocks 2022"}
		clientOutbounds = append(clientOutbounds, outbound)
		nodes = append(nodes, model.SSHSingboxNode{Type: "shadowsocks-2022", Name: host.Name + " Shadowsocks 2022", ListenPort: request.ShadowsocksPort, Network: "TCP + UDP", ShareURI: shareURL.String(), ClientOutbound: outbound})
		protocols = append(protocols, "shadowsocks-2022")
	}

	serverConfig := map[string]interface{}{
		"log":       map[string]interface{}{"level": "info", "timestamp": true},
		"inbounds":  inbounds,
		"outbounds": []interface{}{map[string]interface{}{"type": "direct", "tag": "direct"}},
		"route":     map[string]interface{}{"final": "direct"},
	}
	serverJSON, err := json.MarshalIndent(serverConfig, "", "  ")
	if err != nil {
		return nil, sshError("SSH_SINGBOX_CONFIG_INVALID", "生成服务端配置失败", err)
	}
	clientTags := make([]string, 0, len(nodes))
	for _, node := range nodes {
		clientTags = append(clientTags, node.ClientOutbound["tag"].(string))
	}
	finalOutbound := clientTags[0]
	if len(clientTags) > 1 {
		clientOutbounds = append(clientOutbounds, map[string]interface{}{
			"type": "selector", "tag": "proxy", "outbounds": clientTags, "default": clientTags[0],
		})
		finalOutbound = "proxy"
	}
	clientConfig := map[string]interface{}{
		"log":       map[string]interface{}{"level": "info", "timestamp": true},
		"inbounds":  []interface{}{map[string]interface{}{"type": "mixed", "tag": "mixed-in", "listen": "127.0.0.1", "listen_port": 7890}},
		"outbounds": clientOutbounds,
		"route":     map[string]interface{}{"final": finalOutbound},
	}
	return &sshSingboxDeployment{serverConfig: serverJSON, clientConfig: clientConfig, nodes: nodes, protocols: protocols}, nil
}

func generateSSHX25519KeyPair() (string, string, error) {
	privateKey, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	return base64.RawURLEncoding.EncodeToString(privateKey.Bytes()), base64.RawURLEncoding.EncodeToString(privateKey.PublicKey().Bytes()), nil
}

func generateSSHUUID() (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(value)
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}

func validSSHDeploymentPort(port, sshPort int) bool {
	return port > 0 && port <= 65535 && port != sshPort
}

func validSSHDeploymentAddress(value string) bool {
	if value == "" || len(value) > 253 || strings.ContainsAny(value, "/?#@") || strings.Contains(value, "://") {
		return false
	}
	if net.ParseIP(value) != nil {
		return true
	}
	return validSSHDeploymentDomain(value)
}

func validSSHDeploymentDomain(value string) bool {
	if value == "" || len(value) > 253 || strings.HasPrefix(value, ".") || strings.HasSuffix(value, ".") {
		return false
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') && character != '-' {
				return false
			}
		}
	}
	return true
}

func buildSSHSingboxDeployScript(serverConfig, clientConfig []byte, replaceExisting bool) string {
	replace := "0"
	if replaceExisting {
		replace = "1"
	}
	serverBase64 := base64.StdEncoding.EncodeToString(serverConfig)
	clientBase64 := base64.StdEncoding.EncodeToString(clientConfig)
	return fmt.Sprintf(`set -u
export LC_ALL=C DEBIAN_FRONTEND=noninteractive
fail() { printf 'ACKWRAP_DEPLOY_ERROR\t%%s\n' "$1"; exit 1; }
config_path='%s'
marker_path='%s'
replace_existing='%s'
had_config=0
had_marker=0
managed_unchanged=0
initial_hash=''
marker_hash=''
if test -L "$config_path" || test -L "$marker_path"; then fail unsafe_config_path; fi
if test -e "$config_path" || test -L "$config_path"; then had_config=1; fi
if test -f "$marker_path"; then had_marker=1; fi
if test "$had_config" = 1 && test "$had_marker" = 1 && command -v sha256sum >/dev/null 2>&1; then
  initial_hash="$(sha256sum "$config_path" 2>/dev/null | awk 'NR == 1 { print $1 }')"
  marker_hash="$(sed -n 's/^sha256=//p' "$marker_path" 2>/dev/null | head -n 1)"
  if test -n "$initial_hash" && test "$initial_hash" = "$marker_hash"; then managed_unchanged=1; fi
fi
if test "$had_config" = 1 && test "$managed_unchanged" = 0 && test "$replace_existing" != 1; then
  printf 'ACKWRAP_DEPLOY_CONFLICT\n'
  exit 42
fi
test -r /etc/os-release || fail unsupported_os
. /etc/os-release
case "${ID:-}" in debian|ubuntu) ;; *) fail unsupported_os ;; esac
command -v apt-get >/dev/null 2>&1 || fail apt_unavailable
command -v systemctl >/dev/null 2>&1 || fail systemd_unavailable
command -v flock >/dev/null 2>&1 || fail flock_unavailable
exec 9>/var/lock/ackwrap-singbox-deploy.lock || fail create_deploy_lock
flock -n 9 || fail deploy_locked
was_enabled=0
was_active=0
installed_before=0
systemctl is-enabled --quiet sing-box >/dev/null 2>&1 && was_enabled=1
systemctl is-active --quiet sing-box >/dev/null 2>&1 && was_active=1
command -v sing-box >/dev/null 2>&1 && installed_before=1
if ! command -v sing-box >/dev/null 2>&1; then
  apt-get update >/dev/null 2>&1 || fail apt_update
  apt-get install -y ca-certificates curl >/dev/null 2>&1 || fail install_dependencies
  install -d -m 0755 /etc/apt/keyrings || fail create_keyring
  curl -fsSL https://sing-box.app/gpg.key -o /etc/apt/keyrings/sagernet.asc >/dev/null 2>&1 || fail download_repository_key
  chmod a+r /etc/apt/keyrings/sagernet.asc || fail repository_key_permissions
  printf 'Types: deb\nURIs: https://deb.sagernet.org/\nSuites: *\nComponents: *\nEnabled: yes\nSigned-By: /etc/apt/keyrings/sagernet.asc\n' > /etc/apt/sources.list.d/sagernet.sources || fail write_repository
  apt-get update >/dev/null 2>&1 || fail repository_update
  apt-get install -y sing-box >/dev/null 2>&1 || fail install_singbox
fi
install -d -m 0755 /etc/sing-box || fail create_config_directory
umask 077
server_tmp="$(mktemp /etc/sing-box/.ackwrap-server.XXXXXX)" || fail create_server_temp
client_tmp="$(mktemp /etc/sing-box/.ackwrap-client.XXXXXX)" || { rm -f "$server_tmp"; fail create_client_temp; }
marker_tmp=''
marker_new=''
if test "$had_marker" = 1; then
  marker_tmp="$(mktemp /tmp/.ackwrap-marker.XXXXXX)" || { rm -f "$server_tmp" "$client_tmp"; fail create_marker_temp; }
  cp -pL "$marker_path" "$marker_tmp" || { rm -f "$server_tmp" "$client_tmp" "$marker_tmp"; fail backup_marker; }
fi
cleanup() { rm -f "$server_tmp" "$client_tmp" "$marker_tmp" "$marker_new"; }
trap cleanup EXIT
trap 'exit 130' HUP INT TERM
printf '%%s' '%s' | base64 -d > "$server_tmp" || fail decode_server_config
printf '%%s' '%s' | base64 -d > "$client_tmp" || fail decode_client_config
sing-box check -c "$server_tmp" >/dev/null 2>&1 || fail server_config_invalid
sing-box check -c "$client_tmp" >/dev/null 2>&1 || fail client_config_invalid
latest_had_config=0
latest_had_marker=0
if test -L "$config_path" || test -L "$marker_path"; then fail unsafe_config_path; fi
if test -e "$config_path"; then latest_had_config=1; fi
if test -f "$marker_path"; then latest_had_marker=1; fi
if test "$replace_existing" != 1; then
  if test "$latest_had_marker" != "$had_marker"; then fail config_changed_during_deploy; fi
  if test "$had_config" = 1; then
    latest_hash="$(sha256sum "$config_path" 2>/dev/null | awk 'NR == 1 { print $1 }')"
    latest_marker_hash="$(sed -n 's/^sha256=//p' "$marker_path" 2>/dev/null | head -n 1)"
    if test "$latest_had_config" != 1 || test -z "$latest_hash" || test "$latest_hash" != "$initial_hash" || test "$latest_marker_hash" != "$marker_hash"; then fail config_changed_during_deploy; fi
  elif test "$latest_had_config" = 1; then
    package_md5=''
    actual_md5=''
    if test "$installed_before" = 0 && command -v dpkg-query >/dev/null 2>&1 && command -v md5sum >/dev/null 2>&1; then
      package_md5="$(dpkg-query -W -f='${Conffiles}\n' sing-box 2>/dev/null | awk -v path="$config_path" '$1 == path { print $2; exit }')"
      actual_md5="$(md5sum "$config_path" 2>/dev/null | awk 'NR == 1 { print $1 }')"
    fi
    if test -z "$package_md5" || test "$actual_md5" != "$package_md5"; then fail config_changed_during_deploy; fi
  fi
fi
backup_path=''
if test "$had_config" = 1; then
  backup_path="${config_path}.ackwrap-backup-$(date +%%Y%%m%%d%%H%%M%%S)-$$"
  cp -pL "$config_path" "$backup_path" || fail backup_existing_config
fi
rollback() {
  rollback_ok=1
  if test -n "$backup_path" && test -f "$backup_path"; then cp -pL "$backup_path" "$config_path" >/dev/null 2>&1 || rollback_ok=0; else rm -f "$config_path" >/dev/null 2>&1 || rollback_ok=0; fi
  if test "$had_marker" = 1 && test -n "$marker_tmp"; then cp -pL "$marker_tmp" "$marker_path" >/dev/null 2>&1 || rollback_ok=0; else rm -f "$marker_path" >/dev/null 2>&1 || rollback_ok=0; fi
  if test "$was_enabled" = 1; then systemctl enable sing-box >/dev/null 2>&1 || rollback_ok=0; else systemctl disable sing-box >/dev/null 2>&1 || rollback_ok=0; fi
  if test "$was_active" = 1; then systemctl restart sing-box >/dev/null 2>&1 || rollback_ok=0; else systemctl stop sing-box >/dev/null 2>&1 || rollback_ok=0; fi
  test "$rollback_ok" = 1
}
deployment_fail() {
  failed_step="$1"
  if rollback; then fail "$failed_step"; else fail rollback_failed; fi
}
transaction_started=0
cancel_deployment() {
  trap - HUP INT TERM
  if test "$transaction_started" = 1; then
    if rollback; then printf 'ACKWRAP_DEPLOY_ERROR\tinterrupted\n'; else printf 'ACKWRAP_DEPLOY_ERROR\trollback_failed\n'; fi
  fi
  exit 130
}
trap cancel_deployment HUP INT TERM
chmod 0600 "$server_tmp" || fail config_permissions
transaction_started=1
mv -f "$server_tmp" "$config_path" || fail apply_config
new_hash="$(sha256sum "$config_path" 2>/dev/null | awk 'NR == 1 { print $1 }')"
test -n "$new_hash" || deployment_fail fingerprint_applied_config
marker_new="$(mktemp /etc/sing-box/.ackwrap-marker.XXXXXX)" || deployment_fail create_marker_temp
printf 'sha256=%%s\n' "$new_hash" > "$marker_new" || deployment_fail write_marker
chmod 0600 "$marker_new" || deployment_fail marker_permissions
mv -f "$marker_new" "$marker_path" || deployment_fail apply_marker
systemctl enable sing-box >/dev/null 2>&1 || deployment_fail enable_service
systemctl restart sing-box >/dev/null 2>&1 || deployment_fail restart_service
sleep 1
systemctl is-active --quiet sing-box || deployment_fail service_inactive
version="$(sing-box version 2>/dev/null | head -n 1 | tr '\t\r\n' '   ')"
printf 'ACKWRAP_DEPLOY_OK\t%%s\t%%s\n' "$version" "$backup_path"
transaction_started=0
trap - HUP INT TERM
`, sshSingboxConfigPath, sshSingboxManagedMarkerPath, replace, serverBase64, clientBase64)
}

func runSSHSingboxCommand(ctx context.Context, client *ssh.Client, command string, input io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	session, err := client.NewSession()
	if err != nil {
		return nil, err
	}
	defer session.Close()
	output := &sshSingboxCommandOutput{}
	session.Stdout, session.Stderr = output, output
	if input != nil {
		session.Stdin = input
	}
	completed := make(chan error, 1)
	go func() { completed <- session.Run(command) }()
	select {
	case runErr := <-completed:
		if output.overflow {
			return output.buffer.Bytes(), errors.New("SSH sing-box command output is too large")
		}
		return output.buffer.Bytes(), runErr
	case <-ctx.Done():
		if signalErr := session.Signal(ssh.SIGTERM); signalErr == nil {
			select {
			case <-completed:
				return output.buffer.Bytes(), ctx.Err()
			case <-time.After(sshSingboxRollbackGrace):
			}
		}
		_ = session.Close()
		_ = client.Close()
		return output.buffer.Bytes(), ctx.Err()
	}
}

func parseSSHSingboxPrivilege(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) == 2 && fields[0] == "ACKWRAP_PRIVILEGE" && (fields[1] == "root" || fields[1] == "sudo" || fields[1] == "unsupported") {
			return fields[1]
		}
	}
	return ""
}

func parseSSHSingboxDeployOutput(output []byte, runErr error) (string, string, error) {
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) == 2 && fields[0] == "ACKWRAP_DEPLOY_ERROR" && fields[1] == "rollback_failed" {
			return "", "", sshSingboxStepError(fields[1], runErr)
		}
	}
	if errors.Is(runErr, context.DeadlineExceeded) {
		return "", "", sshError("SSH_SINGBOX_DEPLOY_TIMEOUT", "远端 sing-box 部署超时", runErr)
	}
	var successVersion, successBackup string
	for _, line := range lines {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) == 1 && fields[0] == "ACKWRAP_DEPLOY_CONFLICT" {
			return "", "", &SSHServiceError{Code: "SSH_SINGBOX_CONFIG_CONFLICT", Message: "远端已有非 Ackwrap 管理的 sing-box 配置；确认备份并覆盖后才能继续", Details: map[string]interface{}{"config_path": sshSingboxConfigPath}, Cause: runErr}
		}
		if len(fields) == 2 && fields[0] == "ACKWRAP_DEPLOY_ERROR" {
			return "", "", sshSingboxStepError(fields[1], runErr)
		}
		if len(fields) == 3 && fields[0] == "ACKWRAP_DEPLOY_OK" && fields[1] != "" {
			successVersion, successBackup = monitorText(fields[1]), monitorText(fields[2])
		}
	}
	if successVersion != "" {
		return successVersion, successBackup, nil
	}
	return "", "", sshError("SSH_SINGBOX_DEPLOY_FAILED", "远端 sing-box 部署失败，未收到有效结果", runErr)
}

func sshSingboxStepError(step string, cause error) error {
	if step == "deploy_locked" {
		return &SSHServiceError{Code: "SSH_SINGBOX_DEPLOY_BUSY", Message: "远端主机正在执行另一个 sing-box 部署", Details: map[string]interface{}{"step": step}, Cause: cause}
	}
	if step == "config_changed_during_deploy" {
		return &SSHServiceError{Code: "SSH_SINGBOX_CONFIG_CONFLICT", Message: "远端 sing-box 配置在部署期间发生变化，本次未覆盖", Details: map[string]interface{}{"config_path": sshSingboxConfigPath}, Cause: cause}
	}
	messages := map[string]string{
		"unsupported_os":             "仅支持 Debian 或 Ubuntu 远端主机",
		"unsafe_config_path":         "远端 sing-box 配置或管理标记不能是符号链接",
		"apt_unavailable":            "远端主机缺少 APT 包管理器",
		"systemd_unavailable":        "远端主机缺少 systemd 服务管理器",
		"flock_unavailable":          "远端主机缺少部署锁工具 flock",
		"create_deploy_lock":         "创建远端 sing-box 部署锁失败",
		"apt_update":                 "更新远端 APT 索引失败",
		"install_dependencies":       "安装远端基础依赖失败",
		"create_keyring":             "创建远端 APT 密钥目录失败",
		"download_repository_key":    "下载 sing-box 官方仓库密钥失败",
		"repository_key_permissions": "设置 sing-box 官方仓库密钥权限失败",
		"write_repository":           "写入 sing-box 官方 APT 软件源失败",
		"repository_update":          "更新 sing-box 官方软件源失败",
		"install_singbox":            "安装 sing-box 软件包失败",
		"create_config_directory":    "创建远端 sing-box 配置目录失败",
		"create_server_temp":         "创建远端服务端临时配置失败",
		"create_client_temp":         "创建远端客户端临时配置失败",
		"create_marker_temp":         "创建远端 Ackwrap 管理标记失败",
		"backup_marker":              "备份远端 Ackwrap 管理标记失败",
		"decode_server_config":       "写入远端服务端临时配置失败",
		"decode_client_config":       "写入远端客户端临时配置失败",
		"server_config_invalid":      "生成的服务端配置未通过 sing-box 校验",
		"client_config_invalid":      "生成的客户端配置未通过 sing-box 校验",
		"backup_existing_config":     "备份远端原配置失败",
		"config_permissions":         "设置远端 sing-box 配置权限失败",
		"apply_config":               "写入远端 sing-box 配置失败",
		"fingerprint_applied_config": "计算远端 sing-box 配置指纹失败，原配置已恢复",
		"write_marker":               "写入远端 Ackwrap 管理标记失败，原配置已恢复",
		"marker_permissions":         "设置远端 Ackwrap 管理标记权限失败，原配置已恢复",
		"apply_marker":               "应用远端 Ackwrap 管理标记失败，原配置已恢复",
		"enable_service":             "设置 sing-box 开机启动失败，原配置已恢复",
		"restart_service":            "启动 sing-box 服务失败，原配置已恢复",
		"service_inactive":           "sing-box 启动后未保持运行，原配置已恢复",
		"interrupted":                "sing-box 部署已取消，远端配置与服务状态已恢复",
		"rollback_failed":            "sing-box 部署失败且自动回滚未完整完成，请立即人工检查远端配置与服务状态",
	}
	message := messages[step]
	if message == "" {
		message = "远端 sing-box 部署步骤失败"
	}
	return &SSHServiceError{Code: "SSH_SINGBOX_DEPLOY_FAILED", Message: message, Details: map[string]interface{}{"step": monitorText(step)}, Cause: cause}
}
