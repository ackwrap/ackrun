package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
)

func TestDevelopmentConfigMigrationRemovesOnlyInactiveOptions(t *testing.T) {
	input := []byte(`{"inbounds":[{"type":"tun","stack":"system","multi_queue":false,"auto_redirect_tproxy_mark":"0x0"}],"endpoints":[{"type":"wireguard","on_demand":false},{"type":"tailscale","on_demand":null}],"experimental":{"cache_file":{"enabled":false,"buffer_size":"1 MiB","flush_interval":"1s","store_fakeip":true}}}`)
	result, count, err := migrateDevelopmentConfigData(input, "1.14.3")
	if err != nil || count != 6 {
		t.Fatalf("migrate inactive options: count=%d error=%v", count, err)
	}
	for _, field := range []string{"multi_queue", "auto_redirect_tproxy_mark", "on_demand", "buffer_size", "flush_interval"} {
		if strings.Contains(string(result), field) {
			t.Fatalf("inactive field %s was not removed", field)
		}
	}
	if !strings.Contains(string(result), `"store_fakeip": true`) || !strings.Contains(string(result), `"stack": "system"`) {
		t.Fatal("unrelated config options changed")
	}
	again, count, err := migrateDevelopmentConfigData(result, "1.14.3")
	if err != nil || count != 0 || string(again) != string(result) {
		t.Fatalf("migration was not idempotent: count=%d error=%v", count, err)
	}
	for _, version := range []string{"1.15.0-alpha.1", ""} {
		result, count, err := migrateDevelopmentConfigData(input, version)
		if err != nil || count != 0 || string(result) != string(input) {
			t.Fatalf("development/unknown version %q was changed", version)
		}
	}
}

func TestDevelopmentConfigMigrationRejectsActiveOptionsWithoutValues(t *testing.T) {
	for _, test := range []struct{ input, field string }{
		{`{"inbounds":[{"type":"tun","tag":"private-node-name","stack":"go"}]}`, "inbounds[0].stack"},
		{`{"inbounds":[{"type":"tun","multi_queue":true}]}`, "inbounds[0].multi_queue"},
		{`{"inbounds":[{"type":"tun","auto_redirect_tproxy_mark":"0x2024"}]}`, "inbounds[0].auto_redirect_tproxy_mark"},
		{`{"endpoints":[{"type":"wireguard","tag":"private-node-name","on_demand":true,"private_key":"synthetic-secret"}]}`, "endpoints[0].on_demand"},
		{`{"endpoints":[{"type":"openvpn","on_demand":"false"}]}`, "endpoints[0].on_demand"},
		{`{"experimental":{"cache_file":{"enabled":true,"buffer_size":"1 MiB"}}}`, "experimental.cache_file.buffer_size"},
		{`{"experimental":{"cache_file":{"enabled":true,"flush_interval":"1s"}}}`, "experimental.cache_file.flush_interval"},
	} {
		t.Run(test.field, func(t *testing.T) {
			result, _, err := migrateCoreCompatibilityData([]byte(test.input), "1.14.3")
			if err == nil || result != nil || !strings.Contains(err.Error(), test.field) {
				t.Fatalf("active option was not blocked: %v", err)
			}
			if strings.Contains(err.Error(), "synthetic-secret") || strings.Contains(err.Error(), "private-node-name") {
				t.Fatal("compatibility error exposed config values")
			}
		})
	}
}

func TestCoreCompatibilityRejectsNonObjectConfig(t *testing.T) {
	for _, version := range []string{"1.14.3", "1.15.0-alpha.1", ""} {
		for _, data := range []string{"null", "[]", "invalid JSON"} {
			if result, _, err := migrateCoreCompatibilityData([]byte(data), version); err == nil || result != nil {
				t.Fatalf("non-object config was not rejected for version %q", version)
			}
		}
	}
}

func TestWireGuardOnDemandGenerationFollowsCoreVersion(t *testing.T) {
	for _, version := range []string{"1.14.3", "1.15.0-alpha.1"} {
		for _, enabled := range []bool{false, true} {
			for _, native := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%t/native=%t", version, enabled, native), func(t *testing.T) {
					raw := map[string]interface{}{
						"type": "wireguard", "address": []string{"10.0.0.2/32"}, "private_key": "synthetic-private",
						"on_demand": enabled,
					}
					if native {
						raw["peers"] = []interface{}{map[string]interface{}{"address": "192.0.2.1", "port": 51820, "public_key": "synthetic-public"}}
					} else {
						raw["server"], raw["server_port"], raw["public_key"] = "192.0.2.1", 51820, "synthetic-public"
					}
					data, _ := json.Marshal(raw)
					node := &model.Node{Type: "wireguard", RawJSON: string(data)}
					svc := &ConfigGeneratorService{readCoreVersion: func() string { return version }}
					endpoint, err := svc.generateWireGuardEndpoint(node, "synthetic-wg", nil)
					if version == "1.14.3" && enabled {
						if err == nil || !strings.Contains(err.Error(), "on_demand") {
							t.Fatal("active on_demand was silently dropped")
						}
					} else if err != nil {
						t.Fatal(err)
					} else if version == "1.14.3" {
						if _, exists := endpoint["on_demand"]; exists {
							t.Fatal("inactive on_demand leaked into stable config")
						}
					} else if endpoint["on_demand"] != enabled {
						t.Fatal("development option was not preserved")
					}
					if node.RawJSON != string(data) {
						t.Fatal("generator mutated the imported node")
					}
				})
			}
		}
	}
}

func TestConfigValidationCacheTracksCoreReplacement(t *testing.T) {
	root := t.TempDir()
	config, binary := filepath.Join(root, "config.json"), filepath.Join(root, "core")
	if err := os.WriteFile(config, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, []byte("old"), 0755); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := &ConfigService{paths: &paths.Paths{BinaryPath: binary}, configValidator: func(string) error { calls++; return nil }}
	info, _ := os.Stat(config)
	for range 2 {
		if err := svc.validateFileCached(config, info); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 1 {
		t.Fatalf("unchanged validation calls=%d", calls)
	}
	if err := os.WriteFile(binary, []byte("new core"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := svc.validateFileCached(config, info); err != nil || calls != 2 {
		t.Fatalf("core replacement did not invalidate cache: calls=%d error=%v", calls, err)
	}
}
