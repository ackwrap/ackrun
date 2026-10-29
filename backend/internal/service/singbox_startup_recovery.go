package service

import (
	"fmt"
	"os/exec"

	"github.com/ackwrap/ackrun/internal/logging"
)

// The caller has validated the replacement configuration and holds the core
// lifecycle and network locks. Keep protection in place until markStarted has
// verified the new core; a failed stop or cleanup must never launch a second one.
func (svc *SingboxService) recoverBeforeStart(stopExisting func() error) error {
	if err := svc.protectCoreTransition(); err != nil {
		return err
	}
	if err := stopExisting(); err != nil {
		return fmt.Errorf("停止已有 sing-box 失败，保留防泄漏保护: %w", err)
	}
	if err := svc.recoverStoppedState(false, true); err != nil {
		return fmt.Errorf("清理 sing-box 网络残留失败，保留防泄漏保护: %w", err)
	}
	logging.Info("core.recover", "启动前检查完成：已有核心已退出，网络残留清理完成")
	return nil
}

// Cleanup and ownership preflight can take time. Check again immediately before
// launching, so a competing supervisor cannot silently supply the running core.
func startRecoveredCore(cmd *exec.Cmd, verifyStopped func() error) error {
	if err := verifyStopped(); err != nil {
		return err
	}
	return cmd.Start()
}
