package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const sshSingboxInstallScript = `set -u
export LC_ALL=C DEBIAN_FRONTEND=noninteractive
restore_pending=0
was_enabled=0
was_active=0
restore_service_state() {
  test "$restore_pending" = 1 || return 0
  if ! systemctl cat sing-box >/dev/null 2>&1; then
    restore_pending=0
    test "$was_enabled" = 0 && test "$was_active" = 0
    return
  fi
  restore_ok=1
  if test "$was_enabled" = 1; then systemctl enable sing-box >/dev/null 2>&1 || restore_ok=0; else systemctl disable sing-box >/dev/null 2>&1 || restore_ok=0; fi
  if test "$was_active" = 1; then systemctl start sing-box >/dev/null 2>&1 || restore_ok=0; else systemctl stop sing-box >/dev/null 2>&1 || restore_ok=0; fi
  restore_pending=0
  test "$restore_ok" = 1
}
fail() {
  failed_step="$1"
  if ! restore_service_state; then failed_step=restore_service_state; fi
  printf 'ACKWRAP_INSTALL_ERROR\t%s\n' "$failed_step"
  exit 1
}
cancel_install() { trap - HUP INT TERM; fail interrupted; }
trap cancel_install HUP INT TERM
test -r /etc/os-release || fail unsupported_os
. /etc/os-release
case "${ID:-}" in debian|ubuntu) ;; *) fail unsupported_os ;; esac
command -v apt-get >/dev/null 2>&1 || fail apt_unavailable
command -v dpkg-query >/dev/null 2>&1 || fail dpkg_unavailable
command -v systemctl >/dev/null 2>&1 || fail systemd_unavailable
command -v flock >/dev/null 2>&1 || fail flock_unavailable
exec 9>/var/lock/ackwrap-singbox-deploy.lock || fail create_deploy_lock
flock -n 9 || fail deploy_locked
singbox_bin='/usr/bin/sing-box'
if test -x "$singbox_bin" && dpkg-query -W sing-box >/dev/null 2>&1 && getent group sing-box >/dev/null 2>&1 && systemctl cat sing-box >/dev/null 2>&1; then
  version="$("$singbox_bin" version 2>/dev/null | head -n 1 | tr '\t\r\n' '   ')"
  test -n "$version" || fail read_installed_version
  printf 'ACKWRAP_INSTALL_OK\t%s\n' "$version"
  exit 0
fi
config_path='/etc/sing-box/config.json'
test ! -L "$config_path" || fail unsafe_config_path
had_config=0
test -e "$config_path" && had_config=1
if test "$had_config" = 1; then
  systemctl is-enabled --quiet sing-box >/dev/null 2>&1 && was_enabled=1
  systemctl is-active --quiet sing-box >/dev/null 2>&1 && was_active=1
fi
key_path='/etc/apt/keyrings/sagernet.asc'
source_path='/etc/apt/sources.list.d/sagernet.sources'
pin_path='/etc/apt/preferences.d/sagernet-sing-box'
if test -L "$key_path" || test -L "$source_path" || test -L "$pin_path"; then fail unsafe_repository_path; fi
source_expected="$(printf 'Types: deb\nURIs: https://deb.sagernet.org/\nSuites: *\nComponents: *\nEnabled: yes\nSigned-By: /etc/apt/keyrings/sagernet.asc')"
pin_expected="$(printf 'Package: sing-box\nPin: origin deb.sagernet.org\nPin-Priority: 1001')"
if test -e "$source_path" && { test ! -f "$source_path" || test "$(cat "$source_path" 2>/dev/null)" != "$source_expected"; }; then fail repository_config_conflict; fi
if test -e "$pin_path" && { test ! -f "$pin_path" || test "$(cat "$pin_path" 2>/dev/null)" != "$pin_expected"; }; then fail repository_config_conflict; fi
restore_pending=1
apt-get update >/dev/null 2>&1 || fail apt_update
apt-get install -y ca-certificates curl >/dev/null 2>&1 || fail install_dependencies
install -d -m 0755 /etc/apt/keyrings || fail create_keyring
key_tmp=''
source_tmp=''
pin_tmp=''
cleanup() { rm -f "$key_tmp" "$source_tmp" "$pin_tmp"; }
trap cleanup EXIT
key_tmp="$(mktemp /etc/apt/keyrings/.sagernet.XXXXXX)" || fail create_repository_temp
source_tmp="$(mktemp /etc/apt/sources.list.d/.sagernet.XXXXXX)" || fail create_repository_temp
pin_tmp="$(mktemp /etc/apt/preferences.d/.sagernet.XXXXXX)" || fail create_repository_temp
curl -fsSL https://sing-box.app/gpg.key -o "$key_tmp" >/dev/null 2>&1 || fail download_repository_key
chmod 0644 "$key_tmp" || fail repository_key_permissions
if test -e "$key_path" && { test ! -f "$key_path" || ! cmp -s "$key_path" "$key_tmp"; }; then fail repository_config_conflict; fi
printf '%s\n' "$source_expected" > "$source_tmp" || fail write_repository
chmod 0644 "$source_tmp" || fail write_repository
printf '%s\n' "$pin_expected" > "$pin_tmp" || fail write_repository_pin
chmod 0644 "$pin_tmp" || fail write_repository_pin
mv -f "$key_tmp" "$key_path" || fail write_repository
key_tmp=''
mv -f "$source_tmp" "$source_path" || fail write_repository
source_tmp=''
mv -f "$pin_tmp" "$pin_path" || fail write_repository_pin
pin_tmp=''
apt-get update >/dev/null 2>&1 || fail repository_update
installed_version="$(dpkg-query -W -f='${Version}' sing-box 2>/dev/null || true)"
candidate_version="$(apt-cache policy sing-box 2>/dev/null | awk '/Candidate:/ { print $2; exit }')"
if test -n "$installed_version" && test -n "$candidate_version" && test "$candidate_version" != '(none)' && dpkg --compare-versions "$candidate_version" lt "$installed_version"; then fail singbox_downgrade_refused; fi
if test -n "$installed_version"; then
  apt-get install -y --reinstall sing-box >/dev/null 2>&1 || fail install_singbox
else
  apt-get install -y sing-box >/dev/null 2>&1 || fail install_singbox
fi
test -x "$singbox_bin" || fail singbox_binary_missing
getent group sing-box >/dev/null 2>&1 || fail singbox_group_missing
systemctl cat sing-box >/dev/null 2>&1 || fail singbox_service_missing
restore_service_state || fail restore_service_state
version="$("$singbox_bin" version 2>/dev/null | head -n 1 | tr '\t\r\n' '   ')"
test -n "$version" || fail read_installed_version
printf 'ACKWRAP_INSTALL_OK\t%s\n' "$version"
`

func (svc *SSHHostService) InstallSingbox(ctx context.Context, hostID int64) (*model.SSHSingboxInstallResponse, error) {
	started := svc.now()
	host, err := svc.store.GetSSHHost(hostID)
	if err != nil {
		return nil, normalizeSSHStoreError(err)
	}
	if !host.Enabled {
		return nil, sshError("SSH_HOST_DISABLED", "SSH 主机已停用", nil)
	}
	installCtx, cancel := context.WithTimeout(ctx, sshSingboxDeployTimeout)
	if !svc.beginSingboxDeploy(hostID, cancel) {
		cancel()
		return nil, sshError("SSH_SINGBOX_DEPLOY_BUSY", "该主机正在执行 sing-box 操作", nil)
	}
	defer svc.endSingboxDeploy(hostID)
	defer cancel()

	logging.Info("ssh_singbox.install", "开始安装远端 sing-box: host_id=%d", hostID)
	svc.broadcastSingboxDeploy(hostID, "installing", "正在连接远端主机并安装 sing-box", "")
	client, _, _, _, err := svc.connect(installCtx, host)
	if err != nil {
		svc.finishSingboxInstallAudit(host, started, err)
		return nil, err
	}
	defer client.Close()

	privilegeOutput, privilegeErr := runSSHSingboxCommand(installCtx, client, sshSingboxPrivilegeCommand, nil)
	privilege := parseSSHSingboxPrivilege(privilegeOutput)
	if errors.Is(privilegeErr, context.DeadlineExceeded) {
		err = sshError("SSH_SINGBOX_INSTALL_TIMEOUT", "远端 sing-box 安装超时", privilegeErr)
	} else if privilegeErr != nil || privilege == "" {
		err = sshError("SSH_SINGBOX_PREFLIGHT_FAILED", "无法检查远端安装权限", privilegeErr)
	} else if privilege == "unsupported" {
		err = sshError("SSH_SINGBOX_ROOT_REQUIRED", "安装 sing-box 需要 root 用户或免密 sudo 权限", nil)
	}
	if err != nil {
		svc.finishSingboxInstallAudit(host, started, err)
		return nil, err
	}

	command := "sh -s"
	if privilege == "sudo" {
		command = "sudo -n sh -s"
	}
	output, runErr := runSSHSingboxCommand(installCtx, client, command, strings.NewReader(sshSingboxInstallScript))
	version, resultErr := parseSSHSingboxInstallOutput(output, runErr)
	if resultErr != nil {
		svc.finishSingboxInstallAudit(host, started, resultErr)
		return nil, resultErr
	}

	_ = svc.store.TouchSSHHostKey(hostID)
	svc.audit(host, "singbox_install", "success", "", svc.now().Sub(started), "")
	logging.Info("ssh_singbox.install", "远端 sing-box 安装完成: host_id=%d", hostID)
	svc.broadcastSingboxDeploy(hostID, "installed", "sing-box 安装完成", "")
	return &model.SSHSingboxInstallResponse{Success: true, Message: "sing-box 已安装，未写入或覆盖服务配置", Version: version}, nil
}

func (svc *SSHHostService) finishSingboxInstallAudit(host *model.SSHHost, started time.Time, err error) {
	code, _ := sshErrorInfo(err)
	svc.audit(host, "singbox_install", "error", code, svc.now().Sub(started), "")
	logging.Error("ssh_singbox.install", "远端 sing-box 安装失败: host_id=%d code=%s", host.ID, code)
	svc.broadcastSingboxDeploy(host.ID, "error", "sing-box 安装失败", code)
}

func parseSSHSingboxInstallOutput(output []byte, runErr error) (string, error) {
	if errors.Is(runErr, context.DeadlineExceeded) {
		return "", sshError("SSH_SINGBOX_INSTALL_TIMEOUT", "远端 sing-box 安装超时", runErr)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Split(strings.TrimRight(line, "\r"), "\t")
		if len(fields) == 2 && fields[0] == "ACKWRAP_INSTALL_ERROR" {
			return "", sshSingboxInstallStepError(fields[1], runErr)
		}
		if len(fields) == 2 && fields[0] == "ACKWRAP_INSTALL_OK" && fields[1] != "" {
			return monitorText(fields[1]), nil
		}
	}
	return "", sshError("SSH_SINGBOX_INSTALL_FAILED", "远端 sing-box 安装失败，未收到有效结果", runErr)
}

func sshSingboxInstallStepError(step string, cause error) error {
	if step == "interrupted" {
		return &SSHServiceError{Code: "SSH_SINGBOX_INSTALL_FAILED", Message: "sing-box 安装已取消，已有服务状态已恢复", Details: map[string]interface{}{"step": step}, Cause: cause}
	}
	stepErr := sshSingboxStepError(step, cause)
	var serviceErr *SSHServiceError
	if errors.As(stepErr, &serviceErr) && serviceErr.Code == "SSH_SINGBOX_DEPLOY_BUSY" {
		return stepErr
	}
	message := "远端 sing-box 安装步骤失败"
	var details any = map[string]interface{}{"step": monitorText(step)}
	if errors.As(stepErr, &serviceErr) {
		message, details = serviceErr.Message, serviceErr.Details
	}
	return &SSHServiceError{Code: "SSH_SINGBOX_INSTALL_FAILED", Message: message, Details: details, Cause: cause}
}
