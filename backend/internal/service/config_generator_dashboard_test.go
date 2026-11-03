package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestGeneratedDashboardUpdateURL(t *testing.T) {
	core := &configGeneratorCoreStub{}
	svc, db, p, _ := newScheduledRestartConfigGenerator(t, core)
	tests := []struct {
		name        string
		dashboard   string
		externalUI  string
		downloadURL string
		want        string
	}{
		{name: "no dashboard"},
		{
			name: "default Zashboard", externalUI: filepath.Join(p.DataDir, "custom-ui"),
			want: "https://github.com/Zephyruso/zashboard/archive/gh-pages.zip",
		},
		{
			name: "selected Zashboard", dashboard: "zashboard", externalUI: filepath.Join(p.DashboardsDir, "zashboard"),
			want: "https://github.com/Zephyruso/zashboard/archive/gh-pages.zip",
		},
		{
			name: "selected MetaCubeXD", dashboard: "metacubexd", externalUI: filepath.Join(p.DashboardsDir, "metacubexd"),
			want: "https://github.com/MetaCubeX/metacubexd/archive/gh-pages.zip",
		},
		{
			name: "selected Yacd", dashboard: "yacd", externalUI: filepath.Join(p.DashboardsDir, "yacd"),
			want: "https://github.com/MetaCubeX/Yacd-meta/archive/gh-pages.zip",
		},
		{
			name: "legacy MetaCubeXD path", externalUI: filepath.Join(p.DashboardsDir, "metacubexd"),
			want: "https://github.com/MetaCubeX/metacubexd/archive/gh-pages.zip",
		},
		{
			name: "custom download source", dashboard: "custom", externalUI: filepath.Join(p.DataDir, "custom-ui"),
			downloadURL: "https://example.com/dashboard.zip", want: "https://example.com/dashboard.zip",
		},
		{
			name: "explicit source overrides default", dashboard: "zashboard", externalUI: filepath.Join(p.DashboardsDir, "zashboard"),
			downloadURL: "https://example.com/pinned-dashboard.zip", want: "https://example.com/pinned-dashboard.zip",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := db.SetExperimentalSettings(&model.ExperimentalSettings{
				ClashAPIPort: "9090", ClashAPISecret: "test-secret", ClashAPIDashboard: test.dashboard,
				ClashAPIExternalUI: test.externalUI, ClashAPIExternalUIDownloadURL: test.downloadURL,
			}); err != nil {
				t.Fatal(err)
			}
			result, err := svc.GenerateCurrent()
			if err != nil {
				t.Fatal(err)
			}
			if !result.Valid {
				t.Fatalf("generated config is invalid: %s", result.Error)
			}
			clashAPI := result.Config["experimental"].(map[string]interface{})["clash_api"].(map[string]interface{})
			if test.want == "" {
				if _, exists := clashAPI["external_ui_download_url"]; exists {
					t.Fatal("disabled dashboard unexpectedly has a download source")
				}
				if _, exists := clashAPI["external_ui"]; exists {
					t.Fatal("default download source unexpectedly enabled a dashboard")
				}
			} else if clashAPI["external_ui_download_url"] != test.want {
				t.Fatalf("external_ui_download_url = %v, want %s", clashAPI["external_ui_download_url"], test.want)
			}
			settings, err := db.GetExperimentalSettings()
			if err != nil {
				t.Fatal(err)
			}
			if settings.ClashAPIExternalUIDownloadURL != test.downloadURL {
				t.Fatal("generating a default download source changed saved settings")
			}
		})
	}
	if reloads, restarts := core.calls(); reloads != 0 || restarts != 0 {
		t.Fatal("generating dashboard download sources must not restart the core")
	}
}
