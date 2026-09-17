package service

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const testLoyalsoldierCNRuleSetURL = "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/cn.srs"

func TestLoyalsoldierNativeGeoIPPreservesBytesAndIsolatesCache(t *testing.T) {
	svc, db := geoSourceTestService(t)
	sagerNetPayload, nativePayload := testBinaryRuleSet(t, 1), testBinaryRuleSet(t, 2)
	legacyDir := filepath.Join(svc.paths.RulesDir, "geo")
	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	legacyPath := filepath.Join(legacyDir, "geoip-cn.srs")
	if err := os.WriteFile(legacyPath, sagerNetPayload, 0644); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/"+testLoyalsoldierCNRuleSetURL {
			t.Errorf("unexpected native download path: %s", r.URL.Path)
		}
		_, _ = w.Write(nativePayload)
	}))
	t.Cleanup(mirror.Close)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		data, contentType, err := svc.loyalsoldierGeoIPRuleSetContent(context.Background(), " GEOIP-CN ")
		if err != nil || !bytes.Equal(data, nativePayload) || contentType != "application/octet-stream" {
			t.Fatalf("native bytes changed: contentType=%s data=%x err=%v", contentType, data, err)
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("native requests=%d, want fresh cache reuse", requests.Load())
	}
	nativePath := filepath.Join(legacyDir, "loyalsoldier", "native", "geoip-cn.srs")
	for path, expected := range map[string][]byte{legacyPath: sagerNetPayload, nativePath: nativePayload} {
		data, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(data, expected) {
			t.Fatalf("source cache was mixed: path=%s err=%v", path, err)
		}
	}
	for _, tag := range []string{"geosite-cn", "geoip-", "geoip-../cn", "geoip-cn/../../other", "geoip-cn@cn", "geoip-cn?url=other", "cn"} {
		if _, _, err := svc.loyalsoldierGeoIPRuleSetContent(context.Background(), tag); err == nil {
			t.Fatalf("invalid native tag accepted: %q", tag)
		}
	}
	if requests.Load() != 1 {
		t.Fatal("invalid tag started an upstream request")
	}
}

func TestLoyalsoldierNativeGeoIPUsesIndependentCacheLock(t *testing.T) {
	svc, db := geoSourceTestService(t)
	sagerNetPayload, nativePayload := testBinaryRuleSet(t, 1), testBinaryRuleSet(t, 2)
	sagerNetStarted, releaseSagerNet := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSagerNet) }) }
	mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch strings.TrimPrefix(r.URL.Path, "/") {
		case generatedGeoRuleSetURL("geoip-cn"):
			close(sagerNetStarted)
			select {
			case <-releaseSagerNet:
				_, _ = w.Write(sagerNetPayload)
			case <-r.Context().Done():
			}
		case testLoyalsoldierCNRuleSetURL:
			_, _ = w.Write(nativePayload)
		default:
			t.Errorf("unexpected download path: %s", r.URL.Path)
			_, _ = w.Write([]byte("invalid"))
		}
	}))
	t.Cleanup(mirror.Close)
	t.Cleanup(release)
	if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
		t.Fatal(err)
	}
	sagerNetDone := make(chan error, 1)
	go func() {
		data, _, err := svc.GeneratedGeoRuleSetContentContext(context.Background(), "geoip-cn")
		if err == nil && !bytes.Equal(data, sagerNetPayload) {
			err = fmt.Errorf("SagerNet response used another source's data")
		}
		sagerNetDone <- err
	}()
	select {
	case <-sagerNetStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("SagerNet download did not start")
	}
	nativeDone := make(chan error, 1)
	go func() {
		data, _, err := svc.loyalsoldierGeoIPRuleSetContent(context.Background(), "geoip-cn")
		if err == nil && !bytes.Equal(data, nativePayload) {
			err = fmt.Errorf("native response used another source's data")
		}
		nativeDone <- err
	}()
	select {
	case err := <-nativeDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("native download blocked on SagerNet's cache lock")
	}
	release()
	select {
	case err := <-sagerNetDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SagerNet download did not finish")
	}
}

func TestLoyalsoldierNativeGeoIPKeepsSameSourceCacheOnRefreshFailure(t *testing.T) {
	for _, failure := range []string{"invalid-response", "download-failure"} {
		t.Run(failure, func(t *testing.T) {
			svc, db := geoSourceTestService(t)
			payload := testBinaryRuleSet(t, 2)
			var refreshing atomic.Bool
			refreshCtx, cancelRefresh := context.WithCancel(context.Background())
			defer cancelRefresh()
			mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/"+testLoyalsoldierCNRuleSetURL {
					t.Errorf("unexpected download path: %s", r.URL.Path)
				}
				if refreshing.Load() {
					if failure == "download-failure" {
						// End the shared attempt budget here so the test never
						// proceeds to a public fallback mirror.
						cancelRefresh()
						w.WriteHeader(http.StatusBadGateway)
					} else {
						_, _ = w.Write([]byte("<html>invalid upstream response</html>"))
					}
					return
				}
				_, _ = w.Write(payload)
			}))
			t.Cleanup(mirror.Close)
			if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
				t.Fatal(err)
			}
			if _, _, err := svc.loyalsoldierGeoIPRuleSetContent(context.Background(), "geoip-cn"); err != nil {
				t.Fatal(err)
			}
			cachePath := filepath.Join(svc.paths.RulesDir, "geo", "loyalsoldier", "native", "geoip-cn.srs")
			expired := time.Now().Add(-generatedGeoRuleSetUpdateInterval - time.Minute)
			if err := os.Chtimes(cachePath, expired, expired); err != nil {
				t.Fatal(err)
			}
			refreshing.Store(true)
			data, contentType, err := svc.loyalsoldierGeoIPRuleSetContent(refreshCtx, "geoip-cn")
			if err != nil || !bytes.Equal(data, payload) || contentType != "application/octet-stream" {
				t.Fatalf("valid native cache was lost: contentType=%s data=%x err=%v", contentType, data, err)
			}
			cached, err := os.ReadFile(cachePath)
			if err != nil || !bytes.Equal(cached, payload) {
				t.Fatalf("failed refresh overwrote cache: %v", err)
			}
		})
	}
}
