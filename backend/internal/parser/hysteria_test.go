package parser

import (
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
)

func TestHysteria2PortHoppingRanges(t *testing.T) {
	tests := []struct {
		name  string
		ports string
		want  []string
	}{
		{name: "hyphen", ports: "20000-30000", want: []string{"20000:30000"}},
		{name: "colon", ports: "20000:30000", want: []string{"20000:30000"}},
		{name: "multiple", ports: " 20000 - 30000, 40000:50000, 443 ", want: []string{"20000:30000", "40000:50000", "443:443"}},
		{name: "single", ports: "443", want: []string{"443:443"}},
		{name: "native open ranges", ports: ":30000,40000:", want: []string{":30000", "40000:"}},
		{name: "native zero bound", ports: "0:30000", want: []string{"0:30000"}},
		{name: "absent"},
	}
	for _, scheme := range []string{"hy2", "hysteria2"} {
		for _, key := range []string{"mport", "server_ports", "server-ports"} {
			for _, test := range tests {
				t.Run(scheme+"/"+key+"/"+test.name, func(t *testing.T) {
					query := url.Values{"hop_interval": {"30s"}}
					if test.ports != "" {
						query.Set(key, test.ports)
					}
					node, err := ParseProxyURI(scheme + "://redacted@hy2.example.com:443?" + query.Encode())
					if err != nil {
						t.Fatalf("parse Hysteria2: %v", err)
					}
					var config struct {
						ServerPorts []string `json:"server_ports"`
						HopInterval string   `json:"hop_interval"`
					}
					if err := json.Unmarshal([]byte(node.RawJSON), &config); err != nil {
						t.Fatalf("unmarshal Hysteria2: %v", err)
					}
					if !reflect.DeepEqual(config.ServerPorts, test.want) {
						t.Fatalf("server_ports = %v, want %v", config.ServerPorts, test.want)
					}
					if config.HopInterval != "30s" || node.ServerPort != 443 {
						t.Fatal("port normalization changed unrelated node options")
					}
				})
			}
		}
	}
}

func TestHysteria2RejectsInvalidPortHoppingRanges(t *testing.T) {
	for _, ports := range []string{
		"0", "65536", "30000-20000", "20000-65536", "20000-", "-30000",
		"20000--30000", "20000:30000:40000", "abc-def", "20000-30000,", "20000-30000,,443",
	} {
		t.Run(ports, func(t *testing.T) {
			_, err := ParseProxyURI("hy2://redacted@hy2.example.com:443?mport=" + url.QueryEscape(ports))
			if err == nil || !strings.Contains(err.Error(), "port hopping range") {
				t.Fatalf("expected invalid port hopping range error, got %v", err)
			}
		})
	}
}

func TestHysteria2IPv6PortHoppingWithObfuscation(t *testing.T) {
	nodes, err := ParseSubscriptionNodes([]byte("hysteria2://test-password@[2001:db8::1]:45231/?mport=20000-30000&sni=tls.example.com&insecure=1&obfs=salamander&obfs-password=test-obfs#Test-HY2-IPv6"))
	if err != nil || len(nodes) != 1 {
		t.Fatalf("parse IPv6 Hysteria2 subscription: count=%d, error=%v", len(nodes), err)
	}
	node := nodes[0]
	var config map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &config); err != nil {
		t.Fatal(err)
	}
	if node.Server != "2001:db8::1" || node.ServerPort != 45231 || node.Name != "Test-HY2-IPv6" || config["password"] != "test-password" {
		t.Fatal("IPv6 Hysteria2 node fields were not preserved")
	}
	if !reflect.DeepEqual(config["server_ports"], []any{"20000:30000"}) {
		t.Fatalf("unexpected server_ports: %v", config["server_ports"])
	}
	tlsOptions, ok := config["tls"].(map[string]any)
	if !ok || tlsOptions["enabled"] != true || tlsOptions["server_name"] != "tls.example.com" || tlsOptions["insecure"] != true {
		t.Fatal("Hysteria2 TLS options were not preserved")
	}
	obfs, ok := config["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "test-obfs" {
		t.Fatal("Hysteria2 obfuscation options were not preserved")
	}
}

func TestSingboxHysteriaPortHoppingRanges(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2"} {
		t.Run(protocol, func(t *testing.T) {
			body, err := json.Marshal(map[string]any{"outbounds": []any{map[string]any{
				"type": protocol, "server": "hy.example.com", "server_port": 443,
				"server_ports": []string{"20000-30000", "40000:50000"},
			}}})
			if err != nil {
				t.Fatal(err)
			}
			nodes, err := ParseSubscriptionNodes(body)
			if err != nil || len(nodes) != 1 {
				t.Fatalf("parse sing-box subscription: count=%d, error=%v", len(nodes), err)
			}
			var config struct {
				ServerPorts []string `json:"server_ports"`
			}
			if err := json.Unmarshal([]byte(nodes[0].RawJSON), &config); err != nil {
				t.Fatal(err)
			}
			if nodes[0].UnsupportedReason != "" || !reflect.DeepEqual(config.ServerPorts, []string{"20000:30000", "40000:50000"}) {
				t.Fatalf("unexpected port ranges: %v, unsupported=%q", config.ServerPorts, nodes[0].UnsupportedReason)
			}
		})
	}
}

func TestSingboxHysteriaMarksInvalidPortHoppingRanges(t *testing.T) {
	nodes := parseSingboxJSON([]byte(`{"outbounds":[{"type":"hysteria2","server":"hy2.example.com","server_port":443,"server_ports":["30000-20000"]}]}`))
	if len(nodes) != 1 || !strings.Contains(nodes[0].UnsupportedReason, "port hopping range") {
		t.Fatal("invalid Hysteria2 port range was not marked unsupported")
	}
}
