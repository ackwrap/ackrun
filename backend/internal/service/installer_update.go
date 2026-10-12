package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

func (svc *InstallerService) isCoreRunning() bool {
	svc.mu.Lock()
	isRunning := svc.coreRunning
	svc.mu.Unlock()
	return isRunning != nil && isRunning()
}

func validateCandidateCore(binary, config, version string) error {
	ctx, cancel := context.WithTimeout(context.Background(), coreStartupReadinessTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行候选核心失败: %w", err)
	}
	match := singboxVersionPattern.FindStringSubmatch(string(output))
	if len(match) < 2 || match[1] != version {
		return errors.New("候选核心版本与下载版本不一致")
	}
	if config == "" {
		return nil
	}
	output, err = exec.CommandContext(ctx, binary, "check", "-c", config).CombinedOutput()
	if err != nil {
		data, readErr := os.ReadFile(config)
		if readErr != nil {
			return fmt.Errorf("候选核心配置校验失败: %w", err)
		}
		var values interface{}
		_ = json.Unmarshal(data, &values)
		message := redactCoreCheckValues(strings.TrimSpace(cleanLogLine(string(output))), values)
		return fmt.Errorf("候选核心配置校验失败: %s: %w", message, err)
	}
	return nil
}

// In addition to credentials, a core diagnostic may quote node addresses.
func redactCoreCheckValues(message string, value interface{}) string {
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			switch key {
			case "server", "server_name", "address", "host", "url", "uuid", "public_key":
				message = redactConfigSecretValue(message, item)
			default:
				if isSensitiveConfigField(key) {
					message = redactConfigSecretValue(message, item)
				} else {
					message = redactCoreCheckValues(message, item)
				}
			}
		}
	case []interface{}:
		for _, item := range typed {
			message = redactCoreCheckValues(message, item)
		}
	}
	return message
}

// The snapshot, core lifecycle and config-file locks use the same order as
// config generation. Restart only the prepared config, never regenerate under
// these locks. Backups are retained until readiness and the final DB migration.
func (svc *InstallerService) installStagedUpdate(stagingDir string, files []stagedRuntimeFile, version string) error {
	releaseSnapshot := svc.store.HoldConfigSnapshot()
	defer releaseSnapshot()
	svc.mu.Lock()
	core, validate, replace := svc.coreService, svc.candidateValidator, svc.replaceFile
	svc.mu.Unlock()
	if core != nil {
		core.lifecycleMu.Lock()
		defer core.lifecycleMu.Unlock()
	}
	configFileMu.Lock()
	defer configFileMu.Unlock()
	if validate == nil {
		validate = validateCandidateCore
	}
	if replace == nil {
		replace = atomicReplaceFile
	}
	candidate := filepath.Join(stagingDir, "files", filepath.Base(svc.paths.BinaryPath))
	configPath, exists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	backupDir := filepath.Join(stagingDir, "backups")
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		return err
	}
	var original, migrated []byte
	var migrationCount int
	stagedConfig := ""
	configBackup := filepath.Join(backupDir, "active-config.json")
	if exists {
		original, err = os.ReadFile(configPath)
		if err != nil {
			return err
		}
		migrated, migrationCount, err = migrateCoreCompatibilityData(original, version)
		if err != nil {
			return fmt.Errorf("更新前配置兼容检查失败: %w", err)
		}
		stagedConfig = filepath.Join(stagingDir, "candidate-config.json")
		if err := os.WriteFile(stagedConfig, migrated, 0600); err != nil {
			return err
		}
		if err := os.WriteFile(configBackup, original, 0600); err != nil {
			return err
		}
	}
	logging.Info("installer.preflight", "验证候选核心及活动配置，版本=%s", version)
	if err := validate(candidate, stagedConfig, version); err != nil {
		return err
	}
	oldVersion := readSingboxVersion(svc.paths.BinaryPath)
	wasRunning, err := svc.stopCoreForUpdate()
	if err != nil {
		if wasRunning && !svc.isCoreRunning() {
			err = errors.Join(err, svc.restoreCoreAfterUpdate(true))
		}
		return fmt.Errorf("更新前停止核心失败: %w", err)
	}
	commits, err := installRuntimeFiles(svc.paths.BinaryDir, backupDir, files, replace)
	if err != nil {
		var unsafe *runtimeInstallRollbackError
		if !errors.As(err, &unsafe) {
			err = errors.Join(err, svc.restoreCoreAfterUpdate(wasRunning))
		}
		return err
	}
	rollback := func(cause error) error {
		logging.Error("installer.rollback", "更新未完成，恢复旧核心和配置: %v", cause)
		if svc.isCoreRunning() {
			svc.mu.Lock()
			stop := svc.stopCore
			svc.mu.Unlock()
			if stop == nil {
				return &runtimeInstallRollbackError{err: errors.Join(cause, errors.New("无法停止候选核心，未执行文件回滚"))}
			}
			if err := stop(); err != nil {
				return &runtimeInstallRollbackError{err: errors.Join(cause, fmt.Errorf("停止候选核心失败，未执行文件回滚: %w", err))}
			}
		}
		if core != nil {
			if err := core.waitUntilStopped(12 * time.Second); err != nil {
				return &runtimeInstallRollbackError{err: errors.Join(cause, err)}
			}
		}
		var configErr error
		if exists {
			backup, err := os.ReadFile(configBackup)
			configErr = err
			if err == nil {
				configErr = writeExtractedFile(configPath, bytes.NewReader(backup), 0600)
			}
		}
		runtimeErr := rollbackRuntimeFiles(commits, replace)
		if configErr != nil || runtimeErr != nil {
			return &runtimeInstallRollbackError{err: errors.Join(cause, configErr, runtimeErr, errors.New("回滚不完整，未启动旧核心"))}
		}
		svc.setState(model.InstallExtracting, "restoring previous core", 0, oldVersion, "")
		if err := svc.restoreCoreAfterUpdate(wasRunning); err != nil {
			return errors.Join(cause, fmt.Errorf("文件已回滚，但旧核心恢复运行失败: %w", err))
		}
		logging.Info("installer.rollback", "旧核心、运行库和活动配置已恢复")
		return cause
	}
	if exists && !bytes.Equal(original, migrated) {
		if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, configPath, time.Now()); err != nil {
			return rollback(fmt.Errorf("备份迁移前配置失败: %w", err))
		}
		if err := writeExtractedFile(configPath, bytes.NewReader(migrated), 0600); err != nil {
			return rollback(fmt.Errorf("应用迁移配置失败: %w", err))
		}
		logging.Info("installer.migrate", "已应用 %d 项配置迁移", migrationCount)
	}
	// Runtime status must reflect the version that is actually being started.
	svc.setState(model.InstallExtracting, "checking updated core", 0, version, "")
	svc.broadcastStatus()
	if err := svc.restoreCoreAfterUpdate(wasRunning); err != nil {
		return rollback(fmt.Errorf("更新核心启动失败: %w", err))
	}
	// This migration is atomic and runs last: no DB preference is changed on a
	// failed preflight, file replacement or restart. The snapshot blocks writers.
	if _, err := svc.store.MigrateDNSIndependentCache(singboxSupportsDNSIndependentCache(version)); err != nil {
		return rollback(fmt.Errorf("DNS 缓存设置迁移失败: %w", err))
	}
	return nil
}
