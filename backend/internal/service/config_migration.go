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

func migrateUpdateProxyConfigData(data []byte) ([]byte, int, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, 0, fmt.Errorf("解析配置失败: %w", err)
	}
	migrated, err := migrateUpdateProxyConfig(config)
	if err != nil {
		return nil, 0, err
	}
	if migrated == 0 {
		return data, 0, nil
	}
	result, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return nil, 0, fmt.Errorf("序列化更新代理迁移配置失败: %w", err)
	}
	return result, migrated, nil
}

func migrateUpdateProxyConfig(config map[string]interface{}) (int, error) {
	migrated := 0
	var inbounds []interface{}
	if rawInbounds, exists := config["inbounds"]; exists {
		var ok bool
		inbounds, ok = rawInbounds.([]interface{})
		if !ok {
			return 0, fmt.Errorf("活动配置 inbounds 格式无效，无法添加 Ackwrap 更新代理")
		}
	}
	var outbounds []interface{}
	if rawOutbounds, exists := config["outbounds"]; exists {
		var ok bool
		outbounds, ok = rawOutbounds.([]interface{})
		if !ok {
			return 0, fmt.Errorf("活动配置 outbounds 格式无效，无法添加 Ackwrap 更新代理")
		}
	}
	if !hasTaggedConfigItem(outbounds, "direct") {
		outbounds = append(outbounds, map[string]interface{}{"type": "direct", "tag": "direct"})
		migrated++
	}
	if !hasTaggedConfigItem(outbounds, "proxy") {
		outbounds = append(outbounds, map[string]interface{}{
			"type": "selector", "tag": "proxy", "outbounds": []interface{}{"direct"},
		})
		migrated++
	} else if proxyOutbound := taggedConfigItem(outbounds, "proxy"); proxyOutbound["type"] == "selector" || proxyOutbound["type"] == "urltest" {
		members, exists := proxyOutbound["outbounds"]
		memberCount, valid := configStringListLength(members)
		if exists && !valid {
			return 0, fmt.Errorf("活动配置 proxy outbound 成员格式无效，无法添加 Ackwrap 更新代理")
		}
		if !exists || memberCount == 0 {
			outbounds = replaceTaggedConfigItem(outbounds, "proxy", map[string]interface{}{
				"type": "selector", "tag": "proxy", "outbounds": []interface{}{"direct"},
			})
			migrated++
		}
	}
	config["outbounds"] = outbounds

	var route map[string]interface{}
	if rawRoute, exists := config["route"]; exists {
		var ok bool
		route, ok = rawRoute.(map[string]interface{})
		if !ok {
			return 0, fmt.Errorf("活动配置 route 格式无效，无法添加 Ackwrap 更新代理")
		}
	} else {
		route = make(map[string]interface{})
		config["route"] = route
		migrated++
	}

	updateInboundIndex := -1
	for index, rawInbound := range inbounds {
		inbound, ok := rawInbound.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := inbound["tag"].(string)
		port := configNumber(inbound["listen_port"])
		if tag == updateProxyInboundTag {
			if updateInboundIndex >= 0 {
				return 0, fmt.Errorf("活动配置存在重复的 %s inbound", updateProxyInboundTag)
			}
			updateInboundIndex = index
			continue
		}
		if port == updateProxyListenPort {
			return 0, fmt.Errorf("活动配置端口 %d 已被 inbound %v 占用，无法添加 Ackwrap 更新代理", updateProxyListenPort, inbound["tag"])
		}
	}

	canonicalInbound := map[string]interface{}{
		"type":        "mixed",
		"tag":         updateProxyInboundTag,
		"listen":      "127.0.0.1",
		"listen_port": updateProxyListenPort,
	}
	if updateInboundIndex < 0 {
		inbounds = append(inbounds, canonicalInbound)
		config["inbounds"] = inbounds
		migrated++
	} else if !isCanonicalUpdateProxyInbound(inbounds[updateInboundIndex].(map[string]interface{})) {
		inbounds[updateInboundIndex] = canonicalInbound
		migrated++
	}
	var rules []interface{}
	if rawRules, exists := route["rules"]; exists {
		var ok bool
		rules, ok = rawRules.([]interface{})
		if !ok {
			return 0, fmt.Errorf("活动配置 route.rules 格式无效，无法添加 Ackwrap 更新代理规则")
		}
	}
	scopedRules := make([]interface{}, 0, len(rules)+1)
	processRulesChanged := false
	for _, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule["outbound"] != "direct" || rule["inbound"] != nil {
			scopedRules = append(scopedRules, rawRule)
			continue
		}
		ackwrapNames, otherNames := splitAckwrapProcessNames(rule["process_name"])
		if len(ackwrapNames) == 0 {
			scopedRules = append(scopedRules, rawRule)
			continue
		}
		if len(otherNames) == 0 {
			rule["process_name"] = ackwrapNames
			rule["inbound"] = []interface{}{"tun-in"}
			scopedRules = append(scopedRules, rule)
		} else {
			scopedRule := make(map[string]interface{}, len(rule)+1)
			for key, value := range rule {
				scopedRule[key] = value
			}
			scopedRule["process_name"] = ackwrapNames
			scopedRule["inbound"] = []interface{}{"tun-in"}
			rule["process_name"] = otherNames
			scopedRules = append(scopedRules, scopedRule, rule)
		}
		processRulesChanged = true
		migrated++
	}
	if processRulesChanged {
		rules = scopedRules
		route["rules"] = rules
	}
	firstRule, firstRuleOK := map[string]interface{}(nil), false
	if len(rules) > 0 {
		firstRule, firstRuleOK = rules[0].(map[string]interface{})
	}
	if !firstRuleOK || !isCanonicalUpdateProxyRule(firstRule) {
		canonicalRule := map[string]interface{}{
			"inbound":  []interface{}{updateProxyInboundTag},
			"action":   "route",
			"outbound": "proxy",
		}
		filteredRules := make([]interface{}, 0, len(rules)+1)
		filteredRules = append(filteredRules, canonicalRule)
		for _, rawRule := range rules {
			rule, ok := rawRule.(map[string]interface{})
			if ok && isCanonicalUpdateProxyRule(rule) {
				continue
			}
			filteredRules = append(filteredRules, rawRule)
		}
		rules = filteredRules
		route["rules"] = rules
		migrated++
	}
	return migrated, nil
}

func isCanonicalUpdateProxyInbound(inbound map[string]interface{}) bool {
	return len(inbound) == 4 &&
		inbound["type"] == "mixed" &&
		inbound["tag"] == updateProxyInboundTag &&
		inbound["listen"] == "127.0.0.1" &&
		configNumber(inbound["listen_port"]) == updateProxyListenPort
}

func isCanonicalUpdateProxyRule(rule map[string]interface{}) bool {
	return len(rule) == 3 &&
		rule["action"] == "route" &&
		rule["outbound"] == "proxy" &&
		stringListEquals(rule["inbound"], updateProxyInboundTag)
}

func splitAckwrapProcessNames(value interface{}) ([]interface{}, []interface{}) {
	var values []interface{}
	switch typed := value.(type) {
	case []interface{}:
		values = typed
	case []string:
		values = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			values = append(values, item)
		}
	default:
		return nil, nil
	}
	ackwrapNames := make([]interface{}, 0, len(values))
	otherNames := make([]interface{}, 0, len(values))
	for _, value := range values {
		name, ok := value.(string)
		if ok && (strings.EqualFold(name, "ackwrap") || strings.EqualFold(name, "ackwrap.exe") || strings.EqualFold(name, "sing-box") || strings.EqualFold(name, "sing-box.exe")) {
			ackwrapNames = append(ackwrapNames, value)
			continue
		}
		otherNames = append(otherNames, value)
	}
	return ackwrapNames, otherNames
}

func hasTaggedConfigItem(items []interface{}, tag string) bool {
	return taggedConfigItem(items, tag) != nil
}

func taggedConfigItem(items []interface{}, tag string) map[string]interface{} {
	for _, rawItem := range items {
		if item, ok := rawItem.(map[string]interface{}); ok && item["tag"] == tag {
			return item
		}
	}
	return nil
}

func replaceTaggedConfigItem(items []interface{}, tag string, replacement map[string]interface{}) []interface{} {
	for index, rawItem := range items {
		if item, ok := rawItem.(map[string]interface{}); ok && item["tag"] == tag {
			items[index] = replacement
			break
		}
	}
	return items
}

func configStringListLength(value interface{}) (int, bool) {
	switch values := value.(type) {
	case []interface{}:
		for _, value := range values {
			if _, ok := value.(string); !ok {
				return 0, false
			}
		}
		return len(values), true
	case []string:
		return len(values), true
	default:
		return 0, false
	}
}

func configNumber(value interface{}) int {
	switch number := value.(type) {
	case float64:
		return int(number)
	case int:
		return number
	default:
		return 0
	}
}

func stringListContains(value interface{}, expected string) bool {
	switch values := value.(type) {
	case []interface{}:
		for _, value := range values {
			if value == expected {
				return true
			}
		}
	case []string:
		for _, value := range values {
			if value == expected {
				return true
			}
		}
	}
	return false
}

func stringListEquals(value interface{}, expected string) bool {
	switch values := value.(type) {
	case []interface{}:
		return len(values) == 1 && values[0] == expected
	case []string:
		return len(values) == 1 && values[0] == expected
	default:
		return false
	}
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
	migratedData, updateProxyMigrated, err := migrateUpdateProxyConfigData(migratedData)
	if err != nil {
		return false, err
	}
	migrated += updateProxyMigrated
	if migrated == 0 {
		return false, nil
	}

	stagedFile, err := os.CreateTemp(filepath.Dir(configPath), ".ackwrap-config-migration-*.tmp")
	if err != nil {
		return false, fmt.Errorf("创建配置迁移暂存文件失败: %w", err)
	}
	stagedPath := stagedFile.Name()
	defer os.Remove(stagedPath)
	if _, err := stagedFile.Write(migratedData); err != nil {
		stagedFile.Close()
		return false, fmt.Errorf("写入配置迁移暂存文件失败: %w", err)
	}
	if err := stagedFile.Close(); err != nil {
		return false, fmt.Errorf("关闭配置迁移暂存文件失败: %w", err)
	}
	validator := svc.validateFile
	if svc.configValidator != nil {
		validator = svc.configValidator
	}
	if err := validator(stagedPath); err != nil {
		return false, fmt.Errorf("迁移配置校验失败: %w", err)
	}

	if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, configPath, time.Now()); err != nil {
		return false, fmt.Errorf("备份迁移前配置失败: %w", err)
	}
	if err := atomicReplaceFile(stagedPath, configPath); err != nil {
		return false, fmt.Errorf("应用配置迁移失败，旧配置保持不变: %w", err)
	}

	logging.Info("config.migrate", "已完成 %d 项活动配置兼容迁移，核心版本: %s", migrated, version)
	return true, nil
}
