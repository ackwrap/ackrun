package parser

import (
	"fmt"
	"strings"
	"testing"
)

func TestClashDialerProxyPreservesDependency(t *testing.T) {
	for _, protocol := range []string{"http", "socks", "shadowsocks", "vless"} {
		t.Run(protocol, func(t *testing.T) {
			config, reason := normalizeClashProxy(map[string]any{
				"type": protocol, "server": "proxy.example.com", "port": 443,
				"password": "test-password", "cipher": "aes-256-gcm",
				"dialer-proxy": "Upstream-Group",
			}, protocol)
			if reason != "" || config["detour"] != "Upstream-Group" {
				t.Fatalf("chain dependency was not preserved: reason=%q", reason)
			}
		})
	}
	for _, dependency := range []any{nil, "", "  "} {
		_, reason := normalizeClashProxy(map[string]any{
			"type": "http", "server": "proxy.example.com", "port": 8080,
			"dialer-proxy": dependency,
		}, "http")
		if reason != "" {
			t.Fatalf("an empty chain dependency must not reject a standalone proxy: %q", reason)
		}
	}
}

func TestClashShadowsocksExplicitlyDisabledTLSIgnoresVerificationFlags(t *testing.T) {
	for _, tlsValue := range []any{false, "false", " FALSE ", "0"} {
		for _, field := range []string{"skip-cert-verify", "skip_cert_verify", "insecure"} {
			t.Run(fmt.Sprintf("%s/%v", field, tlsValue), func(t *testing.T) {
				config, reason := normalizeClashProxy(map[string]any{
					"type": "ss", "server": "ss.example.com", "port": 8388,
					"cipher": "aes-256-gcm", "password": "test-password",
					"tls": tlsValue, field: true,
				}, "shadowsocks")
				if reason != "" {
					t.Fatalf("disabled TLS was incorrectly treated as a TLS wrapper: %q", reason)
				}
				if _, exists := config["tls"]; exists {
					t.Fatal("disabled TLS must not emit TLS options")
				}
				if config["method"] != "aes-256-gcm" || config["password"] != "test-password" {
					t.Fatal("Shadowsocks credentials were not preserved")
				}
			})
		}
	}
}

func TestClashShadowsocksTLSHintsRemainUnsupported(t *testing.T) {
	for _, test := range []struct {
		name   string
		fields map[string]any
	}{
		{name: "enabled TLS", fields: map[string]any{"tls": true}},
		{name: "TLS object", fields: map[string]any{"tls": map[string]any{"enabled": true}}},
		{name: "SNI despite disabled TLS", fields: map[string]any{"tls": false, "sni": "edge.example.com"}},
		{name: "implicit TLS verification", fields: map[string]any{"skip-cert-verify": true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			proxy := map[string]any{
				"type": "ss", "server": "ss.example.com", "port": 8388,
				"cipher": "aes-256-gcm", "password": "test-password",
			}
			for key, value := range test.fields {
				proxy[key] = value
			}
			_, reason := normalizeClashProxy(proxy, "shadowsocks")
			if !strings.Contains(reason, "TLS-wrapped Shadowsocks") {
				t.Fatalf("expected a TLS wrapper rejection, got %q", reason)
			}
		})
	}
}
