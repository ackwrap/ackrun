package service

import (
	"context"
	"net"
	"net/netip"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
)

const (
	nodeBypassResolveTimeout = 8 * time.Second
	nodeBypassLookupTimeout  = 3 * time.Second
	nodeBypassWorkers        = 8
)

type nodeServerBypass struct {
	domains []string
	ipCIDRs []string
}

// The optional snapshot is private to one generation, never shared with previews
// or later subscription updates. Removed/disabled nodes cannot leave stale routes.
func (s *ConfigGeneratorService) nodeServerBypassTargets(snapshot ...*nodeServerBypass) (*nodeServerBypass, error) {
	if len(snapshot) > 0 && snapshot[0] != nil {
		return snapshot[0], nil
	}
	nodes, err := s.store.ListEnabledNodes()
	if err != nil {
		return nil, err
	}
	domains, ipCIDRs := nodeServerBypassTargets(nodes)
	return &nodeServerBypass{domains: domains, ipCIDRs: ipCIDRs}, nil
}

func (s *ConfigGeneratorService) resolveNodeServerBypass(ctx context.Context) (*nodeServerBypass, error) {
	targets, err := s.nodeServerBypassTargets()
	if err != nil || s.store.GetInboundMode() == "mixed" || len(targets.domains) == 0 {
		return targets, err
	}
	fakeIPs, err := s.deviceFakeIPCIDRs()
	if err != nil {
		return nil, err
	}
	var reserved []netip.Prefix
	for _, cidr := range fakeIPs {
		prefix, err := netip.ParsePrefix(cidr)
		if err != nil {
			return nil, err
		}
		reserved = append(reserved, prefix)
	}
	hosts, err := s.store.ListDNSHosts()
	if err != nil {
		return nil, err
	}
	overrides := make(map[string][]net.IP)
	for _, host := range hosts {
		if !host.Enabled {
			continue
		}
		domain := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host.Domain), "."))
		for _, address := range host.Addresses {
			if ip := net.ParseIP(address); ip != nil {
				overrides[domain] = append(overrides[domain], ip)
			}
		}
	}
	lookup := s.nodeBypassLookup
	if lookup == nil {
		// Reuse the marked direct resolver used by node TCP probes. On Linux this
		// avoids both TUN interception and the local DNS/FakeIP listener.
		lookup = resolveTCPingServer
	}
	ctx, cancel := context.WithTimeout(ctx, nodeBypassResolveTimeout)
	defer cancel()
	addresses := make([][]net.IP, len(targets.domains))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for range min(nodeBypassWorkers, len(targets.domains)) {
		workers.Go(func() {
			for index := range jobs {
				domain := targets.domains[index]
				if ips, exists := overrides[domain]; exists {
					addresses[index] = ips
					continue
				}
				queryCtx, queryCancel := context.WithTimeout(ctx, nodeBypassLookupTimeout)
				ips, _ := lookup(queryCtx, domain)
				queryCancel()
				addresses[index] = ips
			}
		})
	}
	for index := range targets.domains {
		jobs <- index
	}
	close(jobs)
	workers.Wait()
	unresolved := 0
	seen := make(map[string]bool, len(targets.ipCIDRs))
	for _, cidr := range targets.ipCIDRs {
		seen[cidr] = true
	}
	for _, ips := range addresses {
		usable := false
		for _, ip := range ips {
			address, ok := netip.AddrFromSlice(ip)
			if !ok {
				continue
			}
			address = address.Unmap()
			if !address.IsGlobalUnicast() || slices.ContainsFunc(reserved, func(prefix netip.Prefix) bool { return prefix.Contains(address) }) {
				continue
			}
			usable = true
			cidr := netip.PrefixFrom(address, address.BitLen()).String()
			if !seen[cidr] {
				seen[cidr] = true
				targets.ipCIDRs = append(targets.ipCIDRs, cidr)
			}
		}
		if !usable {
			unresolved++
		}
	}
	slices.Sort(targets.ipCIDRs)
	logging.Info("config_generator.node_bypass", "节点服务器排除已更新: 域名 %d，IP %d，未取得真实地址 %d", len(targets.domains), len(targets.ipCIDRs), unresolved)
	if unresolved > 0 {
		// Do not make one unreachable subscription endpoint prevent core startup.
		// The real-DNS rule and domain pre-match bypass remain effective on retry.
		logging.Error("config_generator.node_bypass", "%d 个节点域名暂未取得真实 IP，保留域名绕过并在下次生成时重试", unresolved)
	}
	return targets, nil
}

func nodeServerDNSRules(domains, hostDomains []string, hostsTag, bootstrapTag string) []map[string]interface{} {
	var rules []map[string]interface{}
	var overridden []string
	for _, domain := range domains {
		if slices.Contains(hostDomains, domain) {
			overridden = append(overridden, domain)
		}
	}
	if len(overridden) > 0 && hostsTag != "" {
		rules = append(rules, map[string]interface{}{
			"domain": overridden, "query_type": []string{"A", "AAAA"}, "server": hostsTag,
		})
	}
	if len(domains) > 0 {
		rules = append(rules, map[string]interface{}{
			"domain": domains, "query_type": []string{"A", "AAAA"}, "server": bootstrapTag,
		})
	}
	return rules
}
