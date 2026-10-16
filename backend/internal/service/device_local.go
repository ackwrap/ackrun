package service

import "net/netip"

// Keep routing, bypass validation and persistent firewall exceptions on the
// same bounded destination list. FakeIP pools must be subtracted before use.
func deviceLocalDestinationCIDRs() []string {
	return []string{
		"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "169.254.0.0/16",
		"224.0.0.0/24", "239.255.255.250/32", "255.255.255.255/32",
		"fc00::/7", "fe80::/10", "ff02::/16",
	}
}

func isDeviceLocalDestination(prefix netip.Prefix) bool {
	if !prefix.IsValid() || prefix.Addr().Is4In6() {
		return false
	}
	for _, value := range deviceLocalDestinationCIDRs() {
		local := netip.MustParsePrefix(value)
		if prefix.Bits() >= local.Bits() && local.Contains(prefix.Masked().Addr()) {
			return true
		}
	}
	return false
}

func deviceLocalRouteRule(sources, destinations []string) map[string]interface{} {
	return map[string]interface{}{
		"source_ip_cidr": sources,
		"ip_cidr":        destinations,
		"action":         "bypass",
		"outbound":       "direct",
	}
}
