package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func newMCPTestRouter(t *testing.T) (*gin.Engine, *service.SSHMCPService, *service.SSHHostService, string) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	hosts, err := service.NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(hosts.Close)
	settings := service.NewSSHMCPService(db)
	issued, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(SecurityMiddleware("test-admin-token"))
	RegisterSSHMCPRoutes(router, settings, hosts)
	return router, settings, hosts, issued.Token
}

func TestSSHMCPHTTPBoundary(t *testing.T) {
	router, _, _, token := newMCPTestRouter(t)
	for _, test := range []struct {
		name, peer, host, origin, auth, forwarded, query string
		status                                           int
	}{
		{name: "LAN", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", auth: token, status: 405},
		{name: "private IPv6", peer: "[fd00::42]:50000", host: "[fd00::1]:8080", auth: token, status: 405},
		{name: "mapped IPv4", peer: "[::ffff:10.0.0.2]:50000", host: "10.0.0.1:8080", auth: token, status: 405},
		{name: "public", peer: "203.0.113.42:50000", host: "192.168.1.1:8080", auth: token, status: 403},
		{name: "public IPv6", peer: "[2001:db8::42]:50000", host: "[fd00::1]:8080", auth: token, status: 403},
		{name: "rebinding host", peer: "192.168.1.42:50000", host: "evil.example:8080", auth: token, status: 403},
		{name: "cross origin", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", origin: "http://evil.example", auth: token, status: 403},
		{name: "same origin", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", origin: "http://192.168.1.1:8080", auth: token, status: 405},
		{name: "forwarded", peer: "127.0.0.1:50000", host: "192.168.1.1:8080", forwarded: "192.168.1.42", auth: token, status: 403},
		{name: "missing token", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", status: 401},
		{name: "admin token", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", auth: "test-admin-token", status: 401},
		{name: "query token", peer: "192.168.1.42:50000", host: "192.168.1.1:8080", query: "?token=synthetic", status: 400},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "http://"+test.host+service.SSHMCPEndpoint+test.query, nil)
			request.RemoteAddr = test.peer
			request.Header.Set("Accept", "text/event-stream")
			request.AddCookie(&http.Cookie{Name: apiTokenCookie, Value: "test-admin-token"})
			if test.auth != "" {
				request.Header.Set("Authorization", "Bearer "+test.auth)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.forwarded != "" {
				request.Header.Set("X-Forwarded-For", test.forwarded)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.status, response.Body.String())
			}
		})
	}
	for _, path := range []string{"/api/v1/advanced/ssh/mcp/settings", service.SSHMCPEndpoint} {
		request := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:8080"+path, nil)
		request.RemoteAddr = "127.0.0.1:50000"
		if strings.HasPrefix(path, "/api/") {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("missing/separate auth status = %d", response.Code)
		}
	}
}

type mcpTokenTransport struct{ token string }

func (transport mcpTokenTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func TestSSHMCPStreamableHTTPToolsAndRevocation(t *testing.T) {
	router, settings, hosts, token := newMCPTestRouter(t)
	credential, err := hosts.CreateCredential(model.SSHCredentialRequest{Name: "fixture", AuthType: "password", Secret: "synthetic-secret-not-in-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = hosts.CreateHost(model.SSHHostRequest{Name: "fixture", Host: "192.0.2.1", Port: 22, Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "fixture-client", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL + service.SSHMCPEndpoint, HTTPClient: &http.Client{Transport: mcpTokenTransport{token}}, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	listed, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for _, name := range []string{"ssh_list_servers", "ssh_list_credentials", "ssh_add_server", "ssh_update_server", "ssh_delete_server", "ssh_test_connection", "ssh_exec", "ssh_exec_multi", "ssh_read_file", "ssh_write_file", "ssh_upload", "ssh_download", "ssh_list_dir", "ssh_stat"} {
		want[name] = true
	}
	for _, tool := range listed.Tools {
		if !want[tool.Name] {
			t.Fatalf("unexpected tool: %s", tool.Name)
		}
		delete(want, tool.Name)
	}
	if len(want) != 0 {
		t.Fatalf("missing tools: %v", want)
	}
	for _, name := range []string{"ssh_list_servers", "ssh_list_credentials"} {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: map[string]any{}})
		if err != nil || result.IsError {
			t.Fatalf("%s failed: %v", name, err)
		}
		encoded, _ := json.Marshal(result)
		for _, forbidden := range []string{"synthetic-secret-not-in-mcp", "ciphertext", "token_hash"} {
			if bytes.Contains(encoded, []byte(forbidden)) {
				t.Fatal("MCP exposed secret material")
			}
		}
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ssh_exec", Arguments: map[string]any{"host_id": 9999, "command": "synthetic"}})
	if err != nil || !result.IsError {
		t.Fatalf("tool failure not returned as isError: %v", err)
	}
	settingsURL := server.URL + "/api/v1/advanced/ssh/mcp/settings"
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, settingsURL, nil)
	request.Header.Set("Authorization", "Bearer test-admin-token")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 200 || bytes.Contains(body, []byte(token)) || bytes.Contains(body, []byte("token_hash")) {
		t.Fatal("settings response failed or exposed token")
	}
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("existing client retained access after rotation")
	}
	if _, err := settings.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	request, _ = http.NewRequestWithContext(ctx, http.MethodPost, server.URL+service.SSHMCPEndpoint, strings.NewReader(`{}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("disabled MCP status = %d", response.StatusCode)
	}
}

func TestSSHMCPSettingsHTTP(t *testing.T) {
	router, _, _, mcpToken := newMCPTestRouter(t)
	endpoint := "http://127.0.0.1:8080/api/v1/advanced/ssh/mcp/settings"
	for _, test := range []struct {
		name, token, body, origin string
		status                    int
	}{
		{name: "unauthenticated", body: `{}`, status: 401},
		{name: "MCP token is not administrator", token: mcpToken, body: `{}`, status: 401},
		{name: "wrong origin", token: "test-admin-token", body: `{}`, origin: "http://evil.example", status: 403},
		{name: "short token", token: "test-admin-token", body: `{"enabled":true,"token":"short"}`, status: 400},
		{name: "large request", token: "test-admin-token", body: `{"token":"` + strings.Repeat("x", 4096) + `"}`, status: 400},
		{name: "generate", token: "test-admin-token", body: `{"enabled":true,"generate_token":true}`, status: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPut, endpoint, strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status = %d, want %d", response.Code, test.status)
			}
			if test.status == 200 {
				var result model.SSHMCPSettings
				if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Token) < 32 || !result.TokenConfigured {
					t.Fatal("new token missing from settings response")
				}
				if response.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("token response is cacheable")
				}
			}
		})
	}
}
