package service

import (
	"fmt"

	"github.com/ackwrap/ackrun/internal/logging"
)

// The independent firewall table must outlive sing-box's own route/fw4 cleanup.
// Callers hold lifecycleMu. Without a ready recovered core, it remains in place.
type coreTransitionGuard interface {
	Enable(...string) error
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
	}
	if !protected {
		return nil
	}
	fakeIPs, err := svc.deviceFakeIPCIDRs()
	if err != nil {
		return err
	}
	if err := svc.transitionGuard.Enable(fakeIPs...); err != nil {
		logging.Error("core.network_guard", "无法启用防泄漏保护，取消停止核心: %v", err)
		return fmt.Errorf("启用重启防泄漏保护失败，核心未停止: %w", err)
	}
	logging.Info("core.network_guard", "已阻断 IPv4/IPv6 外网与 FakeIP 直连，保留本地网络和管理连接")
	return nil
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
