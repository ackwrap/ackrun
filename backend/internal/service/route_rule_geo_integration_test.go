package service

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

// Opt-in compatibility check against public release files and a real core:
// ACKWRAP_TEST_GEO_DIR contains geoip.dat, geosite.dat, and the official
// upstream-geoip-{cn,private,telegram}.srs samples;
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
	svc, db := geoSourceTestService(t)
	svc.paths.BinaryPath = core
	svc.ruleSetValidator = nil
	assets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	for i := range assets {
		asset := &assets[i]
		asset.Source = model.GeoSourceLoyalsoldier
		asset.URL = model.GeoSourceAssetURL(asset.Source, asset.Type)
		data, err := os.ReadFile(filepath.Join(dir, asset.Type+".dat"))
		if err != nil {
			t.Fatal(err)
		}
		asset.LocalPath, err = svc.cacheGeoDatabase(asset, data)
		if err != nil {
			t.Fatal(err)
		}
	}
	settings, _ := db.GetGeneralSettings()
	settings.GeoSource = model.GeoSourceLoyalsoldier
	if err := db.SetGeneralSettingsWithGeoAssets(settings, assets); err != nil {
		t.Fatal(err)
	}
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, code := range []string{"cn", "private", "telegram"} {
			if r.URL.Path == "/https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/"+code+".srs" {
				data, err := os.ReadFile(filepath.Join(dir, "upstream-geoip-"+code+".srs"))
				if err != nil {
					t.Errorf("read native SRS fixture: %v", err)
					http.Error(w, "missing native fixture", 500)
					return
				}
				_, _ = w.Write(data)
				return
			}
		}
		t.Errorf("unexpected upstream: %s", r.URL.Path)
		http.NotFound(w, r)
	}))
	defer mirror.Close()
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
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
			binary, contentType, err := svc.GeneratedGeoRuleSetContentForSource(ctx, tag, model.GeoSourceLoyalsoldier, "", "binary")
			if err != nil || contentType != "application/octet-stream" {
				t.Fatalf("%s binary endpoint: %s %v", tag, contentType, err)
			}
			binaryPath := filepath.Join(outputDir, tag+".srs")
			if err := os.WriteFile(binaryPath, binary, 0600); err != nil {
				t.Fatal(err)
			}
			ruleSets = append(ruleSets, map[string]any{"type": "local", "tag": tag, "format": "binary", "path": binaryPath})
			routeRules = append(routeRules, map[string]any{"rule_set": []string{tag}, "action": "route", "outbound": "direct"})
			t.Logf("%s: JSON %d bytes -> SRS %d bytes", tag, len(data), len(binary))
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
		t.Fatalf("core binary rule-set check: %s: %v", output, err)
	}
}
