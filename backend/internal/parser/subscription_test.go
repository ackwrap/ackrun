package parser

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseSubscriptionNodesClashYAML(t *testing.T) {
	body := []byte(`proxies:
  - name: HK-01
    type: trojan
    server: hk.example.com
    port: 443
  - name: JP-01
    type: vmess
    server: jp.example.com
    port: 8443
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse clash yaml: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Name != "HK-01" || nodes[0].Type != "trojan" || nodes[0].Server != "hk.example.com" || nodes[0].ServerPort != 443 {
		t.Fatalf("unexpected first node: %+v", nodes[0])
	}
}

func TestParseSubscriptionNodesSingboxJSON(t *testing.T) {
	body := []byte(`{
  "outbounds": [
    { "type": "selector", "tag": "proxy", "outbounds": ["HK-01"] },
    { "type": "direct", "tag": "direct" },
    { "type": "vmess", "tag": "HK-01", "server": "hk.example.com", "server_port": 443, "uuid": "uuid", "security": "auto" },
    { "type": "trojan", "tag": "JP-01", "server": "jp.example.com", "server_port": 8443, "password": "pass" }
  ]
}`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse sing-box json: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[0].Name != "HK-01" || nodes[0].Type != "vmess" || nodes[0].Server != "hk.example.com" || nodes[0].ServerPort != 443 {
		t.Fatalf("unexpected first node: %+v", nodes[0])
	}
}

func TestParseSubscriptionNodesBase64URIList(t *testing.T) {
	plain := "ss://aes-128-gcm:pass@example.com:8388#SS-01\ntrojan://password@trojan.example.com:443#Trojan-01\n"
	body := []byte(base64.StdEncoding.EncodeToString([]byte(plain)))
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse uri list: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}
	if nodes[1].Name != "Trojan-01" || nodes[1].Type != "trojan" || nodes[1].Server != "trojan.example.com" || nodes[1].ServerPort != 443 {
		t.Fatalf("unexpected trojan node: %+v", nodes[1])
	}
}

func TestParseVmessURI(t *testing.T) {
	rawJSON := `{"v":"2","ps":"VMess-01","add":"vmess.example.com","port":"443","id":"uuid"}`
	uri := "vmess://" + base64.StdEncoding.EncodeToString([]byte(rawJSON))
	node, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("parse vmess: %v", err)
	}
	if node.Name != "VMess-01" || node.Type != "vmess" || node.Server != "vmess.example.com" || node.ServerPort != 443 {
		t.Fatalf("unexpected vmess node: %+v", node)
	}
}

func TestParseMigratedProxyURIs(t *testing.T) {
	cases := []struct {
		name   string
		uri    string
		typ    string
		server string
		port   int
	}{
		{name: "ss", uri: "ss://aes-128-gcm:pass@example.com:8388#SS-01", typ: "shadowsocks", server: "example.com", port: 8388},
		{name: "trojan", uri: "trojan://password@trojan.example.com:443?sni=sni.example.com#Trojan-01", typ: "trojan", server: "trojan.example.com", port: 443},
		{name: "vless", uri: "vless://uuid@vless.example.com:443?security=tls&type=ws&sni=sni.example.com#VLESS-01", typ: "vless", server: "vless.example.com", port: 443},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseProxyURI(tc.uri)
			if err != nil {
				t.Fatalf("parse proxy uri: %v", err)
			}
			if node.Type != tc.typ || node.Server != tc.server || node.ServerPort != tc.port {
				t.Fatalf("unexpected node: %+v", node)
			}
			var cfg map[string]any
			if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
				t.Fatalf("unmarshal raw json: %v", err)
			}
			if cfg["type"] != tc.typ {
				t.Fatalf("expected raw json type %s, got %+v", tc.typ, cfg)
			}
		})
	}
}

func TestParseAdvancedTransportFields(t *testing.T) {
	node, err := ParseProxyURI("vless://uuid@vless.example.com:443?security=reality&type=grpc&serviceName=svc&sni=example.com&fp=chrome&pbk=pub&sid=01&spx=%2F#VLESS-Advanced")
	if err != nil {
		t.Fatalf("parse advanced vless: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal raw json: %v", err)
	}
	if cfg["network"] != "grpc" {
		t.Fatalf("missing advanced fields: %+v", cfg)
	}
	assertUTLSFingerprint(t, cfg, "chrome")
	if _, ok := cfg["grpc-opts"].(map[string]any); !ok {
		t.Fatalf("missing grpc opts: %+v", cfg)
	}
	if _, ok := cfg["reality-opts"].(map[string]any); !ok {
		t.Fatalf("missing reality opts: %+v", cfg)
	}

	ss, err := ParseProxyURI("ss://aes-128-gcm:pass@example.com:8388?plugin=obfs-local%3Bobfs%3Dhttp%3Bobfs-host%3Dexample.com#SS-Plugin")
	if err != nil {
		t.Fatalf("parse ss plugin: %v", err)
	}
	if err := json.Unmarshal([]byte(ss.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal ss raw json: %v", err)
	}
	if cfg["plugin"] != "obfs" {
		t.Fatalf("missing ss plugin: %+v", cfg)
	}
}

func assertTLSEnabled(t *testing.T, cfg map[string]any) {
	t.Helper()
	tlsVal, ok := cfg["tls"]
	if !ok {
		t.Fatalf("expected tls field, got nil")
	}
	switch v := tlsVal.(type) {
	case map[string]any:
		if v["enabled"] != true {
			t.Fatalf("expected tls.enabled=true, got %v", v["enabled"])
		}
	case bool:
		if !v {
			t.Fatalf("expected tls=true, got false")
		}
	default:
		t.Fatalf("expected tls to be map or bool, got %T: %v", tlsVal, tlsVal)
	}
}

func TestHysteriaAdvancedFields(t *testing.T) {
	node, err := ParseProxyURI("hysteria://auth@hy.example.com:443?sni=hy.example.com&alpn=h2,http/1.1&insecure=1&obfs=salamander&obfs-param=obfsval&up=100&down=200#HY-Adv")
	if err != nil {
		t.Fatalf("parse hysteria: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "hy.example.com" {
		t.Fatalf("expected tls.server_name=hy.example.com, got %v", tlsMap)
	}
	if cfg["skip-cert-verify"] != true {
		t.Fatalf("expected skip-cert-verify, got %v", cfg["skip-cert-verify"])
	}
	if cfg["obfs"] != "salamander" {
		t.Fatalf("expected obfs, got %v", cfg["obfs"])
	}
	if cfg["obfs-param"] != "obfsval" {
		t.Fatalf("expected obfs-param, got %v", cfg["obfs-param"])
	}
	alpn, ok := cfg["alpn"].([]any)
	if !ok || len(alpn) != 2 {
		t.Fatalf("expected alpn with 2 entries, got %v", cfg["alpn"])
	}
}

func TestHysteria2AdvancedFields(t *testing.T) {
	node, err := ParseProxyURI("hy2://pass@hy2.example.com:443?sni=hy2.example.com&alpn=h3&fp=chrome&insecure=1&obfs=gecko&obfs-password=obfspass&min_packet_size=100&max_packet_size=1200&hop_interval=30s&hop_interval_max=45s&bbr_profile=mobile&up=20&down=100#HY2-Adv")
	if err != nil {
		t.Fatalf("parse hysteria2: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	assertUTLSFingerprint(t, cfg, "chrome")
	obfs, ok := cfg["obfs"].(map[string]any)
	if !ok || obfs["type"] != "gecko" || obfs["password"] != "obfspass" || obfs["min_packet_size"] != float64(100) || obfs["max_packet_size"] != float64(1200) {
		t.Fatalf("unexpected obfs options: %v", cfg["obfs"])
	}
	if cfg["hop_interval"] != "30s" || cfg["hop_interval_max"] != "45s" || cfg["bbr_profile"] != "mobile" {
		t.Fatalf("unexpected Hysteria2 options: %+v", cfg)
	}
}

func TestTuicAdvancedFields(t *testing.T) {
	node, err := ParseProxyURI("tuic://uuid:pass@tuic.example.com:443?sni=tuic.example.com&congestion_control=bbr&udp_relay_mode=datagram&reduce_rtt=1&alpn=h3&fp=safari&insecure=1#TUIC-Adv")
	if err != nil {
		t.Fatalf("parse tuic: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	if cfg["congestion_control"] != "bbr" {
		t.Fatalf("expected congestion_control=bbr, got %v", cfg["congestion_control"])
	}
	if cfg["udp-relay-mode"] != "datagram" {
		t.Fatalf("expected udp-relay-mode=datagram, got %v", cfg["udp-relay-mode"])
	}
	if cfg["reduce-rtt"] != true {
		t.Fatalf("expected reduce-rtt=true, got %v", cfg["reduce-rtt"])
	}
	assertUTLSFingerprint(t, cfg, "safari")
}

func TestNaiveTLSFields(t *testing.T) {
	node, err := ParseProxyURI("naive+https://user:pass@naive.example.com:443?sni=naive.example.com&skip-cert-verify=1&alpn=h2,http/1.1#Naive-Adv")
	if err != nil {
		t.Fatalf("parse naive: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "naive.example.com" {
		t.Fatalf("expected tls.server_name=naive.example.com, got %v", tlsMap)
	}
	if cfg["skip-cert-verify"] != true {
		t.Fatalf("expected skip-cert-verify=true")
	}
}

func TestAnytlsTLSFields(t *testing.T) {
	node, err := ParseProxyURI("anytls://pass@anytls.example.com:443?sni=anytls.example.com&alpn=h2&fp=chrome&insecure=1&idle_session_check_interval=20s&idle_session_timeout=40s&min_idle_session=3#AnyTLS-Adv")
	if err != nil {
		t.Fatalf("parse anytls: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "anytls.example.com" {
		t.Fatalf("expected tls.server_name=anytls.example.com, got %v", tlsMap)
	}
	assertUTLSFingerprint(t, cfg, "chrome")
	if cfg["idle_session_check_interval"] != "20s" || cfg["idle_session_timeout"] != "40s" || cfg["min_idle_session"] != float64(3) {
		t.Fatalf("unexpected AnyTLS session options: %+v", cfg)
	}
}

func TestSocksTLSAndVersion(t *testing.T) {
	node, err := ParseProxyURI("socks5://user:pass@socks.example.com:1080?tls=1&sni=socks.example.com#SOCKS-TLS")
	if err != nil {
		t.Fatalf("parse socks: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg["version"] != "5" {
		t.Fatalf("expected version=5, got %v", cfg["version"])
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "socks.example.com" {
		t.Fatalf("expected tls.server_name=socks.example.com, got %v", tlsMap)
	}
}

func TestHTTPProxyTLS(t *testing.T) {
	node, err := ParseProxyURI("https://user:pass@http.example.com:8080?sni=http.example.com&alpn=h2,http/1.1&fp=chrome#HTTPS-Adv")
	if err != nil {
		t.Fatalf("parse http proxy: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "http.example.com" {
		t.Fatalf("expected tls.server_name=http.example.com, got %v", tlsMap)
	}
	assertUTLSFingerprint(t, cfg, "chrome")
}

func TestTLSCertificateFingerprintIsNotUTLS(t *testing.T) {
	const fingerprint = "dd9dd03d942400ad4c1400879bda98f4fa097183aa9a91a1423cdd42a3e183d7"
	node, err := ParseProxyURI("vless://uuid@vless.example.com:443?security=tls&sni=example.com&fingerprint=" + fingerprint + "#VLESS-Pin")
	if err != nil {
		t.Fatalf("parse vless with certificate fingerprint: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	tlsMap, ok := cfg["tls"].(map[string]any)
	if !ok {
		t.Fatalf("expected tls map, got %+v", cfg)
	}
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("certificate fingerprint must not be mapped to uTLS: %+v", tlsMap)
	}
	pins, ok := tlsMap["certificate_public_key_sha256"].([]any)
	if !ok || len(pins) != 1 || pins[0] != fingerprint {
		t.Fatalf("expected certificate_public_key_sha256 pin, got %+v", tlsMap["certificate_public_key_sha256"])
	}
}

func assertUTLSFingerprint(t *testing.T, cfg map[string]any, want string) {
	t.Helper()
	tlsMap, ok := cfg["tls"].(map[string]any)
	if !ok {
		t.Fatalf("expected tls map, got %+v", cfg["tls"])
	}
	utls, ok := tlsMap["utls"].(map[string]any)
	if !ok {
		t.Fatalf("expected tls.utls, got %+v", tlsMap)
	}
	if utls["fingerprint"] != want {
		t.Fatalf("expected tls.utls.fingerprint=%s, got %+v", want, utls)
	}
}

func TestSSRAdvancedParams(t *testing.T) {
	password := base64.StdEncoding.EncodeToString([]byte("pass"))
	remarks := base64.StdEncoding.EncodeToString([]byte("SSR-Adv"))
	obfsParam := base64.StdEncoding.EncodeToString([]byte("obfsparam_val"))
	protoParam := base64.StdEncoding.EncodeToString([]byte("protoparam_val"))
	group := base64.StdEncoding.EncodeToString([]byte("MyGroup"))
	decoded := "ssr.example.com:8388:origin:aes-128-cfb:plain:" + password + "/?remarks=" + remarks + "&obfsparam=" + obfsParam + "&protoparam=" + protoParam + "&group=" + group
	uri := "ssr://" + base64.StdEncoding.EncodeToString([]byte(decoded))
	node, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("parse ssr: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg["obfs_param"] != "obfsparam_val" {
		t.Fatalf("expected obfs_param, got %v", cfg["obfs_param"])
	}
	if cfg["protocol_param"] != "protoparam_val" {
		t.Fatalf("expected protocol_param, got %v", cfg["protocol_param"])
	}
	if cfg["method"] != "aes-128-cfb" {
		t.Fatalf("expected SSR method, got %+v", cfg)
	}
	if _, exists := cfg["group"]; exists {
		t.Fatalf("SSR subscription metadata leaked into outbound config: %+v", cfg)
	}
}

func TestClashSSRPreservesOutboundOptions(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-SSR
    type: ssr
    server: ssr.example.com
    port: 8388
    cipher: aes-256-cfb
    password: redacted
    protocol: auth_aes128_sha1
    protocol-param: 1000:test
    obfs: http_simple
    obfs-param: cdn.example.com
    udp: true
  - name: Clash-SSR-TCP
    type: ssr
    server: ssr-tcp.example.com
    port: 8388
    cipher: aes-256-cfb
    password: redacted
    protocol: origin
    obfs: plain
    udp: false
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse Clash SSR: %v", err)
	}
	if len(nodes) != 2 || nodes[0].Type != "ssr" || nodes[0].UnsupportedReason != "" {
		t.Fatalf("unexpected SSR nodes: %+v", nodes)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &config); err != nil {
		t.Fatalf("unmarshal Clash SSR: %v", err)
	}
	if config["method"] != "aes-256-cfb" || config["protocol"] != "auth_aes128_sha1" || config["protocol_param"] != "1000:test" || config["obfs"] != "http_simple" || config["obfs_param"] != "cdn.example.com" {
		t.Fatalf("Clash SSR options were not preserved: %+v", config)
	}
	if _, exists := config["network"]; exists {
		t.Fatalf("UDP-capable SSR must not be restricted to TCP: %+v", config)
	}
	var tcpConfig map[string]any
	if err := json.Unmarshal([]byte(nodes[1].RawJSON), &tcpConfig); err != nil {
		t.Fatalf("unmarshal TCP-only Clash SSR: %v", err)
	}
	if tcpConfig["network"] != "tcp" {
		t.Fatalf("Clash SSR udp=false must map to TCP: %+v", tcpConfig)
	}
}

func TestSnellObfsOpts(t *testing.T) {
	node, err := ParseProxyURI("snell://psk@snell.example.com:440?version=4&obfs=http&obfs-host=snell.example.com&reuse=true#Snell-Adv")
	if err != nil {
		t.Fatalf("parse snell: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg["version"] != float64(4) {
		t.Fatalf("expected version=4, got %v", cfg["version"])
	}
	if cfg["obfs_mode"] != "http" || cfg["obfs_host"] != "snell.example.com" || cfg["reuse"] != true {
		t.Fatalf("unexpected Snell options: %+v", cfg)
	}
}

func TestSnellVersionMapping(t *testing.T) {
	v5, err := ParseProxyURI("snell://redacted@snell.example.com:440?version=5#Snell-v5")
	if err != nil {
		t.Fatalf("parse Snell v5: %v", err)
	}
	var v5Config map[string]any
	if err := json.Unmarshal([]byte(v5.RawJSON), &v5Config); err != nil {
		t.Fatalf("unmarshal Snell v5: %v", err)
	}
	if v5Config["version"] != float64(4) {
		t.Fatalf("Snell v5 version = %v, want wire-compatible v4", v5Config["version"])
	}

	v6, err := ParseProxyURI("snell://long-redacted-psk@snell.example.com:440?version=6&mode=unshaped#Snell-v6")
	if err != nil {
		t.Fatalf("parse Snell v6: %v", err)
	}
	var v6Config map[string]any
	if err := json.Unmarshal([]byte(v6.RawJSON), &v6Config); err != nil {
		t.Fatalf("unmarshal Snell v6: %v", err)
	}
	if v6Config["version"] != float64(6) || v6Config["mode"] != "unshaped" {
		t.Fatalf("unexpected Snell v6 config: %+v", v6Config)
	}
}

func TestWireguardMTUAndAddress(t *testing.T) {
	node, err := ParseProxyURI("wireguard://wg.example.com:51820?private-key=priv&public-key=pub&address=10.0.0.2/32,fd00::2/128&mtu=1280#WG-Adv")
	if err != nil {
		t.Fatalf("parse wireguard: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if mtu, ok := cfg["mtu"].(float64); !ok || int(mtu) != 1280 {
		t.Fatalf("expected mtu=1280, got %v", cfg["mtu"])
	}
	localAddr, ok := cfg["address"].([]any)
	if !ok || len(localAddr) != 2 {
		t.Fatalf("expected address with 2 entries, got %v", cfg["address"])
	}
}

func TestSSTLSAndTransport(t *testing.T) {
	node, err := ParseProxyURI("ss://aes-128-gcm:pass@ss.example.com:8388?tls=1&sni=ss.example.com&type=ws&host=ws.example.com&path=%2Fws#SS-TLS-WS")
	if err != nil {
		t.Fatalf("parse ss: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	assertTLSEnabled(t, cfg)
	tlsMap, _ := cfg["tls"].(map[string]any)
	if tlsMap["server_name"] != "ss.example.com" {
		t.Fatalf("expected tls.server_name=ss.example.com, got %v", tlsMap)
	}
	if cfg["network"] != "ws" {
		t.Fatalf("expected network=ws, got %v", cfg["network"])
	}
	wsOpts, ok := cfg["ws-opts"].(map[string]any)
	if !ok {
		t.Fatalf("expected ws-opts map, got %v", cfg["ws-opts"])
	}
	if wsOpts["path"] != "/ws" {
		t.Fatalf("expected ws path=/ws, got %v", wsOpts["path"])
	}
}

func TestSocks4Version(t *testing.T) {
	node, err := ParseProxyURI("socks4://user:pass@socks4.example.com:1080#SOCKS4-01")
	if err != nil {
		t.Fatalf("parse socks4: %v", err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg["version"] != "4" {
		t.Fatalf("expected version=4, got %v", cfg["version"])
	}
}

func TestMieruTLS(t *testing.T) {
	_, err := ParseProxyURI("mieru://user:pass@mieru.example.com:2999?protocol=TCP&tls=1&sni=mieru.example.com&Mieru-01")
	if err != nil {
		t.Fatalf("parse mieru: %v", err)
	}
}

func TestClashNormalized(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-VMess
    type: vmess
    server: clash.example.com
    port: 443
    uuid: test-uuid
    alterId: 0
    cipher: auto
    tls: true
    servername: clash.example.com
    network: ws
    ws-opts:
      path: /ws
      headers:
        Host: ws.example.com
    skip-cert-verify: false
    client-fingerprint: chrome
    udp: true
  - name: Clash-Trojan
    type: trojan
    server: trojan.example.com
    port: 443
    password: trojan-pass
    sni: trojan.example.com
    skip-cert-verify: true
  - name: Clash-SOCKS5
    type: socks5
    server: socks.example.com
    port: 1080
    username: user
    password: pass
  - name: Clash-TUIC
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: 33333333-3333-4333-8333-333333333333
    password: tuic-pass
    alpn:
      - h3
    sni: tuic.example.com
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse clash yaml: %v", err)
	}
	if len(nodes) != 4 {
		t.Fatalf("expected 4 nodes, got %d", len(nodes))
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if cfg["uuid"] != "test-uuid" {
		t.Fatalf("expected uuid, got %v", cfg["uuid"])
	}
	assertTLSEnabled(t, cfg)
	tlsMap, ok := cfg["tls"].(map[string]any)
	if !ok || tlsMap["server_name"] != "clash.example.com" {
		t.Fatalf("expected tls.server_name=clash.example.com, got %v", cfg["tls"])
	}
	transport, ok := cfg["transport"].(map[string]any)
	if !ok {
		t.Fatalf("expected transport map, got %v", cfg["transport"])
	}
	if transport["type"] != "ws" {
		t.Fatalf("expected transport.type=ws, got %v", transport["type"])
	}
	if transport["path"] != "/ws" {
		t.Fatalf("expected transport.path=/ws, got %v", transport["path"])
	}
	utls, ok := cfg["tls"].(map[string]any)
	if !ok {
		t.Fatalf("expected tls map, got %v", cfg["tls"])
	}
	utlsMap, ok := utls["utls"].(map[string]any)
	if !ok || utlsMap["fingerprint"] != "chrome" {
		t.Fatalf("expected tls.utls.fingerprint=chrome, got %v", utls)
	}
	if cfg["security"] != "auto" {
		t.Fatalf("expected security=auto, got %v", cfg["security"])
	}
	if _, exists := cfg["alter_id"]; exists {
		t.Fatalf("zero alterId must not be persisted in normalized VMess config: %+v", cfg)
	}
	if nodes[2].Type != "socks" {
		t.Fatalf("expected socks5 alias normalized to socks, got %+v", nodes[2])
	}
	if err := json.Unmarshal([]byte(nodes[2].RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal socks: %v", err)
	}
	if cfg["type"] != "socks" {
		t.Fatalf("expected raw json type=socks, got %+v", cfg)
	}
	if nodes[3].Type != "tuic" {
		t.Fatalf("expected tuic node, got %+v", nodes[3])
	}
	if err := json.Unmarshal([]byte(nodes[3].RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal tuic: %v", err)
	}
	tlsMap, ok = cfg["tls"].(map[string]any)
	if !ok || tlsMap["enabled"] != true || tlsMap["server_name"] != "tuic.example.com" {
		t.Fatalf("expected TUIC tls enabled with server_name, got %+v", cfg["tls"])
	}
}

func TestClashVLESSCipherIsNotEmitted(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-VLESS
    type: vless
    server: clash.example.com
    port: 443
    uuid: test-uuid
    cipher: auto
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse clash yaml: %v", err)
	}
	if len(nodes) != 1 {
		t.Fatalf("parsed node count = %d, want 1", len(nodes))
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal VLESS node: %v", err)
	}
	if _, exists := cfg["cipher"]; exists {
		t.Fatalf("normalized VLESS node contains unsupported cipher: %+v", cfg)
	}
	if _, exists := cfg["method"]; exists {
		t.Fatalf("normalized VLESS node contains unsupported method: %+v", cfg)
	}
}

func TestClashAnyTLSPreservesTLSOptions(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-AnyTLS
    type: anytls
    server: anytls.example.com
    port: 8443
    password: redacted
    skip-cert-verify: true
    sni: tls.example.com
    udp: true
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse clash AnyTLS: %v", err)
	}
	if len(nodes) != 1 || nodes[0].Type != "anytls" {
		t.Fatalf("parsed AnyTLS nodes = %+v", nodes)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &cfg); err != nil {
		t.Fatalf("unmarshal AnyTLS node: %v", err)
	}
	tlsOptions, ok := cfg["tls"].(map[string]any)
	if !ok || tlsOptions["enabled"] != true || tlsOptions["insecure"] != true || tlsOptions["server_name"] != "tls.example.com" {
		t.Fatalf("AnyTLS TLS options were not preserved: %+v", cfg["tls"])
	}
	if _, exists := cfg["udp"]; exists {
		t.Fatalf("Clash udp capability flag must not leak into sing-box config: %+v", cfg)
	}
}

func TestClashPreservesMTLSAndECHOptions(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-AnyTLS-Advanced-TLS
    type: anytls
    server: anytls.example.com
    port: 8443
    password: redacted
    certificate: client.pem
    private-key: client.key
    client-fingerprint: chrome
    ech-opts:
      enable: true
      config: AEn+DQBFKwAgACABWIHUGj4u+PIggYXcR5JF0gYk3dCRioBW8uJq9H4mKAAIAAEAAQABAANAEnB1YmxpYy50bHMtZWNoLmRldgAA
      query-server-name: ech.example.com
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse Clash TLS extensions: %v", err)
	}
	if len(nodes) != 1 || nodes[0].UnsupportedReason != "" {
		t.Fatalf("unexpected parsed nodes: %+v", nodes)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &config); err != nil {
		t.Fatalf("unmarshal Clash TLS extensions: %v", err)
	}
	tlsOptions, ok := config["tls"].(map[string]any)
	if !ok || tlsOptions["client_certificate_path"] != "client.pem" || tlsOptions["client_key_path"] != "client.key" {
		t.Fatalf("mTLS options were not preserved: %+v", config["tls"])
	}
	utls, ok := tlsOptions["utls"].(map[string]any)
	if !ok || utls["enabled"] != true || utls["fingerprint"] != "chrome" {
		t.Fatalf("uTLS options were not preserved: %+v", tlsOptions["utls"])
	}
	ech, ok := tlsOptions["ech"].(map[string]any)
	if !ok || ech["enabled"] != true || ech["query_server_name"] != "ech.example.com" {
		t.Fatalf("ECH options were not preserved: %+v", tlsOptions["ech"])
	}
	echConfig, ok := ech["config"].([]any)
	if !ok || len(echConfig) != 1 || !strings.Contains(echConfig[0].(string), "-----BEGIN ECH CONFIGS-----") {
		t.Fatalf("ECH config was not converted to sing-box PEM: %+v", ech["config"])
	}
}

func TestClashHysteria2PreservesProtocolOptions(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-Hysteria2-Ports
    type: hysteria2
    server: hy2.example.com
    port: 60000
    ports: 60000-65530
    password: redacted
    skip-cert-verify: true
    sni: tls.example.com
    alpn:
      - h3
    udp: true
  - name: Clash-Hysteria2-MPort
    type: hysteria2
    server: hy2-obfs.example.com
    port: 23065
    mport: 23065-23100
    hop-interval: 15-30
    password: redacted
    skip-cert-verify: true
    sni: tls-obfs.example.com
    obfs: salamander
    obfs-password: redacted
    obfs-min-packet-size: 512
    obfs-max-packet-size: 1200
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse clash Hysteria2: %v", err)
	}
	if len(nodes) != 2 {
		t.Fatalf("parsed Hysteria2 node count = %d, want 2", len(nodes))
	}

	var portsConfig map[string]any
	if err := json.Unmarshal([]byte(nodes[0].RawJSON), &portsConfig); err != nil {
		t.Fatalf("unmarshal Hysteria2 ports node: %v", err)
	}
	ports, ok := portsConfig["server_ports"].([]any)
	if !ok || len(ports) != 1 || ports[0] != "60000:65530" {
		t.Fatalf("Hysteria2 ports were not preserved: %+v", portsConfig["server_ports"])
	}
	tlsOptions, ok := portsConfig["tls"].(map[string]any)
	if !ok || tlsOptions["enabled"] != true || tlsOptions["insecure"] != true || tlsOptions["server_name"] != "tls.example.com" {
		t.Fatalf("Hysteria2 TLS options were not preserved: %+v", portsConfig["tls"])
	}
	if _, exists := portsConfig["udp"]; exists {
		t.Fatalf("Clash udp capability flag must not leak into sing-box config: %+v", portsConfig)
	}

	var obfsConfig map[string]any
	if err := json.Unmarshal([]byte(nodes[1].RawJSON), &obfsConfig); err != nil {
		t.Fatalf("unmarshal Hysteria2 obfs node: %v", err)
	}
	ports, ok = obfsConfig["server_ports"].([]any)
	if !ok || len(ports) != 1 || ports[0] != "23065:23100" {
		t.Fatalf("Hysteria2 mport alias was not preserved: %+v", obfsConfig["server_ports"])
	}
	if obfsConfig["hop_interval"] != "15s" || obfsConfig["hop_interval_max"] != "30s" {
		t.Fatalf("Hysteria2 hop interval range was not preserved: min=%v max=%v", obfsConfig["hop_interval"], obfsConfig["hop_interval_max"])
	}
	obfs, ok := obfsConfig["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "redacted" || obfs["min_packet_size"] != float64(512) || obfs["max_packet_size"] != float64(1200) {
		t.Fatalf("Hysteria2 obfs options were not preserved: %+v", obfsConfig["obfs"])
	}
}

func TestClashProtocolSpecificMappings(t *testing.T) {
	body := []byte(`proxies:
  - name: Clash-Hysteria
    type: hysteria
    server: hy.example.com
    port: 443
    ports: 20000-20010
    hop-interval: 10
    auth-str: redacted
    up: 100 Mbps
    down: 1 Gbps
    recv-window-conn: 1048576
    recv-window: 2097152
    disable-mtu-discovery: true
    sni: tls.example.com
  - name: Clash-AnyTLS
    type: anytls
    server: anytls.example.com
    port: 443
    password: redacted
    idle-session-check-interval: 30
    idle-session-timeout: 45
    min-idle-session: 2
  - name: Clash-TUIC
    type: tuic
    server: tuic.example.com
    port: 443
    uuid: 33333333-3333-4333-8333-333333333333
    password: redacted
    heartbeat-interval: 10000
    udp-over-stream: true
    max-open-streams: 64
    recv-window-conn: 1048576
    recv-window: 2097152
  - name: Clash-SOCKS4
    type: socks4
    server: socks.example.com
    port: 1080
  - name: Clash-HTTP
    type: http
    server: http.example.com
    port: 443
    tls: true
    headers:
      User-Agent: AckWrap-Test
  - name: Clash-Snell
    type: snell
    server: snell.example.com
    port: 443
    psk: redacted
    version: 5
    obfs-opts:
      mode: tls
      host: cover.example.com
  - name: Clash-Naive
    type: naive
    server: naive.example.com
    port: 443
    username: user
    password: redacted
    insecure-concurrency: 4
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse Clash protocol mappings: %v", err)
	}
	if len(nodes) != 7 {
		t.Fatalf("parsed node count = %d, want 7", len(nodes))
	}
	configs := make(map[string]map[string]any, len(nodes))
	for _, node := range nodes {
		if node.UnsupportedReason != "" {
			t.Fatalf("unexpected unsupported %s node: %s", node.Type, node.UnsupportedReason)
		}
		var config map[string]any
		if err := json.Unmarshal([]byte(node.RawJSON), &config); err != nil {
			t.Fatalf("unmarshal %s node: %v", node.Type, err)
		}
		configs[node.Type] = config
	}

	hysteria := configs["hysteria"]
	if hysteria["auth_str"] != "redacted" || hysteria["up_mbps"] != float64(100) || hysteria["down_mbps"] != float64(1000) {
		t.Fatalf("Hysteria authentication or bandwidth mapping is incomplete: %+v", hysteria)
	}
	if ports, ok := hysteria["server_ports"].([]any); !ok || len(ports) != 1 || ports[0] != "20000:20010" {
		t.Fatalf("Hysteria port hopping mapping is incomplete: %+v", hysteria["server_ports"])
	}
	if hysteria["recv_window_conn"] != float64(1048576) || hysteria["recv_window"] != float64(2097152) || hysteria["disable_mtu_discovery"] != true {
		t.Fatalf("Hysteria QUIC compatibility fields are incomplete: %+v", hysteria)
	}

	anyTLS := configs["anytls"]
	if anyTLS["idle_session_check_interval"] != "30s" || anyTLS["idle_session_timeout"] != "45s" || anyTLS["min_idle_session"] != float64(2) {
		t.Fatalf("AnyTLS session options are incomplete: %+v", anyTLS)
	}

	tuic := configs["tuic"]
	if tuic["heartbeat"] != "10000ms" || tuic["udp_over_stream"] != true || tuic["max_concurrent_streams"] != float64(64) {
		t.Fatalf("TUIC options are incomplete: %+v", tuic)
	}
	if configs["socks"]["version"] != "4" {
		t.Fatalf("SOCKS version was not preserved: %+v", configs["socks"])
	}
	if _, ok := configs["http"]["headers"].(map[string]any); !ok {
		t.Fatalf("HTTP headers were not preserved: %+v", configs["http"])
	}
	if configs["snell"]["version"] != float64(4) || configs["snell"]["obfs_mode"] != "tls" {
		t.Fatalf("Snell options are incomplete: %+v", configs["snell"])
	}
	if configs["naive"]["insecure_concurrency"] != float64(4) {
		t.Fatalf("Naive options are incomplete: %+v", configs["naive"])
	}
}

func TestClashUnsupportedProtocolVariantsAreMarked(t *testing.T) {
	body := []byte(`proxies:
  - name: TUIC-v4
    type: tuic
    server: tuic.example.com
    port: 443
    token: redacted
  - name: VLESS-XHTTP
    type: vless
    server: vless.example.com
    port: 443
    uuid: 33333333-3333-4333-8333-333333333333
    network: xhttp
  - name: SOCKS-TLS
    type: socks5
    server: socks.example.com
    port: 1080
    tls: true
  - name: Hysteria-FakeTCP
    type: hysteria
    server: hy.example.com
    port: 443
    protocol: faketcp
  - name: Trojan-SS
    type: trojan
    server: trojan.example.com
    port: 443
    password: redacted
    ss-opts:
      enabled: true
  - name: Snell-v3
    type: snell
    server: snell.example.com
    port: 443
    psk: redacted
    version: 3
  - name: Hysteria2-Certificate-Fingerprint
    type: hysteria2
    server: hy2.example.com
    port: 443
    password: redacted
    fingerprint: 95b89cf256ca58006f8d2f090bc8a6ca89b385f424852e82b59f2af384b142d7
  - name: Hysteria2-Invalid-Ports
    type: hysteria2
    server: hy2-invalid.example.com
    port: 443
    ports: 65535-60000
    password: redacted
  - name: Hysteria2-Invalid-Hop
    type: hysteria2
    server: hy2-invalid-hop.example.com
    port: 443
    hop-interval: 30-bad
    password: redacted
  - name: AnyTLS-Invalid-ECH
    type: anytls
    server: anytls-invalid-ech.example.com
    port: 443
    password: redacted
    ech-opts:
      enable: true
      config: not-valid-base64!
`)
	nodes, err := ParseSubscriptionNodes(body)
	if err != nil {
		t.Fatalf("parse unsupported Clash variants: %v", err)
	}
	if len(nodes) != 10 {
		t.Fatalf("parsed node count = %d, want 10", len(nodes))
	}
	for _, node := range nodes {
		if node.UnsupportedReason == "" {
			t.Fatalf("%s variant was not marked unsupported", node.Name)
		}
	}
}

func TestParseSSRURI(t *testing.T) {
	password := base64.StdEncoding.EncodeToString([]byte("pass"))
	remarks := base64.StdEncoding.EncodeToString([]byte("SSR-01"))
	decoded := "ssr.example.com:8388:origin:aes-128-gcm:plain:" + password + "/?remarks=" + remarks
	uri := "ssr://" + base64.StdEncoding.EncodeToString([]byte(decoded))
	node, err := ParseProxyURI(uri)
	if err != nil {
		t.Fatalf("parse ssr: %v", err)
	}
	if node.Name != "SSR-01" || node.Type != "ssr" || node.Server != "ssr.example.com" || node.ServerPort != 8388 {
		t.Fatalf("unexpected ssr node: %+v", node)
	}
}

func TestParseRemainingProxyURIs(t *testing.T) {
	cases := []struct {
		name   string
		uri    string
		typ    string
		server string
		port   int
	}{
		{name: "socks", uri: "socks5://user:pass@socks.example.com:1080#SOCKS-01", typ: "socks", server: "socks.example.com", port: 1080},
		{name: "http", uri: "http://user:pass@http.example.com:8080#HTTP-01", typ: "http", server: "http.example.com", port: 8080},
		{name: "hysteria", uri: "hysteria://auth@hy.example.com:443?sni=hy.example.com#HY-01", typ: "hysteria", server: "hy.example.com", port: 443},
		{name: "hysteria2", uri: "hy2://pass@hy2.example.com:443?sni=hy2.example.com#HY2-01", typ: "hysteria2", server: "hy2.example.com", port: 443},
		{name: "tuic", uri: "tuic://uuid:pass@tuic.example.com:443?sni=tuic.example.com#TUIC-01", typ: "tuic", server: "tuic.example.com", port: 443},
		{name: "anytls", uri: "anytls://pass@anytls.example.com:443?sni=anytls.example.com#AnyTLS-01", typ: "anytls", server: "anytls.example.com", port: 443},
		{name: "wireguard", uri: "wireguard://wg.example.com:51820?public-key=pub&private-key=priv#WG-01", typ: "wireguard", server: "wg.example.com", port: 51820},
		{name: "naive", uri: "naive+https://user:pass@naive.example.com:443#Naive-01", typ: "naive", server: "naive.example.com", port: 443},
		{name: "mieru", uri: "mieru://user:pass@mieru.example.com:2999?protocol=TCP#Mieru-01", typ: "mieru", server: "mieru.example.com", port: 2999},
		{name: "snell", uri: "snell://psk@snell.example.com:440?version=4#Snell-01", typ: "snell", server: "snell.example.com", port: 440},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, err := ParseProxyURI(tc.uri)
			if err != nil {
				t.Fatalf("parse proxy uri: %v", err)
			}
			if node.Type != tc.typ || node.Server != tc.server || node.ServerPort != tc.port {
				t.Fatalf("unexpected node: %+v", node)
			}
		})
	}
}
