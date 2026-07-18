package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

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
}

func TestEnabledDNSServerTagsAddsGeneratedFakeIP(t *testing.T) {
	tags := enabledDNSServerTags(nil, true)
	if !tags["fakeip"] {
		t.Fatal("generated fakeip server tag is missing")
	}
}

func TestSelectDefaultDomainResolverRequiresGeneratedServer(t *testing.T) {
	settings := &model.DNSGlobalSettings{Enabled: true, Final: "dns_proxy"}
	if got := selectDefaultDomainResolver(settings, nil); got != "" {
		t.Fatalf("resolver = %q, want empty when no DNS server is generated", got)
	}

	servers := []model.DNSServer{
		{Tag: "dns_proxy", Enabled: false},
		{Tag: "dns_direct", Enabled: true},
	}
	if got := selectDefaultDomainResolver(settings, servers); got != "dns_direct" {
		t.Fatalf("resolver = %q, want dns_direct fallback", got)
	}
}

func TestSelectDefaultDomainResolverUsesGeneratedFakeIP(t *testing.T) {
	settings := &model.DNSGlobalSettings{Enabled: true, Final: "dns_proxy", FakeIPEnabled: true}
	if got := selectDefaultDomainResolver(settings, nil); got != "fakeip" {
		t.Fatalf("resolver = %q, want generated fakeip server", got)
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

func TestSafeNodeResolverFallsBackFromProxyDetour(t *testing.T) {
	servers := []model.DNSServer{
		{Tag: "dns_proxy", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query", Detour: "proxy"},
		{Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "1.1.1.1"},
	}
	tags := enabledDNSServerTags(servers, false)
	if got := safeNodeResolverTag("dns_proxy", servers, tags); got != "dns_direct" {
		t.Fatalf("safe node resolver = %q, want dns_direct", got)
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
	rules, ok := route["rules"].([]map[string]interface{})
	if !ok {
		t.Fatalf("route rules type = %T", route["rules"])
	}
	if len(rules) == 0 || rules[0]["outbound"] != "proxy" || !stringListContains(rules[0]["inbound"], updateProxyInboundTag) {
		t.Fatalf("update proxy rule must be first: %+v", rules)
	}
	var updateProxyRule, processRule, processRuleScoped, domainRule, ipRule bool
	for _, rule := range rules {
		if inbound, ok := rule["inbound"].([]string); ok && len(inbound) == 1 && inbound[0] == updateProxyInboundTag && rule["outbound"] == "proxy" {
			updateProxyRule = true
		}
		if rule["outbound"] != "direct" {
			continue
		}
		if rule["process_name"] != nil {
			processRule = true
			inbound, ok := rule["inbound"].([]string)
			processRuleScoped = ok && len(inbound) == 1 && inbound[0] == "tun-in"
		}
		domainRule = domainRule || rule["domain"] != nil
		ipRule = ipRule || rule["ip_cidr"] != nil
	}
	if !updateProxyRule || !processRule || !processRuleScoped || !domainRule || !ipRule {
		t.Fatalf("missing loop bypass rule: %+v", rules)
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

func TestGenerateInboundsDefaultsToLoopback(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewConfigGeneratorService(db, nil)
	inbounds, err := service.generateInbounds("", 0)
	if err != nil {
		t.Fatal(err)
	}
	foundMixed := false
	foundUpdateProxy := false
	for _, item := range inbounds {
		inbound, ok := item.(map[string]interface{})
		if !ok || inbound["type"] != "mixed" {
			continue
		}
		switch inbound["tag"] {
		case "mixed-in":
			foundMixed = true
			if inbound["listen"] != "127.0.0.1" || inbound["listen_port"] != model.DefaultMixedInboundPort {
				t.Fatalf("mixed inbound = %+v, want loopback:%d", inbound, model.DefaultMixedInboundPort)
			}
		case updateProxyInboundTag:
			foundUpdateProxy = true
			if inbound["listen"] != "127.0.0.1" || inbound["listen_port"] != updateProxyListenPort {
				t.Fatalf("update proxy inbound = %+v, want loopback:%d", inbound, updateProxyListenPort)
			}
		}
	}
	if !foundMixed || !foundUpdateProxy {
		t.Fatalf("generated inbounds missing mixed=%t update_proxy=%t: %+v", foundMixed, foundUpdateProxy, inbounds)
	}
}

func TestGenerateInboundsRejectsReservedUpdateProxyPort(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	service := NewConfigGeneratorService(db, nil)
	if _, err := service.generateInbounds("127.0.0.1", updateProxyListenPort); err == nil || !strings.Contains(err.Error(), "保留") {
		t.Fatalf("reserved port error = %v", err)
	}
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
	if preview.DefaultOutbound != "custom-proxy" || preview.InboundListen != stored.InboundListen || preview.InboundPort != stored.InboundPort || preview.LogLevel != stored.LogLevel {
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
}
