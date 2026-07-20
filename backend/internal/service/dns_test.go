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
	if _, err := svc.CreateDNSServer(&model.DNSServerRequest{Tag: "custom-fakeip", Enabled: true, ServerType: "fakeip"}); err == nil || !strings.Contains(err.Error(), "自动管理") {
		t.Fatalf("manual FakeIP Server error = %v", err)
	}
}

func TestDNSRulesRejectOutboundConditionsAndFakeIPServers(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewDNSService(db)
	if _, err := svc.CreateDNSServer(&model.DNSServerRequest{Tag: "dns_proxy", Enabled: true, ServerType: "udp", Address: "1.1.1.1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "legacy_fakeip", Enabled: true, ServerType: "fakeip"}); err != nil {
		t.Fatal(err)
	}
	for name, conditions := range map[string]map[string]interface{}{
		"pure outbound":   {"outbound": []string{"proxy"}},
		"hybrid outbound": {"outbound": []string{"proxy"}, "domain_suffix": []string{"example.com"}},
		"empty outbound":  {"outbound": []string{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := svc.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, Conditions: conditions, Server: "dns_proxy"})
			if err == nil || !strings.Contains(err.Error(), "不再支持 outbound") {
				t.Fatalf("legacy outbound DNS rule error = %v", err)
			}
		})
	}
	created, err := svc.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true, Conditions: map[string]interface{}{"domain_suffix": []string{"example.com"}}, Server: "dns_proxy",
	})
	if err != nil {
		t.Fatalf("explicit domain DNS rule rejected: %v", err)
	}
	for _, tag := range []string{"fakeip", "legacy_fakeip"} {
		if _, err := svc.CreateDNSRule(&model.DNSRuleRequest{Enabled: true, Conditions: map[string]interface{}{"domain_suffix": []string{"example.com"}}, Server: tag}); err == nil || !strings.Contains(err.Error(), "不能引用 FakeIP") {
			t.Fatalf("explicit FakeIP rule server %q error = %v", tag, err)
		}
	}
	if err := svc.UpdateDNSRule(created.ID, &model.DNSRuleRequest{Enabled: true, Conditions: map[string]interface{}{"domain_suffix": []string{"example.com"}}, Server: "fakeip"}); err == nil || !strings.Contains(err.Error(), "不能引用 FakeIP") {
		t.Fatalf("updated FakeIP rule error = %v", err)
	}
	settings, err := svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "legacy_fakeip"
	if err := svc.SetDNSGlobalSettings(settings); err == nil || !strings.Contains(err.Error(), "不能引用 FakeIP") {
		t.Fatalf("FakeIP global final error = %v", err)
	}
}
