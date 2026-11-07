package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeoReadCacheReusesAndInvalidatesFileIdentity(t *testing.T) {
	var cache geoDatabaseReadCache
	path := filepath.Join(t.TempDir(), "geo.dat")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	key := geoReadCacheKey{path: path, kind: "geoip"}
	loads := 0
	load := func() (geoReadCacheValue, error) {
		loads++
		data, err := os.ReadFile(path)
		return geoReadCacheValue{codes: []string{string(data)}}, err
	}
	for range 3 {
		value, err := cache.load(key, load)
		if err != nil || value.codes[0] != "old" {
			t.Fatalf("cached read: %+v %v", value, err)
		}
	}
	if loads != 1 {
		t.Fatalf("warm reads parsed %d times", loads)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Atomic replacement must invalidate even when both length and mtime match.
	replacement := path + ".next"
	if err := os.WriteFile(replacement, []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplaceFile(replacement, path); err != nil {
		t.Fatal(err)
	}
	value, err := cache.load(key, load)
	if err != nil || value.codes[0] != "new" || loads != 2 {
		t.Fatalf("stale replacement: %+v %v loads=%d", value, err, loads)
	}
	if err := os.WriteFile(path, []byte("changed in place"), 0600); err != nil {
		t.Fatal(err)
	}
	value, err = cache.load(key, load)
	if err != nil || value.codes[0] != "changed in place" || loads != 3 {
		t.Fatalf("stale edit: %+v %v loads=%d", value, err, loads)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := cache.load(key, load); !os.IsNotExist(err) {
		t.Fatalf("deleted database returned from cache: %v", err)
	}
}

func TestGeoReadCacheBoundsAndDoesNotCacheFailures(t *testing.T) {
	var cache geoDatabaseReadCache
	path := filepath.Join(t.TempDir(), "geo.dat")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	key := geoReadCacheKey{path: path, kind: "geosite"}
	failed := errors.New("invalid database")
	if _, err := cache.load(key, func() (geoReadCacheValue, error) { return geoReadCacheValue{}, failed }); !errors.Is(err, failed) {
		t.Fatal(err)
	}
	loads := 0
	load := func() (geoReadCacheValue, error) { loads++; return geoReadCacheValue{codes: []string{"cn"}}, nil }
	if _, err := cache.load(key, load); err != nil {
		t.Fatal(err)
	}
	if loads != 1 {
		t.Fatal("failed load poisoned cache")
	}
	for i := range geoReadCacheMaxEntries + 1 {
		key.code = fmt.Sprintf("category-%d", i)
		if _, err := cache.load(key, load); err != nil {
			t.Fatal(err)
		}
	}
	if len(cache.entries) > geoReadCacheMaxEntries || cache.bytes > geoReadCacheMaxBytes {
		t.Fatal("cache exceeded bounds")
	}
	key.code = "oversized"
	for range 2 {
		if _, err := cache.load(key, func() (geoReadCacheValue, error) {
			loads++
			return geoReadCacheValue{codes: []string{strings.Repeat("x", geoReadCacheMaxBytes)}}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, retained := cache.entries[key]; retained {
		t.Fatal("oversized value retained")
	}
}

func TestGeoReadCacheColdParseDoesNotBlockWarmCategory(t *testing.T) {
	var cache geoDatabaseReadCache
	path := filepath.Join(t.TempDir(), "geo.dat")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	warm := geoReadCacheKey{path: path, kind: "geosite", code: "cn"}
	load := func() (geoReadCacheValue, error) { return geoReadCacheValue{codes: []string{"cn"}}, nil }
	if _, err := cache.load(warm, load); err != nil {
		t.Fatal(err)
	}
	started, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(unblock) }) }
	defer release()
	coldDone := make(chan error, 1)
	go func() {
		_, err := cache.load(geoReadCacheKey{path: path, kind: "geosite", code: "google"}, func() (geoReadCacheValue, error) {
			close(started)
			<-unblock
			return load()
		})
		coldDone <- err
	}()
	<-started
	warmDone := make(chan error, 1)
	go func() { _, err := cache.load(warm, load); warmDone <- err }()
	select {
	case err := <-warmDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cold parse blocked warm category")
	}
	release()
	if err := <-coldDone; err != nil {
		t.Fatal(err)
	}
}

func TestGeoDomainsCachePreservesPaginationAndRefresh(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	first, err := svc.GeoDomains("cn", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.GeoDomains("cn", 2, 2)
	if err != nil || first.Total != 4 || second.Total != 4 || len(second.Items) != 2 || first.Items[0].Value != "exact.example" {
		t.Fatalf("pagination: %+v %+v %v", first, second, err)
	}
	asset, err := svc.selectedGeoAsset("geosite")
	if err != nil {
		t.Fatal(err)
	}
	codes, err := cachedGeoAssetCodes("geosite", asset.LocalPath)
	if err != nil {
		t.Fatal(err)
	}
	codes[0] = "mutated-by-caller"
	codes, err = cachedGeoAssetCodes("geosite", asset.LocalPath)
	if err != nil || codes[0] == "mutated-by-caller" {
		t.Fatal("caller mutated cached index")
	}
	path, err := svc.cacheGeoDatabase(&asset, bytes.ReplaceAll(geoTestSiteDAT(), []byte("exact.example"), []byte("other.example")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.UpdateGeoAssetSyncResult(asset.ID, path); err != nil {
		t.Fatal(err)
	}
	updated, err := svc.GeoDomains("cn", 2, 0)
	if err != nil || updated.Items[0].Value != "other.example" {
		t.Fatalf("stale category after sync: %+v %v", updated, err)
	}
}

func TestGeoValidationCacheTracksBytesAndCoreReplacement(t *testing.T) {
	svc, _ := geoSourceTestService(t)
	dir := t.TempDir()
	core := filepath.Join(dir, "core.exe")
	if err := os.WriteFile(core, []byte("version1"), 0755); err != nil {
		t.Fatal(err)
	}
	svc.paths.BinaryPath = core
	validations := 0
	fail := false
	svc.ruleSetValidator = func(context.Context, string) error {
		validations++
		if fail {
			return errors.New("core rejected file")
		}
		return nil
	}
	path := filepath.Join(dir, "test.srs")
	check := func(data []byte) error {
		t.Helper()
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return svc.validateCachedGeoRuleSet(context.Background(), data, path)
	}
	data := testBinaryRuleSet(t, 1)
	for range 3 {
		if err := check(data); err != nil {
			t.Fatal(err)
		}
	}
	if validations != 1 {
		t.Fatalf("warm validations=%d", validations)
	}
	data = testBinaryRuleSet(t, 2)
	if err := check(data); err != nil {
		t.Fatal(err)
	}
	if validations != 2 {
		t.Fatal("new content was not validated")
	}
	info, err := os.Stat(core)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(core+".next", []byte("version2"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(core+".next", info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := atomicReplaceFile(core+".next", core); err != nil {
		t.Fatal(err)
	}
	if err := check(data); err != nil {
		t.Fatal(err)
	}
	if validations != 3 {
		t.Fatal("replacement core reused prior validation")
	}
	corrupt := bytes.Clone(data)
	corrupt[len(corrupt)-1] ^= 1
	if err := check(corrupt); err == nil {
		t.Fatal("same-size corrupted data accepted")
	}
	fail = true
	for range 2 {
		if err := check(testBinaryRuleSet(t, 3)); err == nil {
			t.Fatal("rejection cached as success")
		}
	}
	if validations != 5 {
		t.Fatal("failed validation was cached")
	}
	fail = false
	if err := check(testBinaryRuleSet(t, 3)); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := svc.validateCachedGeoRuleSet(ctx, data, path); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled cache hit: %v", err)
	}
	if err := os.Remove(core); err != nil {
		t.Fatal(err)
	}
	if err := check(data); err == nil {
		t.Fatal("missing core reused prior validation")
	}
}

func TestGeoDifferentCategoriesCompileConcurrentlyAndProtectSnapshot(t *testing.T) {
	svc, db := geoSourceTestService(t)
	selectTestGeoSource(t, svc, db)
	started, unblock := make(chan struct{}), make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(unblock) }) }
	defer release()
	var cnCompiles atomic.Int32
	binary := testBinaryRuleSet(t, 2)
	svc.ruleSetCompiler = func(_ context.Context, _ []byte, path string) error {
		if strings.Contains(filepath.Base(path), ".geosite-cn-") {
			if cnCompiles.Add(1) == 1 {
				close(started)
			}
			<-unblock
		}
		return os.WriteFile(path, binary, 0600)
	}
	request := func(tag string, done chan<- error) {
		_, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), tag, model.GeoSourceLoyalsoldier, "", "binary")
		done <- err
	}
	cnDone := make(chan error, 3)
	go request("geosite-cn", cnDone)
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("compile did not start")
	}
	go request("geosite-cn", cnDone)
	go request("geosite-cn", cnDone)
	googleDone := make(chan error, 1)
	go request("geosite-google", googleDone)
	select {
	case err := <-googleDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("unrelated category blocked by compile")
	}
	publicationDone := make(chan error, 1)
	go func() {
		_, err := svc.cacheGeoDatabase(&model.GeoAsset{Source: model.GeoSourceLoyalsoldier, Type: "geosite"}, geoTestSiteDAT())
		publicationDone <- err
	}()
	select {
	case err := <-publicationDone:
		t.Fatalf("publication bypassed snapshot read lock: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	release()
	for range 3 {
		select {
		case err := <-cnDone:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("category deadlock")
		}
	}
	select {
	case err := <-publicationDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("publication deadlock")
	}
	if cnCompiles.Load() != 1 {
		t.Fatalf("same category compiled %d times", cnCompiles.Load())
	}
}
