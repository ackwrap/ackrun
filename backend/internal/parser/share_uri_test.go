package parser

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestEncodeProxyURISupportedProtocols(t *testing.T) {
	tests := []struct {
		name            string
		typ             string
		prefix          string
		credentialKey   string
		credentialValue string
		options         map[string]any
	}{
		{name: "vmess", typ: "vmess", prefix: "vmess://", credentialKey: "uuid", credentialValue: "11111111-1111-1111-1111-111111111111", options: map[string]any{"uuid": "11111111-1111-1111-1111-111111111111", "security": "auto", "tls": map[string]any{"enabled": true, "insecure": true, "server_name": "edge.example"}}},
		{name: "vless reality websocket", typ: "vless", prefix: "vless://", credentialKey: "uuid", credentialValue: "22222222-2222-2222-2222-222222222222", options: map[string]any{"uuid": "22222222-2222-2222-2222-222222222222", "flow": "xtls-rprx-vision", "tls": map[string]any{"enabled": true, "server_name": "edge.example", "reality": map[string]any{"enabled": true, "public_key": "public-key", "short_id": "abcd"}}, "transport": map[string]any{"type": "ws", "path": "/ws", "headers": map[string]any{"Host": "edge.example"}}}},
		{name: "shadowsocks", typ: "shadowsocks", prefix: "ss://", credentialKey: "method", credentialValue: "aes-128-gcm", options: map[string]any{"method": "aes-128-gcm", "password": "secret"}},
		{name: "shadowsocks none", typ: "shadowsocks", prefix: "ss://", credentialKey: "method", credentialValue: "none", options: map[string]any{"method": "none", "password": ""}},
		{name: "ssr", typ: "ssr", prefix: "ssr://", credentialKey: "method", credentialValue: "aes-128-cfb", options: map[string]any{"method": "aes-128-cfb", "password": "secret", "protocol": "auth_sha1_v4", "obfs": "tls1.2_ticket_auth"}},
		{name: "trojan", typ: "trojan", prefix: "trojan://", credentialKey: "password", credentialValue: "secret@value", options: map[string]any{"password": "secret@value", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}}},
		{name: "socks", typ: "socks", prefix: "socks5://", credentialKey: "username", credentialValue: "user", options: map[string]any{"version": "5", "username": "user", "password": "secret"}},
		{name: "http", typ: "http", prefix: "http://", credentialKey: "username", credentialValue: "user", options: map[string]any{"username": "user", "password": "secret"}},
		{name: "hysteria", typ: "hysteria", prefix: "hysteria://", credentialKey: "auth_str", credentialValue: "secret", options: map[string]any{"auth_str": "secret", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}, "up_mbps": 10, "down_mbps": 20}},
		{name: "hysteria2", typ: "hysteria2", prefix: "hysteria2://", credentialKey: "password", credentialValue: "secret", options: map[string]any{"password": "secret", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}, "obfs": map[string]any{"type": "salamander", "password": "obfs-secret"}}},
		{name: "tuic", typ: "tuic", prefix: "tuic://", credentialKey: "uuid", credentialValue: "33333333-3333-3333-3333-333333333333", options: map[string]any{"uuid": "33333333-3333-3333-3333-333333333333", "password": "secret", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}}},
		{name: "anytls", typ: "anytls", prefix: "anytls://", credentialKey: "password", credentialValue: "secret", options: map[string]any{"password": "secret", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}}},
		{name: "wireguard", typ: "wireguard", prefix: "wireguard://", credentialKey: "private_key", credentialValue: "private-key", options: map[string]any{"private_key": "private-key", "public_key": "public-key", "address": []any{"10.0.0.2/32"}, "reserved": []any{1, 2, 3}, "peers": []any{map[string]any{"public_key": "peer-key"}}}},
		{name: "naive", typ: "naive", prefix: "naive+https://", credentialKey: "username", credentialValue: "user", options: map[string]any{"username": "user", "password": "secret", "tls": map[string]any{"enabled": true, "server_name": "edge.example"}}},
		{name: "mieru", typ: "mieru", prefix: "mieru://", credentialKey: "username", credentialValue: "user", options: map[string]any{"username": "user", "password": "secret", "protocol": "TCP"}},
		{name: "snell", typ: "snell", prefix: "snell://", credentialKey: "psk", credentialValue: "long-enough-psk", options: map[string]any{"psk": "long-enough-psk", "version": 4, "obfs_mode": "http", "obfs_host": "edge.example"}},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			testCase.options["type"] = testCase.typ
			testCase.options["server"] = "proxy.example"
			testCase.options["server_port"] = 443
			rawJSON, err := json.Marshal(testCase.options)
			if err != nil {
				t.Fatal(err)
			}
			shareURI, err := EncodeProxyURI(model.Node{
				Name: "Share Node", Type: testCase.typ, Server: "proxy.example",
				ServerPort: 443, RawJSON: string(rawJSON),
			})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(shareURI, testCase.prefix) {
				t.Fatalf("generated %s URI has an unexpected scheme", testCase.typ)
			}
			parsed, err := ParseProxyURI(shareURI)
			if err != nil {
				t.Fatalf("generated URI cannot be parsed: %v", err)
			}
			if parsed.Server != "proxy.example" || parsed.ServerPort != 443 {
				t.Fatal("generated URI changed the endpoint")
			}
			if testCase.credentialKey != "" {
				var normalized map[string]any
				if err := json.Unmarshal([]byte(parsed.RawJSON), &normalized); err != nil {
					t.Fatal(err)
				}
				if getString(normalized, testCase.credentialKey) != testCase.credentialValue {
					t.Fatalf("generated URI changed required %s field", testCase.credentialKey)
				}
				if testCase.name == "vmess" && !boolOrString(nestedMap(normalized, "tls")["insecure"]) {
					t.Fatal("generated VMess URI lost insecure TLS mode")
				}
				if testCase.name == "wireguard" {
					if peers, ok := normalized["peers"].([]any); !ok || len(peers) != 1 {
						t.Fatal("generated WireGuard URI lost structured peers")
					}
				}
			}
		})
	}
}

func TestEncodeProxyURIReusesOriginalURI(t *testing.T) {
	original := "vless://44444444-4444-4444-4444-444444444444@proxy.example:443?security=tls#Original"
	shareURI, err := EncodeProxyURI(model.Node{Type: "vless", Raw: original})
	if err != nil {
		t.Fatal(err)
	}
	if shareURI != original {
		t.Fatal("share response did not preserve the original URI")
	}
}

func TestEncodeProxyURIReusesOriginalSSHURI(t *testing.T) {
	original := "ssh://deploy:redacted@ssh.example.com:22#SSH"
	shareURI, err := EncodeProxyURI(model.Node{Type: "ssh", Raw: original})
	if err != nil {
		t.Fatal(err)
	}
	if shareURI != original {
		t.Fatal("share response did not preserve the original SSH URI")
	}
}

func TestEncodeProxyURIGeneratesSSHURI(t *testing.T) {
	options := map[string]any{
		"type": "ssh", "server": "ssh.example.com", "server_port": 22,
		"user": "deploy", "password": "redacted", "private_key": []any{"test-private-key"},
		"private_key_path": "/keys/id_ed25519", "private_key_passphrase": "redacted-passphrase",
		"host_key": []any{"ssh-ed25519 test-host-key"}, "host_key_algorithms": []any{"ssh-ed25519"},
		"client_version": "SSH-2.0-Test", "cipher": []any{"aes128-gcm@openssh.com"},
		"mac": []any{"hmac-sha2-256-etm@openssh.com"}, "kex_algorithm": []any{"curve25519-sha256"},
	}
	rawJSON, err := json.Marshal(options)
	if err != nil {
		t.Fatal(err)
	}
	shareURI, err := EncodeProxyURI(model.Node{
		Name: "SSH JSON", Type: "ssh", Server: "ssh.example.com", ServerPort: 22, RawJSON: string(rawJSON),
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseProxyURI(shareURI)
	if err != nil {
		t.Fatal(err)
	}
	var normalized map[string]any
	if err := json.Unmarshal([]byte(parsed.RawJSON), &normalized); err != nil {
		t.Fatal(err)
	}
	if parsed.Name != "SSH JSON" || normalized["user"] != "deploy" || normalized["private_key_path"] != "/keys/id_ed25519" {
		t.Fatalf("generated SSH URI lost scalar fields: %+v", normalized)
	}
	for _, key := range []string{"private_key", "host_key", "host_key_algorithms", "cipher", "mac", "kex_algorithm"} {
		if values, ok := normalized[key].([]any); !ok || len(values) != 1 {
			t.Fatalf("generated SSH URI lost %s: %+v", key, normalized[key])
		}
	}
}

func TestEncodeProxyURIRejectsUnsupportedJSONProtocol(t *testing.T) {
	_, err := EncodeProxyURI(model.Node{Type: "unknown", RawJSON: `{"type":"unknown"}`})
	if err == nil {
		t.Fatal("expected unsupported protocol error")
	}
}
