package service

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func cacheTestGeoIP(t *testing.T, svc *RouteRuleService, octet byte) model.GeoAsset {
	t.Helper()
	body := bytes.ReplaceAll(geoTestIPDAT(), []byte{192, 0, 2, 0}, []byte{192, 0, octet, 0})
	asset := model.GeoAsset{Type: "geoip", Source: model.GeoSourceLoyalsoldier}
	path, err := svc.cacheGeoDatabase(&asset, body)
	if err != nil {
		t.Fatal(err)
	}
	asset.LocalPath = path
	// Stable ordering without sleeps, including on filesystems with coarser clocks.
	stamp := time.Unix(1_700_000_000+int64(octet), 0)
	if err := os.Chtimes(path, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	return asset
}

func writeGeoCacheMarker(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"version":3,"rules":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestGeoDatabaseCacheRetainsTwoSnapshotsAndActivePath(t *testing.T) {
	svc, db := geoSourceTestService(t)
	first := cacheTestGeoIP(t, svc, 10)
	assets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, asset := range assets {
		if asset.Type == "geoip" {
			if _, err := db.UpdateGeoAssetSyncResult(asset.ID, first.LocalPath); err != nil {
				t.Fatal(err)
			}
		}
	}
	cacheMarker := func(asset model.GeoAsset) string {
		return filepath.Join(svc.paths.RulesDir, "geo", asset.Source, geoAssetVersion(asset), "geoip-cn.json")
	}
	writeGeoCacheMarker(t, cacheMarker(first))
	second := cacheTestGeoIP(t, svc, 11)
	writeGeoCacheMarker(t, cacheMarker(second))
	third := cacheTestGeoIP(t, svc, 12)
	writeGeoCacheMarker(t, cacheMarker(third))
	unknown := []string{
		filepath.Join(svc.paths.GeoDir, "loyalsoldier", "geoip-custom.dat"),
		filepath.Join(svc.paths.GeoDir, "loyalsoldier", "geoip-"+strings.Repeat("a", 64)+".db"),
		filepath.Join(svc.paths.GeoDir, "loyalsoldier", "geoip-"+strings.Repeat("B", 64)+".dat"),
		filepath.Join(svc.paths.GeoDir, "sagernet", "geoip-"+strings.Repeat("c", 64)+".db"),
		filepath.Join(svc.paths.RulesDir, "geo", "loyalsoldier", "user-notes", "note.json"),
	}
	for _, path := range unknown {
		writeGeoCacheMarker(t, path)
	}
	site := model.GeoAsset{Source: model.GeoSourceLoyalsoldier, Type: "geosite"}
	site.LocalPath, err = svc.cacheGeoDatabase(&site, geoTestSiteDAT())
	if err != nil {
		t.Fatal(err)
	}
	fourth := cacheTestGeoIP(t, svc, 13)
	for _, path := range []string{first.LocalPath, third.LocalPath, fourth.LocalPath, site.LocalPath, cacheMarker(first), cacheMarker(third)} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("retained cache %s missing: %v", filepath.Base(path), err)
		}
	}
	for _, path := range []string{second.LocalPath, filepath.Dir(cacheMarker(second))} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("expired cache was not removed: %s: %v", path, err)
		}
	}
	for _, path := range unknown {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("unowned cache was changed: %s: %v", path, err)
		}
	}
	snapshots, err := svc.geoDatabaseSnapshots("loyalsoldier", "geoip")
	if err != nil || len(snapshots) != 3 {
		t.Fatalf("expected latest two plus active snapshot: %d, %v", len(snapshots), err)
	}
}

func TestGeoDatabaseOldURLFallsBackWithinSameProvider(t *testing.T) {
	svc, _ := geoSourceTestService(t) // The selected provider remains SagerNet.
	first := cacheTestGeoIP(t, svc, 20)
	_ = cacheTestGeoIP(t, svc, 21)
	latest := cacheTestGeoIP(t, svc, 22)
	if _, err := os.Stat(first.LocalPath); !os.IsNotExist(err) {
		t.Fatalf("old snapshot was not pruned: %v", err)
	}
	// A legacy SRS exists, but must never satisfy a Loyalsoldier-pinned URL.
	writeGeoCacheMarker(t, filepath.Join(svc.paths.RulesDir, "geo", "geoip-cn.srs"))
	data, contentType, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", geoAssetVersion(first))
	if err != nil || contentType != "application/json; charset=utf-8" {
		t.Fatalf("old URL fallback: %s, %v", contentType, err)
	}
	if !json.Valid(data) || !bytes.Contains(data, []byte("192.0.22.0/24")) || bytes.Contains(data, []byte("192.0.20.0/24")) {
		t.Fatalf("did not use latest same-provider database: %s", data)
	}
	cachePath := filepath.Join(svc.paths.RulesDir, "geo", "loyalsoldier", geoAssetVersion(latest), "geoip-cn.json")
	if _, err := os.Stat(cachePath); err != nil {
		t.Fatalf("fallback content did not use retained version's cache key: %v", err)
	}
	if _, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-cn", "loyalsoldier", geoAssetVersion(first)); err == nil {
		t.Fatal("missing same-provider/type snapshot fell back to another database")
	}
}

func TestGeoDatabaseOldURLSkipsUnreadableLatestSnapshot(t *testing.T) {
	svc, _ := geoSourceTestService(t)
	first := cacheTestGeoIP(t, svc, 30)
	valid := cacheTestGeoIP(t, svc, 31)
	broken := cacheTestGeoIP(t, svc, 32)
	if err := os.WriteFile(broken.LocalPath, []byte("corrupt database"), 0600); err != nil {
		t.Fatal(err)
	}
	data, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", geoAssetVersion(first))
	if err != nil || !bytes.Contains(data, []byte("192.0.31.0/24")) {
		t.Fatalf("did not skip unreadable newest snapshot: %s, %v", data, err)
	}
	if _, err := os.Stat(valid.LocalPath); err != nil {
		t.Fatal(err)
	}
}

func TestGeoDatabaseConversionAndCleanupShareLock(t *testing.T) {
	svc, _ := geoSourceTestService(t)
	first := cacheTestGeoIP(t, svc, 40)
	_ = cacheTestGeoIP(t, svc, 41)
	unlock := svc.lockGeneratedGeoRuleSet(geoDatabaseCacheLockKey("loyalsoldier", "geoip"))
	var unlockOnce sync.Once
	release := func() { unlockOnce.Do(unlock) }
	defer release()
	started := make(chan struct{}, 2)
	results := make(chan error, 2)
	go func() {
		started <- struct{}{}
		_, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geoip-cn", "loyalsoldier", geoAssetVersion(first))
		results <- err
	}()
	go func() {
		started <- struct{}{}
		asset := model.GeoAsset{Source: model.GeoSourceLoyalsoldier, Type: "geoip"}
		body := bytes.ReplaceAll(geoTestIPDAT(), []byte{192, 0, 2, 0}, []byte{192, 0, 42, 0})
		_, err := svc.cacheGeoDatabase(&asset, body)
		results <- err
	}()
	<-started
	<-started
	select {
	case err := <-results:
		t.Fatalf("cache operation bypassed source/type lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Errorf("concurrent conversion/cleanup failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("cache locks deadlocked")
		}
	}
}

func TestGeoSourceValidationHonorsEnabledSubscriptionTag(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(map[bool]string{true: "enabled", false: "disabled"}[enabled], func(t *testing.T) {
			svc, db := geoSourceTestService(t)
			geoSourceTestMirror(t, db)
			if _, err := db.CreateRouteRule(&model.RouteRuleRequest{Name: "Custom", Enabled: true, RuleType: "geoip", Values: []string{"external"}, Outbound: "direct"}); err != nil {
				t.Fatal(err)
			}
			if _, err := db.CreateRouteRuleSubscription(&model.RouteRuleSubscriptionRequest{Name: "Independent", Enabled: enabled, Tag: "geoip-external", URL: "https://example.invalid/rules.srs", Format: "binary"}); err != nil {
				t.Fatal(err)
			}
			asset := cacheTestGeoIP(t, svc, 50)
			site := model.GeoAsset{Source: model.GeoSourceLoyalsoldier, Type: "geosite"}
			var err error
			site.LocalPath, err = svc.cacheGeoDatabase(&site, geoTestSiteDAT())
			if err != nil {
				t.Fatal(err)
			}
			err = svc.validateGeoSourceCategories([]model.GeoAsset{asset, site})
			if enabled && err != nil {
				t.Fatalf("enabled independent subscription was rejected: %v", err)
			}
			if !enabled && (err == nil || !strings.Contains(err.Error(), "geoip-external")) {
				t.Fatalf("disabled subscription incorrectly supplied a category: %v", err)
			}
		})
	}
}

func TestGeoCachePathRejectsDirectoryEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := geoCachePathWithin(root, outside); err == nil {
		t.Fatal("outside directory accepted")
	}
	if err := geoCachePathWithin(root, root); err == nil {
		t.Fatal("cache root itself accepted for deletion")
	}
	child := filepath.Join(root, "child")
	if err := os.Mkdir(child, 0755); err != nil {
		t.Fatal(err)
	}
	if err := geoCachePathWithin(root, child); err != nil {
		t.Fatalf("ordinary child rejected: %v", err)
	}
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if err := geoCachePathWithin(root, link); err == nil {
		t.Fatal("symlink escape accepted")
	}
}
