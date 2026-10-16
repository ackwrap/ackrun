package service

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func normalizeDeviceDirectDomains(values []string) ([]string, error) {
	if len(values) > 256 {
		return nil, fmt.Errorf("设备直连域名白名单最多支持 256 个域名")
	}
	domains := make([]string, 0, len(values))
	for _, value := range values {
		domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
		_, ipErr := netip.ParseAddr(domain)
		if !validDNSHostDomain(domain) || !strings.Contains(domain, ".") || ipErr == nil {
			return nil, fmt.Errorf("直连域名 %q 无效：请填写域名或 punycode，不含协议、路径、端口或通配符", value)
		}
		domains = appendUniqueStrings(domains, domain)
	}
	return domains, nil
}

func deviceDirectDNSServerTag(id int64) string {
	return fmt.Sprintf("ackwrap-device-direct-dns-%d", id)
}

func deviceDirectRouteRules(device model.RouteRule, fakeIPs []string) []map[string]interface{} {
	if len(device.DirectDomains) == 0 {
		return nil
	}
	return []map[string]interface{}{
		map[string]interface{}{"source_ip_cidr": device.Values, "network": []string{"tcp", "udp"}, "action": "sniff"},
		map[string]interface{}{"source_ip_cidr": device.Values, "domain_suffix": device.DirectDomains, "action": "resolve", "server": deviceDirectDNSServerTag(device.ID)},
		// Never release an unresolved FakeIP, even when sniffing finds an allowed domain.
		map[string]interface{}{"source_ip_cidr": device.Values, "ip_cidr": fakeIPs, "action": "reject"},
		// Keep public whitelist traffic inside the core so persistent protection still
		// blocks it if the core exits. Kernel bypass here would defeat that contract.
		map[string]interface{}{"source_ip_cidr": device.Values, "domain_suffix": device.DirectDomains, "action": "route", "outbound": "direct"},
	}
}
