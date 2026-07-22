package service

import (
	"errors"
	"strings"
	"testing"
)

type recordingDNSMasqLifecycle struct {
	restoreCalls int
}

func (*recordingDNSMasqLifecycle) Supported() bool { return true }
func (*recordingDNSMasqLifecycle) Activate() error { return nil }
func (lifecycle *recordingDNSMasqLifecycle) Restore() (bool, error) {
	lifecycle.restoreCalls++
	return true, nil
}

func TestCleanupAfterProcessExitSkipsCleanupWithoutLock(t *testing.T) {
	dnsmasq := &recordingDNSMasqLifecycle{}
	svc := &SingboxService{dnsmasq: dnsmasq}
	err := svc.cleanupAfterProcessExit(errors.New("lock failed"))
	if err == nil || !strings.Contains(err.Error(), "network lifecycle lock") {
		t.Fatalf("cleanupAfterProcessExit() error = %v", err)
	}
	if dnsmasq.restoreCalls != 0 {
		t.Fatalf("dnsmasq restore calls = %d", dnsmasq.restoreCalls)
	}
}

func TestNetworkRepairResultRejectsRunningCore(t *testing.T) {
	message, err := networkRepairResult(stoppedStateRecovery{ProcessRunning: true}, nil)
	if !errors.Is(err, ErrNetworkRepairCoreRunning) || message != "" {
		t.Fatalf("networkRepairResult() = %q, %v", message, err)
	}
}

func TestNetworkRepairResultReportsCleanup(t *testing.T) {
	message, err := networkRepairResult(stoppedStateRecovery{DNSMasqRestored: true}, nil)
	if err != nil || !strings.Contains(message, "网络修复完成") {
		t.Fatalf("networkRepairResult() = %q, %v", message, err)
	}
}

func TestNetworkRepairResultReportsHealthyState(t *testing.T) {
	message, err := networkRepairResult(stoppedStateRecovery{}, nil)
	if err != nil || !strings.Contains(message, "网络状态正常") {
		t.Fatalf("networkRepairResult() = %q, %v", message, err)
	}
}

func TestNetworkRepairResultPreservesFailure(t *testing.T) {
	want := errors.New("cleanup failed")
	message, err := networkRepairResult(stoppedStateRecovery{}, want)
	if !errors.Is(err, want) || message != "" {
		t.Fatalf("networkRepairResult() = %q, %v", message, err)
	}
}
