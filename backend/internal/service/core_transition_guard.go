package service

import (
	"errors"
	"fmt"

	"github.com/ackwrap/ackrun/internal/logging"
)

var errTransitionProtectionUnsupported = errors.New("当前环境不支持自动重启防泄漏保护")

// The independent firewall table must outlive sing-box's own route/fw4 cleanup.
// Callers hold lifecycleMu. Without a ready recovered core, it remains in place.
type coreTransitionGuard interface {
	Enable([]string, []string, ...string) error
	Disable() error
}

// Check/install protection before launching the detached OpenWrt installer,
// rather than discovering an unsupported firewall during service shutdown.
func (svc *SingboxService) ProtectForAppUpdate() error {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	if !svc.IsRunning() {
		return nil
	}
	return svc.protectCoreTransition()
}

func (svc *SingboxService) protectCoreTransition() error {
	if svc.transitionGuard == nil {
		return nil
	}
	protected := svc.runningTUN
	sources := append([]string(nil), svc.runningDeviceIPs...)
	configPath, exists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	if exists {
		state, err := readActiveTUNState(configPath)
		if err != nil {
			return err
		}
		protected = protected || state.ManagesRoutes
		sources = appendUniqueStrings(sources, state.DeviceDNSCIDRs...)
	}
	if !protected {
		return nil
	}
	sources, err = svc.transitionDeviceSources(sources)
	if err != nil {
		return err
	}
	fakeIPs, err := svc.deviceFakeIPCIDRs()
	if err != nil {
		return err
	}
	excluded, err := deviceExcludedInterfaces(svc.store)
	if err != nil {
		return err
	}
	if err := svc.transitionGuard.Enable(sources, excluded, fakeIPs...); err != nil {
		if errors.Is(err, errTransitionProtectionUnsupported) {
			required, checkErr := svc.requiresDeviceProtection()
			if checkErr != nil {
				return checkErr
			}
			if !required {
				logging.Info("core.network_guard.skip", "跳过当前环境不支持的临时防泄漏保护，继续核心操作（不保证切换期间阻断）: %v", err)
				return nil
			}
		}
		logging.Error("core.network_guard", "无法启用防泄漏保护，取消停止核心: %v", err)
		return fmt.Errorf("启用重启防泄漏保护失败，核心未停止: %w", err)
	}
	return nil
}

func (svc *SingboxService) requiresDeviceProtection() (bool, error) {
	if svc.store != nil {
		settings, err := svc.store.GetGeneralSettings()
		if err != nil {
			return false, err
		}
		if settings.DeviceProtectionEnabled {
			return true, nil
		}
	}
	if svc.deviceProtection != nil {
		sources, err := svc.deviceProtection.Sources()
		return len(sources) > 0, err
	}
	return false, nil
}

func (svc *SingboxService) transitionDeviceSources(sources []string) ([]string, error) {
	if svc.store != nil {
		rules, err := enabledDeviceRules(svc.store)
		if err != nil {
			return nil, err
		}
		sources = appendUniqueStrings(sources, deviceRuleCIDRs(rules)...)
	}
	if svc.deviceProtection != nil {
		previous, err := svc.deviceProtection.Sources()
		if err != nil {
			return nil, err
		}
		sources = appendUniqueStrings(sources, previous...)
	}
	if len(sources) == 0 {
		return nil, nil
	}
	return normalizeDeviceSources(sources)
}

func (svc *SingboxService) releaseCoreTransition() error {
	if svc.transitionGuard == nil {
		return nil
	}
	if err := svc.transitionGuard.Disable(); err != nil {
		logging.Error("core.network_guard", "核心已就绪，但解除防泄漏保护失败: %v", err)
		return fmt.Errorf("解除重启防泄漏保护失败: %w", err)
	}
	return nil
}
