package service

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

func TestSSHSingboxInstallScriptUsesOfficialRepositoryWithoutConfigMutation(t *testing.T) {
	for _, required := range []string{
		"https://sing-box.app/gpg.key",
		"https://deb.sagernet.org/",
		"Pin: origin deb.sagernet.org",
		"dpkg --compare-versions \"$candidate_version\" lt \"$installed_version\"",
		"apt-get install -y --reinstall sing-box",
		"apt-get install -y sing-box",
		"singbox_bin='/usr/bin/sing-box'",
		"getent group sing-box",
		"systemctl cat sing-box",
		"repository_config_conflict",
		"mktemp /etc/apt/keyrings/.sagernet.",
		"had_config=0",
		"was_enabled=0",
		"was_active=0",
		"restore_pending=1",
		"if ! restore_service_state",
		"trap cancel_install HUP INT TERM",
		"restore_service_state",
		"cmp -s \"$key_path\" \"$key_tmp\"",
		"systemctl stop sing-box",
		"systemctl disable sing-box",
		"ACKWRAP_INSTALL_OK",
	} {
		if !strings.Contains(sshSingboxInstallScript, required) {
			t.Fatalf("installation script is missing required workflow marker %q", required)
		}
	}
	for _, forbidden := range []string{
		"curl -fsSL https://sing-box.app/install.sh | sh",
		"--allow-downgrades",
		sshSingboxManagedMarkerPath,
		"mktemp /etc/sing-box",
		"mv -f \"$server_tmp\"",
		"systemctl restart sing-box",
	} {
		if strings.Contains(sshSingboxInstallScript, forbidden) {
			t.Fatalf("installation script contains forbidden mutation %q", forbidden)
		}
	}
	if strings.Index(sshSingboxInstallScript, "if test -x \"$singbox_bin\"") > strings.Index(sshSingboxInstallScript, "apt-get update") {
		t.Fatal("installation script mutates APT before accepting an existing core")
	}
	if strings.Index(sshSingboxInstallScript, "repository_config_conflict") > strings.Index(sshSingboxInstallScript, "mv -f \"$source_tmp\"") {
		t.Fatal("installation script overwrites repository configuration before checking ownership")
	}
	firstAPTUpdate := strings.Index(sshSingboxInstallScript, "apt-get update")
	firstAPTInstall := strings.Index(sshSingboxInstallScript, "apt-get install")
	for _, guard := range []string{"unsafe_repository_path", "repository_config_conflict"} {
		guardIndex := strings.Index(sshSingboxInstallScript, guard)
		if guardIndex < 0 || guardIndex > firstAPTUpdate || guardIndex > firstAPTInstall {
			t.Fatalf("repository guard %q runs only after an APT mutation", guard)
		}
	}
	if strings.Index(sshSingboxInstallScript, "cmp -s \"$key_path\" \"$key_tmp\"") > strings.Index(sshSingboxInstallScript, "mv -f \"$key_tmp\"") {
		t.Fatal("installation script overwrites an existing repository key before comparing it")
	}
	if shellPath, err := exec.LookPath("sh"); err == nil {
		command := exec.Command(shellPath, "-n")
		command.Stdin = strings.NewReader(sshSingboxInstallScript)
		if err := command.Run(); err != nil {
			t.Fatal("generated remote installation script has invalid POSIX shell syntax")
		}
	}
}

func TestParseSSHSingboxInstallOutput(t *testing.T) {
	version, err := parseSSHSingboxInstallOutput([]byte("ACKWRAP_INSTALL_OK\tsing-box version 1.14.0\n"), nil)
	if err != nil || version != "sing-box version 1.14.0" {
		t.Fatalf("unexpected successful installation result: version=%q err=%v", version, err)
	}
	_, err = parseSSHSingboxInstallOutput([]byte("ACKWRAP_INSTALL_ERROR\tinstall_singbox\n"), errors.New("exit status 1"))
	var serviceError *SSHServiceError
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_INSTALL_FAILED" {
		t.Fatalf("unexpected installation failure: %v", err)
	}
	_, err = parseSSHSingboxInstallOutput([]byte("ACKWRAP_INSTALL_ERROR\tdeploy_locked\n"), errors.New("exit status 1"))
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_DEPLOY_BUSY" {
		t.Fatalf("unexpected installation lock failure: %v", err)
	}
	_, err = parseSSHSingboxInstallOutput(nil, context.DeadlineExceeded)
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_INSTALL_TIMEOUT" {
		t.Fatalf("unexpected installation timeout: %v", err)
	}
}
