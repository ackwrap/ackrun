package service

import (
	"encoding/json"
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
	paths    *paths.Paths
	store    *store.Store
	realtime *RealtimeService
}

var configFileMu sync.Mutex

func NewConfigService(p *paths.Paths, s *store.Store, rt *RealtimeService) *ConfigService {
	return &ConfigService{paths: p, store: s, realtime: rt}
}

type MinimalConfig struct {
	Log       MinimalLog        `json:"log"`
	Inbounds  []MinimalInbound  `json:"inbounds"`
	Outbounds []MinimalOutbound `json:"outbounds"`
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
	Type string `json:"type"`
	Tag  string `json:"tag"`
}

type configBackup struct {
	path      string
	updatedAt time.Time
}

func (svc *ConfigService) HasConfig() (bool, error) {
	_, ok, err := svc.paths.ActiveConfigPath()
	return ok, err
}

func (svc *ConfigService) GetConfigStatus() (*model.ConfigStatusResponse, error) {
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
		FileName:  filepath.Base(configPath),
		UpdatedAt: info.ModTime().UnixMilli(),
	}

	if err := svc.validateFile(configPath); err != nil {
		status.Valid = false
		return status, nil
	}
	status.Valid = true
	return status, nil
}

func (svc *ConfigService) ListConfigFiles() ([]model.ConfigFileItem, error) {
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
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
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
			Valid:     true,
		}
		if err := svc.validateFile(path); err != nil {
			item.Valid = false
			item.Error = err.Error()
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
				Listen:     "127.0.0.1",
				ListenPort: 2080,
			},
		},
		Outbounds: []MinimalOutbound{
			{Type: "direct", Tag: "direct"},
		},
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	tmpPath := svc.paths.ConfigPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("write temp config: %w", err)
	}
	defer os.Remove(tmpPath)

	if err := svc.validateFile(tmpPath); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	if err := atomicReplaceFile(tmpPath, svc.paths.ConfigPath); err != nil {
		return fmt.Errorf("rename config: %w", err)
	}

	logging.Info("config.generate", "config generated successfully")

	svc.realtime.Broadcast("config.status", model.ConfigStatusResponse{
		HasConfig: true,
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
	status, err := svc.GetConfigStatus()
	if err != nil {
		return err
	}
	if !status.HasConfig {
		return fmt.Errorf("no config file found")
	}
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no config file found")
	}
	return svc.validateFile(configPath)
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

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	backupDir := filepath.Join(svc.paths.ConfigDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	name := fmt.Sprintf("%s.%s.bak.json", filepath.Base(configPath), time.Now().Format("20060102150405"))
	backupPath := filepath.Join(backupDir, name)
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write backup: %w", err)
	}
	return &model.ActionResponse{Success: true, Message: "config backed up"}, nil
}

func (svc *ConfigService) RestoreLatestBackup() (*model.ActionResponse, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	logging.Info("config.restore", "restoring latest config backup")
	backupDir := filepath.Join(svc.paths.ConfigDir, "backup")
	entries, err := os.ReadDir(backupDir)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("no config backup found")
	}
	if err != nil {
		return nil, err
	}

	backupPath, ok := latestConfigBackup(backupDir, entries)
	if !ok {
		return nil, fmt.Errorf("no config backup found")
	}

	data, err := os.ReadFile(backupPath)
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
	tmpPath := svc.paths.ConfigPath + ".restore.tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return nil, fmt.Errorf("write temp config: %w", err)
	}
	defer os.Remove(tmpPath)
	if err := svc.validateFile(tmpPath); err != nil {
		return nil, fmt.Errorf("backup config invalid: %w", err)
	}
	if err := atomicReplaceFile(tmpPath, svc.paths.ConfigPath); err != nil {
		return nil, fmt.Errorf("restore config: %w", err)
	}

	status, _ := svc.GetConfigStatus()
	if status != nil {
		svc.realtime.Broadcast("config.status", status)
	}
	return &model.ActionResponse{Success: true, Message: "config restored"}, nil
}

func latestConfigBackup(backupDir string, entries []os.DirEntry) (string, bool) {
	backups := make([]configBackup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, configBackup{
			path:      filepath.Join(backupDir, entry.Name()),
			updatedAt: info.ModTime(),
		})
	}
	if len(backups) == 0 {
		return "", false
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].updatedAt.Equal(backups[j].updatedAt) {
			return backups[i].path < backups[j].path
		}
		return backups[i].updatedAt.Before(backups[j].updatedAt)
	})
	return backups[len(backups)-1].path, true
}

func (svc *ConfigService) validateFile(path string) error {
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

func getConfigDir(p *paths.Paths) string {
	return p.ConfigDir
}
