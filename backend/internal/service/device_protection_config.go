package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
)

var protectedConfigKeys = []string{"route", "dns", "outbounds", "endpoints"}

// Use the same desired sections for startup reconciliation and the final
// protection check. These sections are managed by the database while protection
// is enabled; runtime API, logging and listener options remain independent.
func (g *ConfigGeneratorService) protectedConfigSections() (map[string]interface{}, error) {
	if err := g.validateDevicePolicies(); err != nil {
		return nil, err
	}
	route, err := g.generateRoute("direct")
	if err != nil {
		return nil, err
	}
	final, _ := route["final"].(string)
	dns, err := g.generateDNSFromDatabase(final)
	if err != nil {
		return nil, err
	}
	required, err := g.dnsRequiresUsableProxy(final)
	if err != nil {
		return nil, err
	}
	outbounds, endpoints, err := g.generateOutbounds(required)
	if err != nil {
		return nil, err
	}
	expected := map[string]interface{}{"route": route, "dns": dns, "outbounds": outbounds}
	if len(endpoints) > 0 {
		expected["endpoints"] = endpoints
	}
	// Normalize typed slices and numbers to the representation of decoded JSON.
	data, err := json.Marshal(expected)
	if err != nil {
		return nil, err
	}
	var normalized map[string]interface{}
	if err := json.Unmarshal(data, &normalized); err != nil {
		return nil, err
	}
	return normalized, nil
}

// Caller holds configFileMu, including restart preflight and the already-locked
// core-update path. Do not acquire lifecycle/snapshot locks or call ReconcileCurrent
// here: those callers may already own them.
func (svc *SingboxService) reconcileProtectedConfig(path string) (resultErr error) {
	if svc.store == nil || svc.deviceProtection == nil {
		return nil
	}
	settings, err := svc.store.GetGeneralSettings()
	if err != nil {
		return err
	}
	if !settings.DeviceProtectionEnabled {
		return nil
	}
	defer func() {
		if resultErr != nil {
			logging.Error("device_protection.reconcile", "启动前同步受管配置失败，保留原配置: %v", resultErr)
		}
	}()
	g := NewConfigGeneratorService(svc.store, svc.paths)
	expected, err := g.protectedConfigSections()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	updated, changed, err := reconcileProtectedConfigData(data, expected)
	if err != nil || len(changed) == 0 {
		return err
	}
	logging.Info("device_protection.reconcile", "检测到受管配置差异，正在同步: %s", strings.Join(changed, ", "))
	staged, err := os.CreateTemp(filepath.Dir(path), ".ackwrap-protection-*.tmp")
	if err != nil {
		return fmt.Errorf("创建受管配置暂存文件失败: %w", err)
	}
	stagedPath := staged.Name()
	defer os.Remove(stagedPath)
	if _, err := staged.Write(updated); err != nil {
		staged.Close()
		return fmt.Errorf("写入受管配置暂存文件失败: %w", err)
	}
	if err := staged.Close(); err != nil {
		return fmt.Errorf("关闭受管配置暂存文件失败: %w", err)
	}
	if valid, message := g.validateConfig(stagedPath); !valid {
		message = redactCoreCheckValues(message, expected)
		message = redactConfigFileValuesFromMessage(message, stagedPath)
		return fmt.Errorf("同步受管配置校验失败: %s", redactAccessToken(g.redactConfigSecrets(message)))
	}
	// A syntactically valid config can still violate the installed firewall or
	// device listener contract. Reject it before touching the active file, and
	// before restart stops the running core. Startup verifies protection again.
	if err := svc.validateProtectedConfig(stagedPath); err != nil {
		return fmt.Errorf("同步受管配置保护校验失败: %w", err)
	}
	if _, _, err := ensureDailyConfigBackup(svc.paths, svc.store, path, time.Now()); err != nil {
		return fmt.Errorf("备份同步前配置失败: %w", err)
	}
	if err := atomicReplaceFile(stagedPath, path); err != nil {
		return fmt.Errorf("应用受管配置失败: %w", err)
	}
	logging.Info("device_protection.reconcile", "启动前已同步受管配置: %s", strings.Join(changed, ", "))
	return nil
}

func reconcileProtectedConfigData(data []byte, expected map[string]interface{}) ([]byte, []string, error) {
	// Keep unrelated values as raw JSON, including large integers and custom
	// runtime/API options that must not be rewritten through float64.
	var actual map[string]json.RawMessage
	if err := json.Unmarshal(data, &actual); err != nil {
		return nil, nil, err
	}
	if actual == nil {
		return nil, nil, fmt.Errorf("活动配置必须是 JSON 对象")
	}
	var changed []string
	for _, key := range protectedConfigKeys {
		var current interface{}
		if raw := actual[key]; len(raw) > 0 {
			if err := json.Unmarshal(raw, &current); err != nil {
				return nil, nil, err
			}
		}
		if reflect.DeepEqual(current, expected[key]) {
			continue
		}
		changed = append(changed, key)
		if value, exists := expected[key]; exists {
			encoded, err := json.Marshal(value)
			if err != nil {
				return nil, nil, err
			}
			actual[key] = encoded
		} else {
			delete(actual, key)
		}
	}
	if len(changed) == 0 {
		return data, nil, nil
	}
	updated, err := json.MarshalIndent(actual, "", "  ")
	return updated, changed, err
}
