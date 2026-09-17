package service

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeoFallbackSyncPreservesWorkingDataWhenCategoryDisappears(t *testing.T) {
	for _, fallbackAvailable := range []bool{true, false} {
		t.Run(map[bool]string{true: "fallback", false: "missing_both"}[fallbackAvailable], func(t *testing.T) {
			svc, db := geoSourceTestService(t)
			selectTestGeoSource(t, svc, db)
			if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
				Name: "Google", Enabled: true, RuleType: "geosite", Values: []string{"google"}, Outbound: "direct",
			}); err != nil {
				t.Fatal(err)
			}
			before, err := svc.selectedGeoAsset("geosite")
			if err != nil {
				t.Fatal(err)
			}
			withoutGoogle := bytes.ReplaceAll(geoTestSiteDAT(), []byte("GOOGLE"), []byte("CHANGE"))
			mirror := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/" + model.GeoSourceAssetURL(model.GeoSourceLoyalsoldier, "geosite"):
					_, _ = w.Write(withoutGoogle)
				case "/" + model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geosite"):
					if fallbackAvailable {
						_, _ = w.Write(geoTestSiteDAT())
					} else {
						_, _ = w.Write(withoutGoogle)
					}
				default:
					t.Errorf("unexpected download: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer mirror.Close()
			if err := db.SetUpdateSettings(&model.UpdateSettings{Acceleration: "custom", CustomMirrorURL: mirror.URL}); err != nil {
				t.Fatal(err)
			}
			svc.runGeoAssetSync(before.ID)
			after, err := db.GetGeoAsset(before.ID)
			if err != nil {
				t.Fatal(err)
			}
			if fallbackAvailable {
				if after.SyncStatus != "updated" || after.LocalPath == before.LocalPath {
					t.Fatalf("valid fallback did not allow update: %+v", after)
				}
				result, err := svc.GeoDomains("google", 100, 0)
				if err != nil || result.Total == 0 || !strings.Contains(result.Message, "SagerNet") {
					t.Fatalf("updated category did not use fallback: %+v %v", result, err)
				}
			} else {
				if after.SyncStatus != "failed" || after.LocalPath != before.LocalPath || !strings.Contains(after.SyncError, "geosite-google") || !strings.Contains(after.SyncError, "均不存在") {
					t.Fatalf("invalid update replaced usable data or lost its error: %+v", after)
				}
			}
			data, _, err := svc.GeneratedGeoRuleSetContentForSource(context.Background(), "geosite-google", model.GeoSourceLoyalsoldier, "")
			if err != nil || !bytes.Contains(data, []byte("suffix.example")) {
				t.Fatalf("working core content was lost: %s %v", data, err)
			}
		})
	}
}
