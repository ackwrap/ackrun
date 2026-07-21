package service

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

const (
	dashboardArchiveMaxSize   = 64 << 20
	dashboardExtractedMaxSize = 128 << 20
	dashboardMaxFiles         = 10000
)

var (
	ErrDashboardNotFound     = errors.New("控制面板不存在")
	ErrDashboardInUse        = errors.New("当前控制面板正在使用，不能删除")
	ErrDashboardNotInstalled = errors.New("控制面板尚未安装")
)

type dashboardCatalogItem struct {
	ID          string
	Name        string
	Description string
	Owner       string
	Repository  string
	Branch      string
}

var dashboardCatalog = []dashboardCatalogItem{
	{ID: "metacubexd", Name: "MetaCubeXD", Description: "现代化 Mihomo/sing-box 控制面板", Owner: "MetaCubeX", Repository: "metacubexd", Branch: "gh-pages"},
	{ID: "yacd", Name: "Yacd-meta", Description: "轻量经典控制面板", Owner: "MetaCubeX", Repository: "Yacd-meta", Branch: "gh-pages"},
	{ID: "zashboard", Name: "Zashboard", Description: "面向桌面与移动端的现代控制面板", Owner: "Zephyruso", Repository: "zashboard", Branch: "gh-pages"},
}

type dashboardVersion struct {
	Commit    string
	UpdatedAt time.Time
}

type dashboardMetadata struct {
	Commit      string `json:"commit"`
	UpdatedAt   string `json:"updated_at"`
	InstalledAt int64  `json:"installed_at"`
}

type DashboardService struct {
	store     *store.Store
	paths     *paths.Paths
	installMu sync.Mutex
}

func NewDashboardService(db *store.Store, p *paths.Paths) *DashboardService {
	return &DashboardService{store: db, paths: p}
}

func (svc *DashboardService) List() ([]model.Dashboard, error) {
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return nil, err
	}
	selectedPath := ""
	if settings != nil {
		selectedPath = filepath.Clean(settings.ClashAPIExternalUI)
	}
	items := make([]model.Dashboard, 0, len(dashboardCatalog))
	for _, catalogItem := range dashboardCatalog {
		localPath := filepath.Join(svc.paths.DashboardsDir, catalogItem.ID)
		info, statErr := os.Stat(filepath.Join(localPath, "index.html"))
		installed := statErr == nil && !info.IsDir()
		if statErr != nil && !os.IsNotExist(statErr) {
			return nil, statErr
		}
		item := model.Dashboard{
			ID:          catalogItem.ID,
			Name:        catalogItem.Name,
			Description: catalogItem.Description,
			Installed:   installed,
		}
		if installed {
			item.LocalPath = localPath
			item.UpdatedAt = info.ModTime().Unix()
			item.Selected = selectedPath != "." && sameDashboardPath(selectedPath, filepath.Clean(localPath))
			if metadata, err := readDashboardMetadata(localPath); err == nil {
				item.CurrentVersion = shortDashboardCommit(metadata.Commit)
				if metadata.InstalledAt > 0 {
					item.UpdatedAt = metadata.InstalledAt
				}
			}
		}
		items = append(items, item)
	}
	return items, nil
}

func (svc *DashboardService) CheckUpdates(ctx context.Context) ([]model.Dashboard, error) {
	items, err := svc.List()
	if err != nil {
		return nil, err
	}
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return nil, fmt.Errorf("读取更新代理设置失败: %w", err)
	}
	for i := range items {
		catalogItem := findDashboardCatalogItem(items[i].ID)
		version, err := fetchLatestDashboardVersion(ctx, settings, catalogItem)
		if err != nil {
			items[i].CheckError = err.Error()
			continue
		}
		items[i].LatestVersion = shortDashboardCommit(version.Commit)
		items[i].UpdateAvailable = !items[i].Installed || items[i].CurrentVersion == "" || items[i].CurrentVersion != items[i].LatestVersion
	}
	logging.Info("dashboard.check", "dashboard update check completed")
	return items, nil
}

func (svc *DashboardService) Install(ctx context.Context, id string) (*model.Dashboard, error) {
	svc.installMu.Lock()
	defer svc.installMu.Unlock()
	catalogItem := findDashboardCatalogItem(id)
	if catalogItem == nil {
		return nil, ErrDashboardNotFound
	}
	settings, err := svc.store.GetUpdateSettings()
	if err != nil {
		return nil, fmt.Errorf("读取更新代理设置失败: %w", err)
	}
	version, err := fetchLatestDashboardVersion(ctx, settings, catalogItem)
	if err != nil {
		return nil, err
	}
	archiveURL := fmt.Sprintf("https://github.com/%s/%s/archive/%s.zip", catalogItem.Owner, catalogItem.Repository, version.Commit)
	attempts, err := buildAppUpdateRequestAttempts(settings, archiveURL)
	if err != nil {
		return nil, err
	}
	for i := range attempts {
		client := *attempts[i].client
		client.Timeout = 3 * time.Minute
		attempts[i].client = &client
	}
	archive, err := os.CreateTemp(svc.paths.DownloadsDir, "dashboard-*.zip")
	if err != nil {
		return nil, fmt.Errorf("创建控制面板临时文件失败: %w", err)
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		os.Remove(archivePath)
		return nil, err
	}
	defer os.Remove(archivePath)

	logging.Info("dashboard.install", "downloading dashboard id=%s through configured update proxy", catalogItem.ID)
	if err := downloadDashboardArchive(ctx, attempts, archivePath); err != nil {
		return nil, err
	}
	if err := svc.installArchive(catalogItem.ID, archivePath, version); err != nil {
		return nil, err
	}
	logging.Info("dashboard.install", "dashboard installed id=%s", catalogItem.ID)
	items, err := svc.List()
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, ErrDashboardNotFound
}

func (svc *DashboardService) Delete(id string) (*model.ActionResponse, error) {
	svc.installMu.Lock()
	defer svc.installMu.Unlock()
	catalogItem := findDashboardCatalogItem(id)
	if catalogItem == nil {
		return nil, ErrDashboardNotFound
	}
	targetPath := filepath.Join(svc.paths.DashboardsDir, catalogItem.ID)
	settings, err := svc.store.GetExperimentalSettings()
	if err != nil {
		return nil, err
	}
	if settings != nil && strings.TrimSpace(settings.ClashAPIExternalUI) != "" && sameDashboardPath(settings.ClashAPIExternalUI, targetPath) {
		return nil, ErrDashboardInUse
	}
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		return nil, ErrDashboardNotInstalled
	} else if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(targetPath); err != nil {
		return nil, fmt.Errorf("删除控制面板失败: %w", err)
	}
	logging.Info("dashboard.delete", "dashboard deleted id=%s", catalogItem.ID)
	return &model.ActionResponse{Success: true, Message: "dashboard deleted"}, nil
}

func findDashboardCatalogItem(id string) *dashboardCatalogItem {
	id = strings.TrimSpace(strings.ToLower(id))
	for i := range dashboardCatalog {
		if dashboardCatalog[i].ID == id {
			return &dashboardCatalog[i]
		}
	}
	return nil
}

func downloadDashboardArchive(ctx context.Context, attempts []updateRequestAttempt, destination string) error {
	var lastErr error
	for _, attempt := range attempts {
		if err := downloadDashboardArchiveOnce(ctx, attempt.client, attempt.url, destination); err == nil {
			return nil
		} else {
			lastErr = err
			logging.Info("dashboard.download", "download attempt failed: %s: %v", attempt.name, err)
		}
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的控制面板下载代理")
	}
	return lastErr
}

func downloadDashboardArchiveOnce(ctx context.Context, client *http.Client, rawURL, destination string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Ackwrap dashboard manager")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("下载控制面板失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("下载控制面板返回 HTTP %d", resp.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(resp.Body, dashboardArchiveMaxSize+1))
	closeErr := file.Close()
	if copyErr != nil {
		return fmt.Errorf("保存控制面板失败: %w", copyErr)
	}
	if closeErr != nil {
		return closeErr
	}
	if written > dashboardArchiveMaxSize {
		return fmt.Errorf("控制面板归档超过 %d 字节限制", dashboardArchiveMaxSize)
	}
	return nil
}

func (svc *DashboardService) installArchive(id, archivePath string, version *dashboardVersion) error {
	workDir, err := os.MkdirTemp(svc.paths.DashboardsDir, "."+id+"-tmp-")
	if err != nil {
		return fmt.Errorf("创建控制面板解压目录失败: %w", err)
	}
	keepWorkDir := false
	defer func() {
		if !keepWorkDir {
			os.RemoveAll(workDir)
		}
	}()
	if err := extractDashboardArchive(archivePath, workDir); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(workDir, "index.html")); err != nil {
		return fmt.Errorf("控制面板归档缺少 index.html")
	}
	metadata := dashboardMetadata{Commit: version.Commit, UpdatedAt: version.UpdatedAt.UTC().Format(time.RFC3339), InstalledAt: time.Now().Unix()}
	metadataData, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(workDir, ".ackwrap-dashboard.json"), metadataData, 0644); err != nil {
		return fmt.Errorf("写入控制面板版本信息失败: %w", err)
	}

	targetPath := filepath.Join(svc.paths.DashboardsDir, id)
	backupPath := targetPath + ".old"
	if err := os.RemoveAll(backupPath); err != nil {
		return err
	}
	hadTarget := false
	if _, err := os.Stat(targetPath); err == nil {
		if err := os.Rename(targetPath, backupPath); err != nil {
			return fmt.Errorf("备份旧控制面板失败: %w", err)
		}
		hadTarget = true
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(workDir, targetPath); err != nil {
		if hadTarget {
			os.Rename(backupPath, targetPath)
		}
		return fmt.Errorf("应用控制面板失败: %w", err)
	}
	keepWorkDir = true
	if err := os.RemoveAll(backupPath); err != nil {
		logging.Info("dashboard.install", "remove old dashboard backup failed id=%s: %v", id, err)
	}
	return nil
}

func fetchLatestDashboardVersion(ctx context.Context, settings *model.UpdateSettingsResponse, item *dashboardCatalogItem) (*dashboardVersion, error) {
	if item == nil {
		return nil, ErrDashboardNotFound
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/commits/%s", item.Owner, item.Repository, item.Branch)
	attempts, err := buildAppUpdateRequestAttempts(settings, apiURL)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, attempt := range attempts {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, attempt.url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("User-Agent", "Ackwrap dashboard manager")
		resp, err := attempt.client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		var payload struct {
			SHA    string `json:"sha"`
			Commit struct {
				Committer struct {
					Date time.Time `json:"date"`
				} `json:"committer"`
			} `json:"commit"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&payload)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("GitHub 提交接口返回 HTTP %d", resp.StatusCode)
			continue
		}
		if decodeErr != nil || len(payload.SHA) < 7 {
			lastErr = fmt.Errorf("解析控制面板版本失败")
			continue
		}
		return &dashboardVersion{Commit: payload.SHA, UpdatedAt: payload.Commit.Committer.Date}, nil
	}
	if lastErr == nil {
		lastErr = errors.New("没有可用的控制面板版本检查代理")
	}
	return nil, lastErr
}

func readDashboardMetadata(localPath string) (*dashboardMetadata, error) {
	data, err := os.ReadFile(filepath.Join(localPath, ".ackwrap-dashboard.json"))
	if err != nil {
		return nil, err
	}
	var metadata dashboardMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return nil, err
	}
	return &metadata, nil
}

func shortDashboardCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) > 7 {
		return commit[:7]
	}
	return commit
}

func sameDashboardPath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func extractDashboardArchive(archivePath, destination string) error {
	reader, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("打开控制面板归档失败: %w", err)
	}
	defer reader.Close()
	if len(reader.File) == 0 || len(reader.File) > dashboardMaxFiles {
		return fmt.Errorf("控制面板归档文件数量无效")
	}
	root := dashboardArchiveRoot(reader.File)
	var extracted int64
	for _, item := range reader.File {
		archiveName := strings.ReplaceAll(item.Name, "\\", "/")
		cleanArchiveName := pathpkg.Clean(archiveName)
		if pathpkg.IsAbs(cleanArchiveName) || cleanArchiveName == ".." || strings.HasPrefix(cleanArchiveName, "../") {
			return fmt.Errorf("控制面板归档包含非法路径")
		}
		name := strings.TrimPrefix(item.Name, root)
		name = strings.TrimPrefix(name, "/")
		if name == "" {
			continue
		}
		cleanName := filepath.Clean(filepath.FromSlash(name))
		if cleanName == "." || filepath.IsAbs(cleanName) || cleanName == ".." || strings.HasPrefix(cleanName, ".."+string(filepath.Separator)) {
			return fmt.Errorf("控制面板归档包含非法路径")
		}
		if item.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("控制面板归档不允许符号链接")
		}
		targetPath := filepath.Join(destination, cleanName)
		if item.FileInfo().IsDir() {
			if err := os.MkdirAll(targetPath, 0755); err != nil {
				return err
			}
			continue
		}
		extracted += int64(item.UncompressedSize64)
		if extracted > dashboardExtractedMaxSize {
			return fmt.Errorf("控制面板解压后超过 %d 字节限制", dashboardExtractedMaxSize)
		}
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		source, err := item.Open()
		if err != nil {
			return err
		}
		target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			source.Close()
			return err
		}
		_, copyErr := io.Copy(target, io.LimitReader(source, int64(item.UncompressedSize64)+1))
		closeTargetErr := target.Close()
		closeSourceErr := source.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeTargetErr != nil {
			return closeTargetErr
		}
		if closeSourceErr != nil {
			return closeSourceErr
		}
		if err := os.Chtimes(targetPath, time.Now(), item.Modified); err != nil {
			return err
		}
	}
	return nil
}

func dashboardArchiveRoot(items []*zip.File) string {
	root := ""
	for _, item := range items {
		name := strings.Trim(item.Name, "/")
		if name == "" || item.FileInfo().IsDir() {
			continue
		}
		parts := strings.SplitN(name, "/", 2)
		if len(parts) < 2 {
			return ""
		}
		if root == "" {
			root = parts[0]
		} else if root != parts[0] {
			return ""
		}
	}
	if root == "" {
		return ""
	}
	return root + "/"
}
