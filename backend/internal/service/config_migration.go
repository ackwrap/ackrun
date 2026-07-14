package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
)

func singboxSupportsCertificateProvider(version string) bool {
	parsed, _ := parseSingboxVersion(version)
	return parsed[0] > 1 || parsed[0] == 1 && parsed[1] >= 14
}

func migrateInlineACMEConfig(data []byte, version string) ([]byte, int, error) {
	if !singboxSupportsCertificateProvider(version) {
		return data, 0, nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, 0, fmt.Errorf("解析配置失败: %w", err)
	}
	migrated := migrateInlineACME(config)
	if migrated == 0 {
		return data, 0, nil
	}

	result, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, 0, fmt.Errorf("序列化迁移配置失败: %w", err)
	}
	return result, migrated, nil
}

func migrateInlineACME(config map[string]interface{}) int {
	inbounds, ok := config["inbounds"].([]interface{})
	if !ok {
		return 0
	}

	migrated := 0
	for _, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]interface{})
		if !ok {
			continue
		}
		tlsOptions, ok := inbound["tls"].(map[string]interface{})
		if !ok {
			continue
		}
		rawACME, exists := tlsOptions["acme"]
		if !exists {
			continue
		}

		if hasCertificateProvider(tlsOptions["certificate_provider"]) {
			delete(tlsOptions, "acme")
			migrated++
			continue
		}

		acme, ok := rawACME.(map[string]interface{})
		if !ok {
			continue
		}
		if !hasACMEDomain(acme["domain"]) {
			delete(tlsOptions, "acme")
			migrated++
			continue
		}

		provider := make(map[string]interface{}, len(acme)+1)
		for key, value := range acme {
			provider[key] = value
		}
		provider["type"] = "acme"
		tlsOptions["certificate_provider"] = provider
		delete(tlsOptions, "acme")
		migrated++
	}
	return migrated
}

func hasCertificateProvider(value interface{}) bool {
	switch provider := value.(type) {
	case string:
		return strings.TrimSpace(provider) != ""
	case map[string]interface{}:
		return len(provider) > 0
	default:
		return false
	}
}

func hasACMEDomain(value interface{}) bool {
	switch domains := value.(type) {
	case string:
		return strings.TrimSpace(domains) != ""
	case []interface{}:
		for _, domain := range domains {
			if value, ok := domain.(string); ok && strings.TrimSpace(value) != "" {
				return true
			}
		}
	case []string:
		for _, domain := range domains {
			if strings.TrimSpace(domain) != "" {
				return true
			}
		}
	}
	return false
}

// MigrateCompatibility upgrades deprecated fields supported by the installed core.
func (svc *ConfigService) MigrateCompatibility(version string) (bool, error) {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	if version == "" {
		version = readSingboxVersion(svc.paths.BinaryPath)
	}
	if !singboxSupportsCertificateProvider(version) {
		return false, nil
	}

	configPath, exists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return false, fmt.Errorf("获取活动配置失败: %w", err)
	}
	if !exists {
		return false, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, fmt.Errorf("读取活动配置失败: %w", err)
	}
	migratedData, migrated, err := migrateInlineACMEConfig(data, version)
	if err != nil {
		return false, err
	}
	if migrated == 0 {
		return false, nil
	}

	stagedFile, err := os.CreateTemp(filepath.Dir(configPath), ".ackwrap-acme-migration-*.tmp")
	if err != nil {
		return false, fmt.Errorf("创建 ACME 迁移暂存文件失败: %w", err)
	}
	stagedPath := stagedFile.Name()
	defer os.Remove(stagedPath)
	if _, err := stagedFile.Write(migratedData); err != nil {
		stagedFile.Close()
		return false, fmt.Errorf("写入 ACME 迁移暂存文件失败: %w", err)
	}
	if err := stagedFile.Close(); err != nil {
		return false, fmt.Errorf("关闭 ACME 迁移暂存文件失败: %w", err)
	}
	if err := svc.validateFile(stagedPath); err != nil {
		return false, fmt.Errorf("ACME 迁移配置校验失败: %w", err)
	}

	backupDir := filepath.Join(svc.paths.ConfigDir, "backup")
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return false, fmt.Errorf("创建 ACME 迁移备份目录失败: %w", err)
	}
	backupName := fmt.Sprintf("%s.pre-certificate-provider.%d.bak.json", filepath.Base(configPath), time.Now().UnixNano())
	backupPath := filepath.Join(backupDir, backupName)
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return false, fmt.Errorf("备份 ACME 迁移前配置失败: %w", err)
	}
	if err := atomicReplaceFile(stagedPath, configPath); err != nil {
		return false, fmt.Errorf("应用 ACME 迁移失败，旧配置保持不变: %w", err)
	}

	logging.Info("config.migrate", "已迁移 %d 个 inline ACME 配置到 certificate provider，核心版本: %s", migrated, version)
	return true, nil
}
