package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

var ErrSimpleSetupBusy = errors.New("一键配置正在进行，请等待完成后再操作")

type SimpleSetupService struct {
	store        *store.Store
	paths        *paths.Paths
	installer    *InstallerService
	subscription *SubscriptionService
	rules        *RouteRuleService
	generator    *ConfigGeneratorService
	core         *SingboxService
	realtime     *RealtimeService
	supported    func() bool
	mu           sync.Mutex
	mutations    sync.RWMutex
	state        model.SimpleSetupStatus
	closed       bool
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

func NewSimpleSetupService(db *store.Store, p *paths.Paths, installer *InstallerService, subscription *SubscriptionService, rules *RouteRuleService, generator *ConfigGeneratorService, core *SingboxService, rt *RealtimeService) *SimpleSetupService {
	return &SimpleSetupService{store: db, paths: p, installer: installer, subscription: subscription, rules: rules, generator: generator, core: core, realtime: rt, supported: platformSupportsDNSMasqTakeover, state: model.SimpleSetupStatus{Status: "idle", Stage: "等待配置"}}
}

func (svc *SimpleSetupService) Status() (*model.SimpleSetupStatus, error) {
	svc.mu.Lock()
	state := svc.state
	svc.mu.Unlock()
	marker, err := svc.store.SimpleSetupState()
	if err != nil {
		return nil, err
	}
	state.Supported = svc.supported()
	active, exists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, err
	}
	available, err := svc.store.SimpleSetupAvailable()
	if err != nil {
		return nil, err
	}
	owned := (marker == "applied" || marker == "configured") && filepath.Clean(active) == filepath.Join(svc.paths.ConfigDir, "simple.json")
	state.Configured = marker == "configured" && exists && owned
	state.HasExistingConfig = (exists && !owned) || (!available && marker == "")
	subscription, err := svc.store.SimpleSetupSubscription()
	if err != nil {
		return nil, err
	}
	state.CanRetry = subscription != nil && !state.Configured && !state.HasExistingConfig
	if (state.Status == "idle" || state.Status == "succeeded") && state.Configured {
		state.Status, state.Stage = "succeeded", "已完成一键配置"
	} else if (state.Status == "idle" || state.Status == "succeeded") && marker != "" {
		state.Status, state.Stage, state.Error = "failed", "上次配置未完成", "配置任务曾中断，可以重试继续。"
	}
	return &state, nil
}

func (svc *SimpleSetupService) HoldMutation() (func(), error) {
	svc.mutations.RLock()
	svc.mu.Lock()
	busy := svc.state.Status == "running" || svc.closed
	svc.mu.Unlock()
	if busy {
		svc.mutations.RUnlock()
		return nil, ErrSimpleSetupBusy
	}
	return svc.mutations.RUnlock, nil
}

func (svc *SimpleSetupService) Start(req model.SimpleSetupRequest) (*model.SimpleSetupStatus, error) {
	rawURL := strings.TrimSpace(req.SubscriptionURL)
	svc.mutations.Lock()
	defer svc.mutations.Unlock()
	svc.mu.Lock()
	busy := svc.state.Status == "running" || svc.closed
	svc.mu.Unlock()
	if busy {
		return nil, ErrSimpleSetupBusy
	}
	status, err := svc.Status()
	if err != nil {
		return nil, err
	}
	if !status.Supported {
		return nil, errors.New("一键网络接管仅支持 OpenWrt 路由器")
	}
	if status.HasExistingConfig {
		return nil, errors.New("检测到已有专业配置，请在专业模式管理；向导不会覆盖现有配置")
	}
	if status.Configured {
		return nil, errors.New("已完成一键配置，请使用启停或更新订阅操作")
	}
	saved, err := svc.store.SimpleSetupSubscription()
	if err != nil {
		return nil, err
	}
	marker, err := svc.store.SimpleSetupState()
	if err != nil {
		return nil, err
	}
	if saved != nil {
		if marker == "applied" && rawURL != "" && rawURL != saved.URL {
			return nil, errors.New("配置已保存，请留空订阅地址重试启动；更换订阅请在启动成功后进入专业模式")
		}
		if rawURL == "" {
			rawURL = saved.URL
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return nil, errors.New("请输入有效的 HTTP 或 HTTPS 订阅地址")
	}
	if svc.core.IsRunning() {
		return nil, errors.New("核心正在运行，请先停止后再配置")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	svc.mu.Lock()
	svc.cancel = cancel
	svc.state = model.SimpleSetupStatus{Status: "running", Stage: "检查核心"}
	svc.wg.Add(1)
	svc.mu.Unlock()
	go func() {
		defer svc.wg.Done()
		defer cancel()
		if err := svc.run(ctx, rawURL); err != nil {
			message := strings.ReplaceAll(err.Error(), rawURL, "[订阅地址]")
			svc.update("failed", "配置未完成", message)
			return
		}
		svc.update("succeeded", "配置已应用，核心已启动", "")
	}()
	return svc.Status()
}

func (svc *SimpleSetupService) update(status, stage, failure string) {
	svc.mu.Lock()
	svc.state.Status, svc.state.Stage, svc.state.Error = status, stage, failure
	state := svc.state
	svc.mu.Unlock()
	if failure == "" {
		logging.Info("setup.progress", "%s", stage)
	} else {
		logging.Error("setup.failed", "%s", failure)
	}
	if svc.realtime != nil {
		svc.realtime.Broadcast("setup.progress", state)
	}
}

func (svc *SimpleSetupService) run(ctx context.Context, rawURL string) (runErr error) {
	if os.Geteuid() != 0 {
		return errors.New("一键网络接管需要以 root 身份运行 Ackwrap")
	}
	if _, err := os.Stat("/dev/net/tun"); err != nil {
		return errors.New("未检测到 TUN 设备，请先在 OpenWrt 安装 kmod-tun")
	}
	forwarding, err := os.ReadFile("/proc/sys/net/ipv4/ip_forward")
	if err != nil || strings.TrimSpace(string(forwarding)) != "1" {
		return errors.New("路由器尚未开启 IPv4 转发，请在专业模式检查网络设置")
	}
	if _, err := os.Stat(svc.paths.BinaryPath); os.IsNotExist(err) {
		svc.update("running", "下载并安装核心", "")
		if _, err := svc.installer.Install(); err != nil {
			return err
		}
		for {
			if err := setupWait(ctx); err != nil {
				return err
			}
			state, err := svc.store.GetInstallState()
			if err != nil {
				return err
			}
			if state != nil && state.Status == model.InstallFailed {
				return errors.New("核心下载或安装失败，请检查路由器网络后重试")
			}
			if state != nil && state.Status == model.InstallDone {
				break
			}
		}
	} else if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	marker, err := svc.store.SimpleSetupState()
	if err != nil {
		return err
	}
	_, activeExists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	if marker != "applied" || !activeExists {
		svc.update("running", "准备内置规则与自动选节点", "")
		svc.subscription.cron.Stop()
		release := svc.store.HoldConfigSnapshot()
		_, exists, checkErr := svc.paths.ActiveConfigPath()
		if checkErr != nil || exists {
			release()
			return errors.New("检测到已有活动配置，已停止初始化")
		}
		prepareErr := svc.store.PrepareSimpleSetup()
		if prepareErr == nil && marker == "configured" {
			prepareErr = svc.store.SetSimpleSetupState("prepared")
		}
		release()
		if prepareErr != nil {
			return prepareErr
		}
		svc.update("running", "同步订阅", "")
		if err := svc.syncSubscription(ctx, rawURL); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		svc.update("running", "下载并校验分流规则", "")
		rules, err := svc.store.ListRouteRules()
		if err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			values := []mixedRouteRuleValue{}
			if rule.RuleType == "mixed" {
				values, err = parseMixedRouteRuleValues(rule.Values)
				if err != nil {
					return err
				}
			} else {
				for _, value := range rule.Values {
					values = append(values, mixedRouteRuleValue{RuleType: rule.RuleType, Value: value})
				}
			}
			for _, value := range values {
				if value.RuleType != "geosite" && value.RuleType != "geoip" {
					continue
				}
				tag := generatedGeoRuleSetTag(value.RuleType, value.Value)
				if seen[tag] {
					continue
				}
				seen[tag] = true
				if _, _, err := svc.rules.GeneratedGeoRuleSetContentContext(ctx, tag); err != nil {
					return fmt.Errorf("规则 %s 下载或校验失败，请检查网络后重试", tag)
				}
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		svc.update("running", "校验并应用配置", "")
		if err := svc.apply(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	svc.update("running", "启动核心并接管网络", "")
	if _, err := svc.core.Start(); err != nil {
		return fmt.Errorf("核心启动失败: %w", err)
	}
	defer func() {
		if runErr != nil && svc.core.IsRunning() {
			_, stopErr := svc.core.Stop()
			runErr = errors.Join(runErr, stopErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		_, _ = svc.core.Stop()
		return err
	}
	if !svc.core.IsRunning() {
		return errors.New("核心启动后退出，请在专业模式查看核心日志")
	}
	svc.update("running", "检查核心运行状态", "")
	report, err := svc.core.NetworkCheck()
	if cancelErr := ctx.Err(); cancelErr != nil {
		_, stopErr := svc.core.Stop()
		return errors.Join(cancelErr, stopErr)
	}
	if err != nil || report == nil || !report.Success {
		_, stopErr := svc.core.Stop()
		if stopErr != nil {
			return errors.New("启动检查失败，网络恢复未完成，请在专业模式检查核心日志")
		}
		return errors.New("启动检查失败，已停止核心并恢复网络，请在专业模式检查诊断结果")
	}
	items, err := svc.store.ListSubscriptions()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		_, stopErr := svc.core.Stop()
		return errors.Join(err, stopErr)
	}
	if err := svc.store.SetSimpleSetupState("configured"); err != nil {
		_, stopErr := svc.core.Stop()
		return errors.Join(err, stopErr)
	}
	for i := range items {
		svc.subscription.refreshJob(items[i].ID)
	}
	svc.subscription.cron.Start()
	return nil
}

func setupWait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(time.Second):
		return nil
	}
}

func (svc *SimpleSetupService) syncSubscription(ctx context.Context, rawURL string) error {
	saved, err := svc.store.SimpleSetupSubscription()
	if err != nil {
		return err
	}
	if saved != nil {
		for {
			svc.subscription.syncMu.Lock()
			syncing := svc.subscription.syncing[saved.ID]
			svc.subscription.syncMu.Unlock()
			if !syncing {
				break
			}
			if err := setupWait(ctx); err != nil {
				return err
			}
		}
	}
	req := &model.SubscriptionRequest{Name: "简易订阅", URL: rawURL, SyncMode: "daily", SyncTime: "04:00:00", SyncTimeoutSecs: 60}
	if err := validateSubscription(req); err != nil {
		return err
	}
	item, err := svc.store.SaveSimpleSetupSubscription(req)
	if err != nil {
		return err
	}
	svc.subscription.runSyncWithReconcile(item.ID, false)
	updated, err := svc.store.GetSubscription(item.ID)
	if err != nil {
		return err
	}
	if updated == nil || updated.SyncStatus != "updated" || updated.NodeCount == 0 {
		return errors.New("订阅同步失败或没有可用节点，请检查订阅地址后重试")
	}
	return nil
}

func (svc *SimpleSetupService) apply() error {
	release := svc.store.HoldConfigSnapshot()
	defer release()
	svc.generator.configMu.Lock()
	defer svc.generator.configMu.Unlock()
	defer svc.generator.discardGeneratedConfig()
	_, exists, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return err
	}
	if exists {
		return errors.New("配置期间出现其他活动配置，已停止应用以保护现有配置")
	}
	result, err := svc.generator.generateCurrentLocked()
	if err != nil {
		return err
	}
	if !result.Valid {
		return fmt.Errorf("配置校验失败: %s", result.Error)
	}
	if err := svc.generator.applyLocked("simple.json"); err != nil {
		return err
	}
	return svc.store.SetSimpleSetupState("applied")
}

func (svc *SimpleSetupService) Close() {
	svc.mutations.Lock()
	svc.mu.Lock()
	svc.closed = true
	if svc.cancel != nil {
		svc.cancel()
	}
	svc.mu.Unlock()
	svc.mutations.Unlock()
	svc.wg.Wait()
}

func (s *ConfigGeneratorService) checkSimpleSetupPending() error {
	state, err := s.store.SimpleSetupState()
	if err != nil {
		return err
	}
	if state == "prepared" || state == "applied" {
		return errors.New("一键配置尚未完成，已暂停自动应用，请在简易模式重试")
	}
	return nil
}
