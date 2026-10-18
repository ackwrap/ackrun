package service

import (
	"fmt"
	"os"
	"regexp"
	"slices"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

var bypassInterfaceName = regexp.MustCompile(`^[A-Za-z0-9_.:@-]+$`)

func normalizeExcludedInterfaces(values []string) ([]string, error) {
	var result []string
	for _, value := range values {
		if len(value) == 0 || len(value) > 15 || !bypassInterfaceName.MatchString(value) {
			return nil, fmt.Errorf("网络接口名称必须是 1 到 15 字节的有效接口名")
		}
		result = appendUniqueStrings(result, value)
	}
	slices.Sort(result)
	return result, nil
}

// Interface exemptions affect kernel rules as well as the generated config.
// Narrow exemptions before saving; widen only after the new intent is durable.
func (svc *SettingsService) saveTrafficBypassSettings(settings *model.TrafficBypassSettings) error {
	core := svc.singbox
	if core == nil {
		return svc.store.SetTrafficBypassSettings(settings)
	}
	core.networkLifecycleMu.Lock()
	defer core.networkLifecycleMu.Unlock()
	previousSettings, err := svc.store.GetTrafficBypassSettings()
	if err != nil {
		return err
	}
	// Read old names without validating them so an obsolete/invalid saved
	// interface can still be removed. Only the requested exemptions are applied.
	_, previous, _, _, _ := trafficBypassValues(previousSettings)
	slices.Sort(previous)
	_, next, _, _, _ := trafficBypassValues(settings)
	next, err = normalizeExcludedInterfaces(next)
	if err != nil {
		return err
	}
	unchanged := slices.Equal(previous, next)
	if core.IsRunning() {
		if unchanged {
			return svc.store.SetTrafficBypassSettings(settings)
		}
		return fmt.Errorf("%w: 请先停止核心，再修改网络接口排除", ErrTrafficBypassSettingsInvalid)
	}
	release, err := acquireNetworkLifecycleFileLock(core.paths.NetworkLifecycleLockPath())
	if err != nil {
		return err
	}
	defer release()
	var common []string
	for _, name := range previous {
		if slices.Contains(next, name) {
			common = append(common, name)
		}
	}
	if !unchanged {
		if err := core.syncExcludedInterfaceProtection(common); err != nil {
			return err
		}
	}
	if err := svc.store.SetTrafficBypassSettings(settings); err != nil {
		return err
	}
	if err := core.syncExcludedInterfaceProtection(next); err != nil {
		logging.Error("device_protection.interfaces", "接口排除已保存，但防护规则同步失败: %v", err)
		return fmt.Errorf("接口排除已保存，但防护规则同步失败，请重试保存或启动核心: %w", err)
	}
	logging.Info("device_protection.interfaces", "已同步 %d 个明确排除接口", len(next))
	return nil
}

// Caller holds the network lifecycle locks and has verified the core is stopped.
func (svc *SingboxService) syncExcludedInterfaceProtection(excluded []string) error {
	fakeIPs, err := svc.deviceFakeIPCIDRs()
	if err != nil {
		return err
	}
	if svc.deviceProtection != nil {
		sources, err := svc.deviceProtection.Sources()
		if err != nil {
			return err
		}
		if len(sources) > 0 {
			if err := svc.deviceProtection.Apply(sources, excluded, fakeIPs...); err != nil {
				return err
			}
		}
	}
	if svc.transitionGuard != nil {
		if _, err := os.Stat(svc.paths.CoreTransitionGuardPath()); err == nil {
			sources, err := svc.transitionDeviceSources(svc.runningDeviceIPs)
			if err != nil {
				return err
			}
			return svc.transitionGuard.Enable(sources, excluded, fakeIPs...)
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func deviceExcludedInterfaces(db *store.Store) ([]string, error) {
	if db == nil {
		return nil, nil
	}
	settings, err := db.GetTrafficBypassSettings()
	if err != nil {
		return nil, err
	}
	_, interfaces, _, _, _ := trafficBypassValues(settings)
	return normalizeExcludedInterfaces(interfaces)
}
