package service

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

type geoSourcePreparerFunc func(string) ([]model.GeoAsset, error)

func (f geoSourcePreparerFunc) PrepareGeoSource(source string) ([]model.GeoAsset, error) {
	return f(source)
}

func geoSettingsTestService(t *testing.T) (*SettingsService, *store.Store, *model.GeneralSettings, []model.GeoAsset) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	settings, err := db.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	assets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	return NewSettingsService(db), db, settings, assets
}

func geoSettingsPreparedAssets(assets []model.GeoAsset, source string) []model.GeoAsset {
	prepared := append([]model.GeoAsset(nil), assets...)
	for i := range prepared {
		prepared[i].Source = source
		prepared[i].URL = model.GeoSourceAssetURL(source, prepared[i].Type)
		prepared[i].LocalPath = filepath.Join("synthetic", source, prepared[i].Type+".db")
	}
	return prepared
}

func assertGeoSettingsSnapshot(t *testing.T, db *store.Store, settings *model.GeneralSettings, assets []model.GeoAsset) {
	t.Helper()
	got, err := db.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	gotAssets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, settings) || !reflect.DeepEqual(gotAssets, assets) {
		t.Fatalf("settings or assets changed unexpectedly: settings=%+v assets=%+v", got, gotAssets)
	}
}

func TestGeneralSettingsGeoSourceInvalidAndPreparationFailuresPreserveState(t *testing.T) {
	for _, scenario := range []string{"invalid", "unavailable", "download", "incomplete", "wrong-source"} {
		t.Run(scenario, func(t *testing.T) {
			svc, db, previous, assets := geoSettingsTestService(t)
			source := model.GeoSourceLoyalsoldier
			called := false
			if scenario != "unavailable" {
				svc.SetGeoSourcePreparer(geoSourcePreparerFunc(func(source string) ([]model.GeoAsset, error) {
					called = true
					assertGeoSettingsSnapshot(t, db, previous, assets)
					switch scenario {
					case "download":
						return nil, errors.New("synthetic download failure")
					case "incomplete":
						return nil, nil
					case "wrong-source":
						return geoSettingsPreparedAssets(assets, model.GeoSourceSagerNet), nil
					default:
						t.Fatal("invalid source should not start a download")
						return nil, nil
					}
				}))
			}
			if scenario == "invalid" {
				source = "unknown"
			}
			disabled := false
			err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source, AutoStartCore: &disabled})
			if err == nil {
				t.Fatal("failed preparation should reject the entire request")
			}
			if scenario == "invalid" && (!errors.Is(err, ErrGeoSourceSettingsInvalid) || called) {
				t.Fatalf("invalid source was not rejected before download: %v", err)
			}
			assertGeoSettingsSnapshot(t, db, previous, assets)
		})
	}
}

func TestGeneralSettingsGeoSourceSwitchWhileRunningAndPartialUpdates(t *testing.T) {
	svc, db, previous, assets := geoSettingsTestService(t)
	running := &SingboxService{pid: 1, cmd: &exec.Cmd{Process: &os.Process{}}}
	svc.SetModeDependencies(running, nil)
	calls := 0
	svc.SetGeoSourcePreparer(geoSourcePreparerFunc(func(source string) ([]model.GeoAsset, error) {
		calls++
		assertGeoSettingsSnapshot(t, db, previous, assets)
		return geoSettingsPreparedAssets(assets, source), nil
	}))
	source := model.GeoSourceLoyalsoldier
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source}); err != nil {
		t.Fatal(err)
	}
	disabled := false
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{AutoStartCore: &disabled}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source}); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || got.GeoSource != source || got.AutoStartCore || !got.DNSMasqTakeoverEnabled || !running.IsRunning() {
		t.Fatalf("source update changed unrelated state or redownloaded: calls=%d settings=%+v", calls, got)
	}
	gotAssets, err := db.ListGeoAssets()
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range gotAssets {
		if item.Source != source || item.SyncStatus != "updated" || item.LocalPath == "" {
			t.Fatalf("asset not switched: %+v", item)
		}
	}
}

func TestGeneralSettingsGeoSourceAndDNSMasqRollbackTogether(t *testing.T) {
	svc, db, previous, assets := geoSettingsTestService(t)
	dir := t.TempDir()
	p := &paths.Paths{BinaryPath: filepath.Join(dir, "sing-box"), ConfigDir: dir, ConfigPath: filepath.Join(dir, "config.json")}
	for _, path := range []string{p.BinaryPath, p.ConfigPath} {
		if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	generator := &modeConfigGeneratorStub{err: errors.New("synthetic reconcile failure")}
	svc.SetModeDependencies(&SingboxService{paths: p}, generator)
	svc.SetGeoSourcePreparer(geoSourcePreparerFunc(func(source string) ([]model.GeoAsset, error) {
		return geoSettingsPreparedAssets(assets, source), nil
	}))
	source, disabled := model.GeoSourceLoyalsoldier, false
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{GeoSource: &source, DNSMasqTakeoverEnabled: &disabled}); err == nil {
		t.Fatal("reconcile failure should reject the update")
	}
	if generator.calls != 1 {
		t.Fatalf("reconcile calls=%d", generator.calls)
	}
	assertGeoSettingsSnapshot(t, db, previous, assets)
}
