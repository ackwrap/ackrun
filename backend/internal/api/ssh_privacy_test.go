package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

func TestSSHPrivacyRESTAndMCP(t *testing.T) {
	router, _, hosts, token := newMCPTestRouter(t)
	registerSSHHostRoutes(router.Group("/api/v1"), handler.NewSSHHostHandler(hosts))
	credential, err := hosts.CreateCredential(model.SSHCredentialRequest{Name: "fixture", AuthType: "password", Secret: "synthetic-secret"})
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, path, body, auth, origin string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, "http://127.0.0.1:8080/api/v1/advanced/ssh"+path, strings.NewReader(body))
		req.RemoteAddr = "127.0.0.1:12345"
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+auth)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		res := httptest.NewRecorder()
		router.ServeHTTP(res, req)
		return res
	}
	const admin = "test-admin-token"
	const address = "2001:db8::53"
	host, err := hosts.CreateHost(model.SSHHostRequest{Name: "fixture", Host: address, Port: 22, Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		body, auth, origin string
		status             int
	}{
		{`{"hide_host_address":true}`, "", "", http.StatusUnauthorized},
		{`{"hide_host_address":true}`, token, "", http.StatusUnauthorized},
		{`{"hide_host_address":true}`, admin, "http://evil.example", http.StatusForbidden},
		{`{}`, admin, "", http.StatusBadRequest},
		{`{"hide_host_address":"true"}`, admin, "", http.StatusBadRequest},
		{`{"hide_host_address":true}`, admin, "", http.StatusOK},
	} {
		response := request(http.MethodPut, "/settings", test.body, test.auth, test.origin)
		if response.Code != test.status {
			t.Fatalf("settings status = %d, want %d", response.Code, test.status)
		}
	}
	for _, path := range []string{"/hosts", fmt.Sprintf("/hosts/%d", host.ID)} {
		response := request(http.MethodGet, path, "", admin, "")
		if response.Code != http.StatusOK || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("host response status/cache: %d", response.Code)
		}
		assertSSHPrivacyPayload(t, response.Body.Bytes(), address)
	}
	server := httptest.NewServer(router)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "privacy-test", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + service.SSHMCPEndpoint, HTTPClient: &http.Client{Transport: mcpTokenTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	config := map[string]any{"name": "MCP updated", "group_name": "", "host": "", "keep_host_address": true, "port": 22, "username": "tester", "credential_id": credential.ID, "connection_mode": "direct", "node_exposure_id": nil, "terminal_type": "xterm-256color", "enabled": true, "tags": []string{}, "notes": ""}
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"ssh_list_servers", map[string]any{}},
		{"ssh_update_server", map[string]any{"host_id": host.ID, "config": config}},
		{"ssh_add_server", map[string]any{"name": "MCP created", "group_name": "", "host": address, "port": 22, "username": "tester", "credential_id": credential.ID, "connection_mode": "direct", "node_exposure_id": nil, "terminal_type": "xterm-256color", "enabled": true, "tags": []string{}, "notes": ""}},
	} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v", call.name, err)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		// Inspect both text content and structuredContent.
		if bytes.Contains(encoded, []byte(address)) || !bytes.Contains(encoded, []byte("2001:0db8:*")) {
			t.Fatalf("%s exposed/missed the masked address", call.name)
		}
	}
	configJSON, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	response := request(http.MethodPut, fmt.Sprintf("/hosts/%d", host.ID), string(configJSON), admin, "")
	if response.Code != http.StatusOK {
		t.Fatalf("masked REST update status = %d", response.Code)
	}
	assertSSHPrivacyPayload(t, response.Body.Bytes(), address)
	if response := request(http.MethodPut, "/settings", `{"hide_host_address":false}`, admin, ""); response.Code != http.StatusOK {
		t.Fatalf("disable hiding status = %d", response.Code)
	}
	response = request(http.MethodGet, fmt.Sprintf("/hosts/%d", host.ID), "", admin, "")
	var visible model.SSHHost
	if err := json.Unmarshal(response.Body.Bytes(), &visible); err != nil {
		t.Fatal(err)
	}
	if visible.Host != address || visible.HostHidden || visible.Name != "MCP updated" {
		t.Fatal("hidden updates did not preserve the stored address")
	}
	// Existing MCP sessions must observe setting changes without reconnecting.
	if _, err := hosts.UpdatePrivacySettings(model.SSHPrivacySettings{HideHostAddress: true}); err != nil {
		t.Fatal(err)
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ssh_list_servers", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("list after privacy change failed: %v", err)
	}
	encoded, err := json.Marshal(result)
	if err != nil || bytes.Contains(encoded, []byte(address)) {
		t.Fatal("existing MCP session ignored privacy setting change")
	}
}

func assertSSHPrivacyPayload(t *testing.T, payload []byte, address string) {
	t.Helper()
	if bytes.Contains(payload, []byte(address)) || !bytes.Contains(payload, []byte(`"host_hidden":true`)) {
		t.Fatal("REST response did not hide SSH address")
	}
}
