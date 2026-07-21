package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/buildinfo"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
)

func TestBuildAppUpdateRequestAttemptsRequiresConfiguredProxy(t *testing.T) {
	const releaseURL = "https://api.github.com/repos/ackwrap/ackrun/releases/latest"
	attempts, err := buildAppUpdateRequestAttempts(&model.UpdateSettingsResponse{Acceleration: "ghproxy"}, releaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) == 0 || attempts[0].url != "https://gh-proxy.com/"+releaseURL {
		t.Fatalf("attempts = %+v", attempts)
	}
	for _, attempt := range attempts {
		if attempt.url == releaseURL {
			t.Fatal("configured update proxy must not fall back to a direct request")
		}
	}

	direct, err := buildAppUpdateRequestAttempts(&model.UpdateSettingsResponse{}, releaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if len(direct) != 1 || direct[0].url != releaseURL {
		t.Fatalf("direct attempts = %+v", direct)
	}
}

func TestFetchLatestAppRelease(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Accept") != "application/vnd.github+json" {
			t.Errorf("accept = %q", request.Header.Get("Accept"))
		}
		writer.Header().Set("Content-Type", "application/json")
		fmt.Fprint(writer, `{"tag_name":"v1.2.3","html_url":"https://example.com/release","published_at":"2026-07-21T00:00:00Z","assets":[{"name":"ackwrap_1.2.3-1_x86_64.ipk","digest":"sha256:abc","size":10,"browser_download_url":"https://example.com/file"}]}`)
	}))
	defer server.Close()

	release, err := fetchAppRelease(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatal(err)
	}
	if release.Version != "1.2.3" || release.PublishedAt == "" || len(release.Assets) != 1 {
		t.Fatalf("release = %+v", release)
	}
}

func TestAppUpdateStatusSupportsOpenWrtAMD64(t *testing.T) {
	originalVersion := buildinfo.Version
	buildinfo.Version = "1.0.0"
	t.Cleanup(func() { buildinfo.Version = originalVersion })

	openWrtRelease := filepath.Join(t.TempDir(), "openwrt_release")
	if err := os.WriteFile(openWrtRelease, []byte("DISTRIB_ID='OpenWrt'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	service := &AppUpdateService{
		goos:               "linux",
		goarch:             "amd64",
		openWrtReleasePath: openWrtRelease,
		lookPath: func(name string) (string, error) {
			return "/bin/" + name, nil
		},
	}
	release := &appRelease{Version: "1.1.0", Assets: []appReleaseAsset{{Name: "ackwrap_1.1.0-1_x86_64.ipk"}}}
	status := service.statusForRelease(release)
	if !status.UpdateAvailable || !status.CanInstall || status.AssetName != "ackwrap_1.1.0-1_x86_64.ipk" {
		t.Fatalf("status = %+v", status)
	}
}

func TestDownloadAppUpdateAssetVerifiesDigest(t *testing.T) {
	content := []byte("verified update package")
	digest := sha256.Sum256(content)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Write(content)
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "update.ipk")
	attempts := []updateRequestAttempt{{name: "test", url: server.URL, client: server.Client()}}
	asset := appReleaseAsset{Size: int64(len(content)), Digest: "sha256:" + hex.EncodeToString(digest[:])}
	if err := downloadAppUpdateAsset(context.Background(), attempts, destination, asset); err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(stored) != string(content) {
		t.Fatalf("stored content = %q", stored)
	}
}

type appUpdateCoreStub struct {
	running    bool
	startCalls int
}

func (core *appUpdateCoreStub) IsRunning() bool { return core.running }

func (core *appUpdateCoreStub) Start() (*model.ActionResponse, error) {
	core.running = true
	core.startCalls++
	return &model.ActionResponse{Success: true}, nil
}

func TestRestoreCoreAfterUpdateConsumesMarker(t *testing.T) {
	dataDir := t.TempDir()
	p := &paths.Paths{DataDir: dataDir}
	if err := os.WriteFile(p.AppUpdateRestoreMarkerPath(), []byte("1\n"), 0600); err != nil {
		t.Fatal(err)
	}
	core := &appUpdateCoreStub{}
	service := &AppUpdateService{paths: p, core: core}
	service.RestoreCoreAfterUpdate()
	if core.startCalls != 1 || !core.running {
		t.Fatalf("core = %+v", core)
	}
	if _, err := os.Stat(p.AppUpdateRestoreMarkerPath()); !os.IsNotExist(err) {
		t.Fatalf("restore marker still exists: %v", err)
	}
}

func TestAppUpdateLockBlocksConcurrentInstall(t *testing.T) {
	p := &paths.Paths{DataDir: t.TempDir()}
	first := &AppUpdateService{paths: p}
	second := &AppUpdateService{paths: p}
	if err := first.beginUpdate(); err != nil {
		t.Fatal(err)
	}
	if err := second.beginUpdate(); !errors.Is(err, ErrAppUpdateInProgress) {
		t.Fatalf("second beginUpdate error = %v", err)
	}
	os.Remove(p.AppUpdateLockPath())
}

func TestAppUpdateStatusReportsInstallerFailure(t *testing.T) {
	p := &paths.Paths{DataDir: t.TempDir()}
	if err := os.WriteFile(p.AppUpdateResultPath(), []byte("opkg install failed (exit 1)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status := &model.AppUpdateStatus{CanInstall: true}
	service := &AppUpdateService{paths: p}
	service.applyInstallState(status)
	if status.UpdateError == "" || status.Message != "上次更新安装失败" {
		t.Fatalf("status = %+v", status)
	}
}
