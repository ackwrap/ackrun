package mcpserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAckrunToolMappingAndErrorResults(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer synthetic-admin" {
			t.Error("internal API authentication missing")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/node-groups/reorder", "/api/v1/dns/servers/reorder", "/api/v1/dns/rules/reorder":
			body, _ := io.ReadAll(r.Body)
			if r.Method != http.MethodPost || string(body) != "[2,1]" {
				t.Errorf("ordering request did not use an ID array: %s", body)
			}
			_, _ = io.WriteString(w, `{"success":true}`)
		case "/api/v1/rules/geo/lookup":
			if r.URL.Query().Get("target") != "example.invalid" || r.URL.Query().Get("dns_server") != "192.0.2.53" {
				t.Error("Geo lookup query not forwarded")
			}
			_, _ = io.WriteString(w, `{}`)
		case "/api/v1/nodes":
			if r.URL.Query().Get("enabled") != "false" || r.URL.Query().Get("subscription_id") != "9007199254740991" {
				t.Error("query lost explicit false or integer precision")
			}
			_, _ = io.WriteString(w, `{"items":[{"name":"fixture","raw":"synthetic-secret","raw_json":"synthetic-secret"}]}`)
		case "/api/v1/config/generate":
			_, _ = io.WriteString(w, `{"valid":false,"error":"synthetic check failed","config":{"password":"synthetic-secret","uuid":"synthetic-uuid"}}`)
		case "/api/v1/node-groups/9999":
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{"error":{"code":"NOT_FOUND","message":"fixture missing"}}`)
		default:
			t.Errorf("unexpected API route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, `{}`)
		}
	})
	server := NewAckrunServer(api, "synthetic-admin")
	httpServer := httptest.NewServer(mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "mapping-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: httpServer.URL, MaxRetries: -1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	for _, tc := range []struct {
		name      string
		args      map[string]any
		wantError bool
	}{
		{"reorder_node_groups", map[string]any{"ids": []int64{2, 1}}, false},
		{"reorder_dns_servers", map[string]any{"ids": []int64{2, 1}}, false},
		{"reorder_dns_rules", map[string]any{"ids": []int64{2, 1}}, false},
		{"lookup_geo", map[string]any{"target": "example.invalid", "dns_server": "192.0.2.53"}, false},
		{"list_nodes", map[string]any{"enabled": false, "subscription_id": int64(9007199254740991)}, false},
		{"generate_config", map[string]any{}, true},
		{"get_node_group", map[string]any{"id": 9999}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ackrun_" + tc.name, Arguments: tc.args})
			if err != nil || result.IsError != tc.wantError {
				t.Fatalf("tool result mismatch: err=%v", err)
			}
			encoded, _ := json.Marshal(result)
			if strings.Contains(string(encoded), "synthetic-secret") || strings.Contains(string(encoded), "synthetic-uuid") {
				t.Fatal("ordinary tool response exposed credentials")
			}
		})
	}
	if _, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ackrun_list_nodes", Arguments: map[string]any{"subscription_id": int64(9007199254740993)}}); err == nil {
		t.Fatal("an ID outside the exact JSON integer range was accepted")
	}
}

func TestAckrunAPIResponseLimitsAndContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"oversize", strings.Repeat("x", (4<<20)+1)},
		{"non JSON", "unexpected html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			executor := apiExecutor{handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, tc.body) })}
			if _, _, err := executor.call(context.Background(), http.MethodGet, apiCall{path: "/runtime"}); err == nil {
				t.Fatal("unbounded or invalid response accepted")
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	executor := apiExecutor{handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("canceled request dispatched") })}
	if _, _, err := executor.call(ctx, http.MethodGet, apiCall{path: "/runtime"}); err == nil {
		t.Fatal("cancellation ignored")
	}
}
