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
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

func TestSSHMCPBuiltinLocalHTTPTransfer(t *testing.T) {
	router, _, _, token := newMCPTestRouter(t)
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	setLocal := func(enabled bool) {
		t.Helper()
		body, _ := json.Marshal(map[string]bool{"enabled": true, "local_enabled": enabled})
		request, _ := http.NewRequestWithContext(ctx, http.MethodPut, server.URL+"/api/v1/advanced/ssh/mcp/settings", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer test-admin-token")
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var settings model.SSHMCPSettings
		if err := json.NewDecoder(response.Body).Decode(&settings); err != nil || response.StatusCode != http.StatusOK || settings.LocalEnabled != enabled {
			t.Fatalf("local settings HTTP update: %d %v", response.StatusCode, err)
		}
	}
	setLocal(true)
	client := mcp.NewClient(&mcp.Implementation{Name: "local-target-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + service.SSHMCPEndpoint, HTTPClient: &http.Client{Transport: mcpTokenTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(tool string, args map[string]any, target any) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil || result.IsError {
			t.Fatalf("local MCP %s failed: %v", tool, err)
		}
		if target != nil {
			var envelope struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &envelope); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(envelope.Data, target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var hosts []model.SSHHost
	call("ssh_list_servers", map[string]any{}, &hosts)
	if len(hosts) != 1 || hosts[0].ID != service.SSHMCPLocalHostID || hosts[0].ConnectionMode != "local" {
		t.Fatal("MCP did not discover built-in local target")
	}
	var execution model.SSHMCPExecResult
	call("ssh_exec", map[string]any{"host_id": service.SSHMCPLocalHostID, "command": "exit 0"}, &execution)
	if execution.ExitCode != 0 {
		t.Fatal("native local command failed")
	}
	name := filepath.Join(t.TempDir(), "native-upload.bin")
	content := bytes.Repeat([]byte("local-binary\x00"), 100<<10)
	digest := sha1.Sum(content)
	var ticket model.SSHMCPUploadTicket
	call("upload_file", map[string]any{"host_id": service.SSHMCPLocalHostID, "path": name, "sha1": hex.EncodeToString(digest[:])}, &ticket)
	put := func(auth bool) *http.Response {
		t.Helper()
		request, _ := http.NewRequestWithContext(ctx, ticket.Method, ticket.UploadURL, bytes.NewReader(content))
		for key, value := range ticket.Headers {
			if auth || key != "Authorization" {
				request.Header.Set(key, value)
			}
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response := put(false)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("local upload accepted missing temporary authorization: %d", response.StatusCode)
	}
	response = put(true)
	var uploaded model.SSHMCPTransferResult
	err = json.NewDecoder(response.Body).Decode(&uploaded)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !uploaded.Success || uploaded.Bytes != int64(len(content)) || uploaded.SHA1 != hex.EncodeToString(digest[:]) {
		t.Fatalf("native local PUT failed: %d %v", response.StatusCode, err)
	}
	actual, err := os.ReadFile(name)
	if err != nil || !bytes.Equal(content, actual) {
		t.Fatal("HTTP upload did not reach local filesystem")
	}
	var download struct {
		EndpointPath string `json:"endpoint_path"`
	}
	call("ssh_download", map[string]any{"host_id": service.SSHMCPLocalHostID, "path": name}, &download)
	get := func() *http.Response {
		t.Helper()
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+download.EndpointPath, nil)
		request.Header.Set("Authorization", "Bearer "+token)
		request.Header.Set("X-SSH-Path", name)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response
	}
	response = get()
	downloaded, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !bytes.Equal(content, downloaded) {
		t.Fatalf("native local GET failed: %d %v", response.StatusCode, err)
	}
	response = put(true)
	response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatal("local upload URL was reusable")
	}
	setLocal(false)
	response = get()
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("local GET survived disabling local target: %d", response.StatusCode)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ssh_exec", Arguments: map[string]any{"host_id": service.SSHMCPLocalHostID, "command": "exit 0"}})
	if err != nil || !result.IsError {
		t.Fatal("local command survived disabling local target")
	}
}
