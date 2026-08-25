package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestBuildSSHSingboxDeploymentGeneratesValidatedProtocolShapes(t *testing.T) {
	host := &model.SSHHost{ID: 7, Name: "edge", Host: "198.51.100.10", Port: 22, Enabled: true}
	deployment, err := buildSSHSingboxDeployment(host, model.SSHSingboxDeployRequest{
		ServerAddress:       "198.51.100.10",
		VLESSRealityEnabled: true,
		VLESSRealityPort:    443,
		RealityServerName:   "www.microsoft.com",
		ShadowsocksEnabled:  true,
		ShadowsocksPort:     8388,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(deployment.nodes) != 2 || len(deployment.protocols) != 2 {
		t.Fatalf("unexpected generated protocol count: nodes=%d protocols=%d", len(deployment.nodes), len(deployment.protocols))
	}

	var serverConfig map[string]interface{}
	if err := json.Unmarshal(deployment.serverConfig, &serverConfig); err != nil {
		t.Fatal(err)
	}
	inbounds, ok := serverConfig["inbounds"].([]interface{})
	if !ok || len(inbounds) != 2 {
		t.Fatalf("unexpected server inbound count: %T", serverConfig["inbounds"])
	}
	vlessInbound := inbounds[0].(map[string]interface{})
	if vlessInbound["type"] != "vless" || vlessInbound["listen"] != "::" || int(vlessInbound["listen_port"].(float64)) != 443 {
		t.Fatal("VLESS Reality inbound shape is invalid")
	}
	reality := vlessInbound["tls"].(map[string]interface{})["reality"].(map[string]interface{})
	privateKey := reality["private_key"].(string)
	if decoded, decodeErr := base64.RawURLEncoding.DecodeString(privateKey); decodeErr != nil || len(decoded) != 32 {
		t.Fatal("Reality private key is not a 32-byte URL-safe X25519 key")
	}
	clientJSON, err := json.Marshal(deployment.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(clientJSON), privateKey) {
		t.Fatal("Reality private key leaked into generated client configuration")
	}
	if deployment.clientConfig["route"].(map[string]interface{})["final"] != "proxy" {
		t.Fatal("multi-protocol client configuration does not use its selector")
	}

	vlessShare, err := url.Parse(deployment.nodes[0].ShareURI)
	if err != nil || vlessShare.Scheme != "vless" || vlessShare.Query().Get("security") != "reality" || vlessShare.Query().Get("pbk") == "" {
		t.Fatal("generated VLESS Reality share URI is invalid")
	}
	ssShare, err := url.Parse(deployment.nodes[1].ShareURI)
	if err != nil || ssShare.Scheme != "ss" || ssShare.User == nil {
		t.Fatal("generated Shadowsocks share URI is invalid")
	}
	userinfo, err := base64.RawURLEncoding.DecodeString(ssShare.User.Username())
	if err != nil || !strings.HasPrefix(string(userinfo), "2022-blake3-aes-128-gcm:") {
		t.Fatal("generated Shadowsocks SIP002 user info is invalid")
	}
}

func TestGeneratedSSHSingboxConfigsPassInstalledCoreCheck(t *testing.T) {
	binaryPath := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if binaryPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("cannot locate installed sing-box")
		}
		binaryPath = filepath.Join(home, "ackwrap", "bin", "sing-box.exe")
	}
	if info, err := os.Stat(binaryPath); err != nil || info.IsDir() {
		t.Skip("installed sing-box verification binary unavailable")
	}
	deployment, err := buildSSHSingboxDeployment(&model.SSHHost{Name: "edge", Port: 22}, model.SSHSingboxDeployRequest{
		ServerAddress: "198.51.100.10", VLESSRealityEnabled: true, VLESSRealityPort: 443,
		RealityServerName: "www.microsoft.com", ShadowsocksEnabled: true, ShadowsocksPort: 8388,
	})
	if err != nil {
		t.Fatal(err)
	}
	clientConfig, err := json.Marshal(deployment.clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	for name, content := range map[string][]byte{"server": deployment.serverConfig, "client": clientConfig} {
		t.Run(name, func(t *testing.T) {
			configPath := filepath.Join(t.TempDir(), name+".json")
			if err := os.WriteFile(configPath, content, 0600); err != nil {
				t.Fatal(err)
			}
			if err := exec.Command(binaryPath, "check", "-c", configPath).Run(); err != nil {
				t.Fatalf("installed sing-box rejected generated %s configuration", name)
			}
		})
	}
}

func TestBuildSSHSingboxDeploymentRejectsUnsafeInputs(t *testing.T) {
	host := &model.SSHHost{Name: "edge", Port: 22}
	base := model.SSHSingboxDeployRequest{
		ServerAddress:       "proxy.example.com",
		VLESSRealityEnabled: true,
		VLESSRealityPort:    443,
		RealityServerName:   "www.microsoft.com",
	}
	tests := map[string]model.SSHSingboxDeployRequest{
		"URL address": func() model.SSHSingboxDeployRequest {
			value := base
			value.ServerAddress = "https://proxy.example.com"
			return value
		}(),
		"invalid IPv4": func() model.SSHSingboxDeployRequest {
			value := base
			value.ServerAddress = "999.999.999.999"
			return value
		}(),
		"SSH port conflict": func() model.SSHSingboxDeployRequest { value := base; value.VLESSRealityPort = 22; return value }(),
		"invalid reality": func() model.SSHSingboxDeployRequest {
			value := base
			value.RealityServerName = "https://example.com"
			return value
		}(),
		"no protocol": {ServerAddress: "proxy.example.com"},
		"duplicate ports": func() model.SSHSingboxDeployRequest {
			value := base
			value.ShadowsocksEnabled, value.ShadowsocksPort = true, value.VLESSRealityPort
			return value
		}(),
	}
	for name, request := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := buildSSHSingboxDeployment(host, request); err == nil {
				t.Fatal("expected invalid deployment request to be rejected")
			}
		})
	}
}

func TestSSHSingboxDeployScriptUsesOfficialAtomicWorkflow(t *testing.T) {
	serverConfig := []byte(`{"inbounds":[]}`)
	clientConfig := []byte(`{"outbounds":[]}`)
	script := buildSSHSingboxDeployScript(serverConfig, clientConfig, false)
	for _, required := range []string{
		"https://sing-box.app/gpg.key",
		"https://deb.sagernet.org/",
		"sing-box check -c \"$server_tmp\"",
		"sing-box check -c \"$client_tmp\"",
		"ACKWRAP_DEPLOY_CONFLICT",
		"managed_unchanged=0",
		"fail unsafe_config_path",
		"sha256sum \"$config_path\"",
		"test -e \"$config_path\" || test -L \"$config_path\"",
		"backup_path=",
		"rollback",
		"was_enabled=0",
		"was_active=0",
		"flock -n 9",
		"config_changed_during_deploy",
		"latest_had_config=0",
		"dpkg-query -W",
		"rollback_failed",
		"trap cancel_deployment HUP INT TERM",
		"transaction_started=1",
		"systemctl is-active --quiet sing-box",
		"replace_existing='0'",
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("deployment script is missing required workflow marker %q", required)
		}
	}
	if strings.Contains(script, "curl -fsSL https://sing-box.app/install.sh | sh") {
		t.Fatal("deployment script executes the remote convenience installer directly")
	}
	if strings.Contains(script, string(serverConfig)) || strings.Contains(script, string(clientConfig)) {
		t.Fatal("deployment script embeds sensitive JSON as plaintext")
	}
	if strings.Index(script, "ACKWRAP_DEPLOY_CONFLICT") > strings.Index(script, "apt-get update") {
		t.Fatal("existing unmanaged configuration is checked only after package mutations")
	}
	if replaced := buildSSHSingboxDeployScript(serverConfig, clientConfig, true); !strings.Contains(replaced, "replace_existing='1'") {
		t.Fatal("explicit replacement confirmation was not included in deployment script")
	}
	if shellPath, err := exec.LookPath("sh"); err == nil {
		command := exec.Command(shellPath, "-n")
		command.Stdin = strings.NewReader(script)
		if err := command.Run(); err != nil {
			t.Fatal("generated remote deployment script has invalid POSIX shell syntax")
		}
	}
}

func TestParseSSHSingboxDeployOutput(t *testing.T) {
	version, backup, err := parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_OK\tsing-box version 1.13.14\t\n"), nil)
	if err != nil || version != "sing-box version 1.13.14" || backup != "" {
		t.Fatalf("unexpected successful deployment result: version=%q backup=%q err=%v", version, backup, err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_CONFLICT\n"), errors.New("exit status 42"))
	var serviceError *SSHServiceError
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_CONFIG_CONFLICT" {
		t.Fatalf("unexpected conflict result: %v", err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_ERROR\trestart_service\n"), errors.New("exit status 1"))
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_DEPLOY_FAILED" || !strings.Contains(serviceError.Message, "原配置已恢复") {
		t.Fatalf("unexpected rollback failure result: %v", err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_ERROR\trestart_service\n"), context.DeadlineExceeded)
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_DEPLOY_TIMEOUT" {
		t.Fatalf("unexpected deployment timeout result: %v", err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_ERROR\trollback_failed\n"), errors.New("exit status 1"))
	if !errors.As(err, &serviceError) || !strings.Contains(serviceError.Message, "人工检查") {
		t.Fatalf("unexpected failed rollback result: %v", err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_ERROR\tdeploy_locked\n"), errors.New("exit status 1"))
	if !errors.As(err, &serviceError) || serviceError.Code != "SSH_SINGBOX_DEPLOY_BUSY" {
		t.Fatalf("unexpected remote deployment lock result: %v", err)
	}
	_, _, err = parseSSHSingboxDeployOutput([]byte("ACKWRAP_DEPLOY_OK\tsing-box version 1.13.14\t\nACKWRAP_DEPLOY_ERROR\trollback_failed\n"), errors.New("exit status 130"))
	if !errors.As(err, &serviceError) || !strings.Contains(serviceError.Message, "人工检查") {
		t.Fatalf("successful marker hid a later rollback failure: %v", err)
	}
}

func TestSSHSingboxDeployLockIsPerHost(t *testing.T) {
	service := &SSHHostService{singboxDeploying: make(map[int64]bool), singboxDeployCancels: make(map[int64]context.CancelFunc)}
	firstCtx, firstCancel := context.WithCancel(context.Background())
	defer firstCancel()
	secondCtx, secondCancel := context.WithCancel(context.Background())
	defer secondCancel()
	if !service.beginSingboxDeploy(1, firstCancel) || service.beginSingboxDeploy(1, secondCancel) {
		t.Fatal("same host accepted concurrent sing-box deployments")
	}
	if !service.beginSingboxDeploy(2, secondCancel) {
		t.Fatal("different host was incorrectly blocked")
	}
	service.Close()
	select {
	case <-firstCtx.Done():
	default:
		t.Fatal("service shutdown did not cancel active deployment")
	}
	select {
	case <-secondCtx.Done():
	default:
		t.Fatal("service shutdown did not cancel deployment on second host")
	}
	service.endSingboxDeploy(1)
	_, thirdCancel := context.WithCancel(context.Background())
	defer thirdCancel()
	if service.beginSingboxDeploy(1, thirdCancel) {
		t.Fatal("closed SSH service accepted a new deployment")
	}
}
