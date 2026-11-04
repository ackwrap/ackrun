package service

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func enableTestMCPLocal(t *testing.T, svc *SSHHostService) *SSHMCPService {
	t.Helper()
	settings := NewSSHMCPService(svc.store)
	local := true
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, LocalEnabled: &local, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	return settings
}

func TestSSHMCPBuiltinLocalTargetLifecycle(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	ctx := context.Background()
	request := model.SSHMCPExecRequest{HostID: SSHMCPLocalHostID, Command: "exit 0"}
	if _, err := svc.ExecuteMCP(ctx, request); sshServiceCode(err) != "SSH_MCP_LOCAL_DISABLED" {
		t.Fatalf("local target enabled without opt-in: %v", err)
	}
	hosts, err := svc.ListMCPHosts()
	if err != nil || len(hosts) != 0 {
		t.Fatalf("disabled local target listed: %v", err)
	}
	settings := enableTestMCPLocal(t, svc)
	hosts, err = svc.ListMCPHosts()
	if err != nil || len(hosts) != 1 || hosts[0].ID != SSHMCPLocalHostID || hosts[0].ConnectionMode != "local" || hosts[0].CredentialID != 0 || hosts[0].HostKeyStatus != "not_required" {
		t.Fatalf("local target missing: %+v err=%v", hosts, err)
	}
	if result, err := svc.TestMCPHost(ctx, SSHMCPLocalHostID); err != nil || !result.Success {
		t.Fatalf("local target test: %v", err)
	}
	for _, table := range []string{"ssh_hosts", "ssh_credentials", "ssh_host_keys", "ssh_sftp_profiles"} {
		var count int
		if err := db.DB().QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("local target created SSH records in %s: %d %v", table, count, err)
		}
	}
	var auditCount int
	if err := db.DB().QueryRow("SELECT COUNT(*) FROM ssh_session_audits WHERE host_id IS NULL AND connection_mode = 'local' AND result = 'success'").Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("local operation audit missing: %d %v", auditCount, err)
	}
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if cfg, err := NewSSHMCPService(db).Settings(); err != nil || !cfg.LocalEnabled {
		t.Fatal("omitted local setting was not preserved")
	}
	ticket, err := settings.PrepareUpload(model.SSHMCPTransferRequest{HostID: SSHMCPLocalHostID, Path: filepath.Join(t.TempDir(), "unused.bin")})
	if err != nil {
		t.Fatal(err)
	}
	local := false
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, LocalEnabled: &local}); err != nil {
		t.Fatal(err)
	}
	id := filepath.Base(ticket.EndpointPath)
	if _, err := settings.ClaimUpload(id, strings.TrimPrefix(ticket.Headers["Authorization"], "Bearer ")); err == nil {
		t.Fatal("local upload ticket survived revocation")
	}
	if _, err := svc.ExecuteMCP(ctx, request); sshServiceCode(err) != "SSH_MCP_LOCAL_DISABLED" {
		t.Fatalf("disabled local target still executed: %v", err)
	}
}

func TestSSHMCPBuiltinLocalExecution(t *testing.T) {
	svc, _ := newSSHHostShareTestService(t)
	enableTestMCPLocal(t, svc)
	command := "printf '本机输出'; printf '本机错误' >&2; exit 7"
	if runtime.GOOS == "windows" {
		command = "[Console]::Out.Write('本机输出'); [Console]::Error.Write('本机错误'); exit 7"
	}
	result, err := svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: SSHMCPLocalHostID, Command: command})
	if sshServiceCode(err) != "SSH_MCP_COMMAND_FAILED" || result.ExitCode != 7 || result.Stdout != "本机输出" || result.Stderr != "本机错误" {
		t.Fatalf("local command output: %+v err=%v", result, err)
	}
	command = "sleep 10"
	if runtime.GOOS == "windows" {
		command = "Start-Sleep -Seconds 10"
	}
	started := time.Now()
	_, err = svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: SSHMCPLocalHostID, Command: command, Timeout: 1})
	if sshServiceCode(err) != "SSH_MCP_CANCELLED" || time.Since(started) > 5*time.Second {
		t.Fatalf("local command timeout: %v after %s", err, time.Since(started))
	}
	command = "printf local-ok"
	if runtime.GOOS == "windows" {
		command = "[Console]::Out.Write('local-ok')"
	}
	result, err = svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: SSHMCPLocalHostID, Command: command})
	if err != nil || result.ExitCode != 0 || result.Stdout != "local-ok" {
		t.Fatalf("local command success: %+v err=%v", result, err)
	}
}

func TestSSHMCPBuiltinLocalFilesAndStreaming(t *testing.T) {
	svc, _ := newSSHHostShareTestService(t)
	enableTestMCPLocal(t, svc)
	ctx := context.Background()
	root := t.TempDir()
	name := filepath.Join(root, "text.txt")
	write := model.SSHMCPWriteRequest{HostID: SSHMCPLocalHostID, Path: name, Content: "本机文本"}
	if err := svc.WriteMCPFile(ctx, write, false); err != nil {
		t.Fatal(err)
	}
	read := model.SSHMCPFileRequest{HostID: SSHMCPLocalHostID, Path: name}
	value, err := svc.ReadMCPFile(ctx, read, "read_file")
	if err != nil || value.(map[string]any)["content"] != write.Content {
		t.Fatalf("local text read: %v", err)
	}
	value, err = svc.ReadMCPFile(ctx, read, "stat")
	if err != nil || value.(map[string]any)["size"] != int64(len(write.Content)) {
		t.Fatalf("local stat: %v", err)
	}
	value, err = svc.ReadMCPFile(ctx, model.SSHMCPFileRequest{HostID: SSHMCPLocalHostID, Path: root}, "list_dir")
	if err != nil || len(value.(map[string]any)["entries"].([]map[string]any)) != 1 {
		t.Fatalf("local list: %v", err)
	}
	payload := bytes.Repeat([]byte{0, 1, 255, 7}, 600<<10)
	digest := sha1.Sum(payload)
	upload := model.SSHMCPTransferRequest{HostID: SSHMCPLocalHostID, Path: filepath.Join(root, "binary.bin"), SHA1: hex.EncodeToString(digest[:])}
	result, err := svc.UploadMCPStream(ctx, upload, io.NopCloser(bytes.NewReader(payload)), int64(len(payload)))
	if err != nil || !result.Success || result.Bytes != int64(len(payload)) || result.SHA1 != upload.SHA1 {
		t.Fatalf("local stream upload: %+v %v", result, err)
	}
	var downloaded bytes.Buffer
	err = svc.DownloadMCPStream(ctx, model.SSHMCPFileRequest{HostID: SSHMCPLocalHostID, Path: upload.Path}, func(ctx context.Context, content io.Reader, info os.FileInfo) error {
		_, err := io.Copy(&downloaded, content)
		return err
	})
	if err != nil || !bytes.Equal(payload, downloaded.Bytes()) {
		t.Fatalf("local stream download: %v", err)
	}
	if _, err := svc.ReadMCPFile(ctx, model.SSHMCPFileRequest{HostID: SSHMCPLocalHostID, Path: upload.Path}, "read_file"); sshServiceCode(err) != "SSH_MCP_FILE_LIMIT" {
		t.Fatalf("large local file bypassed inline limit: %v", err)
	}
	if _, err := svc.UploadMCPStream(ctx, upload, io.NopCloser(strings.NewReader("replacement")), -1); sshServiceCode(err) != "SSH_MCP_FILE_EXISTS" {
		t.Fatalf("local overwrite without consent: %v", err)
	}
	upload.Overwrite = true
	for _, test := range []struct {
		name string
		size int64
		code string
	}{
		{"checksum", 11, "SSH_MCP_CHECKSUM_MISMATCH"},
		{"length", 99, "SSH_MCP_TRANSFER_INCOMPLETE"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if result, err := svc.UploadMCPStream(ctx, upload, io.NopCloser(strings.NewReader("replacement")), test.size); sshServiceCode(err) != test.code || result.Success {
				t.Fatalf("local failure: %+v %v", result, err)
			}
			current, err := os.ReadFile(upload.Path)
			if err != nil || !bytes.Equal(payload, current) {
				t.Fatal("failed upload replaced existing file")
			}
		})
	}
	upload.SHA1 = ""
	if result, err := svc.UploadMCPStream(ctx, upload, io.NopCloser(strings.NewReader("replacement")), 11); err != nil || !result.Success {
		t.Fatalf("local overwrite: %v", err)
	}
	if err := svc.WriteMCPFile(ctx, model.SSHMCPWriteRequest{HostID: SSHMCPLocalHostID, Path: root, Content: "invalid", Overwrite: true}, false); sshServiceCode(err) != "SSH_MCP_FILE_EXISTS" {
		t.Fatalf("local directory overwritten: %v", err)
	}
	entries, err := filepath.Glob(filepath.Join(root, ".ackwrap-mcp-*"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("local temporary files leaked: %v", err)
	}
}

func TestSSHMCPBuiltinLocalPaths(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"", ".", "folder/../file.txt"} {
		actual, err := normalizeMCPPath(SSHMCPLocalHostID, path, true)
		if err != nil || actual != filepath.Join(home, path) {
			t.Fatalf("local relative path: %q %v", actual, err)
		}
	}
	root := filepath.VolumeName(home) + string(filepath.Separator)
	for _, path := range []string{"", root, "invalid\x00path", "invalid\npath"} {
		if _, err := normalizeMCPPath(SSHMCPLocalHostID, path, false); err == nil {
			t.Fatalf("accepted invalid local file path %q", path)
		}
	}
	if actual, err := normalizeMCPPath(1, "/remote/path", false); err != nil || actual != "/remote/path" {
		t.Fatalf("remote path changed: %q %v", actual, err)
	}
}

func TestSSHMCPBuiltinLocalUploadCancellation(t *testing.T) {
	svc, _ := newSSHHostShareTestService(t)
	enableTestMCPLocal(t, svc)
	root := t.TempDir()
	name := filepath.Join(root, "preserved.txt")
	if err := os.WriteFile(name, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	source, writer := io.Pipe()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	result, err := svc.UploadMCPStream(ctx, model.SSHMCPTransferRequest{HostID: SSHMCPLocalHostID, Path: name, Overwrite: true}, source, -1)
	if sshServiceCode(err) != "SSH_MCP_CANCELLED" || result.Success {
		t.Fatalf("local cancellation: %+v %v", result, err)
	}
	current, err := os.ReadFile(name)
	if err != nil || string(current) != "original" {
		t.Fatal("cancelled upload changed destination")
	}
	entries, err := filepath.Glob(filepath.Join(root, ".ackwrap-mcp-*"))
	if err != nil || len(entries) != 0 {
		t.Fatal("cancelled upload leaked temporary file")
	}
	if _, err := writer.Write([]byte("closed")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal("cancelled upload did not close its input")
	}
}
