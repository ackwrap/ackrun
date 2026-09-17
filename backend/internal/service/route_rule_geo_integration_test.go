package service

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Opt-in compatibility check against public release files and a real core:
// ACKWRAP_TEST_GEO_DIR contains geoip.dat and geosite.dat;
// ACKWRAP_TEST_SING_BOX points to the executable. No download occurs in tests.
func TestGeoSourceRealDataCompatibility(t *testing.T) {
	dir, core := os.Getenv("ACKWRAP_TEST_GEO_DIR"), os.Getenv("ACKWRAP_TEST_SING_BOX")
	if dir == "" || core == "" {
		t.Skip("set ACKWRAP_TEST_GEO_DIR and ACKWRAP_TEST_SING_BOX for release compatibility testing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if output, err := exec.CommandContext(ctx, core, "version").CombinedOutput(); err != nil {
		t.Fatalf("core version: %s: %v", output, err)
	} else {
		t.Logf("%s", output)
	}
	outputDir := t.TempDir()
	var ruleSets, routeRules []map[string]any
	// Optional original-format fixtures also exercise switching back to SagerNet.
	for _, kind := range []string{"geoip", "geosite"} {
		path := filepath.Join(dir, kind+".db")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			continue
		}
		codes, err := geoAssetCodes(kind, path)
		if err != nil || len(codes) == 0 {
			t.Fatalf("native %s: %d categories, %v", kind, len(codes), err)
		}
		data, err := geoDatabaseRuleSetSource(kind, "cn", path)
		if err != nil {
			t.Fatal(err)
		}
		jsonPath := filepath.Join(outputDir, "native-"+kind+"-cn.json")
		if err := os.WriteFile(jsonPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.CommandContext(ctx, core, "rule-set", "compile", "--output", jsonPath+".srs", jsonPath).CombinedOutput(); err != nil {
			t.Fatalf("native %s compile: %s: %v", kind, output, err)
		}
		t.Logf("native %s: %d categories, CN conversion and core compilation passed", kind, len(codes))
	}
	for kind, samples := range map[string][]string{
		"geoip":   {"cn", "private", "telegram"},
		"geosite": {"cn", "geolocation-cn", "category-ads-all", "google", "google@cn", "apple"},
	} {
		path := filepath.Join(dir, kind+".dat")
		codes, err := geoAssetCodes(kind, path)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %d categories including attribute subsets", kind, len(codes))
		for _, code := range codes {
			if !isGeneratedGeoRuleSetTag(generatedGeoRuleSetTag(kind, code)) {
				t.Errorf("unsupported upstream category: %s/%s", kind, code)
			}
		}
		for _, code := range samples {
			tag := generatedGeoRuleSetTag(kind, code)
			data, err := geoDatabaseRuleSetSource(kind, code, path)
			if err != nil {
				t.Fatalf("%s conversion: %v", tag, err)
			}
			jsonPath := filepath.Join(outputDir, tag+".json")
			if err := os.WriteFile(jsonPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			if output, err := exec.CommandContext(ctx, core, "rule-set", "compile", "--output", filepath.Join(outputDir, tag+".srs"), jsonPath).CombinedOutput(); err != nil {
				t.Fatalf("%s compile: %s: %v", tag, output, err)
			}
			ruleSets = append(ruleSets, map[string]any{"type": "local", "tag": tag, "format": "source", "path": jsonPath})
			routeRules = append(routeRules, map[string]any{"rule_set": []string{tag}, "action": "route", "outbound": "direct"})
			t.Logf("%s: converted %d bytes, core compilation passed", tag, len(data))
		}
	}
	config, err := json.Marshal(map[string]any{
		"outbounds": []map[string]any{{"type": "direct", "tag": "direct"}},
		"route":     map[string]any{"rule_set": ruleSets, "rules": routeRules},
	})
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(outputDir, "config.json")
	if err := os.WriteFile(configPath, config, 0600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.CommandContext(ctx, core, "check", "-c", configPath).CombinedOutput(); err != nil {
		t.Fatalf("core source rule-set check: %s: %v", output, err)
	}
}
