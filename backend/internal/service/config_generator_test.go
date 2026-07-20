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

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
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

func TestDNSRuleOutboundConditions(t *testing.T) {
	tests := []struct {
		name       string
		conditions map[string]interface{}
		want       []string
	}{
		{name: "single outbound", conditions: map[string]interface{}{"outbound": "proxy"}, want: []string{"proxy"}},
		{name: "multiple outbounds", conditions: map[string]interface{}{"outbound": []interface{}{"direct", "香港"}}, want: []string{"direct", "香港"}},
		{name: "missing outbound", conditions: map[string]interface{}{"domain_suffix": []interface{}{"example.com"}}, want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dnsRuleOutboundConditions(tt.conditions)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestDNSStrategyBindingsOnlyUsesPureOutboundConditions(t *testing.T) {
	rules := []model.DNSRule{
		{Enabled: true, ConditionsJSON: `{"outbound":["Developer"],"domain_suffix":["manual.example"]}`, Server: "dns_hybrid"},
		{Enabled: true, ConditionsJSON: `{"outbound":["Developer"]}`, Server: "dns_strategy"},
	}
	bindings := dnsStrategyBindings(rules, map[string]bool{"dns_hybrid": true, "dns_strategy": true})
	if len(bindings) != 1 || bindings["Developer"].Server != "dns_strategy" {
		t.Fatalf("strategy bindings = %+v, want only pure outbound rule", bindings)
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
	tags := enabledDNSServerTags(nil, true)
	if !tags["fakeip"] {
		t.Fatal("generated fakeip server tag is missing")
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
	inbound := generatedTUNInbound(true, defaultTUNIPv4Address, defaultTUNIPv6Address)
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
	withoutRedirect := generatedTUNInbound(false, defaultTUNIPv4Address, defaultTUNIPv6Address)
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
	collection := &model.ProxyCollection{
		Name: "Auto Service", Type: "urltest", SourceType: proxyCollectionSourceNodeGroups,
		ReferencedGroupIDs: fmt.Sprintf("[%d]", group.ID), RouteRuleIDs: "[]", NodeUIDs: "[]",
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
		{name: "strategy binding", setup: func(t *testing.T, db *store.Store) {
			if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_proxy", Enabled: true, ServerType: "udp", Address: "1.1.1.1"}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_proxy"}); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "global mode", setup: func(t *testing.T, db *store.Store) {
			if err := db.SetProxyMode("global"); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "route final", defaultOutbound: "proxy"},
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
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_proxy",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: "全球直连", Type: "selector", SourceType: "manual", NodeUIDs: `["direct"]`, ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	_, err = NewConfigGeneratorService(db, &paths.Paths{DataDir: dataDir}).generateLockedTo(&model.ConfigGenerateRequest{
		InboundListen: "127.0.0.1", InboundPort: model.DefaultMixedInboundPort,
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
			name:            "route final strategy",
			defaultOutbound: "regional-proxy",
			setup: func(t *testing.T, db *store.Store) {
				if err := db.CreateProxyCollection(&model.ProxyCollection{
					Name: "regional-proxy", Type: "selector", SourceType: "manual", NodeUIDs: `["direct"]`, ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
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
			if err == nil || !strings.Contains(err.Error(), "没有可用非直连代理路径") {
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

func TestGenerateOutboundsDoesNotApplyStrategyDNSToSharedNode(t *testing.T) {
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
		if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
			Enabled: true, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{name}}, Server: "dns_" + strings.ToLower(name),
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
		}
	}
	if !foundTUN || !foundMixed {
		t.Fatalf("generated inbounds missing tun=%t mixed=%t: %+v", foundTUN, foundMixed, inbounds)
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
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "missing"
	settings.FakeIPEnabled = false
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}

	service := NewConfigGeneratorService(db, nil)
	dns := mustGenerateDNS(t, service)
	if !generatedDNSHasServerType(dns, "fakeip") || !generatedDNSHasRuleServer(dns, "fakeip") {
		t.Fatalf("TUN mode DNS does not contain fakeip server and rule: %+v", dns)
	}
	rules, _ := dns["rules"].([]map[string]interface{})
	if got := generatedDNSDomainSuffixOrder(rules); !reflect.DeepEqual(got, []string{"first.cn", "second.cn", "third.cn"}) {
		t.Fatalf("DNS priority order = %v", got)
	}
	if len(rules) < 4 || rules[len(rules)-1]["server"] != "fakeip" {
		t.Fatalf("DNS rule order = %+v, want user rules before FakeIP fallback", rules)
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

func TestGenerateDNSStrategyBindingsFollowAssociatedRouteRules(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, server := range []*model.DNSServerRequest{
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "proxy"},
		{Tag: "dns_developer", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
		{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://proxy-dns.example.com/dns-query"},
	} {
		if _, err := db.CreateDNSServer(server); err != nil {
			t.Fatal(err)
		}
	}
	domainRule, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Developer Domains", Enabled: true, Priority: 10, RuleType: "domain_suffix", Values: []string{"developer.example"}, Outbound: "proxy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Direct Domains", Enabled: true, Priority: 30, RuleType: "domain_keyword", Values: []string{"intranet"}, Outbound: "direct",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Proxy Domains", Enabled: true, Priority: 40, RuleType: "domain", Values: []string{"proxy.example"}, Outbound: "proxy",
	}); err != nil {
		t.Fatal(err)
	}
	geositeRule, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Developer GeoSite", Enabled: true, Priority: 20, RuleType: "geosite", Values: []string{"cn"}, Outbound: "proxy",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: "Developer", Type: "selector", SourceType: "manual", NodeUIDs: "[]", ReferencedGroupIDs: "[]",
		RouteRuleIDs: fmt.Sprintf("[%d,%d]", domainRule.ID, geositeRule.ID), Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Priority: 10, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{"Developer"}}, Server: "dns_developer",
	}); err != nil {
		t.Fatal(err)
	}
	for priority, binding := range []struct {
		outbound string
		server   string
	}{{outbound: "direct", server: "dns_direct"}, {outbound: "proxy", server: "dns_proxy"}} {
		if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
			Enabled: true, Priority: priority + 20, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{binding.outbound}}, Server: binding.server,
		}); err != nil {
			t.Fatal(err)
		}
	}

	dns := mustGenerateDNS(t, NewConfigGeneratorService(db, nil))
	rules, _ := dns["rules"].([]map[string]interface{})
	foundDomain := false
	foundGeoSite := false
	foundDirect := false
	foundProxy := false
	developerServer := ""
	directServer := ""
	proxyServer := ""
	for _, rule := range rules {
		if stringListContains(rule["domain_suffix"], "developer.example") {
			foundDomain = true
			developerServer, _ = rule["server"].(string)
		}
		if stringListContains(rule["rule_set"], "geosite-cn") {
			foundGeoSite = true
			if server, _ := rule["server"].(string); developerServer != "" && server != developerServer {
				t.Fatalf("developer DNS rules use different servers: %s and %s", developerServer, server)
			}
		}
		if stringListContains(rule["domain_keyword"], "intranet") {
			foundDirect = true
			directServer, _ = rule["server"].(string)
		}
		if stringListContains(rule["domain"], "proxy.example") {
			foundProxy = true
			proxyServer, _ = rule["server"].(string)
		}
		if _, exists := rule["outbound"]; exists {
			t.Fatalf("generated DNS strategy rule contains removed outbound matcher: %+v", rule)
		}
	}
	if !foundDomain || !foundGeoSite || !foundDirect || !foundProxy {
		t.Fatalf("DNS rules missing strategy domain=%t geosite=%t direct=%t proxy=%t: %+v", foundDomain, foundGeoSite, foundDirect, foundProxy, rules)
	}
	assertGeneratedDNSServerDetour(t, dns, developerServer, "Developer")
	assertGeneratedDNSServerDetour(t, dns, proxyServer, "proxy")
	assertGeneratedDNSServerDetour(t, dns, directServer, "")
	if developerServer == "dns_developer" || proxyServer == "dns_proxy" || directServer == "dns_direct" {
		t.Fatalf("strategy rules reused base DNS server instead of isolated detour: developer=%s proxy=%s direct=%s", developerServer, proxyServer, directServer)
	}
}

func TestGenerateDNSFinalFollowsRouteFinalStrategy(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, server := range []*model.DNSServerRequest{
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "proxy"},
		{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
	} {
		if _, err := db.CreateDNSServer(server); err != nil {
			t.Fatal(err)
		}
	}
	service := NewConfigGeneratorService(db, nil)
	unboundDNS := mustGenerateDNS(t, service, "proxy")
	unboundFinal, _ := unboundDNS["final"].(string)
	assertGeneratedDNSServerDetour(t, unboundDNS, unboundFinal, "proxy")
	if unboundFinal == "dns_proxy" {
		t.Fatal("unbound route.final reused the base DNS server instead of forcing proxy detour")
	}
	for priority, binding := range []struct {
		outbound string
		server   string
	}{{outbound: "direct", server: "dns_direct"}, {outbound: "proxy", server: "dns_proxy"}} {
		if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
			Enabled: true, Priority: priority + 10, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{binding.outbound}}, Server: binding.server,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, testCase := range []struct {
		outbound string
		detour   string
	}{{outbound: "proxy", detour: "proxy"}, {outbound: "direct", detour: ""}} {
		dns := mustGenerateDNS(t, service, testCase.outbound)
		finalServer, _ := dns["final"].(string)
		if finalServer == "" {
			t.Fatalf("route final %s did not generate DNS final", testCase.outbound)
		}
		assertGeneratedDNSServerDetour(t, dns, finalServer, testCase.detour)
	}
}

func TestGenerateDNSRejectsNonRemoteStrategyServer(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_local", Enabled: true, ServerType: "local"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_local",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase("proxy"); err == nil || !strings.Contains(err.Error(), "不能用于防泄漏策略绑定") {
		t.Fatalf("non-remote strategy DNS error = %v", err)
	}
}

func TestGenerateDNSRejectsInvalidPersistedStrategyBindings(t *testing.T) {
	for _, testCase := range []struct {
		name          string
		outboundValue interface{}
		extra         map[string]interface{}
		duplicate     bool
		rawConditions string
		mutate        func(*store.Store, int64) error
		wantError     string
	}{
		{name: "malformed conditions", outboundValue: []string{"proxy"}, rawConditions: `{`, wantError: "conditions_json 无效"},
		{name: "empty outbound", outboundValue: []string{}, wantError: "只能包含一个 outbound"},
		{name: "numeric outbound", outboundValue: []interface{}{123}, wantError: "只能包含一个 outbound"},
		{name: "multiple outbounds", outboundValue: []string{"direct", "proxy"}, wantError: "只能包含一个 outbound"},
		{name: "blocked outbound", outboundValue: []string{"block"}, wantError: "无效或未启用"},
		{name: "whitespace outbound", outboundValue: []string{"proxy "}, wantError: "无效或未启用"},
		{name: "unknown outbound", outboundValue: []string{"missing-strategy"}, wantError: "无效或未启用"},
		{name: "duplicate outbound", outboundValue: []string{"proxy"}, duplicate: true, wantError: "多个启用"},
		{name: "hybrid conditions", outboundValue: []string{"proxy"}, extra: map[string]interface{}{"domain_suffix": []string{"manual.example"}}, wantError: "只能包含 outbound 条件"},
		{name: "disabled server", outboundValue: []string{"proxy"}, mutate: func(db *store.Store, serverID int64) error {
			return db.UpdateDNSServer(serverID, &model.DNSServerRequest{Tag: "dns_remote", Enabled: false, ServerType: "https", Address: "https://dns.example.com/dns-query"})
		}, wantError: "已停用"},
		{name: "missing server", outboundValue: []string{"proxy"}, mutate: func(db *store.Store, serverID int64) error {
			return db.DeleteDNSServer(serverID)
		}, wantError: "不存在"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			server, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_remote", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"})
			if err != nil {
				t.Fatal(err)
			}
			conditions := map[string]interface{}{"outbound": testCase.outboundValue}
			for key, value := range testCase.extra {
				conditions[key] = value
			}
			storedRule, err := db.CreateDNSRule(&model.DNSRuleRequest{
				Enabled: true, Conditions: conditions, Server: server.Tag,
			})
			if err != nil {
				t.Fatal(err)
			}
			if testCase.rawConditions != "" {
				if _, err := db.DB().Exec(`UPDATE dns_rules SET conditions_json = ? WHERE id = ?`, testCase.rawConditions, storedRule.ID); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.duplicate {
				if _, err := db.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, Conditions: conditions, Server: server.Tag}); err != nil {
					t.Fatal(err)
				}
			}
			if testCase.mutate != nil {
				if err := testCase.mutate(db, server.ID); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase("proxy"); err == nil || !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("invalid persisted strategy binding error = %v, want %q", err, testCase.wantError)
			}
		})
	}
}

func TestGenerateDNSFailsWhenStrategyRuleSourceUnavailable(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_remote", Enabled: true, ServerType: "udp", Address: "1.1.1.1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: server.Tag,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`DROP TABLE route_rules`); err != nil {
		t.Fatal(err)
	}
	if _, err := NewConfigGeneratorService(db, nil).generateDNSFromDatabase("direct"); err == nil || !strings.Contains(err.Error(), "读取策略关联路由规则失败") {
		t.Fatalf("strategy rule source error = %v", err)
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

func TestStrategyBindingSourceErrorsFailConfigGeneration(t *testing.T) {
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
	if _, err := service.generateDNSStrategyRulePlans(map[string]model.DNSRule{"proxy": {Enabled: true, Server: "dns_remote"}}); err == nil || !strings.Contains(err.Error(), "读取策略组规则绑定失败") {
		t.Fatalf("DNS strategy binding source error = %v", err)
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

func TestDNSConditionsFromMixedRouteRuleSkipsAddressConditions(t *testing.T) {
	rules := dnsConditionsFromRouteRule(&model.RouteRule{
		RuleType: "mixed", Values: []string{"domain_suffix:example.com", "geosite:cn", "ip_cidr:192.0.2.0/24", "geoip:private"}, Invert: true,
	})
	if len(rules) != 2 {
		t.Fatalf("DNS-compatible mixed rules = %+v, want domain and geosite groups", rules)
	}
	if !stringListContains(rules[0]["domain_suffix"], "example.com") || rules[0]["invert"] != true {
		t.Fatalf("mixed domain DNS rule = %+v", rules[0])
	}
	if !stringListContains(rules[1]["rule_set"], "geosite-cn") || rules[1]["invert"] != true {
		t.Fatalf("mixed geosite DNS rule = %+v", rules[1])
	}
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
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "Proxy GeoSite", Enabled: true, Priority: 10, RuleType: "geosite", Values: []string{"cn"}, Outbound: "proxy",
	}); err != nil {
		t.Fatal(err)
	}
	for _, rule := range []*model.DNSRuleRequest{
		{Enabled: true, Priority: 10, RuleType: "default", Conditions: map[string]interface{}{"geosite": []string{"cn"}}, Server: "dns_direct"},
		{Enabled: true, Priority: 20, RuleType: "default", Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_proxy"},
	} {
		if _, err := db.CreateDNSRule(rule); err != nil {
			t.Fatal(err)
		}
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

func TestGetGenerateRequestBackfillsLegacyTUNAddresses(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	legacy := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     8888,
		LogLevel:        "warn",
	}
	if err := db.SetConfigGenerateRequest(legacy); err != nil {
		t.Fatal(err)
	}
	service := NewConfigGeneratorService(db, nil)
	request, err := service.GetGenerateRequest()
	if err != nil {
		t.Fatal(err)
	}
	if request.TUNIPv4Address != defaultTUNIPv4Address || request.TUNIPv6Address != defaultTUNIPv6Address {
		t.Fatalf("backfilled TUN addresses = %q, %q", request.TUNIPv4Address, request.TUNIPv6Address)
	}
}
