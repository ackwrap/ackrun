package service

import (
	"fmt"
	"net/netip"
	"runtime"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

const deviceDNSInboundTag = "ackwrap-device-dns-in"
const deviceDNSPort = 1054

// Device rules are source-only proxy rules. Keeping them out of mixed OR rules
// makes DNS and traffic matching identical, with no accidental catch-all.
func normalizeDeviceRule(req *model.RouteRuleRequest) error {
	if req.Outbound != "proxy" || req.Invert {
		return fmt.Errorf("设备规则必须使用代理策略，且不能反向匹配")
	}
	values, err := normalizeDeviceSources(req.Values)
	if err != nil {
		return err
	}
	req.Values = values
	req.DirectDomains, err = normalizeDeviceDirectDomains(req.DirectDomains)
	return err
}

func normalizeDeviceCIDRs(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 128 {
		return nil, fmt.Errorf("设备地址数量必须在 1 到 128 之间")
	}
	result := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil || address.Zone() != "" {
				return nil, fmt.Errorf("设备地址必须是 IP 或 CIDR 网段")
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		prefix = prefix.Masked()
		if prefix.Bits() == 0 || prefix.Addr().Is4In6() || !prefix.Addr().IsGlobalUnicast() || prefix.Addr().IsUnspecified() {
			return nil, fmt.Errorf("设备地址不能使用默认路由、回环、组播、链路本地或 IPv4 映射地址")
		}
		result = appendUniqueStrings(result, prefix.String())
	}
	return result, nil
}

func deviceDNSServerTag(id int64) string { return fmt.Sprintf("ackwrap-device-dns-%d", id) }

func validateDeviceCollection(req *model.ProxyCollectionRequest) error {
	if req.Type != "selector" || req.SourceType != "manual" || len(req.ReferencedGroupIDs) > 0 || len(req.NodeUIDs) != 1 || req.NodeUIDs[0] == "direct" || strings.TrimSpace(req.NodeUIDs[0]) == "" {
		return fmt.Errorf("设备策略必须手动固定一个代理节点，不能选择节点组、自动测速或直连")
	}
	return nil
}

func (s *ConfigGeneratorService) deviceRules() ([]model.RouteRule, error) {
	return enabledDeviceRules(s.store)
}

func enabledDeviceRules(db *store.Store) ([]model.RouteRule, error) {
	if db.GetProxyMode() != "rule" {
		return nil, nil
	}
	rules, err := db.ListRouteRules()
	if err != nil {
		return nil, err
	}
	var devices []model.RouteRule
	for _, rule := range rules {
		if !rule.Enabled || rule.RuleType != "source_ip_cidr" {
			continue
		}
		req := model.RouteRuleRequest{Values: rule.Values, DirectDomains: rule.DirectDomains, Outbound: rule.Outbound, Invert: rule.Invert}
		if err := normalizeDeviceRule(&req); err != nil {
			return nil, fmt.Errorf("设备规则 %d 无效: %w", rule.ID, err)
		}
		rule.Values = req.Values
		rule.DirectDomains = req.DirectDomains
		devices = append(devices, rule)
	}
	return devices, nil
}

func deviceRuleCIDRs(rules []model.RouteRule) []string {
	var cidrs []string
	for _, rule := range rules {
		cidrs = appendUniqueStrings(cidrs, rule.Values...)
	}
	return cidrs
}

// Runtime platform rules take precedence over static routing. Explicit proxy
// exposures have their own routing, but an unscoped platform rule must not
// silently redirect a device's TUN traffic or its dedicated DNS listener.
func validateDevicePlatformRoutes(devices []model.RouteRule, routes []model.PlatformRoute) error {
	for _, route := range routes {
		if !route.Enabled || len(route.InboundExposureIDs) > 0 {
			continue
		}
		for _, device := range devices {
			conflict := len(route.SourceCIDRs) == 0
			cidrs, macs := splitDeviceSources(device.Values)
			conflict = conflict || len(macs) > 0
			for _, source := range route.SourceCIDRs {
				prefix, err := netip.ParsePrefix(source)
				if err != nil {
					return fmt.Errorf("平台路由 %d 的来源地址无效", route.ID)
				}
				for _, value := range cidrs {
					conflict = conflict || prefix.Overlaps(netip.MustParsePrefix(value))
				}
			}
			if conflict {
				return fmt.Errorf("设备规则 %d 与高级平台路由 %d 冲突，请停用该平台路由或将其限定到独立代理入口/其他设备", device.ID, route.ID)
			}
		}
	}
	return nil
}

func (s *ConfigGeneratorService) validateDevicePolicies() error {
	rules, err := s.deviceRules()
	if err != nil || len(rules) == 0 {
		return err
	}
	mode := s.store.GetInboundMode()
	if runtime.GOOS != "linux" || (mode != "tun" && mode != "tun_mixed") {
		return fmt.Errorf("设备规则需要 Linux/OpenWrt 的 TUN 或 TUN + Mixed 接管，请切换运行模式或停用设备规则")
	}
	if len(deviceRuleCIDRs(rules)) > 128 {
		return fmt.Errorf("所有启用设备规则合计最多支持 128 个地址范围")
	}
	advanced, err := s.store.GetAdvancedSettings()
	if err != nil {
		return err
	}
	if advanced.RoutingEnabled {
		routes, err := s.store.ListPlatformRoutes()
		if err != nil {
			return err
		}
		if err := validateDevicePlatformRoutes(rules, routes); err != nil {
			return err
		}
	}
	collections, err := s.store.ListProxyCollectionsWithNodes()
	if err != nil {
		return err
	}
	for _, rule := range rules {
		found := false
		for _, col := range collections {
			if col.RouteRuleID != rule.ID || !col.Enabled {
				continue
			}
			req := &model.ProxyCollectionRequest{Type: col.Type, SourceType: col.SourceType, NodeUIDs: col.NodeUIDs}
			if err := validateDeviceCollection(req); err != nil {
				return fmt.Errorf("设备规则 %d: %w", rule.ID, err)
			}
			found = true
		}
		if !found {
			return fmt.Errorf("设备规则 %d 尚未绑定启用的固定节点策略", rule.ID)
		}
	}
	dns, err := s.effectiveDNSGlobalSettings()
	if err != nil {
		return err
	}
	if dns == nil || !dns.Enabled {
		return fmt.Errorf("设备规则需要启用 DNS 管理，才能保证 DNS 跟随设备出口")
	}
	if _, err := s.deviceFakeIPCIDRs(); err != nil {
		return err
	}
	settings, err := s.store.GetTrafficBypassSettings()
	if err != nil {
		return err
	}
	_, interfaces, targets, sources, domains := trafficBypassValues(settings)
	if _, err := normalizeExcludedInterfaces(interfaces); err != nil {
		return err
	}
	if len(domains) > 0 {
		return fmt.Errorf("设备规则不能与域名流量排除同时启用：域名可能解析到外网，请移除域名排除；本地目标 IP/CIDR 可以保留")
	}
	for _, value := range targets {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || !isDeviceLocalDestination(prefix) {
			return fmt.Errorf("设备规则仅允许本地目标 IP/CIDR 流量排除，请移除包含外网地址的目标排除")
		}
	}
	for i, rule := range rules {
		cidrs, macs := splitDeviceSources(rule.Values)
		if len(macs) > 0 && (len(interfaces) > 0 || len(sources) > 0) {
			return fmt.Errorf("MAC 设备规则不能与接口或来源直连排除同时启用，请移除冲突排除")
		}
		for _, prior := range rules[:i] {
			_, previousMACs := splitDeviceSources(prior.Values)
			for _, mac := range macs {
				for _, previous := range previousMACs {
					if mac == previous {
						return fmt.Errorf("设备规则 %d 与 %d 的 MAC 重叠", rule.ID, prior.ID)
					}
				}
			}
		}
		for _, value := range cidrs {
			prefix := netip.MustParsePrefix(value)
			for _, excluded := range sources {
				other, err := netip.ParsePrefix(excluded)
				if err == nil && prefix.Overlaps(other) {
					return fmt.Errorf("设备规则 %d 与设备直连排除重叠，请先移除直连排除", rule.ID)
				}
			}
			for _, prior := range rules[:i] {
				previousCIDRs, _ := splitDeviceSources(prior.Values)
				for _, other := range previousCIDRs {
					if prefix.Overlaps(netip.MustParsePrefix(other)) {
						return fmt.Errorf("设备规则 %d 与 %d 的地址范围重叠，请为设备保留唯一出口", rule.ID, prior.ID)
					}
				}
			}
		}
	}
	return nil
}

func (s *ConfigGeneratorService) appendDeviceDNS(servers []map[string]interface{}, tags map[string]bool, templateTag, directTemplateTag string) ([]map[string]interface{}, []map[string]interface{}, error) {
	devices, err := s.deviceRules()
	if err != nil || len(devices) == 0 {
		return servers, nil, err
	}
	overrides, err := s.routeRuleOutboundOverrides()
	if err != nil {
		return nil, nil, err
	}
	var template map[string]interface{}
	for _, server := range servers {
		if server["tag"] == templateTag {
			template = server
			break
		}
	}
	if template == nil {
		return nil, nil, fmt.Errorf("设备 DNS 缺少可用的代理 DNS Final，请先配置远程 DNS")
	}
	directTemplate := template
	for _, server := range servers {
		if server["tag"] == directTemplateTag {
			if serverType, _ := server["type"].(string); isStrategyDNSRemoteType(serverType) {
				directTemplate = server
			}
		}
	}
	rules := make([]map[string]interface{}, 0, len(devices))
	for _, device := range devices {
		outbound := overrides[device.ID]
		if outbound == "" {
			return nil, nil, fmt.Errorf("设备规则 %d 尚未绑定启用的代理策略", device.ID)
		}
		tag := deviceDNSServerTag(device.ID)
		if tags[tag] {
			return nil, nil, fmt.Errorf("DNS Server 标签 %s 被设备 DNS 保留，请重命名同名 Server", tag)
		}
		clone := make(map[string]interface{}, len(template))
		for key, value := range template {
			clone[key] = value
		}
		clone["tag"], clone["detour"] = tag, outbound
		servers = append(servers, clone)
		tags[tag] = true
		if len(device.DirectDomains) > 0 {
			directTag := deviceDirectDNSServerTag(device.ID)
			if tags[directTag] {
				return nil, nil, fmt.Errorf("DNS Server 标签 %s 被设备 DNS 保留，请重命名同名 Server", directTag)
			}
			directClone := make(map[string]interface{}, len(directTemplate))
			for key, value := range directTemplate {
				directClone[key] = value
			}
			directClone["tag"] = directTag
			// No detour uses the resolver's own direct dialer, preserving bootstrap options.
			delete(directClone, "detour")
			servers = append(servers, directClone)
			tags[directTag] = true
			rules = append(rules, deviceSourceRule(device.Values, map[string]interface{}{
				"domain_suffix": device.DirectDomains, "server": directTag,
			}))
			logging.Info("config_generator.device", "设备规则 %d 生成 %d 个直连域名白名单", device.ID, len(device.DirectDomains))
		}
		rules = append(rules, deviceSourceRule(device.Values, map[string]interface{}{"server": tag}))
	}
	if _, macs := splitDeviceSources(deviceRuleCIDRs(devices)); len(macs) > 0 {
		rules = append(rules, map[string]interface{}{"inbound": deviceMACInbounds(), "action": "reject"})
	}
	logging.Info("config_generator.device", "生成 %d 条设备路由与 DNS 出口绑定", len(devices))
	return servers, rules, nil
}
