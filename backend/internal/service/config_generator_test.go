package service

import (
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
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

func TestNeedsGeneratedDNSBootstrapForUnresolvedDoH(t *testing.T) {
	servers := []model.DNSServer{{Tag: "dns_doh", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"}}
	if !needsGeneratedDNSBootstrap(servers, enabledDNSServerTags(servers, false)) {
		t.Fatal("domain-based DoH without another resolver should generate a local bootstrap")
	}
}

func TestBuiltinOutboundTagsOnlyIncludesRealOutbounds(t *testing.T) {
	got := collectionBuiltinOutboundTags(&model.ProxyCollectionWithNodes{
		ProxyCollection: model.ProxyCollection{
			Name:     "应用净化",
			NodeUIDs: `["reject","block","direct"]`,
		},
		NodeUIDs: []string{"reject", "block", "direct"},
	})
	if len(got) != 1 || got[0] != "direct" {
		t.Fatalf("builtin outbound tags = %v, want [direct]", got)
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

func TestMapTLSFingerprintFields(t *testing.T) {
	const fingerprint = "dd9dd03d942400ad4c1400879bda98f4fa097183aa9a91a1423cdd42a3e183d7"
	nodeData := map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled": true,
			"utls":    map[string]interface{}{"enabled": true, "fingerprint": fingerprint},
		},
	}
	mapTLSFingerprintFields(nodeData)
	tlsMap := nodeData["tls"].(map[string]interface{})
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("invalid uTLS fingerprint should be removed: %+v", tlsMap)
	}
	pins, ok := tlsMap["certificate_public_key_sha256"].([]string)
	if !ok || len(pins) != 1 || pins[0] != fingerprint {
		t.Fatalf("expected certificate pin, got %+v", tlsMap["certificate_public_key_sha256"])
	}

	nodeData = map[string]interface{}{
		"tls": map[string]interface{}{
			"enabled": true,
			"utls":    map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
		},
	}
	mapTLSFingerprintFields(nodeData)
	tlsMap = nodeData["tls"].(map[string]interface{})
	utlsMap := tlsMap["utls"].(map[string]interface{})
	if utlsMap["fingerprint"] != "chrome" {
		t.Fatalf("valid uTLS fingerprint should be preserved: %+v", tlsMap)
	}
}

func TestGenerateNodeOutboundSupportsNewCoreProtocols(t *testing.T) {
	svc := &ConfigGeneratorService{}
	tests := []struct {
		typ     string
		rawJSON string
	}{
		{typ: "anytls", rawJSON: `{"type":"anytls","server":"example.com","server_port":443,"password":"redacted","tls":{"enabled":true}}`},
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
		})
	}
}
