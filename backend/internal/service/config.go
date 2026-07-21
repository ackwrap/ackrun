package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

type ConfigService struct {
	paths           *paths.Paths
	store           *store.Store
	realtime        *RealtimeService
	configValidator func(string) error
	validationMu    sync.Mutex
	validationCache map[string]configValidationCacheEntry
}

type configValidationCacheEntry struct {
	modTimeNS int64
	size      int64
	err       error
}

var (
	ErrConfigFileNotFound = errors.New("配置文件不存在")
	ErrConfigFileInvalid  = errors.New("配置文件校验失败")
)

var configFileMu sync.Mutex

func NewConfigService(p *paths.Paths, s *store.Store, rt *RealtimeService) *ConfigService {
	return &ConfigService{paths: p, store: s, realtime: rt}
}

type MinimalConfig struct {
	Log       MinimalLog        `json:"log"`
	Inbounds  []MinimalInbound  `json:"inbounds"`
	Outbounds []MinimalOutbound `json:"outbounds"`
	Route     MinimalRoute      `json:"route"`
}

type MinimalLog struct {
	Level string `json:"level"`
}

type MinimalInbound struct {
	Type       string `json:"type"`
	Tag        string `json:"tag"`
	Listen     string `json:"listen"`
	ListenPort int    `json:"listen_port"`
}

type MinimalOutbound struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds,omitempty"`
}

type MinimalRoute struct {
	Rules []map[string]interface{} `json:"rules"`
}

func (svc *ConfigService) HasConfig() (bool, error) {
	_, ok, err := svc.paths.ActiveConfigPath()
	return ok, err
}

func (svc *ConfigService) GetConfigStatus() (*model.ConfigStatusResponse, error) {
	return svc.getConfigStatus(true)
}

func (svc *ConfigService) GetConfigStatusMetadata() (*model.ConfigStatusResponse, error) {
	return svc.getConfigStatus(false)
}

func (svc *ConfigService) getConfigStatus(validate bool) (*model.ConfigStatusResponse, error) {
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, err
	}
	if !ok {
		return &model.ConfigStatusResponse{HasConfig: false}, nil
	}

	info, err := os.Stat(configPath)
	if err != nil {
		return nil, err
	}

	status := &model.ConfigStatusResponse{
		HasConfig: true,
		Validated: validate,
		FileName:  filepath.Base(configPath),
		UpdatedAt: info.ModTime().UnixMilli(),
	}

	if !validate {
		return status, nil
	}
	if err := svc.validateFileCached(configPath, info); err != nil {
		status.Valid = false
		return status, nil
	}
	status.Valid = true
	return status, nil
}

func (svc *ConfigService) ListConfigFiles() ([]model.ConfigFileItem, error) {
	return svc.listConfigFiles(true)
}

func (svc *ConfigService) ListConfigFilesMetadata() ([]model.ConfigFileItem, error) {
	return svc.listConfigFiles(false)
}

func (svc *ConfigService) listConfigFiles(validate bool) ([]model.ConfigFileItem, error) {
	logging.Info("config.list", "listing config files")
	if err := os.MkdirAll(svc.paths.ConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	activePath, hasActive, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(svc.paths.ConfigDir)
	if err != nil {
		return nil, err
	}
	items := make([]model.ConfigFileItem, 0)
	for _, entry := range entries {
		if entry.IsDir() || !paths.IsConfigFileName(entry.Name()) {
			continue
		}
		path := filepath.Join(svc.paths.ConfigDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		item := model.ConfigFileItem{
			Name:      entry.Name(),
			Path:      path,
			Active:    hasActive && filepath.Clean(path) == filepath.Clean(activePath),
			SizeBytes: info.Size(),
			UpdatedAt: info.ModTime().UnixMilli(),
			Validated: validate,
		}
		if validate {
			item.Valid = true
			if err := svc.validateFileCached(path, info); err != nil {
				item.Valid = false
				item.Error = err.Error()
			}
		}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Active != items[j].Active {
			return items[i].Active
		}
		return items[i].UpdatedAt > items[j].UpdatedAt
	})
	return items, nil
}

func (svc *ConfigService) SetActiveConfig(fileName string) (*model.ConfigStatusResponse, error) {
	requestedName := strings.TrimSpace(fileName)
	normalizedName, err := normalizeConfigFileName(requestedName)
	if err != nil {
		return nil, err
	}
	if filepath.Ext(requestedName) == "" {
		requestedName = normalizedName
	}

	configFileMu.Lock()
	defer configFileMu.Unlock()
	targetPath := filepath.Join(svc.paths.ConfigDir, requestedName)
	info, err := os.Stat(targetPath)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("%w: %s", ErrConfigFileNotFound, requestedName)
	}
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: %s", ErrConfigFileNotFound, requestedName)
	}
	validator := svc.validateFile
	if svc.configValidator != nil {
		validator = svc.configValidator
	}
	if err := validator(targetPath); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrConfigFileInvalid, err)
	}
	if err := writeActiveConfigMarker(svc.paths, targetPath); err != nil {
		return nil, fmt.Errorf("设置当前配置失败: %w", err)
	}
	status := &model.ConfigStatusResponse{
		HasConfig: true,
		Validated: true,
		Valid:     true,
		FileName:  requestedName,
		UpdatedAt: info.ModTime().UnixMilli(),
	}
	logging.Info("config.active", "当前配置已切换: %s", requestedName)
	if svc.realtime != nil {
		svc.realtime.Broadcast("config.status", status)
	}
	return status, nil
}

func (svc *ConfigService) GenerateDefault() error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	logging.Info("config.generate", "generating minimal config")
	if err := os.MkdirAll(svc.paths.ConfigDir, 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	cfg := MinimalConfig{
		Log: MinimalLog{Level: "info"},
		Inbounds: []MinimalInbound{
			{
				Type:       "mixed",
				Tag:        "mixed-in",
				Listen:     "0.0.0.0",
				ListenPort: model.DefaultMixedInboundPort,
			},
		},
		Outbounds: []MinimalOutbound{
			{Type: "direct", Tag: "direct"},
			{Type: "selector", Tag: "proxy", Outbounds: []string{"direct"}},
		},
		Route: MinimalRoute{Rules: []map[string]interface{}{}},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := svc.paths.ConfigPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	defer os.Remove(tmpPath)

	validator := svc.validateFile
	if svc.configValidator != nil {
		validator = svc.configValidator
	}
	if err := validator(tmpPath); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}
	if _, err := os.Stat(svc.paths.ConfigPath); err == nil {
		if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, svc.paths.ConfigPath, time.Now()); err != nil {
			return fmt.Errorf("backup current config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check current config: %w", err)
	}

	if err := atomicReplaceFile(tmpPath, svc.paths.ConfigPath); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}
	if err := writeActiveConfigMarker(svc.paths, svc.paths.ConfigPath); err != nil {
		return fmt.Errorf("config generated but activation failed: %w", err)
	}

	logging.Info("config.generate", "config generated successfully")

	svc.realtime.Broadcast("config.status", model.ConfigStatusResponse{
		HasConfig: true,
		Validated: true,
		Valid:     true,
		FileName:  filepath.Base(svc.paths.ConfigPath),
		UpdatedAt: time.Now().UnixMilli(),
	})
	version := ""
	if st, err := svc.store.GetInstallState(); err == nil {
		version = st.Version
	}
	svc.realtime.Broadcast("runtime.status", model.RuntimeResponse{Status: model.RuntimeStopped, Version: version})

	return nil
}

func (svc *ConfigService) Validate() error {
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no config file found")
	}
	info, err := os.Stat(configPath)
	if err != nil {
		return err
	}
	return svc.validateFileAndCache(configPath, info)
}

func (svc *ConfigService) UpdateRules() (*model.ActionResponse, error) {
	logging.Info("config.rules_update", "rule update requested")
	status, err := svc.GetConfigStatus()
	if err != nil {
		return nil, err
	}
	if !status.HasConfig {
		return nil, fmt.Errorf("no config file found")
	}
	return &model.ActionResponse{Success: true, Message: "no rule sets configured"}, nil
}

func (svc *ConfigService) Backup() (*model.ActionResponse, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	logging.Info("config.backup", "backing up config")
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("no config file found")
	}

	backup, created, err := ensureDailyConfigBackup(svc.paths, svc.store, configPath, time.Now())
	if err != nil {
		return nil, fmt.Errorf("backup config: %w", err)
	}
	message := "today's config backup already exists"
	if created {
		message = "config backed up: " + backup.FileName
	}
	return &model.ActionResponse{Success: true, Message: message}, nil
}

func (svc *ConfigService) ListBackups() ([]model.ConfigBackup, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()
	logging.Info("config.backup_list", "listing config backups")
	return syncConfigBackups(svc.paths, svc.store)
}

func (svc *ConfigService) RestoreLatestBackup() (*model.ActionResponse, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	logging.Info("config.restore", "restoring latest config backup")
	backups, err := syncConfigBackups(svc.paths, svc.store)
	if err != nil {
		return nil, err
	}
	if len(backups) == 0 {
		return nil, fmt.Errorf("no config backup found")
	}
	backup := backups[0]

	data, err := os.ReadFile(backup.Path)
	if err != nil {
		return nil, fmt.Errorf("read backup: %w", err)
	}
	version := readSingboxVersion(svc.paths.BinaryPath)
	data, migrated, err := migrateInlineACMEConfig(data, version)
	if err != nil {
		return nil, fmt.Errorf("migrate backup config: %w", err)
	}
	if migrated > 0 {
		logging.Info("config.migrate", "恢复备份时已迁移 %d 个 inline ACME 配置，核心版本: %s", migrated, version)
	}
	if err := os.MkdirAll(svc.paths.ConfigDir, 0755); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	targetPath := filepath.Join(svc.paths.ConfigDir, backup.ConfigName)
	stagedFile, err := os.CreateTemp(svc.paths.ConfigDir, ".ackwrap-restore-*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create restore temp config: %w", err)
	}
	tmpPath := stagedFile.Name()
	if _, err := stagedFile.Write(data); err != nil {
		stagedFile.Close()
		os.Remove(tmpPath)
		return nil, fmt.Errorf("write temp config: %w", err)
	}
	if err := stagedFile.Close(); err != nil {
		os.Remove(tmpPath)
		return nil, fmt.Errorf("close temp config: %w", err)
	}
	defer os.Remove(tmpPath)
	if err := svc.validateFile(tmpPath); err != nil {
		return nil, fmt.Errorf("backup config invalid: %w", err)
	}
	if _, err := os.Stat(targetPath); err == nil {
		if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, targetPath, time.Now()); err != nil {
			return nil, fmt.Errorf("backup current config before restore: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("check restore target: %w", err)
	}
	if err := atomicReplaceFile(tmpPath, targetPath); err != nil {
		return nil, fmt.Errorf("restore config: %w", err)
	}
	if err := writeActiveConfigMarker(svc.paths, targetPath); err != nil {
		return nil, fmt.Errorf("config restored but activation failed: %w", err)
	}

	status, _ := svc.GetConfigStatus()
	if status != nil && svc.realtime != nil {
		svc.realtime.Broadcast("config.status", status)
	}
	return &model.ActionResponse{Success: true, Message: "config restored"}, nil
}

func (svc *ConfigService) validateFile(path string) error {
	if svc.configValidator != nil {
		if err := svc.configValidator(path); err != nil {
			return err
		}
		logging.Info("config.validate", "config validated: %s", path)
		return nil
	}
	binPath := svc.paths.BinaryPath
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("sing-box binary not found")
	}

	cmd := exec.Command(binPath, "check", "-c", path)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sing-box check failed: %s: %w", strings.TrimSpace(cleanLogLine(string(output))), err)
	}

	logging.Info("config.validate", "config validated: %s", path)
	return nil
}

func (svc *ConfigService) validateFileCached(path string, info os.FileInfo) error {
	svc.validationMu.Lock()
	defer svc.validationMu.Unlock()

	key := filepath.Clean(path)
	if cached, ok := svc.validationCache[key]; ok && cached.modTimeNS == info.ModTime().UnixNano() && cached.size == info.Size() {
		return cached.err
	}
	return svc.validateFileAndCacheLocked(key, path, info)
}

func (svc *ConfigService) validateFileAndCache(path string, info os.FileInfo) error {
	svc.validationMu.Lock()
	defer svc.validationMu.Unlock()
	return svc.validateFileAndCacheLocked(filepath.Clean(path), path, info)
}

func (svc *ConfigService) validateFileAndCacheLocked(key, path string, info os.FileInfo) error {
	err := svc.validateFile(path)
	if svc.validationCache == nil {
		svc.validationCache = make(map[string]configValidationCacheEntry)
	}
	svc.validationCache[key] = configValidationCacheEntry{
		modTimeNS: info.ModTime().UnixNano(),
		size:      info.Size(),
		err:       err,
	}
	return err
}

func getConfigDir(p *paths.Paths) string {
	return p.ConfigDir
}
