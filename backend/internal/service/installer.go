package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

var (
	singboxVersionURL = "https://api.github.com/repos/ackwrap/sing-box-wrap/releases/latest"
)

type InstallerService struct {
	store       *store.Store
	paths       *paths.Paths
	realtime    *RealtimeService
	mu          sync.Mutex
	busy        bool
	latestMu    sync.Mutex
	latest      string
	latestAt    time.Time
	postInstall func(version string) error
}

func NewInstallerService(s *store.Store, p *paths.Paths, rt *RealtimeService) *InstallerService {
	return &InstallerService{store: s, paths: p, realtime: rt}
}

func (svc *InstallerService) SetPostInstallHook(hook func(version string) error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.postInstall = hook
}

func (svc *InstallerService) GetStatus() (*model.InstallStateResponse, error) {
	state, err := svc.store.GetInstallState()
	if err != nil {
		return nil, err
	}
	if isActiveInstallStatus(state.Status) && !svc.isBusy() {
		state.Status = model.InstallFailed
		state.Message = "installation interrupted"
		state.Error = "安装任务已中断，请重新安装"
		state.Progress = 0
		if err := svc.store.SetInstallState(state); err != nil {
			return nil, err
		}
	}

	version := state.Version
	if !isSingboxVersion(version) {
		version = readSingboxVersion(svc.paths.BinaryPath)
	}
	if version != "" && !isInstallStatus(state.Status) {
		state.Status = model.InstallDone
		state.Message = "installed"
		state.Error = ""
		state.Progress = 0
	}
	if version != "" {
		state.Version = version
	}

	// 获取最新可用版本
	latestVersion, err := svc.getLatestVersion()
	if err != nil {
		logging.Info("installer.latest_version", "failed to fetch: %v", err)
	}
	state.LatestVersion = latestVersion

	return state, nil
}

func isInstallStatus(status model.InstallStatus) bool {
	switch status {
	case model.InstallIdle, model.InstallDownloading, model.InstallExtracting, model.InstallDone, model.InstallFailed:
		return true
	default:
		return false
	}
}

func isActiveInstallStatus(status model.InstallStatus) bool {
	return status == model.InstallDownloading || status == model.InstallExtracting
}

func (svc *InstallerService) isBusy() bool {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	return svc.busy
}

func (svc *InstallerService) Install() (*model.ActionResponse, error) {
	svc.mu.Lock()
	if svc.busy {
		svc.mu.Unlock()
		return nil, fmt.Errorf("installation already in progress")
	}
	svc.busy = true
	svc.mu.Unlock()
	svc.setState(model.InstallDownloading, "preparing download", 0, "", "")
	svc.broadcastStatus()

	go func() {
		defer func() {
			svc.mu.Lock()
			svc.busy = false
			svc.mu.Unlock()
		}()
		svc.runInstall()
	}()

	return &model.ActionResponse{Success: true, Message: "install started"}, nil
}

func (svc *InstallerService) runInstall() {
	logging.Info("installer.start", "starting sing-box installation")

	// 获取最新版本
	latestVersion, err := svc.getLatestVersion()
	if err != nil {
		logging.Info("installer.version", "failed to fetch latest version: %v", err)
		svc.setState(model.InstallFailed, "", 0, "", fmt.Sprintf("fetch latest version failed: %v", err))
		svc.broadcastStatus()
		return
	}
	logging.Info("installer.version", "using latest version: %s", latestVersion)

	svc.setState(model.InstallDownloading, "downloading", 0, "", "")
	svc.broadcastStatus()

	url, err := svc.buildDownloadURL(latestVersion)
	if err != nil {
		svc.setState(model.InstallFailed, "", 0, "", err.Error())
		svc.broadcastStatus()
		return
	}

	tmpFile := filepath.Join(svc.paths.DataDir, "sing-box-download.tmp")
	if err := svc.download(url, tmpFile); err != nil {
		svc.setState(model.InstallFailed, "", 0, "", fmt.Sprintf("download failed: %v", err))
		svc.broadcastStatus()
		return
	}

	logging.Info("installer.extract", "extracting sing-box")
	svc.setState(model.InstallExtracting, "extracting", 0, "", "")
	svc.broadcastStatus()

	if err := svc.extract(tmpFile); err != nil {
		os.Remove(tmpFile)
		svc.setState(model.InstallFailed, "", 0, "", fmt.Sprintf("extract failed: %v", err))
		svc.broadcastStatus()
		return
	}

	os.Remove(tmpFile)

	migrationError := ""
	svc.mu.Lock()
	postInstall := svc.postInstall
	svc.mu.Unlock()
	if postInstall != nil {
		if err := postInstall(latestVersion); err != nil {
			migrationError = fmt.Sprintf("核心已安装，但配置兼容迁移失败: %v", err)
			logging.Error("installer.migrate", "%s", migrationError)
		}
	}

	svc.setState(model.InstallDone, "installed", 0, latestVersion, migrationError)
	svc.broadcastStatus()

	runtimeStatus := model.RuntimeNoConfig
	if _, ok, err := svc.paths.ActiveConfigPath(); err == nil && ok {
		runtimeStatus = model.RuntimeStopped
	}
	svc.realtime.Broadcast("runtime.status", model.RuntimeResponse{Status: runtimeStatus, Version: latestVersion})
	logging.Info("installer.start", "sing-box installed successfully, version=%s", latestVersion)
}

func (svc *InstallerService) fetchLatestVersion() (string, error) {
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return "", fmt.Errorf("读取更新设置失败: %w", err)
	}
	return fetchLatestSingboxVersionWithSettings(settings, singboxVersionURL)
}

func (svc *InstallerService) getLatestVersion() (string, error) {
	svc.latestMu.Lock()
	defer svc.latestMu.Unlock()
	if svc.latest != "" && time.Since(svc.latestAt) < 5*time.Minute {
		return svc.latest, nil
	}
	version, err := svc.fetchLatestVersion()
	if err != nil {
		return "", err
	}
	svc.latest = version
	svc.latestAt = time.Now()
	return version, nil
}

func (svc *InstallerService) buildDownloadURL(version string) (string, error) {
	return buildDownloadURLFor(version, runtime.GOOS, runtime.GOARCH)
}

func buildDownloadURLFor(version, goos, arch string) (string, error) {
	if goos != "windows" && goos != "linux" && goos != "darwin" {
		return "", fmt.Errorf("unsupported operating system: %s", goos)
	}

	var archStr string
	switch arch {
	case "amd64":
		archStr = "amd64"
	case "arm64":
		archStr = "arm64"
	case "386":
		archStr = "386"
	default:
		archStr = arch
	}

	var ext string
	if goos == "windows" {
		ext = ".zip"
	} else {
		ext = ".tar.gz"
	}

	variant := ""
	if goos == "linux" {
		variant = "-musl"
	}
	name := fmt.Sprintf("sing-wrap-%s-%s-%s%s", version, goos, archStr, variant)
	url := fmt.Sprintf("https://github.com/ackwrap/sing-box-wrap/releases/download/v%s/%s%s", version, name, ext)
	return url, nil
}

func (svc *InstallerService) download(url, dest string) error {
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return fmt.Errorf("读取更新设置失败: %w", err)
	}
	attempts, err := buildUpdateRequestAttempts(settings, url)
	if err != nil {
		return err
	}
	var lastErr error
	for _, attempt := range attempts {
		logging.Info("installer.download", "download attempt: %s", attempt.name)
		if err := svc.downloadOnce(attempt.client, attempt.url, dest); err == nil {
			return nil
		} else {
			lastErr = err
			logging.Info("installer.download", "download attempt failed: %s: %v", attempt.name, err)
		}
	}
	return lastErr
}

func (svc *InstallerService) downloadOnce(client *http.Client, downloadURL, dest string) error {
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("请求下载地址失败: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载地址返回 HTTP %d", resp.StatusCode)
	}

	total := resp.ContentLength

	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	buf := make([]byte, 32*1024)
	var downloaded int64

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write file: %w", werr)
			}
			downloaded += int64(n)
			if total > 0 {
				progress := float64(downloaded) / float64(total) * 100
				svc.setState(model.InstallDownloading, fmt.Sprintf("downloading %.1f%%", progress), progress, "", "")
				svc.broadcastProgress(downloaded, total)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read body: %w", err)
		}
	}

	logging.Info("installer.download", "download complete: %d bytes", downloaded)
	return nil
}

func (svc *InstallerService) extract(archive string) error {
	if runtime.GOOS == "windows" {
		return svc.extractZip(archive)
	}
	return svc.extractTarGz(archive)
}

func (svc *InstallerService) extractZip(archive string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	foundBinary := false
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := archiveEntryBase(f.Name)
		mode, ok := runtimeArchiveFileMode(name)
		if !ok {
			continue
		}
		foundBinary = foundBinary || name == "sing-box.exe"
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open entry: %w", err)
		}
		if err := writeExtractedFile(filepath.Join(svc.paths.BinaryDir, name), rc, mode); err != nil {
			rc.Close()
			return err
		}
		rc.Close()
		logging.Info("installer.extract", "extracted: %s", filepath.Join(svc.paths.BinaryDir, name))
	}
	if !foundBinary {
		return fmt.Errorf("archive does not contain sing-box.exe")
	}
	return nil
}

func (svc *InstallerService) extractTarGz(archive string) error {
	f, err := os.Open(archive)
	if err != nil {
		return fmt.Errorf("open tar.gz: %w", err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("open gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	foundBinary := false
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read tar: %w", err)
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			continue
		}
		name := archiveEntryBase(header.Name)
		mode, ok := runtimeArchiveFileMode(name)
		if !ok {
			continue
		}
		foundBinary = foundBinary || name == "sing-box"
		outPath := filepath.Join(svc.paths.BinaryDir, name)
		if err := writeExtractedFile(outPath, tr, mode); err != nil {
			return err
		}
		logging.Info("installer.extract", "extracted: %s", outPath)
	}
	if !foundBinary {
		return fmt.Errorf("archive does not contain sing-box")
	}
	return nil
}

func archiveEntryBase(name string) string {
	name = filepath.ToSlash(name)
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	if len(parts) == 0 {
		return ""
	}
	base := parts[len(parts)-1]
	if base == "" || base == "." || base == ".." {
		return ""
	}
	return base
}

func runtimeArchiveFileMode(name string) (os.FileMode, bool) {
	switch strings.ToLower(name) {
	case "sing-box":
		return 0755, true
	case "sing-box.exe":
		return 0755, true
	case "libcronet.so", "libcronet.dylib", "libcronet.dll":
		return 0644, true
	default:
		return 0, false
	}
}

func writeExtractedFile(outPath string, src io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return fmt.Errorf("create binary directory: %w", err)
	}
	out, err := os.CreateTemp(filepath.Dir(outPath), ".ackwrap-install-*")
	if err != nil {
		return fmt.Errorf("create extracted file: %w", err)
	}
	tmpPath := out.Name()
	defer os.Remove(tmpPath)
	_, copyErr := io.Copy(out, src)
	closeErr := out.Close()
	if copyErr != nil {
		return fmt.Errorf("copy extracted file: %w", copyErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close extracted file: %w", closeErr)
	}
	if err := os.Chmod(tmpPath, mode); err != nil {
		return fmt.Errorf("set extracted file permissions: %w", err)
	}
	if runtime.GOOS == "windows" {
		if err := os.Remove(outPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("replace extracted file: %w", err)
		}
	}
	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("commit extracted file: %w", err)
	}
	return nil
}

func (svc *InstallerService) setState(status model.InstallStatus, message string, progress float64, version string, errMsg string) {
	svc.store.SetInstallState(&model.InstallStateResponse{
		Status:   status,
		Version:  version,
		Progress: progress,
		Message:  message,
		Error:    errMsg,
	})
}

func (svc *InstallerService) broadcastStatus() {
	state, _ := svc.store.GetInstallState()
	svc.realtime.Broadcast("installer.status", state)
}

func (svc *InstallerService) broadcastProgress(downloaded, total int64) {
	speed := int64(0)
	svc.realtime.Broadcast("installer.progress", map[string]any{
		"percent":          float64(downloaded) / float64(total) * 100,
		"downloaded_bytes": downloaded,
		"total_bytes":      total,
		"speed_bps":        speed,
	})
}
