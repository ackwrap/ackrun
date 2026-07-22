package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestMigrateInlineACMEConfigForSingbox114(t *testing.T) {
	input := []byte(`{"inbounds":[{"type":"trojan","tls":{"enabled":true,"acme":{"domain":["example.com"],"email":"admin@example.com","provider":"letsencrypt"}}}]}`)
	result, migrated, err := migrateInlineACMEConfig(input, "1.14.0")
	if err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	if migrated != 1 {
		t.Fatalf("migrated = %d, want 1", migrated)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	tlsOptions := config["inbounds"].([]interface{})[0].(map[string]interface{})["tls"].(map[string]interface{})
	if _, exists := tlsOptions["acme"]; exists {
		t.Fatal("deprecated tls.acme should be removed")
	}
	provider := tlsOptions["certificate_provider"].(map[string]interface{})
	if provider["type"] != "acme" || provider["email"] != "admin@example.com" || provider["provider"] != "letsencrypt" {
		t.Fatalf("unexpected certificate provider: %+v", provider)
	}
}

func TestMigrateInlineACMEConfigKeepsSingbox113Schema(t *testing.T) {
	input := []byte(`{"inbounds":[{"tls":{"acme":{"domain":"example.com"}}}]}`)
	result, migrated, err := migrateInlineACMEConfig(input, "1.13.14")
	if err != nil {
		t.Fatalf("migrate config: %v", err)
	}
	if migrated != 0 || string(result) != string(input) {
		t.Fatalf("sing-box 1.13 config must stay unchanged: migrated=%d result=%s", migrated, result)
	}
}

func TestMigrateInlineACMERemovesDisabledOrDuplicateOptions(t *testing.T) {
	config := map[string]interface{}{
		"inbounds": []interface{}{
			map[string]interface{}{"tls": map[string]interface{}{"acme": map[string]interface{}{"domain": []interface{}{}}}},
			map[string]interface{}{"tls": map[string]interface{}{
				"acme":                 map[string]interface{}{"domain": "legacy.example.com"},
				"certificate_provider": "shared-cert",
			}},
		},
	}
	if migrated := migrateInlineACME(config); migrated != 2 {
		t.Fatalf("migrated = %d, want 2", migrated)
	}
	for _, rawInbound := range config["inbounds"].([]interface{}) {
		tlsOptions := rawInbound.(map[string]interface{})["tls"].(map[string]interface{})
		if _, exists := tlsOptions["acme"]; exists {
			t.Fatalf("deprecated tls.acme should be removed: %+v", tlsOptions)
		}
	}
}

func TestSingboxSupportsCertificateProviderPrerelease(t *testing.T) {
	if !singboxSupportsCertificateProvider("1.14.0-alpha.1") {
		t.Fatal("sing-box 1.14 prerelease should support certificate providers")
	}
	if singboxSupportsCertificateProvider("1.13.99") {
		t.Fatal("sing-box 1.13 must not receive the 1.14 schema")
	}
}

func TestMigrateManagedConfigRemovesLegacyUpdateProxy(t *testing.T) {
	input := []byte(`{
  "inbounds": [
    {"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":8888},
    {"type":"tun","tag":"tun-in","auto_route":true,"auto_redirect":false},
    {"type":"mixed","tag":"ackwrap-update-in","listen":"127.0.0.1","listen_port":9901}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[
    {"inbound":["ackwrap-update-in"],"action":"route","outbound":"proxy"},
    {"process_name":["ackwrap","ackwrap.exe","sing-box","sing-box.exe"],"action":"route","outbound":"direct"}
  ]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate managed config: %v", err)
	}
	if migrated != 3 {
		t.Fatalf("migrated = %d, want 3", migrated)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	inbounds := config["inbounds"].([]interface{})
	if hasTaggedConfigItem(inbounds, legacyUpdateProxyInboundTag) {
		t.Fatalf("legacy update proxy inbound remains: %+v", config["inbounds"])
	}
	if port := taggedInboundPort(inbounds, "mixed-in"); port != 8888 {
		t.Fatalf("custom mixed port changed to %d", port)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("legacy update proxy route remains: %+v", rules)
	}
	processRule := rules[0].(map[string]interface{})
	if !stringListContains(processRule["inbound"], "tun-in") {
		t.Fatalf("process bypass rule is not limited to tun-in: %+v", processRule)
	}

	secondResult, secondMigrated, err := migrateManagedConfigData(result)
	if err != nil {
		t.Fatalf("repeat managed config migration: %v", err)
	}
	if secondMigrated != 0 || string(secondResult) != string(result) {
		t.Fatalf("managed config migration is not idempotent: migrated=%d", secondMigrated)
	}
}

func TestMigrateManagedConfigAllowsFormerUpdateProxyPort(t *testing.T) {
	input := []byte(`{
  "inbounds": [{"type":"mixed","tag":"custom-in","listen_port":9901}],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatal(err)
	}
	if migrated != 0 || string(result) != string(input) {
		t.Fatalf("port 9901 config changed: migrated=%d result=%s", migrated, result)
	}
}

func TestMigrateManagedConfigUpgradesHistoricalMinimalConfig(t *testing.T) {
	input := []byte(`{
  "inbounds": [{"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":8888}],
  "outbounds": [{"type":"direct","tag":"direct"}]
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate historical minimal config: %v", err)
	}
	if migrated != 2 {
		t.Fatalf("migrated = %d, want 2", migrated)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	if !hasTaggedConfigItem(config["outbounds"].([]interface{}), "proxy") {
		t.Fatalf("proxy fallback missing: %+v", config["outbounds"])
	}
	inbounds := config["inbounds"].([]interface{})
	if hasTaggedConfigItem(inbounds, legacyUpdateProxyInboundTag) || taggedInboundPort(inbounds, "mixed-in") != 8888 {
		t.Fatalf("historical inbounds not migrated: %+v", inbounds)
	}
	if rules, exists := config["route"].(map[string]interface{})["rules"]; exists && len(rules.([]interface{})) != 0 {
		t.Fatalf("historical config gained unexpected route rules: %+v", rules)
	}
	secondResult, secondMigrated, err := migrateManagedConfigData(result)
	if err != nil || secondMigrated != 0 || string(secondResult) != string(result) {
		t.Fatalf("historical migration is not idempotent: migrated=%d err=%v", secondMigrated, err)
	}
}

func TestMigrateManagedConfigRepairsEmptyProxyGroupAndRemovesLegacyProxy(t *testing.T) {
	input := []byte(`{
  "inbounds": [
    {"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":7890},
    {"type":"mixed","tag":"ackwrap-update-in","listen":"127.0.0.1","listen_port":9901}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":[]}],
  "route": {"rules":[{"inbound":["ackwrap-update-in"],"action":"route","outbound":"proxy"}]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("repair empty proxy group: %v", err)
	}
	if migrated != 3 {
		t.Fatalf("migrated = %d, want proxy repair, inbound removal and route removal", migrated)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	proxyOutbound := taggedConfigItem(config["outbounds"].([]interface{}), "proxy")
	if proxyOutbound["type"] != "selector" || !stringListEquals(proxyOutbound["outbounds"], "direct") {
		t.Fatalf("proxy fallback not repaired: %+v", proxyOutbound)
	}
	if hasTaggedConfigItem(config["inbounds"].([]interface{}), legacyUpdateProxyInboundTag) || len(config["route"].(map[string]interface{})["rules"].([]interface{})) != 0 {
		t.Fatalf("legacy update proxy remains: %+v", config)
	}
	secondResult, secondMigrated, err := migrateManagedConfigData(result)
	if err != nil || secondMigrated != 0 || string(secondResult) != string(result) {
		t.Fatalf("proxy fallback migration is not idempotent: migrated=%d err=%v", secondMigrated, err)
	}
}

func TestMigrateManagedConfigRemovesMalformedLegacyProxy(t *testing.T) {
	input := []byte(`{
  "inbounds": [
    {"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":8888},
    {"type":"socks","tag":"ackwrap-update-in","listen":"0.0.0.0","listen_port":9000}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[
    {"process_name":["ackwrap.exe"],"action":"route","outbound":"direct"},
    {"inbound":["ackwrap-update-in"],"network":"tcp","action":"route","outbound":"proxy"}
  ]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate malformed update proxy config: %v", err)
	}
	if migrated != 3 {
		t.Fatalf("migrated = %d, want 3", migrated)
	}

	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	inbounds := config["inbounds"].([]interface{})
	if hasTaggedConfigItem(inbounds, legacyUpdateProxyInboundTag) {
		t.Fatalf("legacy update proxy inbound remains: %+v", inbounds)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("legacy update proxy route remains: %+v", rules)
	}
	processRule := rules[0].(map[string]interface{})
	if !stringListEquals(processRule["inbound"], "tun-in") {
		t.Fatalf("single-process bypass rule is not scoped to tun-in: %+v", processRule)
	}
	secondResult, secondMigrated, err := migrateManagedConfigData(result)
	if err != nil || secondMigrated != 0 || string(secondResult) != string(result) {
		t.Fatalf("normalized migration is not idempotent: migrated=%d err=%v", secondMigrated, err)
	}
}

func TestMigrateManagedConfigSplitsMixedProcessBypassRule(t *testing.T) {
	input := []byte(`{
  "inbounds": [
    {"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":7890},
    {"type":"mixed","tag":"ackwrap-update-in","listen":"127.0.0.1","listen_port":9901}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[
    {"inbound":["ackwrap-update-in"],"action":"route","outbound":"proxy"},
    {"process_name":["ackwrap.exe","helper.exe"],"action":"route","outbound":"direct"}
  ]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("split mixed process rule: %v", err)
	}
	if migrated != 3 {
		t.Fatalf("migrated = %d, want 3", migrated)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	if len(rules) != 2 {
		t.Fatalf("split rules = %+v", rules)
	}
	ackwrapRule := rules[0].(map[string]interface{})
	otherRule := rules[1].(map[string]interface{})
	if !stringListContains(ackwrapRule["process_name"], "ackwrap.exe") || !stringListEquals(ackwrapRule["inbound"], "tun-in") {
		t.Fatalf("Ackwrap process rule not scoped: %+v", ackwrapRule)
	}
	if !stringListContains(otherRule["process_name"], "helper.exe") || otherRule["inbound"] != nil {
		t.Fatalf("other process behavior changed: %+v", otherRule)
	}
}

func TestMigrateManagedConfigAddsTUNRoutingSafetyAndRemovesLegacyProxy(t *testing.T) {
	input := []byte(`{
  "http_clients": [{"tag":"ackwrap-rule-set-direct"}],
  "inbounds": [
    {"type":"tun","tag":"tun-in","interface_name":"tun0","address":["172.19.0.1/30","fdfe:dcba:9876::1/126"],"auto_route":true,"strict_route":true,"auto_redirect":false},
    {"type":"mixed","tag":"ackwrap-update-in","listen":"127.0.0.1","listen_port":9901}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[{"inbound":["ackwrap-update-in"],"action":"route","outbound":"proxy"}]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate TUN routing safety: %v", err)
	}
	if migrated != 4 {
		t.Fatalf("migrated = %d, want TUN addresses, route safety, inbound removal and route removal", migrated)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	if config["route"].(map[string]interface{})["auto_detect_interface"] != true {
		t.Fatalf("route safety missing: %+v", config["route"])
	}
	if hasTaggedConfigItem(config["inbounds"].([]interface{}), legacyUpdateProxyInboundTag) || len(config["route"].(map[string]interface{})["rules"].([]interface{})) != 0 {
		t.Fatalf("legacy update proxy remains: %+v", config)
	}
}

func TestMigrateManagedConfigMovesAckwrapKernelBypassBeforeSniff(t *testing.T) {
	input := []byte(`{
  "http_clients": [{"tag":"ackwrap-rule-set-direct"}],
  "inbounds": [{"type":"tun","tag":"tun-in","interface_name":"tun0","address":["172.254.0.1/30","fdfe:dcba:9876::1/126"],"auto_route":true,"strict_route":true,"auto_redirect":true,"auto_redirect_output_mark":"0x2024"}],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[
    {"action":"sniff"},
    {"process_name":["ackwrap","ackwrap.exe","sing-box","sing-box.exe"],"inbound":["tun-in"],"action":"route","outbound":"direct"},
    {"domain_suffix":["example.com"],"action":"route","outbound":"proxy"}
  ]}
}`)
	result, migrated, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate kernel bypass: %v", err)
	}
	if migrated != 4 {
		t.Fatalf("migrated = %d, want TUN addresses, route safety, bypass action and ordering", migrated)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	rules := config["route"].(map[string]interface{})["rules"].([]interface{})
	first := rules[0].(map[string]interface{})
	if first["action"] != "bypass" || first["outbound"] != "direct" || !stringListContains(first["process_name"], "ackwrap") {
		t.Fatalf("kernel bypass is not first: %+v", rules)
	}
	if rules[1].(map[string]interface{})["action"] != "sniff" {
		t.Fatalf("sniff rule ordering changed unexpectedly: %+v", rules)
	}
	secondResult, secondMigrated, err := migrateManagedConfigData(result)
	if err != nil || secondMigrated != 0 || string(secondResult) != string(result) {
		t.Fatalf("kernel bypass migration is not idempotent: migrated=%d err=%v", secondMigrated, err)
	}
}

func TestMigrateAckwrapProcessBypassKeepsDNSHijackFirst(t *testing.T) {
	rules := []interface{}{
		map[string]interface{}{"action": "sniff"},
		map[string]interface{}{
			"process_name": []interface{}{"ackwrap", "sing-box"},
			"inbound":      []interface{}{"tun-in"},
			"action":       "bypass",
			"outbound":     "direct",
		},
		map[string]interface{}{"inbound": dnsInboundTag, "action": "hijack-dns"},
		map[string]interface{}{"domain_suffix": []interface{}{"example.com"}, "action": "route", "outbound": "proxy"},
	}

	migratedRules, migrated := migrateAckwrapProcessBypassRules(rules)
	if migrated != 1 {
		t.Fatalf("migrated = %d, want ordering migration", migrated)
	}
	first := migratedRules[0].(map[string]interface{})
	if first["inbound"] != dnsInboundTag || first["action"] != "hijack-dns" {
		t.Fatalf("DNS hijack is not first: %+v", migratedRules)
	}
	second := migratedRules[1].(map[string]interface{})
	if second["action"] != "bypass" || second["outbound"] != "direct" {
		t.Fatalf("kernel bypass is not second: %+v", migratedRules)
	}
	if migratedRules[2].(map[string]interface{})["action"] != "sniff" {
		t.Fatalf("sniff rule ordering changed unexpectedly: %+v", migratedRules)
	}

	secondPass, secondMigrated := migrateAckwrapProcessBypassRules(migratedRules)
	if secondMigrated != 0 || !reflect.DeepEqual(secondPass, migratedRules) {
		t.Fatalf("migration is not idempotent: migrated=%d rules=%+v", secondMigrated, secondPass)
	}
}

func TestMigrateAckwrapTUNInboundsAddsMissingDefaultsAndPreservesExplicitSettings(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{"type": "tun", "tag": "tun-in", "interface_name": "tun0", "address": []string{defaultTUNIPv4Address}},
		map[string]interface{}{"type": "tun", "tag": "tun-in", "address": []string{"10.0.0.1/30"}, "auto_route": false, "strict_route": false, "auto_redirect": false},
		map[string]interface{}{"type": "mixed"},
	}
	if migrated := migrateAckwrapTUNInbounds(inbounds, true); migrated != 5 {
		t.Fatalf("migrated = %d, want 5", migrated)
	}
	first := inbounds[0].(map[string]interface{})
	if first["auto_route"] != true || first["strict_route"] != true || first["auto_redirect"] != true || first["auto_redirect_output_mark"] != "0x2024" || !stringListContains(first["address"], defaultTUNIPv6Address) {
		t.Fatalf("missing Ackwrap TUN defaults: %+v", first)
	}
	if inbounds[1].(map[string]interface{})["auto_route"] != false || inbounds[1].(map[string]interface{})["strict_route"] != false || inbounds[1].(map[string]interface{})["auto_redirect"] != false {
		t.Fatalf("explicit auto_redirect changed: %+v", inbounds[1])
	}
}

func TestMigrateAckwrapTUNInboundsAddsOutputMarkForExplicitAutoRedirect(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{
			"type":          "tun",
			"tag":           "tun-in",
			"address":       []string{defaultTUNIPv4Address, defaultTUNIPv6Address},
			"auto_route":    false,
			"strict_route":  false,
			"auto_redirect": true,
		},
	}
	if migrated := migrateAckwrapTUNInbounds(inbounds, true); migrated != 1 {
		t.Fatalf("migrated = %d, want output mark only", migrated)
	}
	inbound := inbounds[0].(map[string]interface{})
	if inbound["auto_route"] != false || inbound["auto_redirect_output_mark"] != "0x2024" {
		t.Fatalf("explicit auto-redirect migration = %+v", inbound)
	}
}

func TestMigrateAckwrapTUNInboundsRecognizesLegacyDefaultAddress(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "tun0",
			"address":        []string{legacyDefaultTUNIPv4Address},
			"auto_route":     true,
			"strict_route":   true,
		},
	}
	if !isAckwrapManagedConfig(map[string]interface{}{}, inbounds, map[string]interface{}{}) {
		t.Fatal("legacy default TUN address was not recognized as Ackwrap-managed")
	}
	if migrated := migrateAckwrapTUNInbounds(inbounds, false); migrated != 1 {
		t.Fatalf("legacy TUN migrated fields = %d, want IPv6 address only", migrated)
	}
	if !stringListContains(inbounds[0].(map[string]interface{})["address"], defaultTUNIPv6Address) {
		t.Fatalf("legacy TUN missing default IPv6 address: %+v", inbounds[0])
	}
}

func TestMigrateAckwrapTUNInboundsReplacesPreviousDefaults(t *testing.T) {
	inbounds := []interface{}{
		map[string]interface{}{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "tun0",
			"address":        []interface{}{previousDefaultTUNIPv4, previousDefaultTUNIPv6},
			"auto_route":     true,
			"strict_route":   true,
		},
	}
	if !isAckwrapManagedConfig(map[string]interface{}{}, inbounds, map[string]interface{}{}) {
		t.Fatal("previous default TUN address was not recognized as Ackwrap-managed")
	}
	if migrated := migrateAckwrapTUNInbounds(inbounds, false); migrated != 1 {
		t.Fatalf("previous TUN migrated fields = %d, want address replacement only", migrated)
	}
	addresses := inbounds[0].(map[string]interface{})["address"]
	if !stringListContains(addresses, defaultTUNIPv4Address) || !stringListContains(addresses, defaultTUNIPv6Address) {
		t.Fatalf("previous TUN defaults not replaced: %+v", addresses)
	}
	if stringListContains(addresses, previousDefaultTUNIPv4) || stringListContains(addresses, previousDefaultTUNIPv6) {
		t.Fatalf("previous TUN defaults remain: %+v", addresses)
	}
}

func TestMigrateAckwrapTUNInboundsPreservesCustomAddresses(t *testing.T) {
	customIPv4 := "10.254.0.1/30"
	customIPv6 := "fd12:3456:789a::1/126"
	inbounds := []interface{}{
		map[string]interface{}{
			"type":           "tun",
			"tag":            "tun-in",
			"interface_name": "tun0",
			"address":        []interface{}{customIPv4, customIPv6},
			"auto_route":     true,
			"strict_route":   true,
		},
	}
	if migrated := migrateAckwrapTUNInbounds(inbounds, false); migrated != 0 {
		t.Fatalf("custom TUN migrated fields = %d, want 0", migrated)
	}
	addresses := inbounds[0].(map[string]interface{})["address"]
	if !stringListContains(addresses, customIPv4) || !stringListContains(addresses, customIPv6) {
		t.Fatalf("custom TUN addresses changed: %+v", addresses)
	}
}

func TestMigrateManagedConfigDoesNotEnableCustomTUN(t *testing.T) {
	input := []byte(`{
  "inbounds": [{"type":"tun","tag":"custom-tun","auto_route":true}],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[]}
}`)
	result, _, err := migrateManagedConfigData(input)
	if err != nil {
		t.Fatalf("migrate custom TUN config: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(result, &config); err != nil {
		t.Fatal(err)
	}
	inbound := config["inbounds"].([]interface{})[0].(map[string]interface{})
	if _, exists := inbound["auto_redirect"]; exists {
		t.Fatalf("custom TUN auto_redirect was changed: %+v", inbound)
	}
	if _, exists := config["route"].(map[string]interface{})["auto_detect_interface"]; exists {
		t.Fatalf("custom TUN route was changed: %+v", config["route"])
	}
}

func TestMigrateCompatibilityKeepsOriginalWhenValidationFails(t *testing.T) {
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.json")
	original := []byte(`{
  "inbounds": [
    {"type":"mixed","tag":"mixed-in","listen_port":8888},
    {"type":"mixed","tag":"ackwrap-update-in","listen":"127.0.0.1","listen_port":9901}
  ],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[{"inbound":["ackwrap-update-in"],"action":"route","outbound":"proxy"}]}
}`)
	if err := os.WriteFile(configPath, original, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewConfigService(&paths.Paths{DataDir: dir, ConfigDir: configDir, ConfigPath: configPath}, db, NewRealtimeService())
	svc.configValidator = func(string) error { return errors.New("rejected test config") }
	migrated, err := svc.MigrateCompatibility("1.13.0")
	if err == nil || migrated {
		t.Fatalf("migration result = %t, %v; want validation error", migrated, err)
	}
	after, readErr := os.ReadFile(configPath)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != string(original) {
		t.Fatal("validation failure replaced the original config")
	}
}

func TestMigrateCompatibilityProtectsUnchangedConfig(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose POSIX file permissions")
	}
	dir := t.TempDir()
	configDir := filepath.Join(dir, "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(configDir, "config.json")
	ready, _, err := migrateManagedConfigData([]byte(`{
  "inbounds": [{"type":"mixed","tag":"mixed-in","listen_port":7890}],
  "outbounds": [{"type":"direct","tag":"direct"},{"type":"selector","tag":"proxy","outbounds":["direct"]}],
  "route": {"rules":[]}
}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, ready, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(dir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewConfigService(&paths.Paths{DataDir: dir, ConfigDir: configDir, ConfigPath: configPath}, db, NewRealtimeService())
	updated, err := svc.MigrateCompatibility("1.14.0")
	if err != nil || !updated {
		t.Fatalf("MigrateCompatibility() = %t, %v; want permission update", updated, err)
	}
	info, err := os.Stat(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0600 {
		t.Fatalf("config mode = %o, want 600", mode)
	}
}

func TestMigrateInternalRuleSetAccessTokenData(t *testing.T) {
	data := []byte(`{"route":{"rule_set":[
		{"type":"remote","url":"http://127.0.0.1:8080/api/v1/rules/subscriptions/1/content"},
		{"type":"remote","url":"http://127.0.0.1:8080/api/v1/rules/geo/rule-sets/geosite-google/content?access_token=old"},
		{"type":"remote","url":"https://example.com/rules.srs"}
	]}}`)
	migrated, count, err := migrateInternalRuleSetAccessTokenData(data, "http://127.0.0.1:9090", "new token")
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("migrated count = %d, want 2", count)
	}
	text := string(migrated)
	if strings.Count(text, "access_token=new+token") != 2 || !strings.Contains(text, "127.0.0.1:9090") || !strings.Contains(text, "https://example.com/rules.srs") {
		t.Fatalf("unexpected migrated config: %s", text)
	}
}

func taggedInboundPort(inbounds []interface{}, tag string) int {
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]interface{})
		if ok && inbound["tag"] == tag {
			return configNumber(inbound["listen_port"])
		}
	}
	return 0
}
