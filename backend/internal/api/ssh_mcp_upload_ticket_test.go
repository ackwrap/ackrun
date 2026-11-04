package api

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

func TestSSHMCPUniqueUploadURL(t *testing.T) {
	router, settings, hosts, mcpToken := newMCPTestRouter(t)
	remoteRoot := t.TempDir()
	hostID := addMCPTransferHost(t, hosts, remoteRoot)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "upload-url-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + service.SSHMCPEndpoint, HTTPClient: &http.Client{Transport: mcpTokenTransport{mcpToken}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, tool := range []string{"upload_file", "ssh_upload"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"host_id": hostID, "path": "package.bin", "sha256": strings.Repeat("a", 64)}})
		if err != nil && !strings.Contains(err.Error(), "unexpected additional properties") {
			t.Fatalf("%s rejected sha256 for an unexpected reason: %v", tool, err)
		}
		if err == nil && !result.IsError {
			t.Fatalf("%s silently accepted the removed sha256 parameter", tool)
		}
	}
	prepare := func(tool, checksum string) model.SSHMCPUploadTicket {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: map[string]any{"host_id": hostID, "path": "package.bin", "overwrite": true, "sha1": checksum}})
		if err != nil || result.IsError {
			t.Fatalf("prepare %s: %v", tool, err)
		}
		var payload struct {
			Data model.SSHMCPUploadTicket `json:"data"`
		}
		if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &payload); err != nil {
			t.Fatal(err)
		}
		ticket := payload.Data
		if ticket.UploadURL != server.URL+ticket.EndpointPath || !strings.HasPrefix(ticket.EndpointPath, service.SSHMCPEndpoint+"/uploads/") || ticket.Method != "PUT" || !ticket.SingleUse || ticket.ExpiresAt <= time.Now().UnixMilli() {
			t.Fatal("invalid absolute upload URL or ticket metadata")
		}
		if ticket.Headers["Authorization"] == "Bearer "+mcpToken || ticket.Headers["Authorization"] == "" {
			t.Fatal("upload must use a separate temporary credential")
		}
		return ticket
	}
	put := func(ticket model.SSHMCPUploadTicket, content io.Reader) *http.Response {
		t.Helper()
		request, err := http.NewRequestWithContext(ctx, http.MethodPut, ticket.UploadURL, content)
		if err != nil {
			t.Fatal(err)
		}
		for name, value := range ticket.Headers {
			request.Header.Set(name, value)
		}
		// Ticket parameters are fixed; client headers cannot redirect the write.
		request.Header.Set("X-SSH-Path", "must-not-exist.bin")
		request.Header.Set("X-SSH-Overwrite", "false")
		request.Header.Set("X-SSH-SHA1", "ignored")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	content := bytes.Repeat([]byte{0, 255, 3, 128}, 2<<20)
	hash := sha1.Sum(content)
	checksum := hex.EncodeToString(hash[:])
	ticket := prepare("upload_file", checksum)
	for _, test := range []struct {
		name, peer, origin, forwarded, authorization, query, contentType string
		status                                                           int
	}{
		{name: "public peer", peer: "203.0.113.42:1234", status: 403},
		{name: "cross origin", origin: "http://evil.example", status: 403},
		{name: "forwarded", forwarded: "192.168.1.2", status: 403},
		{name: "query", query: "?token=invalid", status: 400},
		{name: "MCP token is not upload token", authorization: "Bearer " + mcpToken, status: 401},
		{name: "wrong token", authorization: "Bearer invalid", status: 401},
		{name: "multipart is not raw file", contentType: "multipart/form-data; boundary=example", status: 415},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, ticket.UploadURL+test.query, strings.NewReader("invalid"))
			request.RemoteAddr = "127.0.0.1:1234"
			if test.peer != "" {
				request.RemoteAddr = test.peer
			}
			for name, value := range ticket.Headers {
				request.Header.Set(name, value)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-For", test.forwarded)
			}
			if test.authorization != "" {
				request.Header.Set("Authorization", test.authorization)
			}
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
		})
	}
	// The source belongs to the caller; only bytes cross HTTP to Ackwrap.
	localPath := filepath.Join(t.TempDir(), "local-package.bin")
	if err := os.WriteFile(localPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	for _, chunked := range []bool{false, true} {
		if chunked {
			ticket = prepare("ssh_upload", checksum)
		}
		file, err := os.Open(localPath)
		if err != nil {
			t.Fatal(err)
		}
		var reader io.Reader = file
		if !chunked {
			reader = bytes.NewReader(content)
		}
		response := put(ticket, reader)
		var result model.SSHMCPTransferResult
		var payload map[string]json.RawMessage
		body, err := io.ReadAll(response.Body)
		if err == nil {
			err = json.Unmarshal(body, &result)
		}
		if err == nil {
			err = json.Unmarshal(body, &payload)
		}
		_ = response.Body.Close()
		_ = file.Close()
		if _, oldDigest := payload["sha256"]; oldDigest || len(payload["sha1"]) == 0 {
			t.Fatal("upload response must expose sha1 instead of sha256")
		}
		if err != nil || response.StatusCode != http.StatusOK || !result.Success || result.Bytes != int64(len(content)) || result.SHA1 != checksum || response.Header.Get("Cache-Control") != "no-store" {
			t.Fatalf("binary upload chunked=%v: status=%d err=%v", chunked, response.StatusCode, err)
		}
		response = put(ticket, strings.NewReader("replay"))
		_ = response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Fatal("upload URL replay accepted")
		}
	}
	actual, err := os.ReadFile(filepath.Join(remoteRoot, "package.bin"))
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("remote file mismatch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(remoteRoot, "must-not-exist.bin")); !os.IsNotExist(err) {
		t.Fatal("client overrode the ticket destination")
	}
	badChecksum := prepare("upload_file", strings.Repeat("0", 40))
	response := put(badChecksum, strings.NewReader("damaged"))
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnprocessableEntity {
		t.Fatal("checksum failure was not reported")
	}
	actual, err = os.ReadFile(filepath.Join(remoteRoot, "package.bin"))
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatal("failed upload replaced the original")
	}
	response = put(badChecksum, strings.NewReader("retry"))
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("failed attempt retained its upload authorization")
	}
	revoked := prepare("upload_file", checksum)
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	response = put(revoked, strings.NewReader("revoked"))
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("token rotation did not revoke upload URL")
	}
}
