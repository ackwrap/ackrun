package application

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
)

type applicationMCPTransport struct{ token string }

func (transport applicationMCPTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	request = request.Clone(request.Context())
	request.Header.Set("Authorization", "Bearer "+transport.token)
	return http.DefaultTransport.RoundTrip(request)
}

func TestApplicationMCPManagesDraftsAndNodes(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("ACKWRAP_DATA_DIR", dataDir)
	t.Setenv("ACKWRAP_BINARY_DIR", dataDir+"/bin")
	p := paths.Default()
	app, err := New(Options{Paths: p, APIToken: "synthetic-admin-token"})
	if err != nil {
		t.Fatal(err)
	}
	defer app.Close()
	settings := service.NewMCPService(app.store)
	issued, err := settings.UpdateSettings(model.MCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(app.Handler())
	defer httpServer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "ackrun-fixture", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   httpServer.URL + "/mcp",
		HTTPClient: &http.Client{Transport: applicationMCPTransport{issued.Token}}, MaxRetries: -1,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) < 80 {
		t.Fatalf("tools discovery failed: %v", err)
	}
	for _, tool := range tools.Tools {
		if !strings.HasPrefix(tool.Name, "ackrun_") || tool.Annotations == nil || tool.InputSchema == nil {
			t.Fatalf("invalid tool metadata for %s", tool.Name)
		}
	}
	t.Logf("discovered %d Ackrun tools", len(tools.Tools))
	call := func(name string, args any, wantError bool, out any) {
		t.Helper()
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "ackrun_" + name, Arguments: args})
		if err != nil {
			t.Fatalf("%s protocol error: %v", name, err)
		}
		if result.IsError != wantError {
			t.Fatalf("%s IsError=%t, want %t: %v", name, result.IsError, wantError, result.Content)
		}
		if out != nil {
			encoded, _ := json.Marshal(result.StructuredContent)
			var payload struct {
				Data json.RawMessage `json:"data"`
			}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload.Data, out); err != nil {
				t.Fatalf("%s response decode: %v", name, err)
			}
		}
	}
	for _, name := range []string{"get_runtime", "list_subscriptions", "list_rules", "list_rule_strategies", "list_node_groups", "list_dns_servers", "get_config_generation", "get_general_settings", "list_node_entries"} {
		call(name, map[string]any{}, false, nil)
	}
	call("set_general_settings", map[string]any{"auto_start_core": false}, false, nil)
	var preview model.NodeImportPreviewResponse
	fixture := model.NodeImportRequest{Content: "trojan://synthetic-password@192.0.2.1:443?security=tls&sni=example.invalid#香港测试"}
	call("preview_node_import", fixture, false, &preview)
	if preview.Count != 1 {
		t.Fatal("import preview missing synthetic node")
	}
	call("import_nodes", fixture, false, nil)
	var nodes model.NodeListResponse
	call("list_nodes", map[string]any{"limit": 10}, false, &nodes)
	if len(nodes.Items) != 1 || nodes.Items[0].Raw != "" || nodes.Items[0].RawJSON != "" {
		t.Fatal("node metadata missing or raw credentials exposed")
	}
	node := nodes.Items[0]
	var groups []model.NodeGroupWithStats
	call("list_node_groups", map[string]any{}, false, &groups)
	var groupID int64
	for _, group := range groups {
		if group.MatchedNodeCount > 0 {
			groupID = group.ID
			break
		}
	}
	if groupID == 0 {
		t.Fatal("manual import did not create a matching region group")
	}
	call("set_node_preferred", map[string]any{"uid": node.UID, "value": true}, false, nil)
	call("set_node_enabled", map[string]any{"uid": "../subscriptions", "value": true}, true, nil)
	call("share_node", map[string]any{"id": node.ID}, false, nil)
	call("get_node_group", map[string]any{"id": -1}, true, nil)

	ruleReq := model.RouteRuleRequest{Name: "fixture-rule", Enabled: true, RuleType: "domain", Values: []string{"example.invalid"}, Outbound: "proxy"}
	call("create_rule", ruleReq, true, nil)
	ruleReq.Outbound = " proxy "
	call("create_rule", ruleReq, true, nil)
	ruleReq.Outbound = "proxy"
	ruleReq.Strategy = &model.ProxyCollectionRequest{Type: "selector", SourceType: "node_groups", ReferencedGroupIDs: []int64{groupID}, Enabled: true}
	var rule model.RouteRule
	call("create_rule", ruleReq, false, &rule)
	ruleReq.Values = []string{"updated.example.invalid"}
	call("update_rule", map[string]any{"id": rule.ID, "config": ruleReq}, false, nil)
	if _, err := os.Stat(p.ConfigPath); !os.IsNotExist(err) {
		t.Fatalf("rule draft generated an active config: %v", err)
	}
	call("delete_rule", map[string]any{"id": rule.ID}, false, nil)
	call("create_dns_host", model.DNSHostRequest{Domain: "fixture.invalid", Addresses: []string{"192.0.2.4"}, Enabled: true}, false, nil)
	call("set_core_restart", model.CoreRestartSettings{Mode: "off", Time: "04:00"}, false, nil)
	call("delete_nodes", model.NodeUIDsRequest{UIDs: []string{node.UID}}, false, nil)
	call("list_nodes", map[string]any{}, false, &nodes)
	if nodes.Total != 0 {
		t.Fatal("node deletion did not persist")
	}
	if _, err := settings.UpdateSettings(model.MCPSettingsRequest{Enabled: true, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := session.ListTools(ctx, nil); err == nil {
		t.Fatal("old MCP token remained valid after rotation")
	}
}
