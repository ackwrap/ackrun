package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

var ErrInvalidCoreRestartSettings = errors.New("定时重启设置无效")

type scheduledCore interface {
	IsRunning() bool
}

type scheduledConfigGenerator interface {
	CoreRestartGeneration() uint64
	ReconcileCurrentForScheduledRestart(uint64) (*model.ConfigGenerateResponse, error)
}

type CoreRestartScheduler struct {
	store    *store.Store
	core     scheduledCore
	config   scheduledConfigGenerator
	realtime *RealtimeService
	cron     *cron.Cron

	mu      sync.Mutex
	entryID cron.EntryID
	started bool
}

func NewCoreRestartScheduler(db *store.Store, core scheduledCore, config scheduledConfigGenerator, realtime *RealtimeService) *CoreRestartScheduler {
	return &CoreRestartScheduler{
		store:    db,
		core:     core,
		config:   config,
		realtime: realtime,
		cron:     cron.New(cron.WithSeconds()),
	}
}

func (svc *CoreRestartScheduler) StartScheduler() error {
	settings, err := svc.store.GetCoreRestartSettings()
	if err != nil {
		return err
	}
	if err := validateCoreRestartSettings(settings); err != nil {
		return err
	}
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if svc.started {
		return nil
	}
	if err := svc.scheduleLocked(settings); err != nil {
		return err
	}
	svc.cron.Start()
	svc.started = true
	logging.Info("core.restart_scheduler", "定时重启调度器已启动: mode=%s time=%s weekday=%d", settings.Mode, settings.Time, settings.Weekday)
	return nil
}

func (svc *CoreRestartScheduler) StopScheduler() {
	svc.mu.Lock()
	if !svc.started {
		svc.mu.Unlock()
		return
	}
	svc.started = false
	svc.mu.Unlock()
	ctx := svc.cron.Stop()
	select {
	case <-ctx.Done():
	case <-time.After(15 * time.Second):
		logging.Error("core.restart_scheduler", "等待定时重启任务停止超时")
	}
	logging.Info("core.restart_scheduler", "定时重启调度器已停止")
}

func (svc *CoreRestartScheduler) GetSettings() (*model.CoreRestartSettings, error) {
	return svc.store.GetCoreRestartSettings()
}

func (svc *CoreRestartScheduler) UpdateSettings(settings *model.CoreRestartSettings) (*model.CoreRestartSettings, error) {
	if err := validateCoreRestartSettings(settings); err != nil {
		return nil, err
	}
	minute, hour, _ := parseSyncTime(settings.Time)
	settings.Time = fmt.Sprintf("%02d:%02d:00", hour, minute)
	if err := svc.store.SetCoreRestartSettings(settings); err != nil {
		return nil, err
	}
	svc.mu.Lock()
	err := svc.scheduleLocked(settings)
	svc.mu.Unlock()
	if err != nil {
		return nil, err
	}
	logging.Info("core.restart_scheduler", "定时重启设置已更新: mode=%s time=%s weekday=%d", settings.Mode, settings.Time, settings.Weekday)
	return settings, nil
}

func (svc *CoreRestartScheduler) scheduleLocked(settings *model.CoreRestartSettings) error {
	if svc.entryID != 0 {
		svc.cron.Remove(svc.entryID)
		svc.entryID = 0
	}
	spec, enabled, err := coreRestartCronSpec(settings)
	if err != nil || !enabled {
		return err
	}
	entryID, err := svc.cron.AddFunc(spec, svc.runScheduledRestart)
	if err != nil {
		return err
	}
	svc.entryID = entryID
	logging.Info("core.restart_scheduler", "已安排核心定时重启: cron=%s", spec)
	return nil
}

func (svc *CoreRestartScheduler) runScheduledRestart() {
	if svc.core == nil || !svc.core.IsRunning() {
		logging.Info("core.restart_scheduler", "核心未运行，跳过本次定时重启")
		svc.broadcast("skipped", "")
		return
	}
	logging.Info("core.restart_scheduler", "开始执行核心定时重启")
	svc.broadcast("started", "")
	observedRestartGeneration := svc.config.CoreRestartGeneration()
	result, err := svc.config.ReconcileCurrentForScheduledRestart(observedRestartGeneration)
	if err != nil {
		logging.Error("core.restart_scheduler", "核心定时重启失败: %v", err)
		svc.broadcast("failed", err.Error())
		return
	}
	if result == nil || !result.Valid {
		errorMessage := "定时重启前配置校验失败"
		if result != nil && result.Error != "" {
			errorMessage += ": " + result.Error
		}
		logging.Error("core.restart_scheduler", "%s", errorMessage)
		svc.broadcast("failed", errorMessage)
		return
	}
	logging.Info("core.restart_scheduler", "核心定时重启完成")
	svc.broadcast("succeeded", "")
}

func (svc *CoreRestartScheduler) broadcast(status, errorMessage string) {
	if svc.realtime == nil {
		return
	}
	data := map[string]any{"status": status}
	if errorMessage != "" {
		data["error"] = errorMessage
	}
	svc.realtime.Broadcast("core.restart_schedule", data)
}

func validateCoreRestartSettings(settings *model.CoreRestartSettings) error {
	if settings == nil {
		return fmt.Errorf("%w: 设置不能为空", ErrInvalidCoreRestartSettings)
	}
	if settings.Mode != "off" && settings.Mode != "daily" && settings.Mode != "weekly" {
		return fmt.Errorf("%w: mode 仅支持 off/daily/weekly", ErrInvalidCoreRestartSettings)
	}
	if _, _, ok := parseSyncTime(settings.Time); !ok {
		return fmt.Errorf("%w: time 必须是有效的 HH:mm 时间", ErrInvalidCoreRestartSettings)
	}
	if settings.Weekday < 0 || settings.Weekday > 6 {
		return fmt.Errorf("%w: weekday 必须在 0-6 之间", ErrInvalidCoreRestartSettings)
	}
	return nil
}

func coreRestartCronSpec(settings *model.CoreRestartSettings) (string, bool, error) {
	if err := validateCoreRestartSettings(settings); err != nil {
		return "", false, err
	}
	if settings.Mode == "off" {
		return "", false, nil
	}
	minute, hour, _ := parseSyncTime(settings.Time)
	if settings.Mode == "weekly" {
		return fmt.Sprintf("0 %d %d * * %d", minute, hour, settings.Weekday), true, nil
	}
	return fmt.Sprintf("0 %d %d * * *", minute, hour), true, nil
}
