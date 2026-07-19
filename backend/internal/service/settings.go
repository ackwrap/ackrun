package service

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type SettingsService struct {
	store                    *store.Store
	singbox                  *SingboxService
	configGenerator          modeConfigGenerator
	connectivitySettingsHook func()
}

type modeConfigGenerator interface {
	ReconcileCurrent() (*model.ConfigGenerateResponse, error)
}

var ErrModeChangeWhileRunning = errors.New("核心运行时不能切换模式，请先停止核心")
var ErrConnectivitySettingsInvalid = errors.New("连通性测速设置无效")

func NewSettingsService(s *store.Store) *SettingsService {
	return &SettingsService{store: s}
}

func (svc *SettingsService) SetModeDependencies(singbox *SingboxService, generator modeConfigGenerator) {
	svc.singbox = singbox
	svc.configGenerator = generator
}

func (svc *SettingsService) SetConnectivitySettingsHook(hook func()) {
	svc.connectivitySettingsHook = hook
}

func (svc *SettingsService) GetUpdateSettings() (*model.UpdateSettingsResponse, error) {
	return svc.store.GetUpdateSettings()
}

func (svc *SettingsService) SetUpdateSettings(req *model.UpdateSettings) error {
	req.Acceleration = strings.TrimSpace(req.Acceleration)
	req.CustomMirrorURL = strings.TrimSpace(req.CustomMirrorURL)
	switch req.Acceleration {
	case "", "ghproxy", "ghproxy_vip", "jsdelivr_fastly", "jsdelivr_testingcf", "jsdelivr_cdn", "custom":
	default:
		return fmt.Errorf("更新加速方式无效")
	}
	if req.Acceleration == "custom" {
		if req.CustomMirrorURL == "" {
			return fmt.Errorf("自定义镜像 URL 不能为空")
		}
		if err := validateUpdateURL(req.CustomMirrorURL, "自定义镜像 URL"); err != nil {
			return err
		}
	}
	return svc.store.SetUpdateSettings(req)
}

func validateUpdateURL(value, field string) error {
	parsed, err := url.ParseRequestURI(value)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return fmt.Errorf("%s 必须是有效的 http/https URL", field)
	}
	return nil
}

func (svc *SettingsService) GetLogSettings() (*model.LogSettingsResponse, error) {
	return svc.store.GetLogSettings()
}

func (svc *SettingsService) SetLogSettings(req *model.LogSettings) error {
	req.Level = strings.ToLower(strings.TrimSpace(req.Level))
	switch req.Level {
	case "trace", "debug", "info", "warn", "error", "fatal", "panic":
	default:
		return fmt.Errorf("日志级别无效")
	}
	if err := svc.store.SetLogSettings(req); err != nil {
		return err
	}
	generateRequest, err := svc.store.GetConfigGenerateRequest()
	if err != nil {
		return fmt.Errorf("读取配置生成参数失败: %w", err)
	}
	if generateRequest == nil {
		return nil
	}
	generateRequest.LogLevel = req.Level
	return svc.store.SetConfigGenerateRequest(generateRequest)
}

func (svc *SettingsService) GetConnectivitySettings() (*model.ConnectivitySettings, error) {
	return svc.store.GetConnectivitySettings()
}

func (svc *SettingsService) SetConnectivitySettings(req *model.ConnectivitySettings) error {
	req.TestURL = strings.TrimSpace(req.TestURL)
	if err := validateUpdateURL(req.TestURL, "连通性地址"); err != nil {
		return fmt.Errorf("%w: %v", ErrConnectivitySettingsInvalid, err)
	}
	target, err := svc.store.GetConnectivityTargetByURL(req.TestURL)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%w: 请先在连通性地址列表中添加该 URL", ErrConnectivitySettingsInvalid)
	}
	if err != nil {
		return err
	}
	if !target.Enabled {
		return fmt.Errorf("%w: 所选连通性地址已停用", ErrConnectivitySettingsInvalid)
	}
	if req.IntervalSeconds < 60 || req.IntervalSeconds > 3600 {
		return fmt.Errorf("%w: 连通间隔必须在 60 到 3600 秒之间", ErrConnectivitySettingsInvalid)
	}
	if err := svc.store.SetConnectivitySettings(req); err != nil {
		return err
	}
	logging.Info("settings.update", "连通性测速设置已更新，间隔: %ds", req.IntervalSeconds)
	if svc.connectivitySettingsHook != nil {
		svc.connectivitySettingsHook()
	}
	return nil
}

func (svc *SettingsService) GetNTPSettings() (*model.NTPSettingsResponse, error) {
	return svc.store.GetNTPSettings()
}

func (svc *SettingsService) SetNTPSettings(req *model.NTPSettings) error {
	return svc.store.SetNTPSettings(req)
}

func (svc *SettingsService) GetDNSSettings() (*model.DNSSettingsResponse, error) {
	return svc.store.GetDNSSettings()
}

func (svc *SettingsService) SetDNSSettings(req *model.DNSSettings) error {
	return svc.store.SetDNSSettings(req)
}

func (svc *SettingsService) GetInboundMode() string {
	return svc.store.GetInboundMode()
}

func (svc *SettingsService) SetInboundMode(mode string) error {
	switch mode {
	case "tun", "mixed", "tun_mixed":
	default:
		return fmt.Errorf("运行模式无效")
	}
	logging.Info("settings.update", "切换运行模式: %s，FakeIP: %t", mode, mode != "mixed")
	return svc.setMode(svc.store.GetInboundMode(), mode, svc.store.SetInboundMode)
}

func (svc *SettingsService) GetProxyMode() string {
	return svc.store.GetProxyMode()
}

func (svc *SettingsService) SetProxyMode(mode string) error {
	switch mode {
	case "rule", "global", "direct":
	default:
		return fmt.Errorf("代理模式无效")
	}
	logging.Info("settings.update", "切换代理模式: %s", mode)
	return svc.setMode(svc.store.GetProxyMode(), mode, svc.store.SetProxyMode)
}

func (svc *SettingsService) setMode(previous, next string, persist func(string) error) error {
	if svc.singbox != nil && svc.singbox.IsRunning() {
		return ErrModeChangeWhileRunning
	}
	if previous == next {
		return nil
	}
	if err := persist(next); err != nil {
		return err
	}
	if svc.configGenerator == nil {
		return nil
	}
	result, err := svc.configGenerator.ReconcileCurrent()
	if err == nil && result != nil && !result.Valid {
		err = fmt.Errorf("配置校验失败: %s", result.Error)
	}
	if err == nil {
		return nil
	}
	if rollbackErr := persist(previous); rollbackErr != nil {
		return fmt.Errorf("切换模式失败: %v；回滚模式也失败: %w", err, rollbackErr)
	}
	return fmt.Errorf("切换模式失败，已回滚: %w", err)
}

func (svc *SettingsService) GetExperimentalSettings() (*model.ExperimentalSettingsResponse, error) {
	return svc.store.GetExperimentalSettings()
}

func (svc *SettingsService) SetExperimentalSettings(req *model.ExperimentalSettings) error {
	req.ClashAPIPort = strings.TrimSpace(req.ClashAPIPort)
	if req.ClashAPIPort == "" {
		req.ClashAPIPort = "9090"
	}
	port, err := strconv.Atoi(req.ClashAPIPort)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("Clash API 端口必须是 1-65535 之间的整数")
	}
	req.ClashAPIEnabled = true
	return svc.store.SetExperimentalSettings(req)
}

func (svc *SettingsService) ListNodeFilters() ([]model.NodeFilter, error) {
	return svc.store.ListNodeFilters()
}

func (svc *SettingsService) CreateNodeFilter(req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	if err := validateNodeFilter(req); err != nil {
		return nil, err
	}
	return svc.store.CreateNodeFilter(req)
}

func (svc *SettingsService) UpdateNodeFilter(id int64, req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	if err := validateNodeFilter(req); err != nil {
		return nil, err
	}
	item, err := svc.store.UpdateNodeFilter(id, req)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fmt.Errorf("node filter not found")
	}
	return item, nil
}

func (svc *SettingsService) DeleteNodeFilter(id int64) (*model.ActionResponse, error) {
	if err := svc.store.DeleteNodeFilter(id); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "node filter deleted"}, nil
}

func validateNodeFilter(req *model.NodeFilterRequest) error {
	req.Name = strings.TrimSpace(req.Name)
	req.Target = strings.TrimSpace(req.Target)
	req.Pattern = strings.TrimSpace(req.Pattern)
	if req.Name == "" {
		return fmt.Errorf("filter name is required")
	}
	switch req.Target {
	case "all", "name", "type", "server", "raw", "raw_json":
	default:
		return fmt.Errorf("filter target must be all, name, type, server, raw, or raw_json")
	}
	if req.Pattern == "" {
		return fmt.Errorf("filter pattern is required")
	}
	if _, err := regexp.Compile(req.Pattern); err != nil {
		return fmt.Errorf("invalid regex pattern: %w", err)
	}
	return nil
}
