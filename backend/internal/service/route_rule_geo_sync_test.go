package service

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeoAssetURLChangeDuringDownloadStartsFreshSync(t *testing.T) {
	svc, db := geoSourceTestService(t)
	assets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	asset := assets[0]
	oldBody := geoTestIPDAT()
	newBody := bytes.Replace(oldBody, []byte("CN"), []byte("US"), 1)
	oldStarted := make(chan struct{})
	newStarted := make(chan struct{})
	releaseOld := make(chan struct{})
	var oldStartOnce, newStartOnce, releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseOld) }) }
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/old.db":
			oldStartOnce.Do(func() { close(oldStarted) })
			select {
			case <-releaseOld:
				_, _ = w.Write(oldBody)
			case <-r.Context().Done():
			}
		case "/new.db":
			newStartOnce.Do(func() { close(newStarted) })
			_, _ = w.Write(newBody)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(release)
	if _, err := db.UpdateGeoAsset(asset.ID, &model.GeoAssetRequest{URL: server.URL + "/old.db", SyncMode: "off"}); err != nil {
		t.Fatal(err)
	}
	oldDone := make(chan struct{})
	go func() {
		defer close(oldDone)
		svc.runGeoAssetSync(asset.ID)
	}()
	wait := func(done <-chan struct{}, what string) {
		t.Helper()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for %s", what)
		}
	}
	wait(oldStarted, "old download")
	if _, err := svc.UpdateGeoAsset(asset.ID, &model.GeoAssetRequest{URL: server.URL + "/new.db", SyncMode: "off"}); err != nil {
		t.Fatal(err)
	}
	release()
	wait(oldDone, "obsolete download to finish")
	wait(newStarted, "fresh download after URL change")
	expectedName := fmt.Sprintf("geoip-%x.db", sha256.Sum256(newBody))
	deadline := time.Now().Add(5 * time.Second)
	for {
		current, err := db.GetGeoAsset(asset.ID)
		if err != nil {
			t.Fatal(err)
		}
		if current.SyncStatus == "updated" {
			if current.URL != server.URL+"/new.db" || filepath.Base(current.LocalPath) != expectedName || !current.Available {
				t.Fatalf("sync published obsolete data: %+v", current)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("fresh download did not finish: %+v", current)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestGeoAssetScheduleFromOldPagePreservesSwitchedSourceURL(t *testing.T) {
	svc, db := geoSourceTestService(t)
	original, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	// The scheduling dialog was opened before the source changed. It carries
	// only scheduling/proxy settings, without the old source's download URL.
	staleDraft := &model.GeoAssetRequest{
		UseProxy: original[0].UseProxy, SyncMode: "weekly", SyncTime: "08:15:00", SyncWeekday: 2,
	}
	selectTestGeoSource(t, svc, db)
	before, err := db.GetGeoAsset(original[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.UpdateGeoAsset(original[0].ID, staleDraft)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Source != model.GeoSourceLoyalsoldier || updated.URL != model.GeoSourceAssetURL(model.GeoSourceLoyalsoldier, updated.Type) || updated.LocalPath != before.LocalPath || updated.SyncStatus != "updated" {
		t.Fatalf("old schedule draft replaced the selected source: %+v", updated)
	}
	if updated.SyncMode != "weekly" || updated.SyncTime != "08:15:00" || updated.SyncWeekday != 2 {
		t.Fatalf("schedule changes were lost: %+v", updated)
	}
}
