package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/nodeidentity"
)

func parseChainTestNodes(t *testing.T, body string) []model.ParsedNode {
	t.Helper()
	nodes, err := ParseSubscriptionNodes([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func parsedNodeChain(t *testing.T, node model.ParsedNode) *model.NodeChain {
	t.Helper()
	var options map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &options); err != nil {
		t.Fatal(err)
	}
	chain, err := model.DecodeNodeChain(options)
	if err != nil || chain == nil || options["detour"] != nil {
		t.Fatalf("expected a resolved chain, got error=%v", err)
	}
	if node.UnsupportedReason != "" || node.UID != nodeidentity.StableUID(node) {
		t.Fatalf("invalid chain node: reason=%q", node.UnsupportedReason)
	}
	return chain
}

func TestSubscriptionChainsResolveForwardNodeReferences(t *testing.T) {
	for name, body := range map[string]string{
		"clash": `proxies:
  - {name: EXIT, type: http, server: exit.example.com, port: 8080, dialer-proxy: FRONT}
  - {name: FRONT, type: ss, server: front.example.com, port: 8388, cipher: aes-256-gcm, password: test-password}
`,
		"singbox": `{"outbounds":[
{"type":"http","tag":"EXIT","server":"exit.example.com","server_port":8080,"detour":"FRONT"},
{"type":"shadowsocks","tag":"FRONT","server":"front.example.com","server_port":8388,"method":"aes-256-gcm","password":"test-password"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			nodes := parseChainTestNodes(t, body)
			if len(nodes) != 2 {
				t.Fatalf("node count = %d, want 2", len(nodes))
			}
			chain := parsedNodeChain(t, nodes[0])
			if chain.Detour != model.NodeChainNodePrefix+nodes[1].UID || len(chain.Groups) != 0 {
				t.Fatal("forward reference did not resolve to the upstream connection UID")
			}
		})
	}
}

const nestedChainSubscription = `proxies:
  - {name: EXIT, type: http, server: exit.example.com, port: 8080, dialer-proxy: SELECT}
  - {name: FRONT, type: ss, server: front.example.com, port: 8388, cipher: aes-256-gcm, password: test-password}
  - {name: Direct-Alias, type: direct}
proxy-groups:
  - {name: SELECT, type: select, proxies: [AUTO, Direct-Alias, DIRECT]}
  - {name: AUTO, type: url-test, proxies: [FRONT], url: https://example.com/test, interval: 300, tolerance: 50}
`

func TestSubscriptionChainsPreserveNestedGroupsAndStableIdentity(t *testing.T) {
	nodes := parseChainTestNodes(t, nestedChainSubscription)
	if len(nodes) != 2 {
		t.Fatalf("groups and direct aliases must not become subscription nodes: %d", len(nodes))
	}
	chain := parsedNodeChain(t, nodes[0])
	if len(chain.Groups) != 2 || !strings.HasPrefix(chain.Detour, model.NodeChainGroupPrefix) {
		t.Fatal("nested dependency groups were not preserved")
	}
	for _, group := range chain.Groups {
		if group.Type == "selector" && (group.Outbounds[1] != model.NodeChainDirect || group.Outbounds[2] != model.NodeChainDirect) {
			t.Fatal("explicit direct choices were not preserved")
		}
		if group.Type == "urltest" && (group.Interval != "300s" || group.Tolerance != 50 || group.URL != "https://example.com/test") {
			t.Fatal("URL-test options were not preserved")
		}
	}
	renamedBody := strings.NewReplacer("EXIT", "Renamed-Exit", "FRONT", "Renamed-Front", "SELECT", "Renamed-Selector", "AUTO", "Renamed-Auto").Replace(nestedChainSubscription)
	renamed := parseChainTestNodes(t, renamedBody)
	if renamed[0].UID != nodes[0].UID || renamed[1].UID != nodes[1].UID {
		t.Fatal("renaming nodes and groups changed their connection identity")
	}
	changed := parseChainTestNodes(t, strings.ReplaceAll(nestedChainSubscription, "test-password", "changed-password"))
	if changed[0].UID == nodes[0].UID || changed[1].UID == nodes[1].UID {
		t.Fatal("a changed upstream connection must change its dependent chain identity")
	}
}

func TestSingboxChainsPreserveSelectorDefault(t *testing.T) {
	nodes := parseChainTestNodes(t, `{"outbounds":[
{"type":"http","tag":"EXIT","server":"exit.example.com","server_port":8080,"detour":"GROUP"},
{"type":"selector","tag":"GROUP","outbounds":["DIRECT","FRONT"],"default":"FRONT","interrupt_exist_connections":true},
{"type":"direct","tag":"DIRECT"},
{"type":"shadowsocks","tag":"FRONT","server":"front.example.com","server_port":8388,"method":"aes-256-gcm","password":"test-password"}]}`)
	chain := parsedNodeChain(t, nodes[0])
	if len(chain.Groups) != 1 || chain.Groups[0].Default != model.NodeChainNodePrefix+nodes[1].UID || !chain.Groups[0].InterruptExistConnections {
		t.Fatal("native selector defaults were not preserved")
	}
}

func TestSubscriptionChainReferencesPreserveWhitespaceInNames(t *testing.T) {
	nodes := parseChainTestNodes(t, `proxies:
  - {name: EXIT, type: http, server: exit.example.com, port: 8080, dialer-proxy: " SELECT "}
  - {name: " FRONT ", type: http, server: front.example.com, port: 8080}
proxy-groups:
  - {name: " SELECT ", type: select, proxies: [" FRONT "]}
`)
	chain := parsedNodeChain(t, nodes[0])
	if len(chain.Groups) != 1 || chain.Groups[0].Outbounds[0] != model.NodeChainNodePrefix+nodes[1].UID {
		t.Fatal("whitespace in a source name must not break its references")
	}
}

func TestSubscriptionChainsKeepEquivalentSourceGroupsIndependent(t *testing.T) {
	nodes := parseChainTestNodes(t, `proxies:
  - {name: Exit-A, type: http, server: a.example.com, port: 8080, dialer-proxy: Group-A}
  - {name: Exit-B, type: http, server: b.example.com, port: 8080, dialer-proxy: Group-B}
  - {name: Exit-C, type: http, server: c.example.com, port: 8080, dialer-proxy: Group-A}
  - {name: Front, type: http, server: front.example.com, port: 8080}
proxy-groups:
  - {name: Group-A, type: select, proxies: [Front, DIRECT]}
  - {name: Group-B, type: select, proxies: [Front, DIRECT]}
`)
	a, b, c := parsedNodeChain(t, nodes[0]), parsedNodeChain(t, nodes[1]), parsedNodeChain(t, nodes[2])
	if a.Detour == b.Detour || a.Detour != c.Detour || a.Identity != b.Identity || c.Groups[0].Name != "Group-A" {
		t.Fatal("equivalent source selectors must preserve independent selection state and stable identities")
	}
}

func TestSubscriptionChainsRejectInvalidDependencies(t *testing.T) {
	for _, test := range []struct {
		name   string
		target string
		extra  string
		want   string
	}{
		{name: "missing", target: "MISSING", want: "missing"},
		{name: "self cycle", target: "EXIT", want: "cycle"},
		{name: "node cycle", target: "FRONT", extra: "  - {name: FRONT, type: http, server: front.example.com, port: 8080, dialer-proxy: EXIT}\n", want: "cycle"},
		{name: "group cycle", target: "GROUP", extra: "proxy-groups:\n  - {name: GROUP, type: select, proxies: [EXIT]}\n", want: "cycle"},
		{name: "unsupported group", target: "GROUP", extra: "proxy-groups:\n  - {name: GROUP, type: fallback, proxies: [DIRECT]}\n", want: "group type"},
		{name: "external provider", target: "GROUP", extra: "proxy-groups:\n  - {name: GROUP, type: select, use: [external]}\n", want: "external providers"},
		{name: "ambiguous node", target: "FRONT", extra: "  - {name: FRONT, type: http, server: a.example.com, port: 8080}\n  - {name: FRONT, type: http, server: b.example.com, port: 8080}\n", want: "ambiguous"},
		{name: "unsupported upstream", target: "FRONT", extra: "  - {name: FRONT, type: vless, server: front.example.com, port: 443, network: xhttp}\n", want: "not a supported node"},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := "proxies:\n  - {name: EXIT, type: http, server: exit.example.com, port: 8080, dialer-proxy: " + test.target + "}\n" + test.extra
			nodes := parseChainTestNodes(t, body)
			if len(nodes) == 0 || !strings.Contains(nodes[0].UnsupportedReason, test.want) {
				t.Fatalf("expected %s rejection, got %+v", test.want, nodes)
			}
		})
	}
}

func TestEncodeProxyURIRejectsChainedNode(t *testing.T) {
	node := parseChainTestNodes(t, nestedChainSubscription)[0]
	_, err := EncodeProxyURI(model.Node{Type: node.Type, RawJSON: node.RawJSON, Raw: "http://test:password@exit.example.com:8080"})
	if err == nil || !strings.Contains(err.Error(), "前置代理") {
		t.Fatalf("expected a standalone sharing URI to be rejected, got %v", err)
	}
}
