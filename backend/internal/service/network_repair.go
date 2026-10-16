package service

import (
	"errors"
	"fmt"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

var ErrNetworkRepairCoreRunning = errors.New("sing-box 核心正在运行，请先停止核心后再执行网络修复")

// RepairNetwork restores network state without opening the database or starting
// services. Force mode stops sing-box and does not require ownership records.
func RepairNetwork(p *paths.Paths, force bool) (string, error) {
	return repairNetwork(p, force, false)
}

// ResetNetwork performs forced repair and explicitly opts out of device leak
// protection. The caller must stop the daemon/respawn before invoking it.
func ResetNetwork(p *paths.Paths) (string, error) {
	return repairNetwork(p, true, true)
}

func repairNetwork(p *paths.Paths, force, resetProtection bool) (string, error) {
	logging.Info("network.repair", "开始检查 Ackwrap 网络残留: force=%t reset_protection=%t", force, resetProtection)
	if p == nil {
		logging.Error("network.repair", "网络修复失败：路径配置不可用")
		return "", errors.New("路径配置不可用")
	}
	if force {
		if err := stopPlatformSingboxForRepair(); err != nil {
			return "", err
		}
	}
	releaseNetworkLock, err := acquireNetworkLifecycleFileLock(p.NetworkLifecycleLockPath())
	if err != nil {
		logging.Error("network.repair", "网络修复失败：无法获取网络生命周期锁")
		return "", err
	}
	defer releaseNetworkLock()

	svc := &SingboxService{paths: p, dnsmasq: newDNSMasqLifecycle(p), deviceDNS: newDeviceDNSLifecycle(p), deviceProtection: newDeviceProtection(p)}
	var resetErr error
	if resetProtection {
		resetErr = resetDeviceProtectionForRepair(p, svc.deviceProtection)
		// An explicit reset must not reinstall the policy (including damaged
		// files). Other repair steps still run if protection release failed.
		svc.deviceProtection = nil
	}
	recovery, err := svc.recoverStoppedStateDetailed(force, force)
	if recovery.ProcessRunning {
		return "", errors.Join(ErrNetworkRepairCoreRunning, resetErr, err)
	}
	message, resultErr := networkRepairResult(recovery, errors.Join(resetErr, err))
	if errors.Is(resultErr, ErrNetworkRepairCoreRunning) {
		logging.Info("network.repair", "核心仍在运行，已拒绝网络修复")
		return "", resultErr
	}
	if resetProtection {
		// An interrupted exit may have lost the DNS ownership marker while its
		// redirect still points to the stopped core's device resolver.
		resultErr = errors.Join(resultErr, resetPlatformDeviceDNS(p))
	}
	if resultErr != nil {
		logging.Error("network.repair", "网络修复失败: %v", resultErr)
		return "", resultErr
	}
	// Only an explicit force repair may opt back into direct networking without
	// a ready core. Routine startup/exit cleanup must preserve leak protection.
	if resetProtection {
		if err := resetPlatformTransitionGuard(p); err != nil {
			return "", err
		}
	} else if force {
		if guard := newCoreTransitionGuard(p); guard != nil {
			if err := guard.Disable(); err != nil {
				return "", err
			}
		}
	}
	logging.Info("network.repair", "网络修复完成，已执行恢复: %t", recovery.NetworkCleaned || recovery.DNSMasqRestored)
	if force {
		message = "强制网络修复完成：已清理 sing-box 网络残留并解除临时重启保护；设备流量防泄漏仍保留，服务保持停止。"
	}
	if resetProtection {
		message = "强制重置完成：已清理 sing-box 网络残留并关闭流量防泄漏，设备可恢复直连；订阅、节点和设备规则已保留，服务保持停止。"
		logging.Info("network.reset", "强制重置完成，设备防泄漏设置与持久规则已解除")
	}
	return message, nil
}

func resetDeviceProtectionForRepair(p *paths.Paths, guard deviceProtectionFirewall) error {
	// Persist opt-out before deleting rules, so a later daemon start cannot
	// silently turn protection back on. A failed database write retains the guard.
	if err := store.DisableDeviceProtectionForRepair(p.DBPath); err != nil {
		return fmt.Errorf("关闭流量防泄漏设置失败，保留保护: %w", err)
	}
	if guard != nil {
		if err := guard.ForceDisable(); err != nil {
			return fmt.Errorf("解除设备防泄漏规则失败，请重试强制重置: %w", err)
		}
	}
	return nil
}

func networkRepairResult(recovery stoppedStateRecovery, err error) (string, error) {
	if err != nil {
		return "", err
	}
	if recovery.ProcessRunning {
		return "", ErrNetworkRepairCoreRunning
	}
	if recovery.NetworkCleaned || recovery.DNSMasqRestored {
		return "网络修复完成：已恢复 Ackwrap 接管的 DNS 和网络状态。", nil
	}
	return "网络状态正常：未发现需要清理的 Ackwrap 网络残留。", nil
}
