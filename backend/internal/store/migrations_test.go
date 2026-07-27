package store

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestMigrateSeedsDefaultLocalDNSRuleOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "223.5.5.5",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := db.ListDNSRules()
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	if len(rules) != 1 || rules[0].Server != "dns_direct" || !rules[0].Enabled {
		db.Close()
		t.Fatalf("default DNS rules = %+v", rules)
	}
	var conditions struct {
		DomainSuffix []string `json:"domain_suffix"`
	}
	if err := json.Unmarshal([]byte(rules[0].ConditionsJSON), &conditions); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if !reflect.DeepEqual(conditions.DomainSuffix, defaultLocalDNSRuleSuffixes) {
		db.Close()
		t.Fatalf("default local suffixes = %v, want %v", conditions.DomainSuffix, defaultLocalDNSRuleSuffixes)
	}
	if err := db.DeleteDNSRule(rules[0].ID); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rules, err = db.ListDNSRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 0 {
		t.Fatalf("deleted default DNS rule was recreated: %+v", rules)
	}
}

func TestMigrateKeepsExistingDefaultLocalDNSRule(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "223.5.5.5",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true,
		Conditions: map[string]interface{}{
			"domain_suffix": append([]string(nil), defaultLocalDNSRuleSuffixes...),
		},
		Server: "dns_direct",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rules, err := db.ListDNSRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 1 {
		t.Fatalf("existing default DNS rule was duplicated: %+v", rules)
	}
}

func TestMigrateDoesNotTreatRestrictedLocalRuleAsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "223.5.5.5",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true,
		Conditions: map[string]interface{}{
			"domain_suffix": append([]string(nil), defaultLocalDNSRuleSuffixes...),
			"query_type":    []string{"A"},
		},
		Server: "dns_direct",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rules, err := db.ListDNSRules()
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("restricted local rule suppressed complete default: %+v", rules)
	}
	foundComplete := false
	for _, rule := range rules {
		if dnsRuleCoversDefaultLocalSuffixes(rule.ConditionsJSON) {
			foundComplete = true
			break
		}
	}
	if !foundComplete {
		t.Fatalf("complete default local rule not created: %+v", rules)
	}
}

func TestMigrateWaitsForUsableDefaultDNSServer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "invalid", Enabled: true, ServerType: "unknown"}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "invalid-address", Enabled: true, ServerType: "https", Address: "https://"}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.CreateDNSRule(&model.DNSRuleRequest{
		Enabled: true,
		Conditions: map[string]interface{}{
			"domain_suffix": append([]string(nil), defaultLocalDNSRuleSuffixes...),
		},
		Server: "invalid-address",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var markerCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key = ?`, defaultLocalDNSRuleMarker).Scan(&markerCount); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if markerCount != 0 {
		db.Close()
		t.Fatal("invalid DNS Server consumed the default-rule marker")
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "dns_direct", Enabled: true, ServerType: "udp", Address: "223.5.5.5",
	}); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.EnsureDefaultLocalDNSRule(); err != nil {
		db.Close()
		t.Fatal(err)
	}
	rules, err := db.ListDNSRules()
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	foundValidDefault := false
	for _, rule := range rules {
		if rule.Server == "dns_direct" && dnsRuleCoversDefaultLocalSuffixes(rule.ConditionsJSON) {
			foundValidDefault = true
			break
		}
	}
	if len(rules) != 2 || !foundValidDefault {
		db.Close()
		t.Fatalf("usable DNS Server did not receive default rule: %+v", rules)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestDefaultLocalDNSRuleCoverageRequiresNormalizedUnrestrictedSuffixes(t *testing.T) {
	conditions, err := json.Marshal(map[string]interface{}{
		"domain_suffix": append([]string(nil), defaultLocalDNSRuleSuffixes...),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !dnsRuleCoversDefaultLocalSuffixes(string(conditions)) {
		t.Fatal("normalized default suffixes were not recognized")
	}

	superset := append(append([]string(nil), defaultLocalDNSRuleSuffixes...), "example.com")
	conditions, err = json.Marshal(map[string]interface{}{"domain_suffix": superset})
	if err != nil {
		t.Fatal(err)
	}
	if !dnsRuleCoversDefaultLocalSuffixes(string(conditions)) {
		t.Fatal("an unrestricted real-IP rule covering all defaults should prevent a duplicate")
	}

	withWhitespace := append([]string(nil), defaultLocalDNSRuleSuffixes...)
	withWhitespace[1] = " lan "
	conditions, err = json.Marshal(map[string]interface{}{"domain_suffix": withWhitespace})
	if err != nil {
		t.Fatal(err)
	}
	if dnsRuleCoversDefaultLocalSuffixes(string(conditions)) {
		t.Fatal("non-normalized suffixes incorrectly suppressed the default rule")
	}

	conditions, err = json.Marshal(map[string]interface{}{
		"domain_suffix": append([]string(nil), defaultLocalDNSRuleSuffixes...),
		"query_type":    []string{"A"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if dnsRuleCoversDefaultLocalSuffixes(string(conditions)) {
		t.Fatal("a restricted rule incorrectly suppressed the default rule")
	}
}

func TestMigrateRemovesSongziAndPromotesIPAPIIS(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE geoip_providers SET is_default = 0 WHERE provider_key = 'ipapi.is'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`INSERT INTO geoip_providers (name, provider_key, template, enabled, is_default, builtin, created_at, updated_at) VALUES ('legacy', 'songzixian', 'builtin', 1, 1, 1, unixepoch(), unixepoch())`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var songziCount, ipapiDefault int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM geoip_providers WHERE provider_key = 'songzixian'`).Scan(&songziCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT is_default FROM geoip_providers WHERE provider_key = 'ipapi.is'`).Scan(&ipapiDefault); err != nil {
		t.Fatal(err)
	}
	if songziCount != 0 || ipapiDefault != 1 {
		t.Fatalf("songzi count = %d, ipapi.is default = %d", songziCount, ipapiDefault)
	}
}

func TestMigrateRemovesLegacyUpdateCredentialsAndProxyMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ackwrap.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"update.github_token":      "legacy-token",
		"update.proxy_url":         "http://127.0.0.1:9901",
		"update.acceleration":      "proxy",
		"update.custom_mirror_url": "https://mirror.example",
	} {
		if _, err := db.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, unixepoch())`, key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var legacyCount, mirrorCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key IN ('update.github_token', 'update.proxy_url') OR (key = 'update.acceleration' AND value = 'proxy')`).Scan(&legacyCount); err != nil {
		t.Fatal(err)
	}
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key = 'update.custom_mirror_url' AND value = 'https://mirror.example'`).Scan(&mirrorCount); err != nil {
		t.Fatal(err)
	}
	if legacyCount != 0 || mirrorCount != 1 {
		t.Fatalf("legacy update settings = %d, preserved mirrors = %d", legacyCount, mirrorCount)
	}
}
