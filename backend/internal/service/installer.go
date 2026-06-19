package service

import (
	"archive/zip"
	"encoding/json"
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
	singboxVersion    = "1.13.13"
	singboxVersionURL = "https://api.github.com/repos/SagerNet/sing-box/releases/latest"
)

type InstallerService struct {
	store    *store.Store
	paths    *paths.Paths
	realtime *RealtimeService
	mu       sync.Mutex
	busy     bool
}

func NewInstallerService(s *store.Store, p *paths.Paths, rt *RealtimeService) *InstallerService {
	return &InstallerService{store: s, paths: p, realtime: rt}
}

func (svc *InstallerService) GetStatus() (*model.InstallStateResponse, error) {
	state, err := svc.store.GetInstallState()
	if err != nil {
		return nil, err
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
	latestVersion, err := svc.fetchLatestVersion()
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

func (svc *InstallerService) Install() (*model.ActionResponse, error) {
	svc.mu.Lock()
	if svc.busy {
		svc.mu.Unlock()
		return nil, fmt.Errorf("installation already in progress")
	}
	svc.busy = true
	svc.mu.Unlock()

	defer func() {
		svc.mu.Lock()
		svc.busy = false
		svc.mu.Unlock()
	}()

	go svc.runInstall()

	return &model.ActionResponse{Success: true, Message: "install started"}, nil
}

func (svc *InstallerService) runInstall() {
	logging.Info("installer.start", "starting sing-box installation")

	// 获取最新版本
	latestVersion, err := svc.fetchLatestVersion()
	if err != nil {
		logging.Info("installer.version", "failed to fetch latest version, using fallback %s: %v", singboxVersion, err)
	} else if latestVersion != "" {
		singboxVersion = latestVersion
		logging.Info("installer.version", "using latest version: %s", singboxVersion)
	}

	svc.setState(model.InstallDownloading, "downloading", 0, "", "")
	svc.broadcastStatus()

	url, err := svc.buildDownloadURL()
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

	svc.setState(model.InstallDone, singboxVersion, 0, "installed", "")
	svc.broadcastStatus()

	svc.realtime.Broadcast("runtime.status", model.RuntimeResponse{Status: model.RuntimeNoConfig, Version: singboxVersion})
	logging.Info("installer.start", "sing-box installed successfully, version=%s", singboxVersion)
}

func (svc *InstallerService) fetchLatestVersion() (string, error) {
	// 尝试多个 API 源
	urls := []string{
		"https://api.github.com/repos/SagerNet/sing-box/releases/latest",
		"https://api.github.com/repos/sagernet/sing-box/releases/latest", // 小写尝试
	}

	client := &http.Client{Timeout: 10 * time.Second}

	for _, url := range urls {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			continue
		}

		req.Header.Set("User-Agent", "Ackwrap/1.0")
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			continue
		}

		var release struct {
			TagName string `json:"tag_name"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
			continue
		}

		version := strings.TrimPrefix(release.TagName, "v")
		if version != "" {
			return version, nil
		}
	}

	return "", fmt.Errorf("all API sources failed")
}

func (svc *InstallerService) buildDownloadURL() (string, error) {
	goos := runtime.GOOS
	arch := runtime.GOARCH

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

	name := fmt.Sprintf("sing-box-%s-%s-%s", singboxVersion, goos, archStr)
	url := fmt.Sprintf("https://github.com/SagerNet/sing-box/releases/download/v%s/%s%s", singboxVersion, name, ext)
	return url, nil
}

func (svc *InstallerService) download(url, dest string) error {
	logging.Info("installer.download", "downloading from: %s", url)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("http get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http status: %d", resp.StatusCode)
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
	return fmt.Errorf("tar.gz extraction not yet implemented on this platform")
}

func (svc *InstallerService) extractZip(archive string) error {
	r, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if strings.HasPrefix(name, "sing-box") && !f.FileInfo().IsDir() {
			rc, err := f.Open()
			if err != nil {
				return fmt.Errorf("open entry: %w", err)
			}

			outPath := filepath.Join(svc.paths.BinaryDir, name)
			out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0755)
			if err != nil {
				rc.Close()
				return fmt.Errorf("create binary: %w", err)
			}

			_, err = io.Copy(out, rc)
			out.Close()
			rc.Close()
			if err != nil {
				return fmt.Errorf("copy binary: %w", err)
			}

			logging.Info("installer.extract", "extracted: %s", outPath)
		}
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
