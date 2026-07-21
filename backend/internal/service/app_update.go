package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/buildinfo"
	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

const (
	ackwrapLatestReleaseURL = "https://api.github.com/repos/ackwrap/ackrun/releases/latest"
	appUpdateMaxSize        = 128 << 20
)

var (
	ErrAppUpdateUnavailable = errors.New("没有可安装的 Ackwrap 更新")
	ErrAppUpdateUnsupported = errors.New("当前平台不支持自动更新")
	ErrAppUpdateInProgress  = errors.New("Ackwrap 更新正在进行")
)

type appUpdateCore interface {
	IsRunning() bool
	Start() (*model.ActionResponse, error)
}

type appReleaseAsset struct {
	Name        string `json:"name"`
	Digest      string `json:"digest"`
	Size        int64  `json:"size"`
	DownloadURL string `json:"browser_download_url"`
}

type appRelease struct {
	Version     string
	ReleaseURL  string
	PublishedAt string
	Assets      []appReleaseAsset
}

type AppUpdateService struct {
	store    *store.Store
	paths    *paths.Paths
	core     appUpdateCore
	realtime *RealtimeService

	mu       sync.Mutex
	updating bool

	releaseAPIURL      string
	goos               string
	goarch             string
	openWrtReleasePath string
	lookPath           func(string) (string, error)
	launchInstaller    func(string) error
}

func NewAppUpdateService(db *store.Store, p *paths.Paths, core appUpdateCore, realtime *RealtimeService) *AppUpdateService {
	service := &AppUpdateService{
		store:              db,
		paths:              p,
		core:               core,
		realtime:           realtime,
		releaseAPIURL:      ackwrapLatestReleaseURL,
		goos:               runtime.GOOS,
		goarch:             runtime.GOARCH,
		openWrtReleasePath: "/etc/openwrt_release",
		lookPath:           exec.LookPath,
	}
	service.launchInstaller = service.launchOpenWrtInstaller
	return service
}

func (svc *AppUpdateService) Check(ctx context.Context) (*model.AppUpdateStatus, error) {
	logging.Info("app_update.check", "checking Ackwrap release")
	status, _, err := svc.check(ctx)
	if err != nil {
		logging.Error("app_update.check", "check failed: %v", err)
		return nil, err
	}
	svc.applyInstallState(status)
	logging.Info("app_update.check", "current=%s latest=%s update_available=%t", status.CurrentVersion, status.LatestVersion, status.UpdateAvailable)
	return status, nil
}

func (svc *AppUpdateService) Install(ctx context.Context) (*model.AppUpdateInstallResponse, error) {
	if err := svc.beginUpdate(); err != nil {
		return nil, err
	}
	launched := false
	defer func() {
		svc.mu.Lock()
		svc.updating = false
		svc.mu.Unlock()
		if !launched {
			os.Remove(svc.paths.AppUpdateLockPath())
		}
	}()

	status, release, err := svc.check(ctx)
	if err != nil {
		return nil, err
	}
	if !status.UpdateAvailable {
		return nil, ErrAppUpdateUnavailable
	}
	if !status.CanInstall {
		return nil, fmt.Errorf("%w: %s", ErrAppUpdateUnsupported, status.Message)
	}
	asset := findAppUpdateAsset(release, status.AssetName)
	if asset == nil {
		return nil, fmt.Errorf("%w: release asset %s not found", ErrAppUpdateUnsupported, status.AssetName)
	}

	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return nil, fmt.Errorf("读取更新代理设置失败: %w", err)
	}
	attempts, err := buildAppUpdateRequestAttempts(settings, asset.DownloadURL)
	if err != nil {
		return nil, err
	}
	staged, err := os.CreateTemp("", "ackwrap-update-*.ipk")
	if err != nil {
		return nil, fmt.Errorf("创建更新临时文件失败: %w", err)
	}
	stagedPath := staged.Name()
	if err := staged.Close(); err != nil {
		os.Remove(stagedPath)
		return nil, fmt.Errorf("关闭更新临时文件失败: %w", err)
	}
	defer func() {
		if !launched {
			os.Remove(stagedPath)
		}
	}()

	svc.broadcast("downloading", 0, "")
	logging.Info("app_update.download", "downloading %s through configured update proxy", asset.Name)
	if err := downloadAppUpdateAsset(ctx, attempts, stagedPath, *asset); err != nil {
		svc.broadcast("failed", 0, err.Error())
		return nil, err
	}

	restoreMarker := svc.paths.AppUpdateRestoreMarkerPath()
	if svc.core != nil && svc.core.IsRunning() {
		if err := os.WriteFile(restoreMarker, []byte("1\n"), 0600); err != nil {
			return nil, fmt.Errorf("记录更新后核心恢复状态失败: %w", err)
		}
	} else if err := os.Remove(restoreMarker); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("清理更新恢复状态失败: %w", err)
	}

	scriptPath, err := svc.writeOpenWrtInstallerScript(stagedPath)
	if err != nil {
		os.Remove(restoreMarker)
		return nil, err
	}
	if err := svc.launchInstaller(scriptPath); err != nil {
		os.Remove(scriptPath)
		os.Remove(restoreMarker)
		return nil, fmt.Errorf("启动 OpenWrt 更新安装失败: %w", err)
	}
	launched = true
	svc.broadcast("installing", 100, "")
	logging.Info("app_update.install", "OpenWrt update scheduled: version=%s", release.Version)
	return &model.AppUpdateInstallResponse{
		Success: true,
		Message: "更新包已校验，正在安装；Ackwrap 将自动重启",
		Version: release.Version,
	}, nil
}

func (svc *AppUpdateService) beginUpdate() error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.updating || svc.installInProgress() {
		return ErrAppUpdateInProgress
	}
	lock, err := os.OpenFile(svc.paths.AppUpdateLockPath(), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		return ErrAppUpdateInProgress
	}
	if err != nil {
		return fmt.Errorf("创建更新锁失败: %w", err)
	}
	if _, err := fmt.Fprintf(lock, "%d\n", time.Now().Unix()); err != nil {
		lock.Close()
		os.Remove(svc.paths.AppUpdateLockPath())
		return fmt.Errorf("写入更新锁失败: %w", err)
	}
	if err := lock.Close(); err != nil {
		os.Remove(svc.paths.AppUpdateLockPath())
		return fmt.Errorf("关闭更新锁失败: %w", err)
	}
	os.Remove(svc.paths.AppUpdateResultPath())
	svc.updating = true
	return nil
}

func (svc *AppUpdateService) installInProgress() bool {
	info, err := os.Stat(svc.paths.AppUpdateLockPath())
	if err != nil {
		return false
	}
	if time.Since(info.ModTime()) <= 30*time.Minute {
		return true
	}
	logging.Info("app_update.install", "removing stale update lock")
	return os.Remove(svc.paths.AppUpdateLockPath()) != nil
}

func (svc *AppUpdateService) applyInstallState(status *model.AppUpdateStatus) {
	if svc.installInProgress() {
		status.Updating = true
		status.CanInstall = false
		status.Message = "更新正在安装，请稍候"
		return
	}
	result, err := os.ReadFile(svc.paths.AppUpdateResultPath())
	if err == nil && strings.TrimSpace(string(result)) != "" {
		status.UpdateError = strings.TrimSpace(string(result))
		status.Message = "上次更新安装失败"
	}
}

func (svc *AppUpdateService) RestoreCoreAfterUpdate() {
	markerPath := svc.paths.AppUpdateRestoreMarkerPath()
	if _, err := os.Stat(markerPath); err != nil {
		if !os.IsNotExist(err) {
			logging.Error("app_update.restore", "read core restore marker failed: %v", err)
		}
		return
	}
	if svc.core == nil {
		return
	}
	if !svc.core.IsRunning() {
		if _, err := svc.core.Start(); err != nil {
			logging.Error("app_update.restore", "restore core after update failed: %v", err)
			return
		}
	}
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		logging.Error("app_update.restore", "remove core restore marker failed: %v", err)
		return
	}
	logging.Info("app_update.restore", "core state restored after Ackwrap update")
}

func (svc *AppUpdateService) check(ctx context.Context) (*model.AppUpdateStatus, *appRelease, error) {
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return nil, nil, fmt.Errorf("读取更新代理设置失败: %w", err)
	}
	attempts, err := buildAppUpdateRequestAttempts(settings, svc.releaseAPIURL)
	if err != nil {
		return nil, nil, err
	}
	release, err := fetchLatestAppRelease(ctx, attempts)
	if err != nil {
		return nil, nil, err
	}
	status := svc.statusForRelease(release)
	return status, release, nil
}

func (svc *AppUpdateService) statusForRelease(release *appRelease) *model.AppUpdateStatus {
	current := strings.TrimPrefix(strings.TrimSpace(buildinfo.Version), "v")
	status := &model.AppUpdateStatus{
		CurrentVersion: current,
		LatestVersion:  release.Version,
		Platform:       svc.goos,
		Architecture:   svc.goarch,
		ReleaseURL:     release.ReleaseURL,
		PublishedAt:    release.PublishedAt,
	}
	if !isSingboxVersion(current) {
		status.Message = "当前是开发构建，无法自动判断版本新旧"
		return status
	}
	status.UpdateAvailable = compareSingboxVersions(current, release.Version) < 0
	if !status.UpdateAvailable {
		status.Message = "当前已是最新版本"
		return status
	}
	assetName, supported := appUpdateAssetName(release.Version, svc.goos, svc.goarch)
	status.AssetName = assetName
	if !supported {
		status.Message = "当前系统或架构暂无自动更新包"
		return status
	}
	if _, err := os.Stat(svc.openWrtReleasePath); err != nil {
		status.Message = "自动安装目前仅支持 OpenWrt"
		return status
	}
	if _, err := svc.lookPath("opkg"); err != nil {
		status.Message = "未找到 opkg，无法自动安装"
		return status
	}
	if findAppUpdateAsset(release, assetName) == nil {
		status.Message = "最新版本缺少当前架构的 OpenWrt IPK"
		return status
	}
	status.CanInstall = true
	status.Message = "发现新版本，可以自动更新"
	return status
}

func appUpdateAssetName(version, goos, goarch string) (string, bool) {
	if goos != "linux" {
		return "", false
	}
	arch := ""
	switch goarch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64_generic"
	default:
		return "", false
	}
	return fmt.Sprintf("ackwrap_%s-1_%s.ipk", version, arch), true
}

func findAppUpdateAsset(release *appRelease, name string) *appReleaseAsset {
	for i := range release.Assets {
		if release.Assets[i].Name == name {
			return &release.Assets[i]
		}
	}
	return nil
}

func buildAppUpdateRequestAttempts(settings *model.UpdateSettingsResponse, rawURL string) ([]updateRequestAttempt, error) {
	if settings == nil || strings.TrimSpace(settings.Acceleration) == "" {
		return []updateRequestAttempt{{name: "direct", url: rawURL, client: &http.Client{Timeout: 60 * time.Second, Transport: directHTTPTransport()}}}, nil
	}
	if settings.Acceleration == "custom" && strings.TrimSpace(settings.CustomMirrorURL) == "" {
		return nil, fmt.Errorf("自定义更新代理 URL 为空，请先保存有效设置")
	}
	attempts := buildGitHubDownloadAttempts(settings, rawURL)
	proxied := make([]updateRequestAttempt, 0, len(attempts))
	for _, attempt := range attempts {
		if attempt.url == rawURL || strings.Contains(attempt.name, "direct") {
			continue
		}
		client := *attempt.client
		client.Timeout = 60 * time.Second
		attempt.client = &client
		proxied = append(proxied, attempt)
	}
	if len(proxied) == 0 {
		return nil, fmt.Errorf("当前更新代理不支持该 GitHub 地址")
	}
	return proxied, nil
}

func fetchLatestAppRelease(ctx context.Context, attempts []updateRequestAttempt) (*appRelease, error) {
	var lastErr error
	for _, attempt := range attempts {
		release, err := fetchAppRelease(ctx, attempt.client, attempt.url)
		if err == nil {
			return release, nil
		}
		lastErr = err
		logging.Info("app_update.check", "release request attempt failed: %s: %v", attempt.name, err)
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的更新代理请求")
	}
	return nil, lastErr
}

func fetchAppRelease(ctx context.Context, client *http.Client, rawURL string) (*appRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建版本检查请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "Ackwrap/"+buildinfo.Version)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求 Ackwrap Release 失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Ackwrap Release API 返回 HTTP %d", resp.StatusCode)
	}
	var payload struct {
		TagName     string            `json:"tag_name"`
		HTMLURL     string            `json:"html_url"`
		PublishedAt string            `json:"published_at"`
		Assets      []appReleaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("解析 Ackwrap Release 失败: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(payload.TagName), "v")
	if !isSingboxVersion(version) {
		return nil, fmt.Errorf("Ackwrap Release 返回了无效版本 %q", payload.TagName)
	}
	return &appRelease{Version: version, ReleaseURL: payload.HTMLURL, PublishedAt: payload.PublishedAt, Assets: payload.Assets}, nil
}

func downloadAppUpdateAsset(ctx context.Context, attempts []updateRequestAttempt, destination string, asset appReleaseAsset) error {
	expectedDigest := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(asset.Digest)), "sha256:")
	if len(expectedDigest) != sha256.Size*2 {
		return fmt.Errorf("Release 资产缺少有效 SHA-256 摘要")
	}
	var lastErr error
	for _, attempt := range attempts {
		if err := downloadAppUpdateAssetOnce(ctx, attempt.client, attempt.url, destination, asset.Size, expectedDigest); err == nil {
			return nil
		} else {
			lastErr = err
			logging.Info("app_update.download", "download attempt failed: %s: %v", attempt.name, err)
		}
	}
	return lastErr
}

func downloadAppUpdateAssetOnce(ctx context.Context, client *http.Client, rawURL, destination string, expectedSize int64, expectedDigest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Ackwrap/"+buildinfo.Version)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载更新包失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载更新包返回 HTTP %d", resp.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(resp.Body, appUpdateMaxSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("保存更新包失败: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("关闭更新包失败: %w", closeErr)
	}
	if written > appUpdateMaxSize {
		return fmt.Errorf("更新包超过 %d 字节限制", appUpdateMaxSize)
	}
	if expectedSize > 0 && written != expectedSize {
		return fmt.Errorf("更新包大小不匹配: got %d want %d", written, expectedSize)
	}
	if actual := hex.EncodeToString(hash.Sum(nil)); actual != expectedDigest {
		return fmt.Errorf("更新包 SHA-256 校验失败")
	}
	return nil
}

func (svc *AppUpdateService) writeOpenWrtInstallerScript(stagedPath string) (string, error) {
	opkgPath, err := svc.lookPath("opkg")
	if err != nil {
		return "", fmt.Errorf("查找 opkg 失败: %w", err)
	}
	script, err := os.CreateTemp("", "ackwrap-install-*.sh")
	if err != nil {
		return "", fmt.Errorf("创建更新安装脚本失败: %w", err)
	}
	scriptPath := script.Name()
	content := fmt.Sprintf("#!/bin/sh\nsleep 1\nrm -f %s\n%s install --force-reinstall %s >/tmp/ackwrap-update.log 2>&1\nstatus=$?\nif [ \"$status\" -ne 0 ]; then\n  printf 'opkg install failed (exit %%s)\\n' \"$status\" > %s\nfi\nrm -f %s %s \"$0\"\nexit $status\n", shellQuote(svc.paths.AppUpdateResultPath()), shellQuote(opkgPath), shellQuote(stagedPath), shellQuote(svc.paths.AppUpdateResultPath()), shellQuote(stagedPath), shellQuote(svc.paths.AppUpdateLockPath()))
	if _, err := script.WriteString(content); err != nil {
		script.Close()
		os.Remove(scriptPath)
		return "", fmt.Errorf("写入更新安装脚本失败: %w", err)
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptPath)
		return "", fmt.Errorf("关闭更新安装脚本失败: %w", err)
	}
	if err := os.Chmod(scriptPath, 0700); err != nil {
		os.Remove(scriptPath)
		return "", fmt.Errorf("设置更新安装脚本权限失败: %w", err)
	}
	return scriptPath, nil
}

func (svc *AppUpdateService) launchOpenWrtInstaller(scriptPath string) error {
	command := exec.Command("/bin/sh", "-c", "nohup "+shellQuote(scriptPath)+" >/dev/null 2>&1 &")
	return command.Run()
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func (svc *AppUpdateService) broadcast(status string, progress float64, errorMessage string) {
	if svc.realtime == nil {
		return
	}
	payload := map[string]any{"status": status, "progress": progress}
	if errorMessage != "" {
		payload["error"] = errorMessage
	}
	svc.realtime.Broadcast("app.update", payload)
}
