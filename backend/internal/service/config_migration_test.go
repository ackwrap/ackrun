package service

import (
	"encoding/json"
	"testing"
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
