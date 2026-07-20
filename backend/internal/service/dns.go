package service

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

type DNSService struct {
	store *store.Store
}

func NewDNSService(s *store.Store) *DNSService {
	return &DNSService{store: s}
}

func validateDNSServerRequest(req *model.DNSServerRequest) error {
	if req == nil {
		return fmt.Errorf("DNS Server 请求不能为空")
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
	return svc.store.CreateDNSServer(req)
}

func (svc *DNSService) UpdateDNSServer(id int64, req *model.DNSServerRequest) error {
	server, err := svc.store.GetDNSServer(id)
	if err != nil {
		return err
	}
	if req == nil {
		return fmt.Errorf("DNS Server 请求不能为空")
	}
	if err := validateDNSServerRequest(req); err != nil {
		return err
	}
	if server.Tag != req.Tag || !req.Enabled || !isStrategyDNSRemoteType(req.ServerType) {
		if err := svc.ensureDNSServerNotUsedByStrategy(server.Tag); err != nil {
			return err
		}
	}
	return svc.store.UpdateDNSServer(id, req)
}

func (svc *DNSService) DeleteDNSServer(id int64) error {
	server, err := svc.store.GetDNSServer(id)
	if err != nil {
		return err
	}
	if err := svc.ensureDNSServerNotUsedByStrategy(server.Tag); err != nil {
		return err
	}
	return svc.store.DeleteDNSServer(id)
}

func (svc *DNSService) ensureDNSServerNotUsedByStrategy(serverTag string) error {
	rules, err := svc.store.ListDNSRules()
	if err != nil {
		return fmt.Errorf("读取 DNS 规则失败: %w", err)
	}
	for _, rule := range rules {
		if !rule.Enabled || rule.Server != serverTag {
			continue
		}
		conditions, err := parseDNSRuleConditions(rule.ConditionsJSON)
		if err != nil {
			return fmt.Errorf("DNS 规则 %d conditions_json 无效，请先修复该规则", rule.ID)
		}
		if dnsRuleHasOutboundCondition(conditions) {
			return fmt.Errorf("DNS Server %s 正被启用的策略 DNS 规则引用，请先停用或修改该规则", serverTag)
		}
	}
	return nil
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
	logging.Info("dns.server.reorder", "调整 %d 个 DNS Server 的顺序", len(ids))
	return svc.store.ReorderDNSServers(ids)
}

func (svc *DNSService) GetDNSOutboundBindingOrder() (*model.DNSOutboundBindingOrder, error) {
	outbounds, err := svc.store.GetDNSOutboundBindingOrder()
	if err != nil {
		return nil, err
	}
	return &model.DNSOutboundBindingOrder{Outbounds: outbounds}, nil
}

func (svc *DNSService) SetDNSOutboundBindingOrder(req *model.DNSOutboundBindingOrder) error {
	if len(req.Outbounds) > 1000 {
		return fmt.Errorf("DNS 出口绑定顺序数量过多")
	}
	seen := make(map[string]bool, len(req.Outbounds))
	normalized := make([]string, 0, len(req.Outbounds))
	for _, outbound := range req.Outbounds {
		outbound = strings.TrimSpace(outbound)
		if outbound == "" || seen[outbound] {
			return fmt.Errorf("DNS 出口绑定顺序包含空值或重复项")
		}
		seen[outbound] = true
		normalized = append(normalized, outbound)
	}
	logging.Info("dns.outbound_binding.reorder", "调整 %d 个 DNS 出口绑定的显示顺序", len(normalized))
	return svc.store.SetDNSOutboundBindingOrder(normalized)
}

// DNS Rules

func (svc *DNSService) ListDNSRules() ([]model.DNSRule, error) {
	return svc.store.ListDNSRules()
}

func (svc *DNSService) GetDNSRule(id int64) (*model.DNSRule, error) {
	return svc.store.GetDNSRule(id)
}

func (svc *DNSService) CreateDNSRule(req *model.DNSRuleRequest) (*model.DNSRule, error) {
	if err := svc.validateStrategyDNSRule(0, req); err != nil {
		return nil, err
	}
	return svc.store.CreateDNSRule(req)
}

func (svc *DNSService) UpdateDNSRule(id int64, req *model.DNSRuleRequest) error {
	if err := svc.validateStrategyDNSRule(id, req); err != nil {
		return err
	}
	return svc.store.UpdateDNSRule(id, req)
}

func (svc *DNSService) validateStrategyDNSRule(excludeRuleID int64, req *model.DNSRuleRequest) error {
	if req == nil || !req.Enabled {
		return nil
	}
	if !dnsRuleHasOutboundCondition(req.Conditions) {
		return nil
	}
	outbounds := dnsRuleOutboundConditions(req.Conditions)
	if len(outbounds) != 1 {
		return fmt.Errorf("策略 DNS 绑定必须且只能包含一个 outbound")
	}
	if len(req.Conditions) != 1 {
		return fmt.Errorf("策略 DNS 绑定只能包含 outbound 条件")
	}
	outbound := strings.TrimSpace(outbounds[0])
	if outbound == "" || outbound != outbounds[0] || outbound == "block" || outbound == "reject" {
		return fmt.Errorf("策略 DNS 绑定引用的 outbound 无效")
	}
	if err := svc.validateDNSStrategyOutbound(outbound); err != nil {
		return err
	}
	rules, err := svc.store.ListDNSRules()
	if err != nil {
		return fmt.Errorf("读取 DNS 规则失败: %w", err)
	}
	if err := validateEnabledDNSRuleConditions(rules); err != nil {
		return err
	}
	for _, rule := range rules {
		if rule.ID == excludeRuleID || !rule.Enabled {
			continue
		}
		conditions := decodeDNSRuleConditions(rule.ConditionsJSON)
		if isDNSStrategyBindingConditions(conditions) && strings.TrimSpace(dnsRuleOutboundConditions(conditions)[0]) == outbound {
			return fmt.Errorf("策略 %s 已存在启用的 DNS 绑定", outbound)
		}
	}
	servers, err := svc.store.ListDNSServers()
	if err != nil {
		return fmt.Errorf("读取 DNS Server 失败: %w", err)
	}
	for _, server := range servers {
		if server.Tag != req.Server {
			continue
		}
		if !server.Enabled {
			return fmt.Errorf("策略 DNS 绑定不能引用已停用的 DNS Server %s", req.Server)
		}
		if !isStrategyDNSRemoteType(server.ServerType) {
			return fmt.Errorf("DNS Server %s 类型 %s 不能用于防泄漏策略绑定，仅支持 udp/tcp/tls/https/quic/h3", req.Server, server.ServerType)
		}
		return nil
	}
	return fmt.Errorf("策略 DNS 绑定引用的 DNS Server %s 不存在", req.Server)
}

func (svc *DNSService) validateDNSStrategyOutbound(outbound string) error {
	if outbound == "direct" || outbound == "proxy" {
		return nil
	}
	collections, err := svc.store.ListProxyCollections()
	if err != nil {
		return fmt.Errorf("读取策略组失败: %w", err)
	}
	for _, collection := range collections {
		if collection.Enabled && collection.Name == outbound {
			return nil
		}
	}
	return fmt.Errorf("策略 DNS 绑定引用的 outbound %s 不存在或未启用", outbound)
}

func (svc *DNSService) DeleteDNSRule(id int64) error {
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
	logging.Info("dns.rule.reorder", "调整 %d 条 DNS 规则的顺序", len(ids))
	return svc.store.ReorderDNSRules(ids)
}

// DNS Global Settings

func (svc *DNSService) GetDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	settings, err := svc.store.GetDNSGlobalSettings()
	if err != nil {
		return nil, err
	}
	applyTUNManagedFakeIP(settings, svc.store.GetInboundMode())
	return settings, nil
}

func (svc *DNSService) SetDNSGlobalSettings(req *model.DNSGlobalSettings) error {
	applyTUNManagedFakeIP(req, svc.store.GetInboundMode())
	logging.Info("dns.global.update", "FakeIP 跟随 TUN 模式，当前状态: %t", req.FakeIPEnabled)
	return svc.store.SetDNSGlobalSettings(req)
}

func applyTUNManagedFakeIP(settings *model.DNSGlobalSettings, inboundMode string) {
	if settings != nil {
		settings.FakeIPEnabled = inboundMode != "mixed"
	}
}
