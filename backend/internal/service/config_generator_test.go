package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRedactConfigAccessTokens(t *testing.T) {
	originalURL := "http://127.0.0.1:8080/api/v1/rules/content?access_token=secret-value&format=source"
	config := map[string]interface{}{
		"route": map[string]interface{}{
			"rule_set": []interface{}{
				map[string]interface{}{"url": originalURL},
			},
		},
	}

	redacted := redactConfigAccessTokens(config).(map[string]interface{})
	redactedURL := redacted["route"].(map[string]interface{})["rule_set"].([]interface{})[0].(map[string]interface{})["url"].(string)
	if strings.Contains(redactedURL, "secret-value") || !strings.Contains(redactedURL, "access_token=[REDACTED]") {
		t.Fatalf("config response did not redact access token: %q", redactedURL)
	}
	original := config["route"].(map[string]interface{})["rule_set"].([]interface{})[0].(map[string]interface{})["url"]
	if original != originalURL {
		t.Fatalf("redaction mutated generated config: %q", original)
	}
}

func TestMapMihomoUDPFlagToSingboxNetwork(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		wantNetwork interface{}
	}{
		{name: "udp true omitted", input: map[string]interface{}{"udp": true}, wantNetwork: nil},
		{name: "udp false maps tcp", input: map[string]interface{}{"udp": false}, wantNetwork: "tcp"},
		{name: "udp string false maps tcp", input: map[string]interface{}{"udp": "false"}, wantNetwork: "tcp"},
		{name: "udp string true omitted", input: map[string]interface{}{"udp": "true"}, wantNetwork: nil},
		{name: "existing network preserved when udp true", input: map[string]interface{}{"udp": true, "network": "tcp"}, wantNetwork: "tcp"},
		{name: "vless tcp transport with udp enabled allows both networks", input: map[string]interface{}{"type": "vless", "udp": true, "network": "tcp"}, wantNetwork: nil},
		{name: "vmess tcp transport with udp enabled allows both networks", input: map[string]interface{}{"type": "vmess", "udp": true, "network": "tcp"}, wantNetwork: nil},
		{name: "trojan tcp transport with udp enabled allows both networks", input: map[string]interface{}{"type": "trojan", "udp": true, "network": "tcp"}, wantNetwork: nil},
		{name: "native sing-box tcp restriction is preserved", input: map[string]interface{}{"type": "vless", "network": "tcp"}, wantNetwork: "tcp"},
		{name: "vless udp disabled remains tcp only", input: map[string]interface{}{"type": "vless", "udp": false, "network": "tcp"}, wantNetwork: "tcp"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mapMihomoUDPFlagToSingboxNetwork(tt.input)
			if _, exists := tt.input["udp"]; exists {
				t.Fatalf("udp should not be emitted to sing-box outbound: %+v", tt.input)
			}
			if got := tt.input["network"]; got != tt.wantNetwork {
				t.Fatalf("network = %v, want %v", got, tt.wantNetwork)
			}
		})
	}
}

func TestGenerateVLESSOutboundDoesNotTreatTCPTransportAsUDPRestriction(t *testing.T) {
	node := &model.Node{
		Type:    "vless",
		RawJSON: `{"type":"vless","server":"node.example.com","server_port":443,"uuid":"00000000-0000-4000-8000-000000000001","network":"tcp","udp":true,"tls":{"enabled":true}}`,
	}
	outbound, err := (&ConfigGeneratorService{}).generateNodeOutbound(node, "test-vless", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := outbound["udp"]; exists {
		t.Fatalf("generated outbound contains unsupported Mihomo udp field: %+v", outbound)
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("TCP transport incorrectly restricted generated VLESS outbound to TCP: %+v", outbound)
	}
}

func TestGenerateProxyDNSFinalUsesOneStableServer(t *testing.T) {
	servers := []map[string]interface{}{
		{"tag": "dns_direct", "type": "udp", "server": "1.1.1.1"},
		{"tag": "dns_proxy", "type": "https", "server": "dns.example.com"},
	}
	generated, final, err := generateProxyDNSFinal("dns_proxy", servers, map[string]bool{"dns_direct": true, "dns_proxy": true})
	if err != nil {
		t.Fatal(err)
	}
	if final != "ackwrap-proxy-dns" || generated["tag"] != final || generated["detour"] != "proxy" {
		t.Fatalf("proxy DNS final = %q, server = %+v", final, generated)
	}
	if generated["server"] != "dns.example.com" {
		t.Fatalf("proxy DNS clone lost base server options: %+v", generated)
	}
	if _, exists := servers[1]["detour"]; exists {
		t.Fatalf("proxy DNS generation mutated base server: %+v", servers[1])
	}

	existing := []map[string]interface{}{{"tag": "dns_proxy", "type": "https", "detour": "proxy"}}
	generated, final, err = generateProxyDNSFinal("dns_proxy", existing, map[string]bool{"dns_proxy": true})
	if err != nil || generated != nil || final != "dns_proxy" {
		t.Fatalf("existing proxy DNS final generated=%+v final=%q err=%v", generated, final, err)
	}
	if _, _, err := generateProxyDNSFinal("missing", servers, map[string]bool{"dns_direct": true, "dns_proxy": true}); err == nil || !strings.Contains(err.Error(), "不存在") {
		t.Fatalf("missing selected proxy DNS final error = %v", err)
	}
}

func TestApplyDomainResolverBinding(t *testing.T) {
	outbound := map[string]interface{}{"type": "vless", "tag": "node-1"}
	applyDomainResolverBinding(outbound, map[string]interface{}{"server": "dns_hk", "rewrite_ttl": 60})
	resolver, ok := outbound["domain_resolver"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected domain_resolver map, got %+v", outbound)
	}
	if resolver["server"] != "dns_hk" || resolver["rewrite_ttl"] != 60 {
		t.Fatalf("unexpected resolver: %+v", resolver)
	}
}

func TestEnabledDNSServerTagsExcludesDisabledServers(t *testing.T) {
	tags := enabledDNSServerTags([]model.DNSServer{
		{Tag: "dns_direct", Enabled: true},
		{Tag: "dns_proxy", Enabled: false},
		{Tag: "custom_fakeip", Enabled: true, ServerType: "fakeip"},
	}, false)
	if !tags["dns_direct"] {
		t.Fatal("enabled DNS server tag is missing")
	}
	if tags["dns_proxy"] {
		t.Fatal("disabled DNS server tag must not be available")
	}
	if tags["fakeip"] {
		t.Fatal("fakeip tag must not be available when fake IP is disabled")
	}
	if tags["custom_fakeip"] {
		t.Fatal("explicit fakeip server must not be available when TUN is disabled")
	}
}

func TestEnabledDNSServerTagsAddsGeneratedFakeIP(t *testing.T) {
	tags := enabledDNSServerTags([]model.DNSServer{{Tag: "custom_fakeip", Enabled: true, ServerType: "fakeip"}}, true)
	if !tags["fakeip"] {
		t.Fatal("generated fakeip server tag is missing")
	}
	if tags["custom_fakeip"] {
		t.Fatal("persisted custom fakeip server tag must not be available")
	}
}

func TestSelectDefaultDomainResolverUsesGeneratedBootstrap(t *testing.T) {
	settings := &model.DNSGlobalSettings{Enabled: true, Final: "dns_proxy"}
	if got := selectDefaultDomainResolver(settings, nil); got != "ackwrap-bootstrap-local" {
		t.Fatalf("resolver = %q, want generated local bootstrap", got)
	}

	servers := []model.DNSServer{
		{Tag: "dns_proxy", Enabled: false},
		{Tag: "custom_fakeip", Enabled: true, ServerType: "fakeip"},
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
	}
	if got := selectDefaultDomainResolver(settings, servers); got != "dns_direct" {
		t.Fatalf("resolver = %q, want dns_direct fallback", got)
	}
}

func TestSelectDefaultDomainResolverNeverUsesFakeIP(t *testing.T) {
	settings := &model.DNSGlobalSettings{Enabled: true, Final: "dns_proxy", FakeIPEnabled: true}
	if got := selectDefaultDomainResolver(settings, nil); got != "ackwrap-bootstrap-local" {
		t.Fatalf("resolver = %q, want generated local bootstrap", got)
	}
}

func TestApplyDNSServerAddressNormalizesDoHURL(t *testing.T) {
	server := map[string]interface{}{}
	if domain := applyDNSServerAddress(server, "https", "https://dns.example.com:8443/custom-query"); !domain {
		t.Fatal("DoH hostname should require a domain resolver")
	}
	if server["server"] != "dns.example.com" || server["server_port"] != uint16(8443) || server["path"] != "/custom-query" {
		t.Fatalf("unexpected normalized DoH server: %+v", server)
	}
}

func TestSelectDNSBootstrapTagUsesIPServer(t *testing.T) {
	servers := []model.DNSServer{
		{Tag: "dns_doh", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
		{Tag: "dns_ip", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
	}
	if got := selectDNSBootstrapTag(servers); got != "dns_ip" {
		t.Fatalf("bootstrap tag = %q, want dns_ip", got)
	}
}

func TestSelectDNSBootstrapTagExcludesProxyDetour(t *testing.T) {
	servers := []model.DNSServer{
		{Tag: "dns_proxy", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "proxy"},
	}
	if got := selectDNSBootstrapTag(servers); got != "" {
		t.Fatalf("bootstrap tag = %q, want empty for proxy-detoured server", got)
	}
}

func TestSafeDNSBootstrapTagRejectsProxyAndDomainResolvers(t *testing.T) {
	servers := []model.DNSServer{
		{Tag: "dns_proxy_ip", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "proxy"},
		{Tag: "dns_direct_domain", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
		{Tag: "dns_direct_ip", Enabled: true, ServerType: "udp", Address: "9.9.9.9"},
	}
	if isSafeDNSBootstrapTag("dns_proxy_ip", servers) {
		t.Fatal("proxy-detoured IP DNS must not bootstrap node or DNS server resolution")
	}
	if isSafeDNSBootstrapTag("dns_direct_domain", servers) {
		t.Fatal("domain-based DNS must not recursively bootstrap itself")
	}
	if !isSafeDNSBootstrapTag("dns_direct_ip", servers) {
		t.Fatal("direct IP DNS should be accepted as controlled bootstrap")
	}
}

func TestProxyDetouredDNSServerRequiresGeneratedBootstrap(t *testing.T) {
	servers := []model.DNSServer{
		{Tag: "dns_proxy", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "proxy"},
	}
	if !hasProxyDetouredDNSServer(servers) {
		t.Fatal("proxy-detoured DNS server should require a generated bootstrap when no direct server exists")
	}
}

func TestNeedsGeneratedDNSBootstrapForUnresolvedDoH(t *testing.T) {
	servers := []model.DNSServer{{Tag: "dns_doh", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"}}
	if !needsGeneratedDNSBootstrap(servers, enabledDNSServerTags(servers, false)) {
		t.Fatal("domain-based DoH without another resolver should generate a local bootstrap")
	}
}

func TestBuiltinOutboundTagsOnlyIncludesRealOutbounds(t *testing.T) {
	got := collectionBuiltinOutboundTags(&model.ProxyCollectionWithNodes{
		ProxyCollection: model.ProxyCollection{
			Name:     "custom",
			NodeUIDs: `["reject","block","direct"]`,
		},
		NodeUIDs: []string{"reject", "block", "direct"},
	})
	if len(got) != 1 || got[0] != "direct" {
		t.Fatalf("builtin outbound tags = %v, want [direct]", got)
	}
}

func TestGenerateCollectionOutboundIncludesGroupsAndTheirNodes(t *testing.T) {
	service := &ConfigGeneratorService{}
	collection := &model.ProxyCollectionWithNodes{
		ProxyCollection: model.ProxyCollection{
			Name:       "Google",
			Type:       "selector",
			SourceType: proxyCollectionSourceNodeGroupsAndNodes,
			NodeUIDs:   `["direct"]`,
		},
		NodeUIDs: []string{"direct"},
		ReferencedGroups: []model.NodeGroup{
			{ID: 1, Name: "新加坡节点"},
			{ID: 2, Name: "日本节点"},
		},
	}
	outbound, err := service.generateCollectionOutbound(
		collection,
		map[string]bool{"新加坡节点": true, "日本节点": true},
		map[string]string{"node-1": "SG-node-1", "node-2": "SG-node-2"},
		map[int64][]string{1: {"node-1", "node-2"}, 2: {"node-2"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := outbound["outbounds"].([]string)
	if !ok {
		t.Fatalf("outbounds type = %T, want []string", outbound["outbounds"])
	}
	want := []string{"direct", "新加坡节点", "SG-node-1", "SG-node-2", "日本节点"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("outbounds = %v, want %v", got, want)
	}
}

func TestRouteRuleBlockUsesRejectAction(t *testing.T) {
	rule := singboxRouteRule("domain_suffix", []string{"example.com"}, "block", false)
	if rule["action"] != "reject" {
		t.Fatalf("action = %v, want reject: %+v", rule["action"], rule)
	}
	if _, exists := rule["outbound"]; exists {
		t.Fatalf("reject action must not emit outbound: %+v", rule)
	}
}

func TestRouteRuleDirectUsesRouteAction(t *testing.T) {
	rule := singboxRouteRule("domain_suffix", []string{"example.com"}, "direct", false)
	if rule["action"] != "route" || rule["outbound"] != "direct" {
		t.Fatalf("route action = %+v, want action=route outbound=direct", rule)
	}
}

func TestDefaultBypassProcessNamesIncludesAckwrapAndCore(t *testing.T) {
	names := defaultBypassProcessNames(
		filepath.Join("custom", "ackwrap-windows-amd64.exe"),
		filepath.Join("bin", "sing-box.exe"),
	)
	for _, expected := range []string{"ackwrap", "ackwrap.exe", "ackwrap-windows-amd64.exe", "sing-box.exe"} {
		found := false
		for _, name := range names {
			if name == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("process whitelist %v does not contain %q", names, expected)
		}
	}
}

func TestNodeServerBypassTargetsSeparatesDomainsAndIPs(t *testing.T) {
	domains, ipCIDRs := nodeServerBypassTargets([]model.Node{
		{Server: "Node.Example.COM."},
		{Server: "node.example.com"},
		{Server: "192.0.2.10"},
		{Server: "[2001:db8::10]"},
		{Server: "192.0.2.10"},
	})
	if len(domains) != 1 || domains[0] != "node.example.com" {
		t.Fatalf("domains = %v, want [node.example.com]", domains)
	}
	if len(ipCIDRs) != 2 || ipCIDRs[0] != "192.0.2.10/32" || ipCIDRs[1] != "2001:db8::10/128" {
		t.Fatalf("ip_cidr = %v, want IPv4 and IPv6 host routes", ipCIDRs)
	}
}

func TestGenerateRouteIncludesDefaultLoopBypassRules(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "bypass-test", URL: "https://example.com/subscription",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{
		{Name: "Domain Node", Type: "socks", Server: "node.example.com", ServerPort: 1080, RawJSON: `{"type":"socks"}`},
		{Name: "IP Node", Type: "socks", Server: "192.0.2.20", ServerPort: 1080, RawJSON: `{"type":"socks"}`},
	}); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	route, err := service.generateRoute("")
	if err != nil {
		t.Fatal(err)
	}
	if route["find_process"] != true {
		t.Fatalf("find_process = %v, want true", route["find_process"])
	}
	if route["default_http_client"] != defaultRuleSetHTTPClientTag {
		t.Fatalf("default_http_client = %v, want %s", route["default_http_client"], defaultRuleSetHTTPClientTag)
	}
	if route["auto_detect_interface"] != true {
		t.Fatalf("auto_detect_interface = %v, want true for TUN mode", route["auto_detect_interface"])
	}
	rules, ok := route["rules"].([]map[string]interface{})
	if !ok {
		t.Fatalf("route rules type = %T", route["rules"])
	}
	var processRule, processRuleScoped, domainRule, ipRule, reachedSniff bool
	standardDNSHijackIndex, firstBypassIndex := -1, -1
	for index, rule := range rules {
		if stringListContains(rule["inbound"], legacyUpdateProxyInboundTag) {
			t.Fatalf("generated route contains legacy update proxy rule: %+v", rule)
		}
		if rule["action"] == "hijack-dns" && rule["port"] == 53 {
			standardDNSHijackIndex = index
		}
		if rule["action"] == "sniff" {
			reachedSniff = true
			continue
		}
		if rule["outbound"] != "direct" {
			continue
		}
		if reachedSniff || rule["action"] != "bypass" {
			t.Fatalf("loop bypass rule must use bypass before sniff: %+v", rules)
		}
		if firstBypassIndex == -1 {
			firstBypassIndex = index
		}
		if rule["process_name"] != nil {
			processRule = true
			inbound, ok := rule["inbound"].([]string)
			processRuleScoped = ok && len(inbound) == 1 && inbound[0] == "tun-in"
		}
		domainRule = domainRule || rule["domain"] != nil
		ipRule = ipRule || rule["ip_cidr"] != nil
	}
	if !processRule || !processRuleScoped || !domainRule || !ipRule {
		t.Fatalf("missing loop bypass rule: %+v", rules)
	}
	if standardDNSHijackIndex == -1 || firstBypassIndex == -1 || standardDNSHijackIndex >= firstBypassIndex {
		t.Fatalf("standard DNS hijack must precede every bypass rule: %+v", rules)
	}
}

func TestGeneratedTUNInboundUsesAutoRedirectOnLinux(t *testing.T) {
	inbound := generatedTUNInbound(true, defaultTUNIPv4Address, defaultTUNIPv6Address, nil, nil)
	if inbound["auto_route"] != true || inbound["strict_route"] != true || inbound["auto_redirect"] != true {
		t.Fatalf("OpenWrt TUN inbound = %+v", inbound)
	}
	if inbound["auto_redirect_output_mark"] != "0x2024" {
		t.Fatalf("OpenWrt TUN output mark = %v, want 0x2024", inbound["auto_redirect_output_mark"])
	}
	if inbound["iproute2_table_index"] != 2022 || inbound["iproute2_rule_index"] != 9000 || inbound["auto_redirect_iproute2_fallback_rule_index"] != 32768 {
		t.Fatalf("OpenWrt TUN lifecycle identity = %+v", inbound)
	}
	if !stringListContains(inbound["address"], defaultTUNIPv4Address) || !stringListContains(inbound["address"], defaultTUNIPv6Address) {
		t.Fatalf("OpenWrt TUN inbound is not dual-stack: %+v", inbound)
	}
	withoutRedirect := generatedTUNInbound(false, defaultTUNIPv4Address, defaultTUNIPv6Address, nil, nil)
	if _, exists := withoutRedirect["auto_redirect"]; exists {
		t.Fatalf("non-Linux TUN inbound contains auto_redirect: %+v", withoutRedirect)
	}
	if _, exists := withoutRedirect["auto_redirect_output_mark"]; exists {
		t.Fatalf("non-Linux TUN inbound contains auto_redirect output mark: %+v", withoutRedirect)
	}
	for _, field := range []string{"iproute2_table_index", "iproute2_rule_index", "auto_redirect_iproute2_fallback_rule_index"} {
		if _, exists := withoutRedirect[field]; exists {
			t.Fatalf("non-auto-redirect TUN inbound contains %s: %+v", field, withoutRedirect)
		}
	}
}

func TestNormalizeTUNAddresses(t *testing.T) {
	ipv4, ipv6, err := normalizeTUNAddresses("10.254.0.1/30", "fd12:3456:789a::1/126")
	if err != nil {
		t.Fatal(err)
	}
	if ipv4 != "10.254.0.1/30" || ipv6 != "fd12:3456:789a::1/126" {
		t.Fatalf("normalized TUN addresses = %q, %q", ipv4, ipv6)
	}
	defaultIPv4, defaultIPv6, err := normalizeTUNAddresses("", "")
	if err != nil {
		t.Fatal(err)
	}
	if defaultIPv4 != defaultTUNIPv4Address || defaultIPv6 != defaultTUNIPv6Address {
		t.Fatalf("default TUN addresses = %q, %q", defaultIPv4, defaultIPv6)
	}
	migratedIPv4, migratedIPv6, err := normalizeTUNAddresses(previousDefaultTUNIPv4, previousDefaultTUNIPv6)
	if err != nil {
		t.Fatal(err)
	}
	if migratedIPv4 != defaultTUNIPv4Address || migratedIPv6 != defaultTUNIPv6Address {
		t.Fatalf("migrated TUN addresses = %q, %q", migratedIPv4, migratedIPv6)
	}
	for _, test := range []struct {
		name string
		ipv4 string
		ipv6 string
	}{
		{name: "missing IPv4 CIDR", ipv4: "10.0.0.1", ipv6: defaultTUNIPv6Address},
		{name: "IPv6 in IPv4 field", ipv4: defaultTUNIPv6Address, ipv6: defaultTUNIPv6Address},
		{name: "IPv4-mapped IPv6 CIDR", ipv4: "::ffff:10.0.0.1/120", ipv6: defaultTUNIPv6Address},
		{name: "IPv4 in IPv6 field", ipv4: defaultTUNIPv4Address, ipv6: defaultTUNIPv4Address},
		{name: "IPv4 network address", ipv4: "10.0.0.0/30", ipv6: defaultTUNIPv6Address},
		{name: "IPv4 broadcast address", ipv4: "10.0.0.3/30", ipv6: defaultTUNIPv6Address},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, _, err := normalizeTUNAddresses(test.ipv4, test.ipv6); err == nil {
				t.Fatal("expected invalid TUN address to fail")
			}
		})
	}
}

func TestMapTLSFingerprintFields(t *testing.T) {
	const fingerprint = "dd9dd03d942400ad4c1400879bda98f4fa097183aa9a91a1423cdd42a3e183d7"
	legacyFingerprint := fingerprint[:2] + ":" + fingerprint[2:]
	nodeData := map[string]interface{}{
		"skip-cert-verify": true,
		"alpn":             []string{"h3"},
		"tls": map[string]interface{}{
			"enabled": true,
			"utls":    map[string]interface{}{"enabled": true, "fingerprint": legacyFingerprint},
		},
	}
	if err := mapTLSFingerprintFields(nodeData); err != nil {
		t.Fatal(err)
	}
	tlsMap := nodeData["tls"].(map[string]interface{})
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("invalid uTLS fingerprint should be removed: %+v", tlsMap)
	}
	pins, ok := tlsMap["certificate_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != fingerprint {
		t.Fatalf("expected certificate pin, got %+v", tlsMap["certificate_sha256"])
	}
	if tlsMap["insecure"] != true {
		t.Fatalf("expected legacy insecure field to move into TLS: %+v", nodeData)
	}
	if _, exists := nodeData["skip-cert-verify"]; exists {
		t.Fatalf("legacy insecure field was not removed: %+v", nodeData)
	}
	if _, exists := nodeData["alpn"]; exists {
		t.Fatalf("legacy ALPN field was not removed: %+v", nodeData)
	}
	if alpn, ok := tlsMap["alpn"].([]string); !ok || len(alpn) != 1 || alpn[0] != "h3" {
		t.Fatalf("expected legacy ALPN field to move into TLS: %+v", nodeData)
	}

	nodeData = map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled": true,
			"utls":    map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
		},
	}
	if err := mapTLSFingerprintFields(nodeData); err != nil {
		t.Fatal(err)
	}
	tlsMap = nodeData["tls"].(map[string]interface{})
	utlsMap := tlsMap["utls"].(map[string]interface{})
	if utlsMap["fingerprint"] != "chrome" {
		t.Fatalf("valid uTLS fingerprint should be preserved: %+v", tlsMap)
	}

	nodeData = map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled":                       true,
			"certificate_public_key_sha256": []interface{}{legacyFingerprint},
		},
	}
	if err := mapTLSFingerprintFields(nodeData); err != nil {
		t.Fatal(err)
	}
	tlsMap = nodeData["tls"].(map[string]interface{})
	if _, exists := tlsMap["certificate_public_key_sha256"]; exists {
		t.Fatalf("legacy certificate pin field was not removed: %+v", tlsMap)
	}
	pins, ok = tlsMap["certificate_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != fingerprint {
		t.Fatalf("legacy certificate pin was not migrated: %+v", tlsMap)
	}

	nodeData = map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled":            true,
			"certificate_sha256": []string{strings.Repeat("a", 64)},
			"utls":               map[string]interface{}{"enabled": true, "fingerprint": legacyFingerprint},
		},
	}
	if err := mapTLSFingerprintFields(nodeData); err != nil {
		t.Fatal(err)
	}
	tlsMap = nodeData["tls"].(map[string]interface{})
	pins = tlsMap["certificate_sha256"].([]string)
	if len(pins) != 2 || pins[0] != strings.Repeat("a", 64) || pins[1] != fingerprint {
		t.Fatalf("legacy certificate pin did not merge with existing pins: %+v", pins)
	}

	nodeData = map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled": true,
			"utls":    map[string]interface{}{"enabled": true, "fingerprint": "unknown-client"},
		},
	}
	if err := mapTLSFingerprintFields(nodeData); err == nil {
		t.Fatal("expected unknown legacy TLS fingerprint error")
	}
}

func TestGenerateNodeOutboundSupportsNewCoreProtocols(t *testing.T) {
	svc := &ConfigGeneratorService{}
	tests := []struct {
		typ     string
		rawJSON string
	}{
		{typ: "anytls", rawJSON: `{"type":"anytls","server":"example.com","server_port":443,"password":"redacted"}`},
		{typ: "snell", rawJSON: `{"type":"snell","server":"example.com","server_port":443,"version":4,"psk":"redacted"}`},
	}
	for _, test := range tests {
		t.Run(test.typ, func(t *testing.T) {
			outbound, err := svc.generateNodeOutbound(&model.Node{Type: test.typ, RawJSON: test.rawJSON}, test.typ+"-node", nil)
			if err != nil {
				t.Fatalf("generate outbound: %v", err)
			}
			if outbound["type"] != test.typ || outbound["tag"] != test.typ+"-node" {
				t.Fatalf("unexpected outbound: %+v", outbound)
			}
			if test.typ == "anytls" {
				tlsOptions, ok := outbound["tls"].(map[string]interface{})
				if !ok || tlsOptions["enabled"] != true {
					t.Fatalf("required TLS was not enabled: %+v", outbound["tls"])
				}
			}
		})
	}
}

func TestEnsureRequiredOutboundTLS(t *testing.T) {
	for _, outboundType := range []string{"anytls", "hysteria", "hysteria2", "naive", "shadowtls", "trojan", "tuic"} {
		t.Run(outboundType, func(t *testing.T) {
			nodeData := map[string]interface{}{"tls": map[string]interface{}{"enabled": false}}
			ensureRequiredOutboundTLS(nodeData, outboundType)
			tlsOptions := nodeData["tls"].(map[string]interface{})
			if tlsOptions["enabled"] != true {
				t.Fatalf("TLS enabled = %v, want true", tlsOptions["enabled"])
			}
		})
	}

	nodeData := map[string]interface{}{}
	ensureRequiredOutboundTLS(nodeData, "vmess")
	if _, exists := nodeData["tls"]; exists {
		t.Fatal("optional TLS protocol should remain unchanged")
	}

}

func TestGenerateNodeOutboundRemovesZeroVMessAlterID(t *testing.T) {
	service := &ConfigGeneratorService{}
	node := &model.Node{
		Name:    "legacy-vmess",
		Type:    "vmess",
		RawJSON: `{"type":"vmess","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","cipher":"auto","alter_id":0}`,
	}
	outbound, err := service.generateNodeOutbound(node, "legacy-vmess", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := outbound["alter_id"]; exists {
		t.Fatalf("generated outbound contains unsupported alter_id: %+v", outbound)
	}
	if _, exists := outbound["cipher"]; exists {
		t.Fatalf("generated outbound contains legacy cipher: %+v", outbound)
	}
	if outbound["security"] != "auto" {
		t.Fatalf("generated outbound security = %v, want auto", outbound["security"])
	}
}

func TestGenerateNodeOutboundMapsSSRToMaintainedCoreType(t *testing.T) {
	service := NewConfigGeneratorService(nil, nil)
	node := &model.Node{
		Type:    "ssr",
		RawJSON: `{"type":"ssr","server":"example.com","server_port":8388,"cipher":"aes-256-cfb","password":"redacted","protocol":"auth_aes128_sha1","protocol-param":"1000:test","obfs":"http_simple","obfs-param":"cdn.example.com","group":"legacy-subscription-group","udp":true}`,
	}
	outbound, err := service.generateNodeOutbound(node, "ssr-test", nil)
	if err != nil {
		t.Fatalf("generate SSR outbound: %v", err)
	}
	if outbound["type"] != "shadowsocksr" || outbound["method"] != "aes-256-cfb" || outbound["protocol_param"] != "1000:test" || outbound["obfs_param"] != "cdn.example.com" {
		t.Fatalf("SSR outbound mapping is incomplete: %+v", outbound)
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("UDP-capable SSR must not be restricted to TCP: %+v", outbound)
	}
	for _, key := range []string{"cipher", "protocol-param", "obfs-param", "group", "udp"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("legacy SSR field %q leaked into outbound: %+v", key, outbound)
		}
	}
}

func TestGenerateNodeOutboundRemovesVLESSCipher(t *testing.T) {
	service := &ConfigGeneratorService{}
	node := &model.Node{
		Name:    "legacy-vless",
		Type:    "vless",
		RawJSON: `{"type":"vless","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","cipher":"auto"}`,
	}
	outbound, err := service.generateNodeOutbound(node, "legacy-vless", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := outbound["cipher"]; exists {
		t.Fatalf("generated VLESS outbound contains unsupported cipher: %+v", outbound)
	}
	if _, exists := outbound["method"]; exists {
		t.Fatalf("generated VLESS outbound contains unsupported method: %+v", outbound)
	}
}

func TestGenerateNodeOutboundNormalizesLegacyVLESSRealityOptions(t *testing.T) {
	svc := &ConfigGeneratorService{}
	outbound, err := svc.generateNodeOutbound(&model.Node{
		Type:    "vless",
		RawJSON: `{"type":"vless","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","flow":"xtls-rprx-vision","tls":{"enabled":true,"server_name":"www.example.com","utls":{"enabled":true,"fingerprint":"chrome"}},"reality-opts":{"public-key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA","short-id":"01234567"},"network":"tcp"}`,
	}, "reality-node", nil)
	if err != nil {
		t.Fatalf("generate Reality outbound: %v", err)
	}
	if _, exists := outbound["reality-opts"]; exists {
		t.Fatalf("legacy reality-opts leaked into outbound: %+v", outbound)
	}
	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing TLS options: %+v", outbound)
	}
	reality, ok := tlsMap["reality"].(map[string]interface{})
	if !ok || reality["public_key"] != "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA" || reality["short_id"] != "01234567" {
		t.Fatalf("unexpected Reality options: %+v", tlsMap)
	}
}

func TestGenerateNodeOutboundNormalizesLegacyV2RayTransport(t *testing.T) {
	svc := &ConfigGeneratorService{}
	outbound, err := svc.generateNodeOutbound(&model.Node{
		Type:    "vless",
		RawJSON: `{"type":"vless","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","network":"ws","ws-opts":{"path":"/socket","headers":{"Host":"edge.example.com"}}}`,
	}, "ws-node", nil)
	if err != nil {
		t.Fatalf("generate legacy WebSocket outbound: %v", err)
	}
	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || transport["type"] != "ws" || transport["path"] != "/socket" {
		t.Fatalf("unexpected WebSocket transport: %+v", outbound)
	}
	for _, legacyKey := range []string{"network", "ws-opts"} {
		if _, exists := outbound[legacyKey]; exists {
			t.Fatalf("legacy transport field %q leaked into outbound: %+v", legacyKey, outbound)
		}
	}
}

func TestGenerateNodeOutboundNormalizesLegacyProtocolFields(t *testing.T) {
	svc := &ConfigGeneratorService{}
	tests := []struct {
		name    string
		typ     string
		rawJSON string
		check   func(*testing.T, map[string]interface{})
	}{
		{
			name:    "hysteria",
			typ:     "hysteria",
			rawJSON: `{"type":"hysteria","server":"example.com","server_port":443,"auth_str":"redacted","obfs-param":"secret","receive-window":"1024","receive-window-conn":"512"}`,
			check: func(t *testing.T, outbound map[string]interface{}) {
				if outbound["obfs"] != "secret" || outbound["recv_window"] != 1024 || outbound["recv_window_conn"] != 512 {
					t.Fatalf("unexpected Hysteria fields: %+v", outbound)
				}
			},
		},
		{
			name:    "tuic",
			typ:     "tuic",
			rawJSON: `{"type":"tuic","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","password":"redacted","udp-relay-mode":"native","reduce-rtt":true}`,
			check: func(t *testing.T, outbound map[string]interface{}) {
				if outbound["udp_relay_mode"] != "native" || outbound["zero_rtt_handshake"] != true {
					t.Fatalf("unexpected TUIC fields: %+v", outbound)
				}
			},
		},
		{
			name:    "shadowsocks-v2ray-plugin",
			typ:     "shadowsocks",
			rawJSON: `{"type":"shadowsocks","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"redacted","network":"ws","ws-opts":{"path":"/socket","headers":{"Host":"edge.example.com"}},"tls":{"enabled":true,"server_name":"edge.example.com"}}`,
			check: func(t *testing.T, outbound map[string]interface{}) {
				options, _ := outbound["plugin_opts"].(string)
				if outbound["plugin"] != "v2ray-plugin" || !strings.Contains(options, "mode=websocket") || !strings.Contains(options, "host=edge.example.com") || !strings.Contains(options, "path=/socket") || !strings.Contains(options, "tls") {
					t.Fatalf("unexpected Shadowsocks plugin fields: %+v", outbound)
				}
				for _, legacyKey := range []string{"network", "ws-opts", "tls"} {
					if _, exists := outbound[legacyKey]; exists {
						t.Fatalf("legacy Shadowsocks field %q leaked into outbound: %+v", legacyKey, outbound)
					}
				}
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outbound, err := svc.generateNodeOutbound(&model.Node{Type: test.typ, RawJSON: test.rawJSON}, test.name, nil)
			if err != nil {
				t.Fatal(err)
			}
			test.check(t, outbound)
		})
	}
}

func TestGenerateNodeOutboundRejectsLossyShadowsocksPluginMigration(t *testing.T) {
	svc := &ConfigGeneratorService{}
	_, err := svc.generateNodeOutbound(&model.Node{
		Type:    "shadowsocks",
		RawJSON: `{"type":"shadowsocks","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"redacted","network":"ws","ws-opts":{"headers":{"Host":"ws.example.com"}},"tls":{"enabled":true,"server_name":"tls.example.com"}}`,
	}, "conflicting-ss", nil)
	if err == nil || !strings.Contains(err.Error(), "无法同时映射") {
		t.Fatalf("expected lossless migration error, got %v", err)
	}
}

func TestGenerateNodeOutboundValidatesShadowsocksPluginStrings(t *testing.T) {
	svc := &ConfigGeneratorService{}
	outbound, err := svc.generateNodeOutbound(&model.Node{
		Type:    "shadowsocks",
		RawJSON: `{"type":"shadowsocks","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"redacted","plugin":"obfs","plugin_opts":"host=edge.example;mode=tls"}`,
	}, "obfs-node", nil)
	if err != nil {
		t.Fatalf("normalize supported obfs migration: %v", err)
	}
	if outbound["plugin"] != "obfs-local" || outbound["plugin_opts"] != "obfs=tls;obfs-host=edge.example" {
		t.Fatalf("unexpected normalized obfs plugin: %+v", outbound)
	}

	for _, options := range []string{"unknown=value", "mux=invalid", `path=/socket\`} {
		_, err := svc.generateNodeOutbound(&model.Node{
			Type:    "shadowsocks",
			RawJSON: fmt.Sprintf(`{"type":"shadowsocks","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"redacted","plugin":"v2ray-plugin","plugin_opts":%q}`, options),
		}, "invalid-plugin", nil)
		if err == nil {
			t.Fatal("expected invalid Shadowsocks plugin options to be rejected")
		}
	}
	_, err = svc.generateNodeOutbound(&model.Node{
		Type:    "shadowsocks",
		RawJSON: `{"type":"shadowsocks","server":"example.com","server_port":8388,"method":"aes-128-gcm","password":"redacted","plugin":"v2ray-plugin","plugin_opts":"tls","alpn":["h2"]}`,
	}, "invalid-tls-migration", nil)
	if err == nil || !strings.Contains(err.Error(), "alpn") {
		t.Fatalf("expected unsupported legacy TLS field error, got %v", err)
	}
}

func TestGenerateNodeOutboundRejectsNonZeroVMessAlterID(t *testing.T) {
	service := &ConfigGeneratorService{}
	node := &model.Node{
		Name:    "legacy-vmess",
		Type:    "vmess",
		RawJSON: `{"type":"vmess","server":"example.com","server_port":443,"uuid":"00000000-0000-0000-0000-000000000000","security":"auto","alter_id":64}`,
	}
	if _, err := service.generateNodeOutbound(node, "legacy-vmess", nil); err == nil {
		t.Fatal("generateNodeOutbound() error = nil, want unsupported legacy alter_id error")
	}
}

func TestGenerateOutboundsDoesNotApplyNodeListPageLimit(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{
		Name: "synthetic-subscription", URL: "https://example.com/subscription",
	})
	if err != nil {
		t.Fatal(err)
	}
	nodes := make([]model.ParsedNode, 0, 75)
	for index := 0; index < 75; index++ {
		server := fmt.Sprintf("node-%03d.example.com", index)
		nodes = append(nodes, model.ParsedNode{
			Name:       fmt.Sprintf("Node %03d", index),
			Type:       "socks",
			Server:     server,
			ServerPort: 1080,
			RawJSON:    fmt.Sprintf(`{"type":"socks","server":%q,"server_port":1080}`, server),
		})
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, nodes); err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "全部节点", Type: "selector", FilterInclude: ".*", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	outbounds, _, err := service.generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range outbounds {
		outbound, ok := item.(map[string]interface{})
		if !ok || outbound["tag"] != group.Name {
			continue
		}
		members, ok := outbound["outbounds"].([]string)
		if !ok {
			t.Fatalf("group outbounds type = %T, want []string", outbound["outbounds"])
		}
		if len(members) != 75 {
			t.Fatalf("group member count = %d, want 75", len(members))
		}
		return
	}
	t.Fatal("generated all-nodes group not found")
}

func TestGenerateOutboundsUsesGlobalConnectivitySettings(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetConnectivitySettings(&model.ConnectivitySettings{
		TestURL: "http://connectivity.example/generate_204", IntervalSeconds: 120,
	}); err != nil {
		t.Fatal(err)
	}
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "connectivity", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		UID: "connectivity-node", Name: "Connectivity Node", Type: "socks", Server: "127.0.0.1", ServerPort: 1080,
		RawJSON: `{"type":"socks","server":"127.0.0.1","server_port":1080}`,
	}}); err != nil {
		t.Fatal(err)
	}
	group, err := db.CreateNodeGroup(&model.NodeGroupRequest{
		Name: "Auto Region", Type: "urltest", FilterInclude: ".*", TestURL: "https://legacy.example/check", TestInterval: 600, Tolerance: 80, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Auto Service", Enabled: true, RuleType: "domain", Values: []string{"auto.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	collection := &model.ProxyCollection{
		Name: "Auto Service", Type: "urltest", SourceType: proxyCollectionSourceNodeGroups,
		ReferencedGroupIDs: fmt.Sprintf("[%d]", group.ID), RouteRuleID: rule.ID, RouteRuleIDs: "[" + fmt.Sprint(rule.ID) + "]", NodeUIDs: "[]",
		TestURL: "https://legacy.example/check", TestInterval: 900, Tolerance: 90, Enabled: true,
	}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatal(err)
	}

	outbounds, _, err := NewConfigGeneratorService(db, nil).generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{"Auto Region": false, "Auto Service": false}
	for _, item := range outbounds {
		outbound, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		tag, ok := outbound["tag"].(string)
		if _, exists := found[tag]; !ok || !exists {
			continue
		}
		if outbound["url"] != "http://connectivity.example/generate_204" || outbound["interval"] != "120s" {
			t.Fatalf("outbound connectivity settings = %+v", outbound)
		}
		found[tag] = true
	}
	for tag, ok := range found {
		if !ok {
			t.Fatalf("generated outbound %q not found", tag)
		}
	}
}
func TestGenerateOutboundsIncludesEnabledNodesForCoreExitIP(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "exit-check", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		Name: "Node A", Type: "socks", Server: "node.example.com", ServerPort: 1080,
		RawJSON: `{"type":"socks","server":"node.example.com","server_port":1080}`,
	}}); err != nil {
		t.Fatal(err)
	}
	outbounds, _, err := NewConfigGeneratorService(db, nil).generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("enabled nodes: %d, %v", len(nodes), err)
	}
	expectedTag := buildNodeOutboundTags(nodes)[nodes[0].UID]
	foundNode := false
	for _, item := range outbounds {
		outbound, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if outbound["tag"] == "ackwrap-internal-node-check" {
			t.Fatal("legacy internal selector must not be generated")
		}
		if outbound["tag"] == expectedTag {
			foundNode = true
		}
	}
	if !foundNode {
		t.Fatal("enabled node outbound required by core exit IP API was not generated")
	}
}

func TestGenerateOutboundsFallsBackWhenProxyCollectionIsEmpty(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: "proxy", Type: "selector", SourceType: "manual",
		ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	outbounds, _, err := NewConfigGeneratorService(db, nil).generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range outbounds {
		outbound, ok := item.(map[string]interface{})
		if !ok || outbound["tag"] != "proxy" {
			continue
		}
		members, ok := outbound["outbounds"].([]string)
		if outbound["type"] != "selector" || !ok || len(members) != 1 || members[0] != "direct" {
			t.Fatalf("empty proxy fallback = %+v", outbound)
		}
		return
	}
	t.Fatal("proxy fallback outbound not found")
}

func TestGenerateRejectsDirectProxyFallbackWhenDNSDependsOnProxy(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		defaultOutbound string
		setup           func(*testing.T, *store.Store)
	}{
		{name: "global mode", setup: func(t *testing.T, db *store.Store) {
			if err := db.SetProxyMode("global"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "proxy route", setup: func(t *testing.T, db *store.Store) {
			rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Proxy Route", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.CreateProxyCollection(&model.ProxyCollection{Name: rule.Name, Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleID: rule.ID, RouteRuleIDs: "[" + fmt.Sprint(rule.ID) + "]", NodeUIDs: `["direct"]`, Enabled: true}); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataDir := t.TempDir()
			db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := db.CreateProxyCollection(&model.ProxyCollection{
				Name: "proxy", Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", NodeUIDs: "[]", Enabled: true,
			}); err != nil {
				t.Fatal(err)
			}
			if testCase.setup != nil {
				testCase.setup(t, db)
			}
			_, err = NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir}).generateLockedTo(&model.ConfigGenerateRequest{
				DefaultOutbound: testCase.defaultOutbound, InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
				TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
			}, filepath.Join(dataDir, "config.json"))
			if err == nil || !strings.Contains(err.Error(), "DNS 依赖 proxy 策略组") {
				t.Fatalf("direct proxy fallback error = %v", err)
			}
		})
	}
}

func TestGenerateRejectsAutomaticProxyWithOnlyDirectMembersWhenDNSDependsOnProxy(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_proxy", Enabled: true, ServerType: "udp", Address: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Proxy Route", Enabled: true, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{Name: rule.Name, Type: "selector", SourceType: "manual", ReferencedGroupIDs: "[]", RouteRuleID: rule.ID, RouteRuleIDs: "[" + fmt.Sprint(rule.ID) + "]", NodeUIDs: `["direct"]`, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	_, err = NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir}).generateLockedTo(&model.ConfigGenerateRequest{
		DefaultOutbound: "proxy", InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
		TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
	}, filepath.Join(dataDir, "config.json"))
	if err == nil || !strings.Contains(err.Error(), "DNS 依赖 proxy 策略组") {
		t.Fatalf("automatic direct-only proxy fallback error = %v", err)
	}
}

func TestGenerateRejectsDNSDetourWithoutNonDirectPath(t *testing.T) {
	for _, testCase := range []struct {
		name            string
		defaultOutbound string
		setup           func(*testing.T, *store.Store)
	}{
		{
			name: "explicit proxy strategy",
			setup: func(t *testing.T, db *store.Store) {
				if err := db.SetProxyMode("global"); err != nil {
					t.Fatal(err)
				}
				if err := db.CreateProxyCollection(&model.ProxyCollection{
					Name: "proxy", Type: "selector", SourceType: "manual", NodeUIDs: `["direct"]`, ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "bound route strategy",
			setup: func(t *testing.T, db *store.Store) {
				rule, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Regional Proxy", Enabled: true, RuleType: "domain", Values: []string{"regional.example"}, Outbound: "proxy"})
				if err != nil {
					t.Fatal(err)
				}
				if err := db.CreateProxyCollection(&model.ProxyCollection{
					Name: "Regional Proxy", Type: "selector", SourceType: "manual", NodeUIDs: `["direct"]`, ReferencedGroupIDs: "[]", RouteRuleID: rule.ID, RouteRuleIDs: "[" + fmt.Sprint(rule.ID) + "]", Enabled: true,
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "manual resolver detour",
			setup: func(t *testing.T, db *store.Store) {
				if err := db.CreateProxyCollection(&model.ProxyCollection{
					Name: "regional-proxy", Type: "selector", SourceType: "manual", NodeUIDs: `["direct"]`, ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
				}); err != nil {
					t.Fatal(err)
				}
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dataDir := t.TempDir()
			db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if testCase.setup != nil {
				testCase.setup(t, db)
			}
			detour := ""
			if testCase.name == "manual resolver detour" {
				detour = "regional-proxy"
			}
			if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_remote", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: detour}); err != nil {
				t.Fatal(err)
			}
			_, err = NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir}).generateLockedTo(&model.ConfigGenerateRequest{
				DefaultOutbound: testCase.defaultOutbound, InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
				TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
			}, filepath.Join(dataDir, "config.json"))
			if err == nil || (!strings.Contains(err.Error(), "没有可用非直连代理路径") && !strings.Contains(err.Error(), "DNS 依赖 proxy 策略组")) {
				t.Fatalf("direct-only DNS detour error = %v", err)
			}
		})
	}
}

func TestHasUsableNonDirectOutboundPath(t *testing.T) {
	if hasUsableNonDirectOutboundPath([]interface{}{
		map[string]interface{}{"tag": "direct", "type": "direct"},
		map[string]interface{}{"tag": "全球直连", "type": "selector", "outbounds": []string{"direct"}},
	}, nil, []string{"全球直连"}) {
		t.Fatal("direct-only collection was treated as a usable proxy path")
	}
	if !hasUsableNonDirectOutboundPath([]interface{}{
		map[string]interface{}{"tag": "direct", "type": "direct"},
		map[string]interface{}{"tag": "node", "type": "socks"},
		map[string]interface{}{"tag": "Developer", "type": "selector", "outbounds": []string{"direct", "node"}},
	}, nil, []string{"Developer"}) {
		t.Fatal("collection with a real proxy node was not treated as a usable proxy path")
	}
}

func TestGenerateOutboundsDoesNotApplyGlobalDNSToSharedNode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "shared", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		UID: "shared-node", Name: "Shared Node", Type: "socks", Server: "node.example.com", ServerPort: 1080,
		RawJSON: `{"type":"socks","server":"node.example.com","server_port":1080}`,
	}}); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Developer", "Streaming"} {
		if err := db.CreateProxyCollection(&model.ProxyCollection{
			Name: name, Type: "selector", SourceType: "manual", NodeUIDs: `["shared-node"]`, ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
		}); err != nil {
			t.Fatal(err)
		}
	}

	outbounds, _, err := NewConfigGeneratorService(db, nil).generateOutbounds()
	if err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("enabled nodes = %d, err = %v", len(nodes), err)
	}
	nodeTag := buildNodeOutboundTags(nodes)[nodes[0].UID]
	for _, item := range outbounds {
		outbound, ok := item.(map[string]interface{})
		if !ok || outbound["tag"] != nodeTag {
			continue
		}
		if _, exists := outbound["domain_resolver"]; exists {
			t.Fatalf("shared node inherited strategy DNS resolver: %+v", outbound)
		}
		return
	}
	t.Fatal("shared node outbound not found")
}

func TestGenerateRouteOmitsLegacyInternalNodeCheckRule(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	route, err := NewConfigGeneratorService(db, nil).generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	rules, _ := route["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if rule["outbound"] == "ackwrap-internal-node-check" {
			t.Fatalf("legacy internal node-check rule = %+v", rule)
		}
	}
}

func TestGenerateInboundsUsesPublicMixedDefaults(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewConfigGeneratorService(db, nil)
	inbounds, err := service.generateInbounds("", 0, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	foundMixed := false
	foundTUN := false
	for _, item := range inbounds {
		inbound, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if inbound["type"] == "tun" {
			foundTUN = true
			if !stringListContains(inbound["address"], defaultTUNIPv4Address) || !stringListContains(inbound["address"], defaultTUNIPv6Address) {
				t.Fatalf("default TUN is not dual-stack: %+v", inbound)
			}
			continue
		}
		if inbound["type"] != "mixed" {
			continue
		}
		if inbound["tag"] == legacyUpdateProxyInboundTag {
			t.Fatalf("generated inbounds contain legacy update proxy: %+v", inbounds)
		}
		switch inbound["tag"] {
		case "mixed-in":
			foundMixed = true
			if inbound["listen"] != "0.0.0.0" || inbound["listen_port"] != model.DefaultMixedInboundPort {
				t.Fatalf("mixed inbound = %+v, want 0.0.0.0:%d", inbound, model.DefaultMixedInboundPort)
			}
			if _, exists := inbound["users"]; exists {
				t.Fatalf("mixed inbound unexpectedly requires authentication: %+v", inbound)
			}
		}
	}
	if !foundTUN || !foundMixed {
		t.Fatalf("generated inbounds missing tun=%t mixed=%t: %+v", foundTUN, foundMixed, inbounds)
	}
}

func TestGenerateOpenWrtTUNAddsLocalDNSInboundAndHijack(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewConfigGeneratorService(db, nil)
	service.dnsmasqSupported = func() bool { return true }
	inbounds, err := service.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	var dnsInbound map[string]interface{}
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]interface{})
		if inbound["tag"] == dnsInboundTag {
			dnsInbound = inbound
		}
	}
	if dnsInbound == nil || dnsInbound["type"] != "direct" || dnsInbound["listen"] != "127.0.0.1" || dnsInbound["listen_port"] != defaultDNSInboundPort {
		t.Fatalf("local DNS inbound = %+v", dnsInbound)
	}
	if _, exists := dnsInbound["override_address"]; exists {
		t.Fatalf("local DNS inbound should route by tag without overriding destination: %+v", dnsInbound)
	}
	if _, exists := dnsInbound["override_port"]; exists {
		t.Fatalf("local DNS inbound should route by tag without overriding destination: %+v", dnsInbound)
	}
	route, err := service.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	rules := route["rules"].([]map[string]interface{})
	if len(rules) == 0 || rules[0]["inbound"] != dnsInboundTag || rules[0]["action"] != "hijack-dns" {
		t.Fatalf("first route rule does not hijack local DNS inbound: %+v", rules)
	}
}

func TestGenerateDNSMasqTakeoverRespectsModeAndSwitch(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewConfigGeneratorService(db, nil)
	service.dnsmasqSupported = func() bool { return true }

	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	inbounds, err := service.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	if hasInboundTag(inbounds, dnsInboundTag) {
		t.Fatal("mixed-only config contains Ackwrap DNS inbound")
	}
	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetGeneralSettings(&model.GeneralSettings{AutoStartCore: true, DNSMasqTakeoverEnabled: false}); err != nil {
		t.Fatal(err)
	}
	inbounds, err = service.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	if hasInboundTag(inbounds, dnsInboundTag) {
		t.Fatal("disabled takeover generated Ackwrap DNS inbound")
	}
}

func hasInboundTag(inbounds []interface{}, tag string) bool {
	for _, raw := range inbounds {
		inbound, _ := raw.(map[string]interface{})
		if inbound["tag"] == tag {
			return true
		}
	}
	return false
}

func TestGenerateInboundsIncludesMixedAuthentication(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetMixedInboundSettings(&model.MixedInboundSettings{Username: "proxy-user", Password: "short-pass"}); err != nil {
		t.Fatal(err)
	}

	inbounds, err := NewConfigGeneratorService(db, nil).generateInbounds("127.0.0.1", 7893, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inbounds {
		inbound, _ := item.(map[string]interface{})
		if inbound["type"] != "mixed" {
			continue
		}
		users, ok := inbound["users"].([]map[string]string)
		if !ok || len(users) != 1 || users[0]["username"] != "proxy-user" || users[0]["password"] != "short-pass" {
			t.Fatalf("mixed inbound users = %#v, want one configured user", inbound["users"])
		}
		return
	}
	t.Fatal("mixed inbound not generated")
}

func TestTrafficBypassSettingsApplyToTUNAndRoute(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	settingsService := NewSettingsService(db)
	if err := settingsService.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{
		{Type: "process_name", Value: "mesh-agent"},
		{Type: "interface", Value: "mesh-tun"},
		{Type: "ip_cidr", Value: "10.20.0.0/16"},
		{Type: "source_ip_cidr", Value: "192.168.50.0/24"},
		{Type: "domain_suffix", Value: "mesh.example"},
	}}); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)
	inbounds, err := service.generateInbounds("", 0, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	var tun map[string]interface{}
	for _, item := range inbounds {
		candidate, _ := item.(map[string]interface{})
		if candidate["type"] == "tun" {
			tun = candidate
			break
		}
	}
	if tun == nil || !stringListContains(tun["exclude_interface"], "mesh-tun") || !stringListContains(tun["route_exclude_address"], "10.20.0.0/16") {
		t.Fatalf("TUN bypass settings missing: %+v", tun)
	}
	rules, err := service.defaultBypassRules()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"process_name":   "mesh-agent",
		"ip_cidr":        "10.20.0.0/16",
		"source_ip_cidr": "192.168.50.0/24",
		"domain_suffix":  "mesh.example",
	}
	for key, value := range want {
		found := false
		for _, rule := range rules {
			if stringListContains(rule[key], value) && rule["action"] == "bypass" && rule["outbound"] == "direct" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s bypass for %s: %+v", key, value, rules)
		}
	}
}

func TestGenerateMixedOnlyDoesNotEnableTransparentRouting(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)
	inbounds, err := service.generateInbounds("127.0.0.1", 8888, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	if hasTUNInbound(inbounds) {
		t.Fatalf("mixed-only config contains TUN: %+v", inbounds)
	}
	route, err := service.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := route["auto_detect_interface"]; exists {
		t.Fatalf("mixed-only route enables TUN interface detection: %+v", route)
	}
}

func TestCacheFileConfigPersistsFakeIPMappings(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090"}); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir})
	generate := func(name string) map[string]interface{} {
		t.Helper()
		result, err := service.generateLockedTo(&model.ConfigGenerateRequest{
			DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
			TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
		}, filepath.Join(dataDir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		return result.Config
	}
	cacheFile := generate("tun")["experimental"].(map[string]interface{})["cache_file"].(map[string]interface{})
	if cacheFile["path"] != filepath.Join(dataDir, "cache.db") {
		t.Fatalf("cache path = %q", cacheFile["path"])
	}
	if cacheFile["store_fakeip"] != true || cacheFile["enabled"] != true {
		t.Fatalf("FakeIP cache config = %+v", cacheFile)
	}
	settings, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.CacheFileEnabled || settings.CacheFileStoreFakeIP {
		t.Fatalf("FakeIP cache generation changed persisted settings: %+v", settings)
	}

	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	if cacheFile, exists := generate("mixed")["experimental"].(map[string]interface{})["cache_file"]; exists || cacheFile != nil {
		t.Fatalf("mixed mode cache file = %+v, want none", cacheFile)
	}
}

func TestGenerateDNSFakeIPFollowsTUNMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "custom_fakeip", Enabled: true, ServerType: "fakeip",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1",
	}); err != nil {
		t.Fatal(err)
	}
	ruleIDs := make([]int64, 0, 3)
	for _, item := range []struct {
		domain   string
		priority int
	}{
		{domain: "third.cn", priority: 30},
		{domain: "first.cn", priority: 10},
		{domain: "second.cn", priority: 20},
	} {
		created, err := db.CreateDNSRule(&model.DNSRuleRequest{
			Enabled:    true,
			Priority:   item.priority,
			RuleType:   "default",
			Conditions: map[string]interface{}{"domain_suffix": []string{item.domain}},
			Server:     "dns_direct",
		})
		if err != nil {
			t.Fatal(err)
		}
		ruleIDs = append(ruleIDs, created.ID)
	}
	legacyFakeIPRule, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Priority: 40, RuleType: "default", Conditions: map[string]interface{}{"domain_suffix": []string{"legacy-fake.example"}}, Server: "custom_fakeip",
	})
	if err != nil {
		t.Fatal(err)
	}
	ruleIDs = append(ruleIDs, legacyFakeIPRule.ID)
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "custom_fakeip"
	settings.FakeIPEnabled = false
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	dns := mustGenerateDNS(t, service)
	if !generatedDNSHasServerType(dns, "fakeip") || !generatedDNSHasRuleServer(dns, "fakeip") {
		t.Fatalf("TUN mode DNS does not contain fakeip server and rule: %+v", dns)
	}
	servers, _ := dns["servers"].([]map[string]interface{})
	fakeIPCount := 0
	for _, server := range servers {
		if server["type"] == "fakeip" {
			fakeIPCount++
			if server["tag"] != "fakeip" {
				t.Fatalf("persisted custom FakeIP Server was generated: %+v", server)
			}
		}
	}
	if fakeIPCount != 1 {
		t.Fatalf("generated FakeIP Server count = %d, servers = %+v", fakeIPCount, servers)
	}
	if dns["final"] != "dns_direct" {
		t.Fatalf("legacy FakeIP final = %v, want dns_direct fallback", dns["final"])
	}
	rules, _ := dns["rules"].([]map[string]interface{})
	if got := generatedDNSDomainSuffixOrder(rules); !reflect.DeepEqual(got, []string{"first.cn", "second.cn", "third.cn"}) {
		t.Fatalf("DNS priority order = %v", got)
	}
	if len(rules) < 4 || rules[len(rules)-1]["server"] != "fakeip" {
		t.Fatalf("DNS rule order = %+v, want user rules before FakeIP fallback", rules)
	}
	if _, exists := rules[len(rules)-1]["inbound"]; exists {
		t.Fatalf("FakeIP fallback must apply to A/AAAA queries from every inbound: %+v", rules[len(rules)-1])
	}
	if !stringListContains(rules[len(rules)-1]["query_type"], "A") || !stringListContains(rules[len(rules)-1]["query_type"], "AAAA") {
		t.Fatalf("FakeIP fallback does not cover A/AAAA: %+v", rules[len(rules)-1])
	}
	if err := db.ReorderDNSRules(ruleIDs); err != nil {
		t.Fatal(err)
	}
	rules, _ = mustGenerateDNS(t, service)["rules"].([]map[string]interface{})
	if got := generatedDNSDomainSuffixOrder(rules); !reflect.DeepEqual(got, []string{"third.cn", "first.cn", "second.cn"}) {
		t.Fatalf("reordered DNS rule order = %v", got)
	}
	if rules[len(rules)-1]["server"] != "fakeip" {
		t.Fatalf("FakeIP rule is not the final fallback after reorder: %+v", rules)
	}

	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	settings.FakeIPEnabled = true
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	dns = mustGenerateDNS(t, service)
	if generatedDNSHasServerType(dns, "fakeip") || generatedDNSHasRuleServer(dns, "fakeip") {
		t.Fatalf("Mixed mode DNS contains fakeip server or rule: %+v", dns)
	}
	if resolver := service.defaultDomainResolver(); resolver["server"] != "dns_direct" {
		t.Fatalf("Mixed mode default domain resolver = %+v, want dns_direct", resolver)
	}
}

func TestGenerateDNSGeositeUsesGeneratedRuleSet(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled:    true,
		RuleType:   "default",
		Conditions: map[string]interface{}{"geosite": []string{"cn"}},
		Server:     "dns_direct",
	}); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	dns := mustGenerateDNS(t, service)
	rules, _ := dns["rules"].([]map[string]interface{})
	if len(rules) < 2 {
		t.Fatalf("DNS rules = %+v, want geosite rule and FakeIP fallback", rules)
	}
	if _, exists := rules[0]["geosite"]; exists {
		t.Fatalf("DNS rule still contains removed geosite field: %+v", rules[0])
	}
	if tags, ok := rules[0]["rule_set"].([]string); !ok || !reflect.DeepEqual(tags, []string{"geosite-cn"}) {
		t.Fatalf("DNS geosite rule_set = %#v, want geosite-cn", rules[0]["rule_set"])
	}

	route, err := service.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	ruleSets, _ := route["rule_set"].([]map[string]interface{})
	found := false
	for _, ruleSet := range ruleSets {
		if ruleSet["tag"] == "geosite-cn" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("route rule_set does not include DNS geosite dependency: %+v", ruleSets)
	}
}

func TestGenerateDNSSimplifiesProxyPoliciesToFakeIPAndOneFinal(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, server := range []*model.DNSServerRequest{
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
		{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://proxy-dns.example.com/dns-query"},
	} {
		if _, err := db.CreateDNSServer(server); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Google", Enabled: true, Priority: 10, RuleType: "geosite", Values: []string{"google"}, Outbound: "proxy",
	}); err != nil {
		t.Fatal(err)
	}
	// This explicit rule is the real-IP exception for domestic domains.
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Priority: 10, Conditions: map[string]interface{}{"geosite": []string{"cn"}}, Server: "dns_direct",
	}); err != nil {
		t.Fatal(err)
	}
	// Persisted bindings from older versions must be ignored without deleting user data.
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Priority: 20, Conditions: map[string]interface{}{"outbound": []string{"Google"}}, Server: "dns_proxy",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "dns_proxy"
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}

	dns := mustGenerateDNS(t, NewConfigGeneratorService(db, nil))
	rules, _ := dns["rules"].([]map[string]interface{})
	if len(rules) != 2 {
		t.Fatalf("DNS rules = %+v, want domestic exception and FakeIP only", rules)
	}
	if !stringListContains(rules[0]["rule_set"], "geosite-cn") || rules[0]["server"] != "dns_direct" {
		t.Fatalf("domestic real-IP DNS rule = %+v", rules[0])
	}
	if rules[1]["server"] != "fakeip" ||
		!stringListContains(rules[1]["query_type"], "A") || !stringListContains(rules[1]["query_type"], "AAAA") {
		t.Fatalf("FakeIP fallback rule = %+v", rules[1])
	}
	if _, exists := rules[1]["inbound"]; exists {
		t.Fatalf("FakeIP fallback unexpectedly limits inbound: %+v", rules[1])
	}
	finalServer, _ := dns["final"].(string)
	if finalServer == "" || finalServer == "dns_proxy" {
		t.Fatalf("unprotected DNS final = %q", finalServer)
	}
	assertGeneratedDNSServerDetour(t, dns, finalServer, "proxy")
	servers, _ := dns["servers"].([]map[string]interface{})
	proxyFinalCount := 0
	for _, server := range servers {
		tag, _ := server["tag"].(string)
		if strings.HasPrefix(tag, "ackwrap-proxy-dns") {
			proxyFinalCount++
		}
		if strings.Contains(tag, "-via-") {
			t.Fatalf("per-strategy DNS clone still generated: %s", tag)
		}
	}
	if proxyFinalCount != 1 {
		t.Fatalf("generated proxy DNS final count = %d, servers = %+v", proxyFinalCount, servers)
	}
}

func TestGenerateDNSLeakProtectionModeMatrix(t *testing.T) {
	for _, proxyMode := range []string{"direct", "global", "rule"} {
		for _, inboundMode := range []string{"mixed", "tun", "tun_mixed"} {
			t.Run(proxyMode+"/"+inboundMode, func(t *testing.T) {
				db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				for _, server := range []*model.DNSServerRequest{
					{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
					{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://proxy-dns.example.com/dns-query"},
				} {
					if _, err := db.CreateDNSServer(server); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
					Enabled: true, Conditions: map[string]interface{}{"domain_suffix": []string{"cn.example"}}, Server: "dns_direct",
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
					Name: "Proxy", Enabled: true, RuleType: "domain_suffix", Values: []string{"proxy.example"}, Outbound: "proxy",
				}); err != nil {
					t.Fatal(err)
				}
				if err := db.SetProxyMode(proxyMode); err != nil {
					t.Fatal(err)
				}
				if err := db.SetInboundMode(inboundMode); err != nil {
					t.Fatal(err)
				}
				settings, err := db.GetDNSGlobalSettings()
				if err != nil {
					t.Fatal(err)
				}
				settings.ProxyFinal = "dns_proxy"
				if err := db.SetDNSGlobalSettings(settings); err != nil {
					t.Fatal(err)
				}

				routeFinal := "direct"
				if proxyMode == "global" {
					routeFinal = "proxy"
				}
				dns, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase(routeFinal)
				if err != nil {
					t.Fatal(err)
				}
				rules, _ := dns["rules"].([]map[string]interface{})
				if len(rules) == 0 || rules[0]["server"] != "dns_direct" {
					t.Fatalf("domestic real-IP rule = %+v", rules)
				}
				hasFakeIP := false
				for _, rule := range rules {
					if rule["server"] != "fakeip" {
						continue
					}
					hasFakeIP = true
					if _, exists := rule["inbound"]; exists {
						t.Fatalf("FakeIP fallback unexpectedly limits inbound: %+v", rule)
					}
					if !stringListContains(rule["query_type"], "A") || !stringListContains(rule["query_type"], "AAAA") {
						t.Fatalf("FakeIP fallback does not cover A/AAAA: %+v", rule)
					}
				}
				wantFakeIP := inboundMode == "tun" || inboundMode == "tun_mixed"
				if hasFakeIP != wantFakeIP {
					t.Fatalf("FakeIP rule present = %t, want %t; rules = %+v", hasFakeIP, wantFakeIP, rules)
				}
				if hasServer := generatedDNSHasServerType(dns, "fakeip"); hasServer != wantFakeIP {
					t.Fatalf("FakeIP server present = %t, want %t; DNS = %+v", hasServer, wantFakeIP, dns)
				}
				if wantFakeIP && (len(rules) == 0 || rules[len(rules)-1]["server"] != "fakeip") {
					t.Fatalf("FakeIP fallback is not the final DNS rule: %+v", rules)
				}

				finalServer, _ := dns["final"].(string)
				if proxyMode == "direct" {
					if finalServer != "dns_proxy" {
						t.Fatalf("direct mode DNS final = %q, want configured server", finalServer)
					}
					assertGeneratedDNSServerDetour(t, dns, finalServer, "")
					return
				}
				if finalServer == "" || finalServer == "dns_proxy" {
					t.Fatalf("%s mode DNS final is not protected: %q", proxyMode, finalServer)
				}
				assertGeneratedDNSServerDetour(t, dns, finalServer, "proxy")
			})
		}
	}
}

func TestGenerateDNSRejectsMissingRemoteProxyFinal(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_local", Enabled: true, ServerType: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Proxy", Enabled: true, RuleType: "domain_suffix", Values: []string{"example.com"}, Outbound: "proxy",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "dns_local"
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase("direct"); err == nil || !strings.Contains(err.Error(), "没有可用远程 Server") {
		t.Fatalf("missing proxy DNS final error = %v", err)
	}
}

func TestGenerateDNSFailsWhenLeakProtectionRuleSourceUnavailable(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`DROP TABLE route_rules`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase("direct"); err == nil || !strings.Contains(err.Error(), "DNS 防泄漏关联路由规则") {
		t.Fatalf("DNS leak protection source error = %v", err)
	}
}

func TestGenerateDNSRejectsUnsafePersistedServerConfiguration(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		detour  string
		options map[string]interface{}
		want    string
	}{
		{name: "block detour", detour: "block", want: "detour 不能是 block"},
		{name: "reject detour", detour: "reject", want: "detour 不能是 reject"},
		{name: "options detour", options: map[string]interface{}{"detour": "direct"}, want: "options 不能覆盖受控字段 detour"},
		{name: "options type", options: map[string]interface{}{"type": "local"}, want: "options 不能覆盖受控字段 type"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.CreateDNSServer(&model.DNSServerRequest{
				Tag: "dns_remote", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: testCase.detour, Options: testCase.options,
			}); err != nil {
				t.Fatal(err)
			}
			if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase(); err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("unsafe persisted DNS Server configuration error = %v, want %q", err, testCase.want)
			}
		})
	}
}

func TestRouteBindingSourceErrorsFailConfigGeneration(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.DB().Exec(`DROP TABLE proxy_collections`); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)
	if _, err := service.generateRoute("direct"); err == nil || !strings.Contains(err.Error(), "读取策略组规则绑定失败") {
		t.Fatalf("route strategy binding source error = %v", err)
	}
}

func TestGenerateRouteRejectsMalformedDNSRuleConditions(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rule, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"domain_suffix": []string{"example.com"}}, Server: "dns_remote",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`UPDATE dns_rules SET conditions_json = '{' WHERE id = ?`, rule.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigGeneratorService(db, nil).generateRoute("direct"); err == nil || !strings.Contains(err.Error(), "conditions_json 无效") {
		t.Fatalf("malformed DNS route rule error = %v", err)
	}
}

func TestDNSGlobalEnablementControlsConfigAndHijackTogether(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_remote", Enabled: true, ServerType: "udp", Address: "1.1.1.1",
	}); err != nil {
		t.Fatal(err)
	}
	legacy := &model.DNSSettings{Enabled: false, Final: "dns_remote", Strategy: "prefer_ipv4"}
	if err := db.SetDNSSettings(legacy); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DELETE FROM app_settings WHERE key LIKE 'dns_global.%'`); err != nil {
		t.Fatal(err)
	}
	global, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if global.Enabled {
		t.Fatal("legacy DNS endpoint did not disable global DNS settings")
	}

	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir})
	generate := func(name string) map[string]interface{} {
		t.Helper()
		result, err := service.generateLockedTo(&model.ConfigGenerateRequest{
			DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
			TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
		}, filepath.Join(dataDir, name+".json"))
		if err != nil {
			t.Fatal(err)
		}
		return result.Config
	}
	assertState := func(config map[string]interface{}, enabled bool) {
		t.Helper()
		_, hasDNS := config["dns"]
		route, _ := config["route"].(map[string]interface{})
		rules, _ := route["rules"].([]map[string]interface{})
		portHijack, protocolHijack := false, false
		for _, rule := range rules {
			if rule["action"] != "hijack-dns" {
				continue
			}
			portHijack = portHijack || rule["port"] == 53
			protocolHijack = protocolHijack || rule["protocol"] == "dns"
		}
		if hasDNS != enabled || portHijack != enabled || protocolHijack != enabled {
			t.Fatalf("DNS state enabled=%t section=%t port-hijack=%t protocol-hijack=%t: %+v", enabled, hasDNS, portHijack, protocolHijack, rules)
		}
	}
	assertState(generate("dns-disabled"), false)

	global.Enabled = true
	global.Final = "dns_remote"
	if err := db.SetDNSGlobalSettings(global); err != nil {
		t.Fatal(err)
	}
	legacyState, err := db.GetDNSSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !legacyState.Enabled {
		t.Fatal("global DNS endpoint did not enable legacy DNS settings")
	}
	assertState(generate("dns-enabled"), true)
}

func TestGeneratedDNSConfigurationPassesAvailableSingBoxCheck(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("workspace sing-box verification binary is Windows-only")
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot locate installed sing-box: %v", err)
	}
	binaryPath := filepath.Join(homeDir, "ackwrap", "bin", "sing-box.exe")
	if _, err := os.Stat(binaryPath); err != nil {
		t.Skipf("installed sing-box verification binary unavailable: %v", err)
	}
	version, err := exec.Command(binaryPath, "version").CombinedOutput()
	if err != nil || !strings.Contains(string(version), "with_clash_api") {
		t.Skip("installed sing-box binary does not include the required Clash API build tag")
	}
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, server := range []*model.DNSServerRequest{
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
		{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query", Detour: "proxy"},
	} {
		if _, err := db.CreateDNSServer(server); err != nil {
			t.Fatal(err)
		}
	}
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "proxy-nodes", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		Name: "Proxy Node", Type: "socks", Server: "192.0.2.1", ServerPort: 1080,
		RawJSON: `{"type":"socks","server":"192.0.2.1","server_port":1080}`,
	}}); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("proxy nodes = %d, %v", len(nodes), err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: "proxy", Type: "selector", SourceType: "manual", NodeUIDs: fmt.Sprintf("[%q]", nodes[0].UID), ReferencedGroupIDs: "[]", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	proxyRule, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Proxy GeoSite", Enabled: true, Priority: 10, RuleType: "geosite", Values: []string{"cn"}, Outbound: "proxy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: proxyRule.Name, Type: "selector", SourceType: "manual", NodeUIDs: fmt.Sprintf("[%q]", nodes[0].UID), ReferencedGroupIDs: "[]", RouteRuleID: proxyRule.ID, RouteRuleIDs: "[" + fmt.Sprint(proxyRule.ID) + "]", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Priority: 10, RuleType: "default", Conditions: map[string]interface{}{"geosite": []string{"cn"}}, Server: "dns_direct",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled = true
	settings.Final = "dns_proxy"
	settings.FakeIPEnabled = true
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir, BinaryPath: binaryPath})
	for _, mode := range []string{"rule", "global", "direct"} {
		t.Run(mode, func(t *testing.T) {
			if err := db.SetProxyMode(mode); err != nil {
				t.Fatal(err)
			}
			result, err := service.generateLockedTo(&model.ConfigGenerateRequest{
				DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
				TUNIPv4Address: defaultTUNIPv4Address, TUNIPv6Address: defaultTUNIPv6Address, LogLevel: "warn",
			}, filepath.Join(dataDir, "config-"+mode+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if !result.Valid {
				t.Fatalf("sing-box rejected %s mode config: %s", mode, result.Error)
			}
		})
	}
}

func mustGenerateDNS(t *testing.T, service *ConfigGeneratorService, routeFinal ...string) map[string]interface{} {
	t.Helper()
	dns, err := service.generateDNSFromDatabase(routeFinal...)
	if err != nil {
		t.Fatal(err)
	}
	return dns
}

func TestSetDNSIndependentCacheOmitsRemovedOption(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "", want: true},
		{version: "1.13.14", want: true},
		{version: "1.14.0-alpha.45", want: false},
		{version: "1.14.0", want: false},
	}
	for _, test := range tests {
		dns := make(map[string]interface{})
		setDNSIndependentCache(dns, true, test.version)
		_, exists := dns["independent_cache"]
		if exists != test.want {
			t.Errorf("version %q independent_cache present = %t, want %t", test.version, exists, test.want)
		}
	}
}

func TestGenerateDNSOmitsIndependentCacheForNewCore(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1",
	}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "dns_direct"
	settings.IndependentCache = true
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	service.readCoreVersion = func() string { return "1.14.0-alpha.45" }
	dns := mustGenerateDNS(t, service)
	if _, exists := dns["independent_cache"]; exists {
		t.Fatal("1.14 alpha DNS config contains removed independent_cache option")
	}
	service.readCoreVersion = func() string { return "1.13.14" }
	dns = mustGenerateDNS(t, service)
	if enabled, exists := dns["independent_cache"]; !exists || enabled != true {
		t.Fatalf("legacy DNS config independent_cache = %v, present = %t", enabled, exists)
	}
}

func generatedDNSHasServerType(dns map[string]interface{}, serverType string) bool {
	servers, _ := dns["servers"].([]map[string]interface{})
	for _, server := range servers {
		if server["type"] == serverType {
			return true
		}
	}
	return false
}

func assertGeneratedDNSServerDetour(t *testing.T, dns map[string]interface{}, tag, expected string) {
	t.Helper()
	servers, _ := dns["servers"].([]map[string]interface{})
	for _, server := range servers {
		if server["tag"] != tag {
			continue
		}
		detour, _ := server["detour"].(string)
		if detour != expected {
			t.Fatalf("DNS server %s detour = %q, want %q: %+v", tag, detour, expected, server)
		}
		return
	}
	t.Fatalf("DNS server %s not found: %+v", tag, servers)
}

func generatedDNSHasRuleServer(dns map[string]interface{}, serverTag string) bool {
	rules, _ := dns["rules"].([]map[string]interface{})
	for _, rule := range rules {
		if rule["server"] == serverTag {
			return true
		}
	}
	return false
}

func generatedDNSDomainSuffixOrder(rules []map[string]interface{}) []string {
	result := make([]string, 0, len(rules))
	for _, rule := range rules {
		values, _ := rule["domain_suffix"].([]interface{})
		if len(values) == 0 {
			continue
		}
		if value, ok := values[0].(string); ok {
			result = append(result, value)
		}
	}
	return result
}

func TestGenerateInboundsAllowsFormerUpdateProxyPort(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewConfigGeneratorService(db, nil)
	inbounds, err := service.generateInbounds("127.0.0.1", 9901, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range inbounds {
		inbound, ok := item.(map[string]interface{})
		if ok && inbound["tag"] == "mixed-in" && inbound["listen_port"] == 9901 {
			return
		}
	}
	t.Fatalf("mixed inbound did not preserve port 9901: %+v", inbounds)
}

func TestPreviewRequestPreservesStoredGenerationSettings(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	stored := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		TUNIPv4Address:  "10.254.0.1/30",
		TUNIPv6Address:  "fd12:3456:789a::1/126",
		LogLevel:        "warn",
	}
	if err := db.SetConfigGenerateRequest(stored); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)

	preview, err := service.previewRequest("custom-proxy")
	if err != nil {
		t.Fatal(err)
	}
	if preview.DefaultOutbound != "custom-proxy" || preview.InboundListen != stored.InboundListen || preview.InboundPort != stored.InboundPort || preview.TUNIPv4Address != stored.TUNIPv4Address || preview.TUNIPv6Address != stored.TUNIPv6Address || preview.LogLevel != stored.LogLevel {
		t.Fatalf("preview request = %+v, want stored settings with overridden outbound", preview)
	}
	persisted, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if *persisted != *stored {
		t.Fatalf("stored generation settings changed: got %+v, want %+v", persisted, stored)
	}
}

func TestPreviewRejectsUnvalidatedConfiguration(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1",
	}); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir})
	if _, err := service.Preview("direct"); err == nil || !strings.Contains(err.Error(), "sing-box 未安装") {
		t.Fatalf("unvalidated preview error = %v", err)
	}
}

func TestPreviewWaitsForConfigUpdates(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir})

	releaseConfigUpdate := db.HoldConfigUpdate()
	done := make(chan error, 1)
	go func() {
		_, err := service.Preview("direct")
		done <- err
	}()
	select {
	case err := <-done:
		releaseConfigUpdate()
		t.Fatalf("preview completed during config update: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	releaseConfigUpdate()
	if err := <-done; err == nil || !strings.Contains(err.Error(), "sing-box 未安装") {
		t.Fatalf("preview error after config update = %v", err)
	}
}

func TestGetGenerateRequestUsesPersistedLogLevelByDefault(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetLogSettings(&model.LogSettings{Level: "debug", Timestamp: true}); err != nil {
		t.Fatal(err)
	}

	request, err := NewConfigGeneratorService(db, nil).GetGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if request.LogLevel != "debug" {
		t.Fatalf("default generation log level = %q, want debug", request.LogLevel)
	}
	if request.DefaultOutbound != "direct" || request.InboundListen != "0.0.0.0" || request.InboundPort != model.DefaultMixedInboundPort {
		t.Fatalf("default generation request = %+v", request)
	}
	if request.TUNIPv4Address != defaultTUNIPv4Address || request.TUNIPv6Address != defaultTUNIPv6Address {
		t.Fatalf("default TUN addresses = %q, %q", request.TUNIPv4Address, request.TUNIPv6Address)
	}
}

func TestGetGenerateRequestMigratesPreviousTUNAddresses(t *testing.T) {
	dataDir := t.TempDir()
	db, err := store.Open(filepath.Join(dataDir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	legacy := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		TUNIPv4Address:  previousDefaultTUNIPv4,
		TUNIPv6Address:  previousDefaultTUNIPv6,
		LogLevel:        "warn",
	}
	if err := db.SetConfigGenerateRequest(legacy); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir})
	request, err := service.GetGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if request.TUNIPv4Address != defaultTUNIPv4Address || request.TUNIPv6Address != defaultTUNIPv6Address {
		t.Fatalf("migrated TUN addresses = %q, %q", request.TUNIPv4Address, request.TUNIPv6Address)
	}
	persisted, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if persisted.TUNIPv4Address != defaultTUNIPv4Address || persisted.TUNIPv6Address != defaultTUNIPv6Address {
		t.Fatalf("persisted TUN addresses = %q, %q", persisted.TUNIPv4Address, persisted.TUNIPv6Address)
	}
	if err := db.SetProxyMode("direct"); err != nil {
		t.Fatal(err)
	}
	result, err := service.GenerateCurrent()
	if err != nil {
		t.Fatal(err)
	}
	for _, rawInbound := range result.Config["inbounds"].([]interface{}) {
		inbound, _ := rawInbound.(map[string]interface{})
		if inbound["tag"] != "tun-in" {
			continue
		}
		if !stringListContains(inbound["address"], defaultTUNIPv4Address) || !stringListContains(inbound["address"], defaultTUNIPv6Address) {
			t.Fatalf("generated TUN addresses = %+v", inbound["address"])
		}
		return
	}
	t.Fatal("generated config does not contain tun-in")
}

func TestGetGenerateRequestSerializesMigrationWrites(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetConfigGenerateRequest(&model.ConfigGenerateRequest{
		DefaultOutbound: "direct", InboundListen: "127.0.0.1", InboundPort: 8000,
		TUNIPv4Address: previousDefaultTUNIPv4, TUNIPv6Address: previousDefaultTUNIPv6, LogLevel: "info",
	}); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)
	service.configMu.Lock()
	locked := true
	defer func() {
		if locked {
			service.configMu.Unlock()
		}
	}()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		_, err := service.GetGenerateRequest()
		done <- err
	}()
	<-started
	select {
	case err := <-done:
		service.configMu.Unlock()
		locked = false
		t.Fatalf("GetGenerateRequest bypassed config lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	replacement := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy", InboundListen: "0.0.0.0", InboundPort: 9000,
		TUNIPv4Address: "10.254.0.1/30", TUNIPv6Address: "fd12:3456:789a::1/126", LogLevel: "warn",
	}
	if err := db.SetConfigGenerateRequest(replacement); err != nil {
		service.configMu.Unlock()
		locked = false
		t.Fatal(err)
	}
	service.configMu.Unlock()
	locked = false
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	persisted, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if *persisted != *replacement {
		t.Fatalf("serialized migration overwrote newer settings: got %+v, want %+v", persisted, replacement)
	}
}
