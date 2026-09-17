package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func geoSourceTestStore(t *testing.T) (*Store, *model.GeneralSettings, []model.GeoAsset) {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	settings, err := s.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	assets, err := s.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	return s, settings, assets
}

func preparedGeoAssets(assets []model.GeoAsset, source string) []model.GeoAsset {
	result := append([]model.GeoAsset(nil), assets...)
	for i := range result {
		result[i].Source = source
		result[i].URL = model.GeoSourceAssetURL(source, result[i].Type)
		result[i].LocalPath = filepath.Join("synthetic", source, result[i].Type+".db")
	}
	return result
}

func assertGeoSourceUnchanged(t *testing.T, s *Store, settings *model.GeneralSettings, assets []model.GeoAsset) {
	t.Helper()
	got, err := s.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	gotAssets, err := s.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, settings) || !reflect.DeepEqual(gotAssets, assets) {
		t.Fatalf("failed update mutated settings or assets: settings=%+v assets=%+v", got, gotAssets)
	}
}

func TestGeneralSettingsGeoSourceDefaultSwitchAndPartialPreserve(t *testing.T) {
	s, settings, assets := geoSourceTestStore(t)
	if settings.GeoSource != model.GeoSourceSagerNet || assets[0].Source != model.GeoSourceSagerNet || assets[1].Source != model.GeoSourceSagerNet {
		t.Fatal("legacy settings and resources must use SagerNet")
	}
	if _, err := s.UpdateGeoAsset(assets[0].ID, &model.GeoAssetRequest{URL: assets[0].URL, UseProxy: true, SyncMode: "weekly", SyncTime: "07:15:00", SyncWeekday: 3}); err != nil {
		t.Fatal(err)
	}
	settings.GeoSource = model.GeoSourceLoyalsoldier
	if err := s.SetGeneralSettingsWithGeoAssets(settings, preparedGeoAssets(assets, settings.GeoSource)); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range got {
		if item.Source != settings.GeoSource || item.URL != model.GeoSourceAssetURL(settings.GeoSource, item.Type) || item.SyncStatus != "updated" || item.LastSyncAt == 0 || item.CachedUpdatedAt == 0 {
			t.Fatalf("source switch incomplete: %+v", item)
		}
	}
	if !got[0].UseProxy || got[0].SyncMode != "weekly" || got[0].SyncTime != "07:15:00" || got[0].SyncWeekday != 3 {
		t.Fatalf("source switch replaced download settings: %+v", got[0])
	}
	if err := s.SetGeneralSettings(&model.GeneralSettings{AutoStartCore: false, DNSMasqTakeoverEnabled: true}); err != nil {
		t.Fatal(err)
	}
	settings.AutoStartCore = false
	assertGeoSourceUnchanged(t, s, settings, got)
}

func TestGeneralSettingsGeoSourceRejectsInvalidOrIncompleteAssets(t *testing.T) {
	for _, scenario := range []string{"invalid-source", "no-assets", "one-asset", "wrong-source", "duplicate-type", "missing-path", "wrong-id"} {
		t.Run(scenario, func(t *testing.T) {
			s, previous, original := geoSourceTestStore(t)
			next := *previous
			next.GeoSource = model.GeoSourceLoyalsoldier
			next.AutoStartCore = false
			assets := preparedGeoAssets(original, next.GeoSource)
			switch scenario {
			case "invalid-source":
				next.GeoSource = "unknown"
			case "no-assets":
				assets = nil
			case "one-asset":
				assets = assets[:1]
			case "wrong-source":
				assets[1].Source = model.GeoSourceSagerNet
			case "duplicate-type":
				assets[1].Type = assets[0].Type
			case "missing-path":
				assets[1].LocalPath = ""
			case "wrong-id":
				assets[1].ID += 100
			}
			if err := s.SetGeneralSettingsWithGeoAssets(&next, assets); err == nil {
				t.Fatal("invalid update succeeded")
			}
			assertGeoSourceUnchanged(t, s, previous, original)
		})
	}
}

func TestGeneralSettingsGeoSourceTransactionRollsBackOnWriteError(t *testing.T) {
	s, previous, assets := geoSourceTestStore(t)
	if _, err := s.db.Exec(`CREATE TRIGGER fail_geosite_source BEFORE UPDATE OF source ON geo_assets WHEN NEW.type = 'geosite' BEGIN SELECT RAISE(ABORT, 'synthetic failure'); END`); err != nil {
		t.Fatal(err)
	}
	next := *previous
	next.GeoSource = model.GeoSourceLoyalsoldier
	next.AutoStartCore = false
	if err := s.SetGeneralSettingsWithGeoAssets(&next, preparedGeoAssets(assets, next.GeoSource)); err == nil {
		t.Fatal("expected transaction failure")
	}
	assertGeoSourceUnchanged(t, s, previous, assets)
}

func TestGeneralSettingsGeoSourceRestorePreservesUnsyncedAssets(t *testing.T) {
	s, previous, assets := geoSourceTestStore(t)
	next := *previous
	next.GeoSource = model.GeoSourceLoyalsoldier
	if err := s.SetGeneralSettingsWithGeoAssets(&next, preparedGeoAssets(assets, next.GeoSource)); err != nil {
		t.Fatal(err)
	}
	if err := s.RestoreGeneralSettingsWithGeoAssets(previous, assets); err != nil {
		t.Fatal(err)
	}
	assertGeoSourceUnchanged(t, s, previous, assets)
}

func TestGeoAssetConditionalSyncRejectsSupersededDownloads(t *testing.T) {
	s, settings, assets := geoSourceTestStore(t)
	settings.GeoSource = model.GeoSourceLoyalsoldier
	if err := s.SetGeneralSettingsWithGeoAssets(settings, preparedGeoAssets(assets, settings.GeoSource)); err != nil {
		t.Fatal(err)
	}
	for _, operation := range []func(*model.GeoAsset) (bool, error){
		func(item *model.GeoAsset) (bool, error) {
			return s.SetGeoAssetSyncStateIfCurrent(item, "failed", "obsolete failure")
		},
		func(item *model.GeoAsset) (bool, error) {
			return s.UpdateGeoAssetSyncResultIfCurrent(item, "obsolete.db")
		},
	} {
		current, err := s.GetGeoAsset(assets[0].ID)
		if err != nil {
			t.Fatal(err)
		}
		staleURL, staleProxy := *current, *current
		staleURL.URL += ".old"
		staleProxy.UseProxy = !current.UseProxy
		for _, stale := range []*model.GeoAsset{&assets[0], &staleURL, &staleProxy} {
			if changed, err := operation(stale); err != nil || changed {
				t.Fatalf("superseded download updated resource: changed=%v error=%v", changed, err)
			}
		}
		if changed, err := operation(current); err != nil || !changed {
			t.Fatalf("current download failed to update resource: changed=%v error=%v", changed, err)
		}
	}
}

func TestGeoAssetSourceMigrationPreservesLegacyResources(t *testing.T) {
	s, _, assets := geoSourceTestStore(t)
	if _, err := s.db.Exec(`ALTER TABLE geo_assets DROP COLUMN source`); err != nil {
		t.Fatal(err)
	}
	if err := s.migrate(); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, assets) {
		t.Fatalf("migration changed legacy data: got=%+v want=%+v", got, assets)
	}
	if err := s.migrate(); err != nil {
		t.Fatalf("migration is not repeatable: %v", err)
	}
}
