package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

type deviceProtectionFirewall interface {
	Supported() bool
	Sources() ([]string, error)
	Apply([]string) error
	Verify([]string) error
	VerifyDisabled() error
	Disable() error
	ForceDisable() error
}

func (svc *SettingsService) checkDeviceProtectionMode(compatible bool) error {
	settings, err := svc.store.GetGeneralSettings()
	if err != nil {
		return err
	}
	if settings.DeviceProtectionEnabled && !compatible {
		return errors.New("流量防泄漏已开启，请先关闭保护再切换到此模式")
	}
	return nil
}

func (svc *RouteRuleService) SetDeviceProtectionCore(core *SingboxService) {
	svc.deviceProtectionCore = core
}

func deviceRequestSources(req *model.RouteRuleRequest) []string {
	if req == nil || !req.Enabled || req.RuleType != "source_ip_cidr" {
		return nil
	}
	return req.Values
}

func (svc *RouteRuleService) mutateProtectedDevice(previous *model.RouteRule, next *model.RouteRuleRequest, persist func() error) error {
	var old []string
	if previous != nil && previous.Enabled && previous.RuleType == "source_ip_cidr" {
		old = previous.Values
	}
	if reflect.DeepEqual(old, deviceRequestSources(next)) {
		return persist()
	}
	core := svc.deviceProtectionCore
	if core == nil {
		return persist()
	}
	core.networkLifecycleMu.Lock()
	defer core.networkLifecycleMu.Unlock()
	settings, err := svc.store.GetGeneralSettings()
	if err != nil {
		return err
	}
	if !settings.DeviceProtectionEnabled {
		return persist()
	}
	if core.IsRunning() {
		return errors.New("流量防泄漏已开启，请先停止核心再修改设备地址或启停设备规则")
	}
	if core.deviceProtection == nil {
		return errors.New("设备防泄漏服务不可用")
	}
	release, err := acquireNetworkLifecycleFileLock(core.paths.NetworkLifecycleLockPath())
	if err != nil {
		return err
	}
	defer release()
	rules, err := enabledDeviceRules(svc.store)
	if err != nil {
		return err
	}
	var desired []string
	for _, rule := range rules {
		if previous != nil && rule.ID == previous.ID {
			continue
		}
		desired = appendUniqueStrings(desired, rule.Values...)
	}
	desired = appendUniqueStrings(desired, deviceRequestSources(next)...)
	if len(desired) == 0 {
		return errors.New("请先关闭流量防泄漏，再停用或删除最后一条设备规则")
	}
	persisted, err := core.deviceProtection.Sources()
	if err != nil {
		return err
	}
	union := appendUniqueStrings(persisted, desired...)
	// Widen protection before committing new addresses. If persistence or the
	// final narrowing fails, retain conservative blocking and report the failure.
	if err := core.deviceProtection.Apply(union); err != nil {
		return err
	}
	if err := persist(); err != nil {
		return err
	}
	if err := core.deviceProtection.Apply(desired); err != nil {
		return fmt.Errorf("设备规则已保存，防泄漏范围同步失败；保留阻断，请重试: %w", err)
	}
	logging.Info("device_protection.sync", "设备规则变更后保护 %d 个地址范围", len(desired))
	return nil
}

func (svc *SingboxService) desiredProtectedSources() ([]string, error) {
	if svc.store == nil {
		return nil, errors.New("设备防泄漏设置存储不可用")
	}
	if svc.store.GetProxyMode() != "rule" || svc.store.GetInboundMode() == "mixed" {
		return nil, errors.New("设备防泄漏需要规则模式和 TUN 接管，请先关闭保护再切换模式")
	}
	rules, err := enabledDeviceRules(svc.store)
	if err != nil {
		return nil, err
	}
	cidrs := deviceRuleCIDRs(rules)
	if len(cidrs) == 0 {
		return nil, errors.New("请先配置并启用至少一条设备固定节点规则")
	}
	if len(cidrs) > 128 {
		return nil, errors.New("设备防泄漏最多支持 128 个地址范围")
	}
	slices.Sort(cidrs)
	return cidrs, nil
}

// Caller holds networkLifecycleMu, as do settings, startup and exit cleanup.
func (svc *SingboxService) restoreDeviceProtection() error {
	if svc.deviceProtection == nil {
		if svc.store != nil {
			settings, err := svc.store.GetGeneralSettings()
			if err != nil {
				return err
			}
			if settings.DeviceProtectionEnabled {
				return errors.New("当前平台无法执行已开启的设备防泄漏保护")
			}
		}
		return nil
	}
	persisted, err := svc.deviceProtection.Sources()
	if err != nil {
		return err
	}
	var enabled bool
	if svc.store != nil {
		settings, err := svc.store.GetGeneralSettings()
		if err != nil {
			return err
		}
		enabled = settings.DeviceProtectionEnabled
	}
	if !enabled && len(persisted) == 0 {
		return nil
	}
	if enabled {
		desired, err := svc.desiredProtectedSources()
		if err != nil {
			return err
		}
		// Recovery never releases addresses left by an interrupted mutation.
		persisted = appendUniqueStrings(persisted, desired...)
	}
	if err := svc.deviceProtection.Apply(persisted); err != nil {
		return err
	}
	logging.Info("device_protection.restore", "已恢复 %d 个地址范围的常驻防泄漏保护", len(persisted))
	return nil
}

func (svc *SettingsService) deviceProtectionStatus(settings *model.GeneralSettings) {
	settings.DeviceProtectionStatus = "disabled"
	if svc.singbox == nil || svc.singbox.deviceProtection == nil {
		if settings.DeviceProtectionEnabled {
			settings.DeviceProtectionStatus, settings.DeviceProtectionError = "error", "当前平台不支持已开启的设备防泄漏保护"
		}
		return
	}
	core := svc.singbox
	core.networkLifecycleMu.Lock()
	defer core.networkLifecycleMu.Unlock()
	guard := core.deviceProtection
	settings.DeviceProtectionSupported = guard.Supported()
	sources, err := guard.Sources()
	settings.DeviceProtectionCount = len(sources)
	if !settings.DeviceProtectionEnabled && err == nil && len(sources) == 0 {
		err = guard.VerifyDisabled()
		if err == nil {
			return
		}
	}
	if err == nil && !settings.DeviceProtectionEnabled {
		err = errors.New("存在未完成的防泄漏操作，请重试启用或明确解除保护")
	}
	if err == nil {
		var desired []string
		desired, err = core.desiredProtectedSources()
		if err == nil {
			err = guard.Verify(desired)
		}
	}
	if err != nil {
		settings.DeviceProtectionStatus, settings.DeviceProtectionError = "error", err.Error()
	} else if core.IsRunning() {
		settings.DeviceProtectionStatus = "protected"
	} else {
		settings.DeviceProtectionStatus = "blocked"
	}
}

// Called under SettingsService's network lock. The switch is a separate
// transaction so rollback of an unrelated Geo/DNS setting cannot remove a guard.
func (svc *SettingsService) setDeviceProtection(enabled bool, previous *model.GeneralSettings) (resultErr error) {
	defer func() {
		if resultErr != nil {
			logging.Error("device_protection.update", "更新设备防泄漏失败，目标状态: %t: %v", enabled, resultErr)
		}
	}()
	core := svc.singbox
	if core == nil || core.deviceProtection == nil {
		return errors.New("当前平台不支持设备防泄漏")
	}
	if core.IsRunning() {
		return errors.New("请先停止核心，再切换流量防泄漏")
	}
	release, err := acquireNetworkLifecycleFileLock(core.paths.NetworkLifecycleLockPath())
	if err != nil {
		return err
	}
	defer release()
	next := *previous
	next.DeviceProtectionEnabled = enabled
	if enabled {
		g := NewConfigGeneratorService(svc.store, core.paths)
		if err := g.validateDevicePolicies(); err != nil {
			return err
		}
		sources, err := core.desiredProtectedSources()
		if err != nil {
			return err
		}
		if err := core.deviceProtection.Apply(sources); err != nil {
			return err
		}
		if err := svc.store.SetGeneralSettings(&next); err != nil {
			return fmt.Errorf("防护已安装但设置保存失败，保持阻断: %w", err)
		}
	} else {
		// Explicit opt-out only. If persistence fails, do not touch the guard.
		if err := svc.store.SetGeneralSettings(&next); err != nil {
			return err
		}
		if err := core.deviceProtection.Disable(); err != nil {
			return errors.Join(err, svc.store.SetGeneralSettings(previous))
		}
	}
	logging.Info("device_protection.update", "设备流量防泄漏开关: %t", enabled)
	return nil
}

// Before a core can use tun0, require its device routes and resolvers to be the
// managed ones. This also rejects accidentally selecting an old/custom config.
// Called with networkLifecycleMu held, without taking svc.mu recursively.
func (svc *SingboxService) validateProtectedConfig(path string) error {
	if svc.deviceProtection == nil || svc.store == nil {
		return nil
	}
	settings, err := svc.store.GetGeneralSettings()
	if err != nil {
		return err
	}
	persisted, err := svc.deviceProtection.Sources()
	if err != nil {
		return err
	}
	if !settings.DeviceProtectionEnabled {
		if len(persisted) > 0 {
			return errors.New("存在未完成的设备防泄漏操作，请先在设置中处理")
		}
		return svc.deviceProtection.VerifyDisabled()
	}
	desired, err := svc.desiredProtectedSources()
	if err != nil {
		return err
	}
	if err := svc.deviceProtection.Verify(desired); err != nil {
		return err
	}
	g := NewConfigGeneratorService(svc.store, svc.paths)
	if err := g.validateDevicePolicies(); err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var actual map[string]interface{}
	if err := json.Unmarshal(data, &actual); err != nil {
		return err
	}
	// Compare routing, DNS and outbounds against the same generator used by
	// normal configuration. Other runtime/API/listen options remain independent.
	expectedRoute, err := g.generateRoute("direct")
	if err != nil {
		return err
	}
	final, _ := expectedRoute["final"].(string)
	expectedDNS, err := g.generateDNSFromDatabase(final)
	if err != nil {
		return err
	}
	required, err := g.dnsRequiresUsableProxy(final)
	if err != nil {
		return err
	}
	outbounds, endpoints, err := g.generateOutbounds(required)
	if err != nil {
		return err
	}
	expected := map[string]interface{}{"route": expectedRoute, "dns": expectedDNS, "outbounds": outbounds}
	if len(endpoints) > 0 {
		expected["endpoints"] = endpoints
	}
	for key, value := range expected {
		encoded, err := json.Marshal(value)
		if err != nil {
			return err
		}
		var normalized interface{}
		if err := json.Unmarshal(encoded, &normalized); err != nil {
			return err
		}
		if !reflect.DeepEqual(actual[key], normalized) {
			return fmt.Errorf("防泄漏保护已开启，%s 与当前设备策略不一致，请重新生成并应用配置", key)
		}
	}
	state, err := readActiveTUNState(path)
	if err != nil {
		return err
	}
	slices.Sort(state.DeviceDNSCIDRs)
	if !reflect.DeepEqual(state.DeviceDNSCIDRs, desired) {
		return errors.New("活动配置缺少受保护设备 DNS 来源，请重新生成配置")
	}
	var inbounds []struct {
		Type          string `json:"type"`
		InterfaceName string `json:"interface_name"`
	}
	raw, _ := json.Marshal(actual["inbounds"])
	if err := json.Unmarshal(raw, &inbounds); err != nil {
		return err
	}
	for _, inbound := range inbounds {
		if inbound.Type == "tun" && inbound.InterfaceName != "tun0" {
			return errors.New("设备防泄漏仅支持受管 tun0 接口")
		}
	}
	return nil
}
