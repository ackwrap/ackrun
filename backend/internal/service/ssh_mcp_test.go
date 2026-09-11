package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSSHMCPSettingsTokenLifecycle(t *testing.T) {
	_, db := newSSHHostShareTestService(t)
	svc := NewSSHMCPService(db)
	settings, err := svc.Settings()
	if err != nil || settings.Enabled || settings.TokenConfigured {
		t.Fatalf("initial settings: %v", err)
	}
	if _, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true}); err == nil {
		t.Fatal("enabled MCP without a token")
	}
	for _, token := range []string{"short", strings.Repeat("x", 31) + " ", strings.Repeat("x", 257)} {
		if _, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Token: token}); err == nil {
			t.Fatal("accepted invalid token")
		}
	}
	issued, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil || len(issued.Token) < 32 {
		t.Fatalf("generate token: %v", err)
	}
	var raw string
	if err := db.DB().QueryRow(`SELECT value FROM app_settings WHERE key = 'ssh.mcp'`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, issued.Token) {
		t.Fatal("plaintext token persisted")
	}
	settings, err = NewSSHMCPService(db).Settings()
	if err != nil || !settings.Enabled || !settings.TokenConfigured || settings.Token != "" {
		t.Fatal("settings not persisted or exposed token")
	}
	encoded, _ := json.Marshal(settings)
	if bytes.Contains(encoded, []byte("token_hash")) {
		t.Fatal("settings exposed token hash")
	}
	if enabled, ok, err := svc.Authenticate(issued.Token); err != nil || !enabled || !ok {
		t.Fatalf("authenticate: %v", err)
	}
	if _, ok, _ := svc.Authenticate("wrong"); ok {
		t.Fatal("wrong token accepted")
	}
	if _, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := svc.Authenticate(issued.Token); !ok {
		t.Fatal("omitted token was replaced")
	}
	rotated, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil || rotated.Token == issued.Token {
		t.Fatal("token rotation failed")
	}
	if _, ok, _ := svc.Authenticate(issued.Token); ok {
		t.Fatal("old token survived rotation")
	}
	if _, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if enabled, ok, _ := svc.Authenticate(rotated.Token); enabled || ok {
		t.Fatal("disabled MCP accepted token")
	}
}

func TestSSHMCPFilesAndHostTrust(t *testing.T) {
	root := t.TempDir()
	server := newSFTPTestSSHServer(t, "test-password", root)
	t.Cleanup(server.Close)
	svc, _ := newSSHHostShareTestService(t)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "test", AuthType: "password", Secret: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{Name: "test", Host: "127.0.0.1", Port: server.Port(), Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	remote := "mcp-file.txt"
	write := model.SSHMCPWriteRequest{HostID: host.ID, Path: remote, Content: "original"}
	if err := svc.WriteMCPFile(ctx, write, false); sshServiceCode(err) != "SSH_HOST_KEY_UNKNOWN" {
		t.Fatalf("untrusted host accepted: %v", err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	if err := svc.WriteMCPFile(ctx, write, false); err != nil {
		t.Fatal(err)
	}
	write.Content = "replacement"
	if err := svc.WriteMCPFile(ctx, write, false); sshServiceCode(err) != "SSH_MCP_FILE_EXISTS" {
		t.Fatalf("unexpected overwrite result: %v", err)
	}
	read := model.SSHMCPFileRequest{HostID: host.ID, Path: remote}
	result, err := svc.ReadMCPFile(ctx, read, "read_file")
	if err != nil || result.(map[string]any)["content"] != "original" {
		t.Fatalf("existing file not preserved: %v", err)
	}
	write.Overwrite = true
	if err := svc.WriteMCPFile(ctx, write, false); err != nil {
		t.Fatal(err)
	}
	result, err = svc.ReadMCPFile(ctx, read, "read_file")
	if err != nil || result.(map[string]any)["content"] != "replacement" {
		t.Fatalf("replacement failed: %v", err)
	}
	binary := []byte{0, 0xff, 0x80, 7}
	write.Content = base64.StdEncoding.EncodeToString(binary)
	if err := svc.WriteMCPFile(ctx, write, true); err != nil {
		t.Fatal(err)
	}
	result, err = svc.ReadMCPFile(ctx, read, "download")
	if err != nil || result.(map[string]any)["content"] != write.Content {
		t.Fatalf("binary roundtrip failed: %v", err)
	}
	if _, err := svc.ReadMCPFile(ctx, read, "read_file"); sshServiceCode(err) != "SSH_MCP_FILE_ENCODING" {
		t.Fatalf("invalid text accepted: %v", err)
	}
	if _, err := svc.ReadMCPFile(ctx, read, "stat"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadMCPFile(ctx, model.SSHMCPFileRequest{HostID: host.ID, Path: "."}, "list_dir"); err != nil {
		t.Fatal(err)
	}
	write.Content = strings.Repeat("x", sshMCPMaxContent+1)
	if err := svc.WriteMCPFile(ctx, write, false); sshServiceCode(err) != "SSH_MCP_FILE_LIMIT" {
		t.Fatalf("oversized write accepted: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte(write.Content), 0o600); err != nil {
		t.Fatal(err)
	}
	read.Path = "large.txt"
	if _, err := svc.ReadMCPFile(ctx, read, "read_file"); sshServiceCode(err) != "SSH_MCP_FILE_LIMIT" {
		t.Fatalf("oversized read accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := svc.ReadMCPFile(cancelled, read, "stat"); err == nil {
		t.Fatal("cancelled operation accepted")
	}
	if _, err := svc.UpdateHost(host.ID, model.SSHHostRequest{Name: host.Name, Host: host.Host, Port: host.Port, Username: host.Username, CredentialID: host.CredentialID, Enabled: false}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReadMCPFile(ctx, read, "stat"); sshServiceCode(err) != "SSH_HOST_DISABLED" {
		t.Fatalf("disabled host accepted: %v", err)
	}
	svc.sessionMu.Lock()
	defer svc.sessionMu.Unlock()
	if svc.pendingSessions != 0 || len(svc.mcpCancels) != 0 {
		t.Fatal("MCP operation leaked a slot or cancellation")
	}
}

func TestSSHMCPExecLimitsAndCancellation(t *testing.T) {
	svc, _ := newSSHHostShareTestService(t)
	for _, request := range []model.SSHMCPExecRequest{{}, {Command: "x\x00"}, {Command: "ok", Timeout: -1}, {Command: "ok", Timeout: 601}} {
		if _, err := svc.ExecuteMCP(context.Background(), request); sshServiceCode(err) != "SSH_MCP_INVALID" {
			t.Fatalf("invalid command accepted: %v", err)
		}
	}
	output := &sshMCPOutput{}
	data := bytes.Repeat([]byte("x"), sshMCPMaxContent+1)
	if n, err := output.Write(data); n != len(data) || err != nil || !output.truncated || output.buffer.Len() != sshMCPMaxContent {
		t.Fatal("command output was not bounded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	svc.sessionMu.Lock()
	svc.mcpCancels = map[context.Context]context.CancelFunc{ctx: cancel}
	svc.sessionMu.Unlock()
	svc.Close()
	if ctx.Err() == nil {
		t.Fatal("closing service did not cancel MCP operations")
	}
}

func TestSSHMCPExecRemoteResults(t *testing.T) {
	server := newConfiguredTestSSHServer(t, testSSHServerOptions{password: "test-password", exec: func(channel ssh.Channel, command string) {
		status := uint32(0)
		switch command {
		case "success":
			_, _ = io.WriteString(channel, "stdout fixture")
			_, _ = io.WriteString(channel.Stderr(), "stderr fixture")
		case "failure":
			status = 7
			_, _ = io.WriteString(channel.Stderr(), "command failed")
		case "large":
			_, _ = io.WriteString(channel, strings.Repeat("x", sshMCPMaxContent+100))
		case "wait":
			<-time.After(2 * time.Second)
			return
		}
		_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{status}))
	}})
	t.Cleanup(server.Close)
	svc, _ := newSSHHostShareTestService(t)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "fixture", AuthType: "password", Secret: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{Name: "fixture", Host: "127.0.0.1", Port: server.Port(), Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	for _, command := range []string{"success", "failure", "large", "wait"} {
		t.Run(command, func(t *testing.T) {
			started := time.Now()
			result, err := svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: host.ID, Command: command, Timeout: 1})
			switch command {
			case "success":
				if err != nil || result.ExitCode != 0 || result.Stdout != "stdout fixture" || result.Stderr != "stderr fixture" {
					t.Fatalf("unexpected result: %#v, %v", result, err)
				}
			case "failure":
				if sshServiceCode(err) != "SSH_MCP_COMMAND_FAILED" || result.ExitCode != 7 || result.Stderr != "command failed" {
					t.Fatalf("nonzero exit lost: %#v, %v", result, err)
				}
			case "large":
				if err != nil || !result.Truncated || len(result.Stdout) != sshMCPMaxContent {
					t.Fatalf("output limit failed: %v", err)
				}
			case "wait":
				if sshServiceCode(err) != "SSH_MCP_CANCELLED" || time.Since(started) > 5*time.Second {
					t.Fatalf("timeout did not close SSH: %v", err)
				}
			}
		})
	}
}
