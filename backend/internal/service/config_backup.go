package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

type configBackupCandidate struct {
	configName string
	path       string
	updatedAt  time.Time
	sizeBytes  int64
}

func syncConfigBackups(p *paths.Paths, db *store.Store) ([]model.ConfigBackup, error) {
	backupDir := filepath.Join(p.ConfigDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("创建配置备份目录失败: %w", err)
	}
	candidates, err := discoverConfigBackups(p.ConfigDir, backupDir)
	if err != nil {
		return nil, err
	}
	groups := make(map[string][]configBackupCandidate)
	for _, candidate := range candidates {
		date := candidate.updatedAt.In(time.Local).Format("2006-01-02")
		key := configBackupNameKey(candidate.configName) + "\x00" + date
		groups[key] = append(groups[key], candidate)
	}

	backups := make([]model.ConfigBackup, 0, len(groups))
	removed := 0
	moved := 0
	for _, group := range groups {
		sort.Slice(group, func(i, j int) bool {
			if group[i].updatedAt.Equal(group[j].updatedAt) {
				return group[i].path < group[j].path
			}
			return group[i].updatedAt.Before(group[j].updatedAt)
		})
		keeper := group[0]
		for _, duplicate := range group[1:] {
			if err := os.Remove(duplicate.path); err != nil && !os.IsNotExist(err) {
				return nil, fmt.Errorf("删除同日重复备份失败: %w", err)
			}
			removed++
		}

		date := keeper.updatedAt.In(time.Local).Format("2006-01-02")
		canonicalPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak.json", keeper.configName, strings.ReplaceAll(date, "-", "")))
		if filepath.Clean(keeper.path) != filepath.Clean(canonicalPath) {
			if _, err := os.Stat(canonicalPath); err == nil {
				return nil, fmt.Errorf("迁移备份失败，目标文件已存在: %s", filepath.Base(canonicalPath))
			} else if !os.IsNotExist(err) {
				return nil, err
			}
			if err := os.Rename(keeper.path, canonicalPath); err != nil {
				return nil, fmt.Errorf("迁移备份到独立目录失败: %w", err)
			}
			keeper.path = canonicalPath
			moved++
		}
		if err := os.Chmod(keeper.path, 0600); err != nil {
			return nil, fmt.Errorf("保护配置备份失败: %w", err)
		}
		info, err := os.Stat(keeper.path)
		if err != nil {
			return nil, err
		}
		backups = append(backups, model.ConfigBackup{
			ConfigName: keeper.configName,
			FileName:   filepath.Base(keeper.path),
			Path:       keeper.path,
			BackupDate: date,
			SizeBytes:  info.Size(),
			CreatedAt:  keeper.updatedAt.UnixMilli(),
		})
	}
	sort.Slice(backups, func(i, j int) bool {
		if backups[i].BackupDate == backups[j].BackupDate {
			if backups[i].CreatedAt != backups[j].CreatedAt {
				return backups[i].CreatedAt > backups[j].CreatedAt
			}
			return backups[i].ConfigName < backups[j].ConfigName
		}
		return backups[i].BackupDate > backups[j].BackupDate
	})
	if db != nil {
		if err := db.ReplaceConfigBackups(backups); err != nil {
			return nil, fmt.Errorf("同步配置备份索引失败: %w", err)
		}
		indexed, err := db.ListConfigBackups()
		if err != nil {
			return nil, fmt.Errorf("读取配置备份索引失败: %w", err)
		}
		backups = indexed
	}
	if removed > 0 || moved > 0 {
		logging.Info("config.backup", "备份整理完成: 迁移 %d 份，删除同日重复 %d 份", moved, removed)
	}
	return backups, nil
}

func discoverConfigBackups(configDir, backupDir string) ([]configBackupCandidate, error) {
	candidates := make([]configBackupCandidate, 0)
	for _, source := range []struct {
		dir  string
		root bool
	}{{dir: configDir, root: true}, {dir: backupDir}} {
		entries, err := os.ReadDir(source.dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			configName, ok := configNameFromBackup(entry.Name(), source.root)
			if !ok {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, configBackupCandidate{
				configName: configName,
				path:       filepath.Join(source.dir, entry.Name()),
				updatedAt:  info.ModTime(),
				sizeBytes:  info.Size(),
			})
		}
	}
	return candidates, nil
}

func configNameFromBackup(name string, root bool) (string, bool) {
	lower := strings.ToLower(name)
	if root && strings.HasPrefix(lower, "config.backup.") && strings.HasSuffix(lower, ".json") {
		return "config.json", true
	}
	if !strings.HasSuffix(lower, ".bak.json") {
		return "", false
	}
	index := strings.LastIndex(lower, ".json.")
	if index < 0 {
		return "", false
	}
	configName := name[:index+len(".json")]
	if !paths.IsConfigFileName(configName) {
		return "", false
	}
	return configName, true
}

func configBackupNameKey(name string) string {
	if runtime.GOOS == "windows" {
		return strings.ToLower(name)
	}
	return name
}

func sameConfigBackupName(left, right string) bool {
	return configBackupNameKey(left) == configBackupNameKey(right)
}

func ensureDailyConfigBackup(p *paths.Paths, db *store.Store, configPath string, now time.Time) (model.ConfigBackup, bool, error) {
	backups, err := syncConfigBackups(p, db)
	if err != nil {
		return model.ConfigBackup{}, false, err
	}
	configName := filepath.Base(configPath)
	date := now.In(time.Local).Format("2006-01-02")
	for _, backup := range backups {
		if sameConfigBackupName(backup.ConfigName, configName) && backup.BackupDate == date {
			return backup, false, nil
		}
	}

	backupDir := filepath.Join(p.ConfigDir, "backup")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("%s.%s.bak.json", configName, now.In(time.Local).Format("20060102")))
	stagedFile, err := os.CreateTemp(backupDir, ".ackwrap-backup-*.tmp")
	if err != nil {
		return model.ConfigBackup{}, false, err
	}
	stagedPath := stagedFile.Name()
	if err := stagedFile.Close(); err != nil {
		os.Remove(stagedPath)
		return model.ConfigBackup{}, false, err
	}
	defer os.Remove(stagedPath)
	if err := copyFile(configPath, stagedPath); err != nil {
		return model.ConfigBackup{}, false, err
	}
	if err := atomicReplaceFile(stagedPath, backupPath); err != nil {
		return model.ConfigBackup{}, false, err
	}
	if err := os.Chtimes(backupPath, now, now); err != nil {
		return model.ConfigBackup{}, false, err
	}

	backups, err = syncConfigBackups(p, db)
	if err != nil {
		return model.ConfigBackup{}, false, err
	}
	for _, backup := range backups {
		if sameConfigBackupName(backup.ConfigName, configName) && backup.BackupDate == date {
			return backup, true, nil
		}
	}
	return model.ConfigBackup{}, false, fmt.Errorf("配置备份创建后未找到索引记录")
}
