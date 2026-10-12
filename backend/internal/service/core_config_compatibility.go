package service

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// These options belong to the 1.15 development line, not the 1.14 stable core.
// Only remove explicitly inactive options; active settings have no equivalent.
func singboxSupportsDevelopmentOptions(version string) bool {
	parsed, _ := parseSingboxVersion(version)
	return parsed[0] > 1 || parsed[0] == 1 && parsed[1] >= 15
}

func migrateDevelopmentConfigData(data []byte, version string) ([]byte, int, error) {
	if version == "" || singboxSupportsDevelopmentOptions(version) {
		return data, 0, nil
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, 0, fmt.Errorf("解析配置失败: %w", err)
	}
	if config == nil {
		return nil, 0, fmt.Errorf("活动配置必须是 JSON 对象，不能为 null")
	}
	migrated := 0
	inbounds, _ := config["inbounds"].([]interface{})
	for index, raw := range inbounds {
		inbound, ok := raw.(map[string]interface{})
		if !ok || inbound["type"] != "tun" {
			continue
		}
		location := fmt.Sprintf("inbounds[%d]", index)
		if inbound["stack"] == "go" {
			return nil, 0, unsupportedCoreOption(location+".stack", version)
		}
		count, err := removeInactiveCoreOption(inbound, "multi_queue", location, version, false)
		if err != nil {
			return nil, 0, err
		}
		migrated += count
		if value, exists := inbound["auto_redirect_tproxy_mark"]; exists {
			inactive := value == nil || value == float64(0)
			if mark, ok := value.(string); ok {
				parsed, err := strconv.ParseUint(mark, 0, 32)
				inactive = err == nil && parsed == 0
			}
			if !inactive {
				return nil, 0, unsupportedCoreOption(location+".auto_redirect_tproxy_mark", version)
			}
			delete(inbound, "auto_redirect_tproxy_mark")
			migrated++
		}
	}
	endpoints, _ := config["endpoints"].([]interface{})
	for index, raw := range endpoints {
		endpoint, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		count, err := removeInactiveCoreOption(endpoint, "on_demand", fmt.Sprintf("endpoints[%d]", index), version, false)
		if err != nil {
			return nil, 0, err
		}
		migrated += count
	}
	experimental, _ := config["experimental"].(map[string]interface{})
	cache, _ := experimental["cache_file"].(map[string]interface{})
	for _, field := range []string{"buffer_size", "flush_interval"} {
		count, err := removeInactiveCoreOption(cache, field, "experimental.cache_file", version, cache["enabled"] != true)
		if err != nil {
			return nil, 0, err
		}
		migrated += count
	}
	if migrated == 0 {
		return data, 0, nil
	}
	result, err := json.MarshalIndent(config, "", "  ")
	return result, migrated, err
}

func removeInactiveCoreOption(options map[string]interface{}, field, location, version string, inactive bool) (int, error) {
	value, exists := options[field]
	if !exists {
		return 0, nil
	}
	inactiveBoolean := (field == "on_demand" || field == "multi_queue") && value == false
	if !inactive && value != nil && !inactiveBoolean {
		return 0, unsupportedCoreOption(location+"."+field, version)
	}
	delete(options, field)
	return 1, nil
}

func unsupportedCoreOption(location, version string) error {
	// Report the field path, never the tag or connection value.
	return fmt.Errorf("核心 %s 不支持 %s（1.15 开发版选项），请明确调整该设置后重试；不会自动改变其行为", version, location)
}

func migrateCoreCompatibilityData(data []byte, version string) ([]byte, int, error) {
	result, count, err := migrateDevelopmentConfigData(data, version)
	if err != nil {
		return nil, 0, err
	}
	result, acmeCount, err := migrateInlineACMEConfig(result, version)
	if err != nil {
		return nil, 0, err
	}
	result, managedCount, err := migrateManagedConfigData(result)
	return result, count + acmeCount + managedCount, err
}
