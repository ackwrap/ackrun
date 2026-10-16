//go:build linux

package service

import (
	"fmt"
	"net/netip"
	"strings"
)

func deviceLocalNFTMatch(family string, fakeIPs ...string) string {
	var cidrs []string
	// Callers validate pools before rendering firewall rules.
	fakeIPs, _ = normalizeDeviceFakeIPCIDRs(fakeIPs...)
	local, _ := excludeDeviceFakeIPCIDRs(deviceLocalDestinationCIDRs(), fakeIPs)
	for _, value := range local {
		if netip.MustParsePrefix(value).Addr().Is6() == (family == "ip6") {
			cidrs = append(cidrs, value)
		}
	}
	if len(cidrs) == 0 {
		// An all-address FakeIP pool must disable the exception, never match all.
		protocol := "ipv4"
		if family == "ip6" {
			protocol = "ipv6"
		}
		return fmt.Sprintf("meta nfproto %s meta nfproto != %s", protocol, protocol)
	}
	set := "{ " + strings.Join(cidrs, ", ") + " }"
	// Do not turn DNAT from a public destination to a private relay into a
	// direct-WAN exception. Both the original and current target must be local.
	return fmt.Sprintf("%s daddr %s ct original %s daddr %s", family, set, family, set)
}
