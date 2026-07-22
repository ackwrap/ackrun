package service

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

type DNSService struct {
	store           *store.Store
	paths           *paths.Paths
	readCoreVersion func() string
	cacheMu         sync.Mutex
}

func NewDNSService(s *store.Store, p *paths.Paths) *DNSService {
	return &DNSService{store: s, paths: p}
}

func validateDNSServerRequest(req *model.DNSServerRequest) error {
	if req == nil {
		return fmt.Errorf("DNS Server 请求不能为空")
	}
	if req.ServerType == "fakeip" {
		return fmt.Errorf("FakeIP Server 由 TUN 模式自动管理，不能手动创建或更新")
	}
	if err := validateDNSServerDetour(req.Detour); err != nil {
		return err
	}
	return validateDNSServerOptions(req.Options)
}

func validateDNSServerDetour(detour string) error {
	if detour != strings.TrimSpace(detour) {
		return fmt.Errorf("DNS Server detour 包含无效空白")
	}
	if detour == "block" || detour == "reject" {
		return fmt.Errorf("DNS Server detour 不能是 %s", detour)
	}
	return nil
}

func validateDNSServerOptions(options map[string]interface{}) error {
	for key := range options {
		switch key {
		case "tag", "type", "server", "server_port", "path", "detour", "domain_resolver", "domain_strategy", "strategy", "client_subnet", "address_resolver", "address_strategy":
			return fmt.Errorf("DNS Server options 不能覆盖受控字段 %s", key)
		}
	}
	return nil
}

// DNS Servers

func (svc *DNSService) ListDNSServers() ([]model.DNSServer, error) {
	return svc.store.ListDNSServers()
}

func (svc *DNSService) GetDNSServer(id int64) (*model.DNSServer, error) {
	return svc.store.GetDNSServer(id)
}

func (svc *DNSService) CreateDNSServer(req *model.DNSServerRequest) (*model.DNSServer, error) {
	if err := validateDNSServerRequest(req); err != nil {
		return nil, err
	}
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	return svc.store.CreateDNSServer(req)
}

func (svc *DNSService) UpdateDNSServer(id int64, req *model.DNSServerRequest) error {
	if req == nil {
		return fmt.Errorf("DNS Server 请求不能为空")
	}
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	svc.cacheMu.Lock()
	defer svc.cacheMu.Unlock()
	if err := validateDNSServerRequest(req); err != nil {
		return err
	}
	current, err := svc.store.GetDNSServer(id)
	if err != nil {
		return err
	}
	settings, err := svc.store.GetDNSGlobalSettings()
	if err != nil {
		return fmt.Errorf("读取 DNS 全局设置失败: %w", err)
	}
	if settings.ProxyFinal == current.Tag && (req.Tag != current.Tag || !req.Enabled || !isStrategyDNSRemoteType(req.ServerType)) {
		return fmt.Errorf("DNS Server %s 正在作为代理 DNS Final，请先更换代理 DNS Final", current.Tag)
	}
	return svc.store.UpdateDNSServer(id, req)
}

func (svc *DNSService) DeleteDNSServer(id int64) error {
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	svc.cacheMu.Lock()
	defer svc.cacheMu.Unlock()
	server, err := svc.store.GetDNSServer(id)
	if err != nil {
		return err
	}
	settings, err := svc.store.GetDNSGlobalSettings()
	if err != nil {
		return fmt.Errorf("读取 DNS 全局设置失败: %w", err)
	}
	if settings.ProxyFinal == server.Tag {
		return fmt.Errorf("DNS Server %s 正在作为代理 DNS Final，请先更换代理 DNS Final", server.Tag)
	}
	return svc.store.DeleteDNSServer(id)
}

func (svc *DNSService) ReorderDNSServers(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("DNS Server ID 不能为空")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("DNS Server ID 无效或重复")
		}
		seen[id] = true
	}
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	logging.Info("dns.server.reorder", "调整 %d 个 DNS Server 的顺序", len(ids))
	return svc.store.ReorderDNSServers(ids)
}

// DNS Rules

func (svc *DNSService) ListDNSRules() ([]model.DNSRule, error) {
	return svc.store.ListDNSRules()
}

func (svc *DNSService) GetDNSRule(id int64) (*model.DNSRule, error) {
	return svc.store.GetDNSRule(id)
}

func (svc *DNSService) CreateDNSRule(req *model.DNSRuleRequest) (*model.DNSRule, error) {
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	if err := svc.validateDNSRuleRequest(req); err != nil {
		return nil, err
	}
	return svc.store.CreateDNSRule(req)
}

func (svc *DNSService) UpdateDNSRule(id int64, req *model.DNSRuleRequest) error {
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	if err := svc.validateDNSRuleRequest(req); err != nil {
		return err
	}
	return svc.store.UpdateDNSRule(id, req)
}

func (svc *DNSService) validateDNSRuleRequest(req *model.DNSRuleRequest) error {
	if req == nil {
		return fmt.Errorf("DNS 规则请求不能为空")
	}
	if dnsRuleHasOutboundCondition(req.Conditions) {
		return fmt.Errorf("DNS 规则不再支持 outbound 条件，请通过 DNS Server detour 配置真实查询出口")
	}
	return svc.rejectFakeIPServerReference(req.Server)
}

func (svc *DNSService) rejectFakeIPServerReference(tag string) error {
	if tag == "fakeip" {
		return fmt.Errorf("显式 DNS 配置不能引用 FakeIP Server，FakeIP 由 TUN 模式下的 A/AAAA 兜底规则自动管理")
	}
	servers, err := svc.store.ListDNSServers()
	if err != nil {
		return fmt.Errorf("读取 DNS Server 失败: %w", err)
	}
	for _, server := range servers {
		if server.Tag == tag && server.ServerType == "fakeip" {
			return fmt.Errorf("显式 DNS 配置不能引用 FakeIP Server，FakeIP 由 TUN 模式下的 A/AAAA 兜底规则自动管理")
		}
	}
	return nil
}

func (svc *DNSService) DeleteDNSRule(id int64) error {
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	return svc.store.DeleteDNSRule(id)
}

func (svc *DNSService) ReorderDNSRules(ids []int64) error {
	if len(ids) == 0 {
		return fmt.Errorf("DNS 规则 ID 不能为空")
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return fmt.Errorf("DNS 规则 ID 无效或重复")
		}
		seen[id] = true
	}
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	logging.Info("dns.rule.reorder", "调整 %d 条 DNS 规则的顺序", len(ids))
	return svc.store.ReorderDNSRules(ids)
}

// DNS Global Settings

func (svc *DNSService) GetDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	settings, err := svc.store.GetDNSGlobalSettings()
	if err != nil {
		return nil, err
	}
	settings.IndependentCacheSupported = svc.independentCacheSupported()
	if !settings.IndependentCacheSupported {
		settings.IndependentCache = false
	}
	applyTUNManagedFakeIP(settings, svc.store.GetInboundMode())
	return settings, nil
}

func (svc *DNSService) SetDNSGlobalSettings(req *model.DNSGlobalSettings) error {
	if req == nil {
		return fmt.Errorf("DNS 全局设置不能为空")
	}
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	svc.cacheMu.Lock()
	defer svc.cacheMu.Unlock()
	req.ProxyFinal = strings.TrimSpace(req.ProxyFinal)
	if err := svc.validateProxyDNSFinal(req.ProxyFinal); err != nil {
		return err
	}
	if err := svc.rejectFakeIPServerReference(req.Final); err != nil {
		return err
	}
	supported := svc.independentCacheSupported()
	req.IndependentCacheSupported = supported
	if !supported {
		req.IndependentCache = false
	}
	applyTUNManagedFakeIP(req, svc.store.GetInboundMode())
	logging.Info("dns.global.update", "代理 DNS Final=%s, FakeIP 跟随 TUN 模式=%t", req.ProxyFinal, req.FakeIPEnabled)
	return svc.store.SetDNSGlobalSettingsForCore(req, supported)
}

func (svc *DNSService) validateProxyDNSFinal(tag string) error {
	if tag == "" {
		return nil
	}
	servers, err := svc.store.ListDNSServers()
	if err != nil {
		return fmt.Errorf("读取 DNS Server 失败: %w", err)
	}
	for _, server := range servers {
		if server.Tag != tag {
			continue
		}
		if !server.Enabled {
			return fmt.Errorf("代理 DNS Final Server %s 未启用", tag)
		}
		if !isStrategyDNSRemoteType(server.ServerType) {
			return fmt.Errorf("代理 DNS Final Server %s 类型 %s 不支持远程查询", tag, server.ServerType)
		}
		return nil
	}
	return fmt.Errorf("代理 DNS Final Server %s 不存在", tag)
}

func (svc *DNSService) MigrateIndependentCache(version string) (bool, error) {
	releaseConfigUpdate := svc.store.HoldConfigUpdate()
	defer releaseConfigUpdate()
	svc.cacheMu.Lock()
	defer svc.cacheMu.Unlock()
	if strings.TrimSpace(version) == "" {
		version = svc.coreVersion()
	}
	if strings.TrimSpace(version) == "" {
		return false, nil
	}
	supported := singboxSupportsDNSIndependentCache(version)
	migrated, err := svc.store.MigrateDNSIndependentCache(supported)
	if err != nil {
		return false, err
	}
	if migrated {
		logging.Info("dns.global.migrate", "DNS 缓存配置已适配当前核心")
	}
	return migrated, nil
}

func (svc *DNSService) independentCacheSupported() bool {
	return singboxSupportsDNSIndependentCache(svc.coreVersion())
}

func (svc *DNSService) coreVersion() string {
	if svc.readCoreVersion != nil {
		return svc.readCoreVersion()
	}
	if svc.paths != nil {
		return readSingboxVersion(svc.paths.BinaryPath)
	}
	return ""
}

func applyTUNManagedFakeIP(settings *model.DNSGlobalSettings, inboundMode string) {
	if settings != nil {
		settings.FakeIPEnabled = inboundMode != "mixed"
	}
}
