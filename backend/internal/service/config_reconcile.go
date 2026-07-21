package service

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const configReconcileDebounce = 1200 * time.Millisecond

type configReconcileGenerator interface {
	ReconcileCurrent() (*model.ConfigGenerateResponse, error)
}

// ConfigReconcileService coalesces data mutations into one validated config update.
type ConfigReconcileService struct {
	generator configReconcileGenerator
	realtime  *RealtimeService
	debounce  time.Duration

	mu      sync.Mutex
	timer   *time.Timer
	reasons map[string]struct{}
	closed  bool
	runMu   sync.Mutex
}

func NewConfigReconcileService(generator *ConfigGeneratorService, realtime *RealtimeService) *ConfigReconcileService {
	return newConfigReconcileService(generator, realtime, configReconcileDebounce)
}

func newConfigReconcileService(generator configReconcileGenerator, realtime *RealtimeService, debounce time.Duration) *ConfigReconcileService {
	return &ConfigReconcileService{
		generator: generator,
		realtime:  realtime,
		debounce:  debounce,
		reasons:   make(map[string]struct{}),
	}
}

func (svc *ConfigReconcileService) Trigger(reason string) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.closed {
		return
	}
	if reason == "" {
		reason = "data.changed"
	}
	svc.reasons[reason] = struct{}{}
	if svc.timer != nil {
		svc.timer.Stop()
	}
	svc.timer = time.AfterFunc(svc.debounce, svc.runPending)
	logging.Info("config.reconcile", "已安排配置协调: %s", reason)
}

func (svc *ConfigReconcileService) runPending() {
	svc.mu.Lock()
	if svc.closed {
		svc.mu.Unlock()
		return
	}
	reasons := make([]string, 0, len(svc.reasons))
	for reason := range svc.reasons {
		reasons = append(reasons, reason)
	}
	svc.reasons = make(map[string]struct{})
	svc.timer = nil
	svc.mu.Unlock()

	sort.Strings(reasons)
	svc.reconcile(strings.Join(reasons, ","))
}

func (svc *ConfigReconcileService) reconcile(reason string) {
	svc.runMu.Lock()
	defer svc.runMu.Unlock()

	logging.Info("config.reconcile", "开始配置协调: %s", reason)
	svc.broadcast("started", reason, "")
	result, err := svc.generator.ReconcileCurrent()
	if err != nil {
		svc.fail(reason, fmt.Errorf("配置协调失败: %w", err))
		return
	}
	if !result.Valid {
		svc.fail(reason, fmt.Errorf("配置校验失败: %s", result.Error))
		return
	}
	logging.Info("config.reconcile", "配置协调完成: %s", reason)
	svc.broadcast("succeeded", reason, "")
}

func (svc *ConfigReconcileService) fail(reason string, err error) {
	logging.Error("config.reconcile", "%v", err)
	svc.broadcast("failed", reason, err.Error())
}

func (svc *ConfigReconcileService) broadcast(status, reason, errorMessage string) {
	if svc.realtime == nil {
		return
	}
	data := map[string]any{"status": status, "reason": reason}
	if errorMessage != "" {
		data["error"] = errorMessage
	}
	svc.realtime.Broadcast("config.reconcile", data)
}

func (svc *ConfigReconcileService) Close() {
	svc.mu.Lock()
	svc.closed = true
	if svc.timer != nil {
		svc.timer.Stop()
	}
	svc.mu.Unlock()
	svc.runMu.Lock()
	svc.runMu.Unlock()
}
