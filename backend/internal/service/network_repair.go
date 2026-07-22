package service

import (
	"errors"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
)

var ErrNetworkRepairCoreRunning = errors.New("sing-box 核心正在运行，请先停止核心后再执行网络修复")

// RepairNetwork restores only Ackwrap-owned network state without starting the
// backend, opening the database, or launching sing-box.
func RepairNetwork(p *paths.Paths) (string, error) {
	logging.Info("network.repair", "开始检查 Ackwrap 网络残留")
	if p == nil {
		logging.Error("network.repair", "网络修复失败：路径配置不可用")
		return "", errors.New("路径配置不可用")
	}
	releaseNetworkLock, err := acquireNetworkLifecycleFileLock(p.NetworkLifecycleLockPath())
	if err != nil {
		logging.Error("network.repair", "网络修复失败：无法获取网络生命周期锁")
		return "", err
	}
	defer releaseNetworkLock()

	svc := &SingboxService{paths: p, dnsmasq: newDNSMasqLifecycle(p)}
	recovery, err := svc.recoverStoppedStateDetailed(false)
	message, resultErr := networkRepairResult(recovery, err)
	if errors.Is(resultErr, ErrNetworkRepairCoreRunning) {
		logging.Info("network.repair", "核心仍在运行，已拒绝网络修复")
		return "", resultErr
	}
	if resultErr != nil {
		logging.Error("network.repair", "网络修复失败：未能安全恢复 Ackwrap 网络状态")
		return "", resultErr
	}
	logging.Info("network.repair", "网络修复完成，已执行恢复: %t", recovery.NetworkCleaned || recovery.DNSMasqRestored)
	return message, nil
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
