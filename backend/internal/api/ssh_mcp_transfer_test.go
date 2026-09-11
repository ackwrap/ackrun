package api

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

func addMCPTransferHost(t *testing.T, hosts *service.SSHHostService, root string) int64 {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				server, channels, requests, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer server.Close()
				go ssh.DiscardRequests(requests)
				for incoming := range channels {
					if incoming.ChannelType() != "session" {
						_ = incoming.Reject(ssh.UnknownChannelType, "unsupported")
						continue
					}
					channel, requests, err := incoming.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for request := range requests {
							var payload struct{ Name string }
							valid := request.Type == "subsystem" && ssh.Unmarshal(request.Payload, &payload) == nil && payload.Name == "sftp"
							_ = request.Reply(valid, nil)
							if valid {
								files, err := sftp.NewServer(channel, sftp.WithServerWorkingDirectory(root))
								if err == nil {
									_ = files.Serve()
									_ = files.Close()
								}
								return
							}
						}
					}()
				}
			}()
		}
	}()
	credential, err := hosts.CreateCredential(model.SSHCredentialRequest{Name: "transfer", AuthType: "password", Secret: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := hosts.CreateHost(model.SSHHostRequest{Name: "transfer", Host: "127.0.0.1", Port: listener.Addr().(*net.TCPAddr).Port, Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	_, err = hosts.TestHost(context.Background(), host.ID)
	var serviceErr *service.SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_UNKNOWN" {
		t.Fatalf("expected host key challenge: %v", err)
	}
	challenge := serviceErr.Details.(*model.SSHHostKeyChallenge)
	_, err = hosts.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256}, false)
	if err != nil {
		t.Fatal(err)
	}
	return host.ID
}

func TestSSHMCPHTTPFileStreaming(t *testing.T) {
	router, settings, hosts, token := newMCPTestRouter(t)
	root := t.TempDir()
	hostID := addMCPTransferHost(t, hosts, root)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "file-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + service.SSHMCPEndpoint, HTTPClient: &http.Client{Transport: mcpTokenTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, name := range []string{"ssh_upload", "ssh_download"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{"host_id": hostID, "path": "package.bin"}})
		if err != nil || result.IsError {
			t.Fatalf("prepare %s: %v", name, err)
		}
		var payload struct {
			Data struct {
				Status string `json:"status"`
				Method string `json:"method"`
				Path   string `json:"endpoint_path"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &payload); err != nil || payload.Data.Status != "ready" || payload.Data.Path != service.SSHMCPEndpoint+"/files/"+strconv.FormatInt(hostID, 10) {
			t.Fatalf("invalid transfer descriptor: %v", err)
		}
		if (name == "ssh_upload" && payload.Data.Method != "PUT") || (name == "ssh_download" && payload.Data.Method != "GET") {
			t.Fatal("invalid transfer method")
		}
	}
	content := bytes.Repeat([]byte{0, 255, 2, 3}, 2<<20)
	hash := sha256.Sum256(content)
	endpoint := server.URL + service.SSHMCPEndpoint + "/files/" + strconv.FormatInt(hostID, 10)
	for _, chunked := range []bool{false, true} {
		var body io.Reader = bytes.NewReader(content)
		if chunked {
			body = struct{ io.Reader }{body}
		}
		request, _ := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, body)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-SSH-Path", "package.bin")
		request.Header.Set("X-SSH-Overwrite", "true")
		request.Header.Set("X-SSH-SHA256", hex.EncodeToString(hash[:]))
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		var result model.SSHMCPTransferResult
		err = json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || err != nil || !result.Success || result.Bytes != int64(len(content)) || result.SHA256 != hex.EncodeToString(hash[:]) {
			t.Fatalf("8 MiB HTTP upload chunked=%v status=%d result=%+v err=%v", chunked, response.StatusCode, result, err)
		}
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-SSH-Path", "package.bin")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	downloaded := sha256.New()
	n, err := io.Copy(downloaded, response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || response.ContentLength != int64(len(content)) || n != int64(len(content)) || !bytes.Equal(downloaded.Sum(nil), hash[:]) {
		t.Fatalf("HTTP download: bytes=%d err=%v", n, err)
	}

	request, _ = http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader("damaged"))
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-SSH-Path", "package.bin")
	request.Header.Set("X-SSH-Overwrite", "true")
	request.Header.Set("X-SSH-SHA256", hex.EncodeToString(hash[:]))
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("checksum failure status = %d", response.StatusCode)
	}
	preserved, err := os.ReadFile(filepath.Join(root, "package.bin"))
	if err != nil || !bytes.Equal(preserved, content) {
		t.Fatalf("failed HTTP upload changed original: %v", err)
	}
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	request, _ = http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked transfer token status = %d", response.StatusCode)
	}
}

func TestSSHMCPHTTPUploadStopsWithSSHService(t *testing.T) {
	router, _, hosts, token := newMCPTestRouter(t)
	root := t.TempDir()
	hostID := addMCPTransferHost(t, hosts, root)
	if err := os.WriteFile(filepath.Join(root, "target"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	request, _ := http.NewRequestWithContext(ctx, http.MethodPut, server.URL+service.SSHMCPEndpoint+"/files/"+strconv.FormatInt(hostID, 10), reader)
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("X-SSH-Path", "target")
	request.Header.Set("X-SSH-Overwrite", "true")
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		response, err := http.DefaultClient.Do(request)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				t.Error("interrupted upload reported success")
			}
		}
	}()
	if _, err := writer.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		files, _ := filepath.Glob(filepath.Join(root, ".ackwrap-mcp-*"))
		if len(files) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upload did not reach SFTP")
		}
		time.Sleep(10 * time.Millisecond)
	}
	hosts.Close()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("HTTP upload remained blocked after SSH service shutdown")
	}
	data, _ := os.ReadFile(filepath.Join(root, "target"))
	if string(data) != "original" {
		t.Fatal("interrupted HTTP upload replaced original")
	}
}
