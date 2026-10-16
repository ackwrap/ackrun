package service

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"slices"
	"strings"

	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func normalizeDeviceFakeIPCIDRs(values ...string) ([]string, error) {
	// Keep the standard pools reserved even after a range change; clients can
	// retain FakeIP addresses longer than the core or DNS configuration.
	result := []string{"198.18.0.0/15", "fc00::/18"}
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("FakeIP 地址池必须是有效的 IPv4/IPv6 CIDR")
		}
		result = appendUniqueStrings(result, prefix.Masked().String())
	}
	slices.Sort(result)
	return result, nil
}

func configDeviceFakeIPCIDRs(data []byte) ([]string, error) {
	var config struct {
		DNS struct {
			Servers []struct {
				Type string `json:"type"`
				IPv4 string `json:"inet4_range"`
				IPv6 string `json:"inet6_range"`
			} `json:"servers"`
			FakeIP struct {
				IPv4 string `json:"inet4_range"`
				IPv6 string `json:"inet6_range"`
			} `json:"fakeip"`
		} `json:"dns"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("读取配置中的 FakeIP 地址池失败: %w", err)
	}
	values := []string{config.DNS.FakeIP.IPv4, config.DNS.FakeIP.IPv6}
	for _, server := range config.DNS.Servers {
		if server.Type == "fakeip" {
			values = append(values, server.IPv4, server.IPv6)
		}
	}
	return normalizeDeviceFakeIPCIDRs(values...)
}

func protectedFakeIPCIDRs(db *store.Store, p *paths.Paths, guard deviceProtectionFirewall) ([]string, error) {
	var values []string
	if db != nil {
		settings, err := db.GetDNSGlobalSettings()
		if err != nil {
			return nil, err
		}
		values = append(values, settings.FakeIPInet4Range, settings.FakeIPInet6Range)
	}
	if p != nil && p.ConfigDir != "" {
		path, exists, err := p.ActiveConfigPath()
		if err != nil {
			return nil, err
		}
		if exists {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			active, err := configDeviceFakeIPCIDRs(data)
			if err != nil {
				return nil, err
			}
			values = append(values, active...)
		}
	}
	if guard != nil {
		persisted, err := guard.FakeIPCIDRs()
		if err != nil {
			return nil, err
		}
		values = append(values, persisted...)
	}
	return normalizeDeviceFakeIPCIDRs(values...)
}

func (s *ConfigGeneratorService) deviceFakeIPCIDRs() ([]string, error) {
	var guard deviceProtectionFirewall
	if core, ok := s.singbox.(*SingboxService); ok && core != nil {
		return core.deviceFakeIPCIDRs()
	} else if s.paths != nil {
		guard = newDeviceProtection(s.paths)
	}
	return protectedFakeIPCIDRs(s.store, s.paths, guard)
}

func (svc *SingboxService) deviceFakeIPCIDRs() ([]string, error) {
	values, err := protectedFakeIPCIDRs(svc.store, svc.paths, svc.deviceProtection)
	if err != nil {
		return nil, err
	}
	if running := svc.runningFakeIPCIDRs.Load(); running != nil {
		values = append(values, (*running)...)
	}
	return normalizeDeviceFakeIPCIDRs(values...)
}

// Subtract complete prefixes, rather than testing only a network's first IP.
// This also lets a broad private TUN exclusion retain its non-FakeIP addresses.
func excludeDeviceFakeIPCIDRs(cidrs, fakeIPs []string) ([]string, error) {
	var prefixes []netip.Prefix
	for _, value := range cidrs {
		prefix, err := netip.ParsePrefix(value)
		if err != nil || prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("本地网络排除包含无效 CIDR")
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	for _, value := range fakeIPs {
		remove, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("FakeIP 地址池包含无效 CIDR")
		}
		var keep []netip.Prefix
		var subtract func(netip.Prefix)
		subtract = func(prefix netip.Prefix) {
			if !prefix.Overlaps(remove) {
				keep = append(keep, prefix)
				return
			}
			if remove.Bits() <= prefix.Bits() {
				return
			}
			bit := prefix.Bits()
			address := prefix.Addr().As16()
			if prefix.Addr().Is4() {
				bit += 96
			}
			address[bit/8] |= 1 << (7 - bit%8)
			right := netip.AddrFrom16(address)
			if prefix.Addr().Is4() {
				right = right.Unmap()
			}
			subtract(netip.PrefixFrom(prefix.Addr(), prefix.Bits()+1))
			subtract(netip.PrefixFrom(right, prefix.Bits()+1))
		}
		for _, prefix := range prefixes {
			subtract(prefix)
		}
		prefixes = keep
	}
	result := make([]string, 0, len(prefixes))
	for _, prefix := range prefixes {
		result = appendUniqueStrings(result, prefix.String())
	}
	return result, nil
}

func (s *ConfigGeneratorService) deviceBypassTargets(targets []string) ([]string, error) {
	devices, err := s.deviceRules()
	if err != nil || len(devices) == 0 {
		return targets, err
	}
	fakeIPs, err := s.deviceFakeIPCIDRs()
	if err != nil {
		return nil, err
	}
	return excludeDeviceFakeIPCIDRs(targets, fakeIPs)
}
