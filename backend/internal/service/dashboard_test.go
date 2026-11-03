package service

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestExtractDashboardArchiveStripsGitHubRoot(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "dashboard.zip")
	writeDashboardTestArchive(t, archivePath, map[string]string{
		"panel-gh-pages/index.html":     "dashboard",
		"panel-gh-pages/assets/main.js": "script",
	})
	destination := t.TempDir()
	if err := extractDashboardArchive(archivePath, destination); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.html", filepath.Join("assets", "main.js")} {
		if _, err := os.Stat(filepath.Join(destination, name)); err != nil {
			t.Fatalf("missing extracted file %s: %v", name, err)
		}
	}
}

func TestExtractDashboardArchiveRejectsTraversal(t *testing.T) {
	archivePath := filepath.Join(t.TempDir(), "dashboard.zip")
	writeDashboardTestArchive(t, archivePath, map[string]string{"../outside.txt": "bad"})
	if err := extractDashboardArchive(archivePath, t.TempDir()); err == nil {
		t.Fatal("archive traversal was accepted")
	}
}

func TestDashboardListReportsInstalledSelectedVersion(t *testing.T) {
	root := t.TempDir()
	p := &paths.Paths{DataDir: root, DashboardsDir: filepath.Join(root, "dash")}
	if err := os.MkdirAll(filepath.Join(p.DashboardsDir, "metacubexd"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.DashboardsDir, "metacubexd", "index.html"), []byte("dashboard"), 0644); err != nil {
		t.Fatal(err)
	}
	metadata, _ := json.Marshal(dashboardMetadata{Commit: "1234567890", InstalledAt: 123})
	if err := os.WriteFile(filepath.Join(p.DashboardsDir, "metacubexd", ".ackwrap-dashboard.json"), metadata, 0644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIEnabled: true, ClashAPIPort: "9090", ClashAPIDashboard: "metacubexd", ClashAPIExternalUI: filepath.Join(p.DashboardsDir, "metacubexd")}); err != nil {
		t.Fatal(err)
	}
	items, err := NewDashboardService(db, p).List()
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 || !items[0].Installed || !items[0].Selected || items[0].CurrentVersion != "1234567" || items[0].UpdatedAt != 123 {
		t.Fatalf("dashboards = %+v", items)
	}
}

func TestExperimentalSettingsMapsDashboardToLocalPath(t *testing.T) {
	root := t.TempDir()
	dashboardsDir := filepath.Join(root, "dash")
	panelDir := filepath.Join(dashboardsDir, "zashboard")
	if err := os.MkdirAll(panelDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(panelDir, "index.html"), []byte("dashboard"), 0644); err != nil {
		t.Fatal(err)
	}
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	service := NewSettingsService(db)
	service.SetDashboardsDir(dashboardsDir)
	if err := service.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090", ClashAPIDashboard: "zashboard", CacheFileEnabled: true}); err == nil {
		t.Fatal("dashboard without Clash API secret was accepted")
	}
	if err := service.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090", ClashAPISecret: "test-secret", ClashAPIDashboard: "zashboard", CacheFileEnabled: true}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ClashAPIDashboard != "zashboard" || settings.ClashAPIExternalUI != panelDir || settings.ClashAPIExternalUIDownloadURL != "" {
		t.Fatalf("settings = %+v", settings)
	}
}

func TestExperimentalSettingsPreservesLegacyCustomDashboard(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacyPath := filepath.Join(root, "legacy-dashboard")
	if err := db.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIEnabled: true, ClashAPIPort: "9090", ClashAPIExternalUI: legacyPath}); err != nil {
		t.Fatal(err)
	}
	service := NewSettingsService(db)
	service.SetDashboardsDir(filepath.Join(root, "dash"))
	settings, err := service.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if settings.ClashAPIDashboard != "custom" {
		t.Fatalf("dashboard = %q, want custom", settings.ClashAPIDashboard)
	}
	if err := service.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIPort: "9090", ClashAPIDashboard: "custom", CacheFileEnabled: true}); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.ClashAPIExternalUI != legacyPath {
		t.Fatalf("external UI = %q, want %q", stored.ClashAPIExternalUI, legacyPath)
	}
}

func TestExperimentalSettingsAcceptsLegacyCustomDashboardRequest(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	legacyPath := filepath.Join(root, "legacy-dashboard")
	service := NewSettingsService(db)
	service.SetDashboardsDir(filepath.Join(root, "dash"))
	if err := service.SetExperimentalSettings(&model.ExperimentalSettings{
		ClashAPIPort:       "9090",
		ClashAPIExternalUI: legacyPath,
		CacheFileEnabled:   true,
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetExperimentalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.ClashAPIDashboard != "custom" || stored.ClashAPIExternalUI != legacyPath {
		t.Fatalf("settings = %+v", stored)
	}
}

func TestClashAPIControllerHost(t *testing.T) {
	tests := []struct {
		name       string
		externalUI string
		secret     string
		want       string
	}{
		{name: "local without dashboard", want: "127.0.0.1"},
		{name: "local without secret", externalUI: "/data/dash/panel", want: "127.0.0.1"},
		{name: "lan with secured dashboard", externalUI: "/data/dash/panel", secret: "secret", want: "0.0.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := clashAPIControllerHost(test.externalUI, test.secret); got != test.want {
				t.Fatalf("host = %q, want %q", got, test.want)
			}
		})
	}
}

func writeDashboardTestArchive(t *testing.T, path string, files map[string]string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	for name, content := range files {
		entry, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}
