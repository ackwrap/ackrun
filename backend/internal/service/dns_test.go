package service

import (
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestDNSGlobalSettingsFakeIPFollowsTUNMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDNSService(db, nil)
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

func TestDNSGlobalSettingsOmitsIndependentCacheForNewCore(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDNSService(db, nil)
	svc.readCoreVersion = func() string { return "1.14.0-alpha.45" }
	settings, err := svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.IndependentCacheSupported {
		t.Fatal("1.14 alpha must report independent_cache as unsupported")
	}
	settings.IndependentCache = true
	if err := svc.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}

	svc.readCoreVersion = func() string { return "1.13.14" }
	settings, err = svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.IndependentCache {
		t.Fatal("new-core save persisted the unsupported independent_cache setting")
	}
}

func TestDNSIndependentCacheMigrationSerializesWithSave(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDNSService(db, nil)
	settings, err := svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.IndependentCache = true
	firstRead := make(chan struct{})
	releaseFirstRead := make(chan struct{})
	secondRead := make(chan struct{})
	var reads atomic.Int32
	svc.readCoreVersion = func() string {
		if reads.Add(1) == 1 {
			close(firstRead)
			<-releaseFirstRead
			return "1.13.14"
		}
		close(secondRead)
		return "1.14.0-alpha.45"
	}

	saveDone := make(chan error, 1)
	go func() { saveDone <- svc.SetDNSGlobalSettings(settings) }()
	<-firstRead
	migrateDone := make(chan error, 1)
	go func() {
		_, err := svc.MigrateIndependentCache("")
		migrateDone <- err
	}()
	select {
	case <-secondRead:
		close(releaseFirstRead)
		t.Fatal("migration read the new version before the in-flight save completed")
	case <-time.After(50 * time.Millisecond):
		close(releaseFirstRead)
	}
	if err := <-saveDone; err != nil {
		t.Fatal(err)
	}
	if err := <-migrateDone; err != nil {
		t.Fatal(err)
	}

	svc.readCoreVersion = func() string { return "1.13.14" }
	settings, err = svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.IndependentCache {
		t.Fatal("new-core migration left independent_cache persisted after a concurrent save")
	}
}

func TestDNSServerRejectsControlledOptionsAndInvalidDetours(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewDNSService(db, nil)
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
	svc := NewDNSService(db, nil)
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
