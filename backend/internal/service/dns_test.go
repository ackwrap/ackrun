package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestDNSGlobalSettingsFakeIPFollowsTUNMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDNSService(db)
	settings, err := svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.FakeIPEnabled {
		t.Fatal("default TUN + Mixed mode must enable FakeIP")
	}

	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	settings.FakeIPEnabled = true
	if err := svc.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.FakeIPEnabled {
		t.Fatal("Mixed mode must persist FakeIP as disabled")
	}

	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	stored.FakeIPEnabled = false
	if err := svc.SetDNSGlobalSettings(stored); err != nil {
		t.Fatal(err)
	}
	stored, err = db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !stored.FakeIPEnabled {
		t.Fatal("TUN mode must persist FakeIP as enabled")
	}
}

func TestDNSServerRejectsControlledOptionsAndInvalidDetours(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewDNSService(db)
	for name, request := range map[string]*model.DNSServerRequest{
		"options detour": {Tag: "dns-options-detour", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Options: map[string]interface{}{"detour": "direct"}},
		"options type":   {Tag: "dns-options-type", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Options: map[string]interface{}{"type": "local"}},
		"block detour":   {Tag: "dns-block", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "block"},
		"reject detour":  {Tag: "dns-reject", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: "reject"},
		"space detour":   {Tag: "dns-space", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Detour: " proxy"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.CreateDNSServer(request); err == nil {
				t.Fatal("unsafe DNS Server configuration was accepted")
			}
		})
	}
	if _, err := svc.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns-safe-options", Enabled: true, ServerType: "udp", Address: "1.1.1.1", Options: map[string]interface{}{"headers": map[string]interface{}{"Accept": "application/dns-message"}},
	}); err != nil {
		t.Fatalf("safe DNS Server options rejected: %v", err)
	}
}

func TestDNSStrategyBindingRequiresRemoteServer(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewDNSService(db)
	for _, server := range []*model.DNSServerRequest{
		{Tag: "dns_local", Enabled: true, ServerType: "local"},
		{Tag: "dns_remote", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
	} {
		if _, err := svc.CreateDNSServer(server); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_local",
	}); err == nil || !strings.Contains(err.Error(), "不能用于防泄漏策略绑定") {
		t.Fatalf("local strategy DNS binding error = %v", err)
	}
	proxyRule, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_remote",
	})
	if err != nil {
		t.Fatalf("remote strategy DNS binding rejected: %v", err)
	}
	if err := svc.UpdateDNSRule(proxyRule.ID, &model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: "dns_remote",
	}); err != nil {
		t.Fatalf("updating an existing strategy DNS binding rejected: %v", err)
	}
	if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true,
		Conditions: map[string]interface{}{
			"outbound":      []string{"proxy"},
			"domain_suffix": []string{"manual.example"},
		},
		Server: "dns_remote",
	}); err == nil || !strings.Contains(err.Error(), "只能包含 outbound 条件") {
		t.Fatalf("outbound-scoped hybrid DNS rule error = %v", err)
	}
	if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"direct", "proxy"}}, Server: "dns_remote",
	}); err == nil || !strings.Contains(err.Error(), "只能包含一个 outbound") {
		t.Fatalf("multi-outbound strategy DNS binding error = %v", err)
	}
	for name, conditions := range map[string]map[string]interface{}{
		"empty":      {"outbound": []string{}},
		"numeric":    {"outbound": []interface{}{123}},
		"block":      {"outbound": []string{"block"}},
		"whitespace": {"outbound": []string{"proxy "}},
		"unknown":    {"outbound": []string{"missing-strategy"}},
		"duplicate":  {"outbound": []string{"proxy"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, Conditions: conditions, Server: "dns_remote"}); err == nil {
				t.Fatal("invalid strategy DNS binding was accepted")
			}
		})
	}
	if err := db.CreateProxyCollection(&model.ProxyCollection{
		Name: "Developer", Type: "selector", SourceType: "manual", NodeUIDs: "[]", ReferencedGroupIDs: "[]", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"Developer"}}, Server: "dns_remote",
	}); err != nil {
		t.Fatalf("enabled named strategy DNS binding rejected: %v", err)
	}
}

func TestDNSServerMutationProtectsActiveStrategyBindings(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewDNSService(db)
	server, err := svc.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_remote", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query",
	})
	if err != nil {
		t.Fatal(err)
	}
	rule, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: server.Tag,
	})
	if err != nil {
		t.Fatal(err)
	}
	for name, request := range map[string]*model.DNSServerRequest{
		"disable": {Tag: server.Tag, Enabled: false, ServerType: "https", Address: "https://dns.example.com/dns-query"},
		"rename":  {Tag: "dns_renamed", Enabled: true, ServerType: "https", Address: "https://dns.example.com/dns-query"},
		"type":    {Tag: server.Tag, Enabled: true, ServerType: "local"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.UpdateDNSServer(server.ID, request); err == nil || !strings.Contains(err.Error(), "正被启用的策略 DNS 规则引用") {
				t.Fatalf("protected DNS server update error = %v", err)
			}
		})
	}
	if err := svc.DeleteDNSServer(server.ID); err == nil || !strings.Contains(err.Error(), "正被启用的策略 DNS 规则引用") {
		t.Fatalf("protected DNS server delete error = %v", err)
	}
	if err := svc.UpdateDNSServer(server.ID, &model.DNSServerRequest{
		Tag: server.Tag, Enabled: true, ServerType: "https", Address: "https://resolver.example/dns-query",
	}); err != nil {
		t.Fatalf("safe DNS server update rejected: %v", err)
	}
	if err := svc.UpdateDNSRule(rule.ID, &model.DNSRuleRequest{
		Enabled: false, Conditions: map[string]interface{}{"outbound": []string{"proxy"}}, Server: server.Tag,
	}); err != nil {
		t.Fatalf("disabling strategy DNS rule failed: %v", err)
	}
	if err := svc.DeleteDNSServer(server.ID); err != nil {
		t.Fatalf("DNS server delete after disabling binding failed: %v", err)
	}
}
