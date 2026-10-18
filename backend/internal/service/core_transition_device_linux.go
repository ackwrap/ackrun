//go:build linux

package service

import (
	"fmt"
	"net/netip"
	"strings"
)

type transitionGuardPolicy struct {
	Sources []string `json:"sources"`
	FakeIPs []string `json:"fake_ips"`
}

// Only selected sources enter the blocking chains. In particular, router
// output and unrelated devices keep working while the core is being replaced.
// Block local/previously DNATed DNS as well, before a resolver can relay it with
// the router's identity. Local management and non-DNS LAN access stay available.
func transitionDeviceGuardRules(replace bool, sources, excluded []string, fakeIPs ...string) string {
	var b, matches, bypass, reverseBypass strings.Builder
	if replace {
		fmt.Fprintf(&b, "delete table inet %s\n", transitionGuardTable)
	}
	cidrs, macs := splitDeviceSources(sources)
	for _, cidr := range cidrs {
		family := "ip"
		if netip.MustParsePrefix(cidr).Addr().Is6() {
			family = "ip6"
		}
		fmt.Fprintf(&matches, " %s saddr %s jump protected\n", family, cidr)
	}
	for _, mac := range macs {
		fmt.Fprintf(&matches, " ether saddr %s jump protected\n", mac)
	}
	for _, name := range excluded {
		fmt.Fprintf(&bypass, " iifname %q return\n", name)
		fmt.Fprintf(&reverseBypass, " oifname %q ct direction reply return\n", name)
	}
	input := strings.ReplaceAll(matches.String(), "jump protected", "jump dns")
	fmt.Fprintf(&b, `table inet %s {
 comment %q
 chain dns {
  meta l4proto { tcp, udp } th dport 53 counter drop
  meta l4proto { tcp, udp } ct original proto-dst 53 counter drop
 }
 chain protected {
  jump dns
  %s return
  %s return
  counter drop
 }
 chain forward {
  type filter hook forward priority -1; policy accept;
%s%s%s }
 chain input {
  type filter hook input priority -1; policy accept;
%s%s }
}
`, transitionGuardTable, transitionGuardOwner, deviceLocalNFTMatch("ip", fakeIPs...), deviceLocalNFTMatch("ip6", fakeIPs...),
		bypass.String(), reverseBypass.String(), matches.String(), bypass.String(), input)
	return b.String()
}
