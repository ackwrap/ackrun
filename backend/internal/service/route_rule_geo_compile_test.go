package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeoSiteBinaryCompilesOnceWithoutJSONCache(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	binary := testBinaryRuleSet(t, 2)
	compiles := 0
	svc.ruleSetCompiler = func(_ context.Context, source []byte, outputPath string) error {
		compiles++
		var doc struct {
			Rules []map[string]json.RawMessage `json:"rules"`
		}
		if err := json.Unmarshal(source, &doc); err != nil || len(doc.Rules) != 1 || len(doc.Rules[0]) != 4 {
			t.Fatalf("domain matching types lost before compilation: %s", source)
		}
		return os.WriteFile(outputPath, binary, 0600)
	}
	for range 2 {
		data, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-google", model.GeoSourceLoyalsoldier, "", "binary")
		if err != nil || contentType != "application/octet-stream" || !bytes.Equal(data, binary) {
			t.Fatalf("binary endpoint: %s, %v", contentType, err)
		}
	}
	if compiles != 1 {
		t.Fatalf("cached category compiled %d times", compiles)
	}
	if err := filepath.WalkDir(svc.paths.RulesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && (filepath.Ext(path) != ".srs" || strings.HasPrefix(entry.Name(), ".")) {
			t.Errorf("unexpected persistent intermediate: %s", entry.Name())
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Old source-format configurations retain their JSON response contract.
	legacy, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-google", model.GeoSourceLoyalsoldier, "")
	if err != nil || !json.Valid(legacy) || !strings.HasPrefix(contentType, "application/json") {
		t.Fatalf("old configuration became incompatible: %s %v", contentType, err)
	}
}

func TestGeoSiteBinaryFailureLeavesPreviousVersionUsable(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	assets, _ := db.ListGeoAssets()
	var asset model.GeoAsset
	for _, item := range assets {
		if item.Type == "geosite" {
			asset = item
		}
	}
	binary := testBinaryRuleSet(t, 2)
	svc.ruleSetCompiler = func(_ context.Context, _ []byte, path string) error { return os.WriteFile(path, binary, 0600) }
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-google", model.GeoSourceLoyalsoldier, "", "binary"); err != nil {
		t.Fatal(err)
	}
	oldCache := filepath.Join(svc.paths.RulesDir, "geo", model.GeoSourceLoyalsoldier, geoAssetVersion(asset), "geosite-google.srs")
	newPath, err := svc.cacheGeoDatabase(&asset, bytes.ReplaceAll(geoTestSiteDAT(), []byte("exact.example"), []byte("other.example")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateGeoAssetSyncResult(asset.ID, newPath); err != nil {
		t.Fatal(err)
	}
	svc.ruleSetCompiler = func(_ context.Context, _ []byte, path string) error {
		if err := os.WriteFile(path, []byte("partial"), 0600); err != nil {
			return err
		}
		return fmt.Errorf("synthetic compiler failure")
	}
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-google", model.GeoSourceLoyalsoldier, "", "binary"); err == nil {
		t.Fatal("compiler failure was hidden")
	}
	if data, err := os.ReadFile(oldCache); err != nil || !bytes.Equal(data, binary) {
		t.Fatalf("failed refresh damaged old cache: %v", err)
	}
	asset.LocalPath = newPath
	entries, err := os.ReadDir(filepath.Join(svc.paths.RulesDir, "geo", model.GeoSourceLoyalsoldier, geoAssetVersion(asset)))
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed compilation left output files: %v %v", entries, err)
	}
}

func TestGeoSiteBinaryRejectsInvalidCompilerOutput(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	svc.ruleSetCompiler = func(_ context.Context, _ []byte, path string) error {
		return os.WriteFile(path, []byte("invalid binary"), 0600)
	}
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-cn", model.GeoSourceLoyalsoldier, "", "binary"); err == nil {
		t.Fatal("invalid compiler output accepted")
	}
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-cn", model.GeoSourceLoyalsoldier, "", "unknown"); err == nil {
		t.Fatal("invalid format accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(cancelled, "geosite-cn", model.GeoSourceLoyalsoldier, "", "binary"); err == nil {
		t.Fatal("cancelled request succeeded")
	}
}
