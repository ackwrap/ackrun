//go:build linux

package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/ackwrap/ackrun/internal/paths"
)

const (
	transitionGuardTable = "ackwrap_transition"
	transitionGuardOwner = "Ackwrap core transition guard v1"
)

type nftTransitionGuard struct {
	run       func(input string, args ...string) ([]byte, error)
	statePath string
}

func newCoreTransitionGuard(p *paths.Paths) coreTransitionGuard {
	return &nftTransitionGuard{statePath: p.CoreTransitionGuardPath(), run: func(input string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), linuxNetworkCommandTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "nft", args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}}
}

func (g *nftTransitionGuard) inspect() (present, offloaded bool, err error) {
	output, err := g.run("", "-j", "list", "ruleset")
	if err != nil {
		// Mixed-only installations may not have nft. Enable still rejects that.
		return false, false, err
	}
	var ruleset struct {
		NFTables []struct {
			Table *struct {
				Family  string `json:"family"`
				Name    string `json:"name"`
				Comment string `json:"comment"`
			} `json:"table"`
			Flowtable *json.RawMessage `json:"flowtable"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(output, &ruleset); err != nil {
		return false, false, fmt.Errorf("解析 nftables 状态失败: %w", err)
	}
	for _, object := range ruleset.NFTables {
		if object.Flowtable != nil {
			offloaded = true
		}
		if table := object.Table; table != nil && table.Family == "inet" && table.Name == transitionGuardTable {
			if table.Comment != transitionGuardOwner {
				return false, offloaded, errors.New("防泄漏表名称被非 Ackwrap 规则占用")
			}
			present = true
		}
	}
	return present, offloaded, nil
}

func (g *nftTransitionGuard) Enable(excluded []string, fakeIPs ...string) error {
	excluded, err := normalizeExcludedInterfaces(excluded)
	if err != nil {
		return err
	}
	present, offloaded, err := g.inspect()
	if err != nil {
		return fmt.Errorf("检查 nftables 防泄漏状态失败: %w", err)
	}
	// Offloaded flows bypass forward/output hooks. Do not claim protection or
	// stop the working core unless those fast paths have been disabled first.
	if offloaded {
		return errors.New("检测到 flowtable 流量卸载，请先关闭软/硬件流量卸载再重启或更新核心")
	}
	// Preserve previous pools through failed reloads and daemon restarts.
	if data, err := os.ReadFile(g.statePath); err == nil {
		lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
		if lines[0] != transitionGuardOwner {
			return errors.New("重启防泄漏状态记录无效")
		}
		if len(lines) == 2 {
			var previous []string
			if err := json.Unmarshal([]byte(lines[1]), &previous); err != nil {
				return errors.New("重启防泄漏 FakeIP 记录无效")
			}
			fakeIPs = append(fakeIPs, previous...)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	fakeIPs, err = normalizeDeviceFakeIPCIDRs(fakeIPs...)
	if err != nil {
		return err
	}
	fakeData, _ := json.Marshal(fakeIPs)
	// Persist intent before the atomic nft operation. If Ackwrap exits after
	// installing the table, the next ready core can still remove it. Mixed-only,
	// unprivileged deployments never need to inspect the host firewall.
	if err := os.WriteFile(g.statePath, []byte(transitionGuardOwner+"\n"+string(fakeData)+"\n"), 0600); err != nil {
		return fmt.Errorf("记录防泄漏保护状态失败: %w", err)
	}
	rules := transitionGuardRules(present, excluded, fakeIPs...)
	if _, err := g.run(rules, "-f", "-"); err != nil {
		// Do not log nft diagnostics: they may contain existing network data.
		return fmt.Errorf("原子安装防泄漏规则失败: %w", err)
	}
	return nil
}

func transitionGuardRules(replace bool, excluded []string, fakeIPs ...string) string {
	var rules strings.Builder
	var bypass strings.Builder
	for _, name := range excluded {
		// Match the same ingress interfaces as TUN exclude_interface. Only
		// replies may use the reverse exemption; unrelated egress stays guarded.
		fmt.Fprintf(&bypass, "\t\tiifname %q accept\n\t\toifname %q ct direction reply accept\n", name, name)
	}
	if replace {
		fmt.Fprintf(&rules, "delete table inet %s\n", transitionGuardTable)
	}
	// No established-flow exemption in forward: cached DNS, existing TCP and
	// UDP sessions must not fall back to direct WAN access. The output mark is
	// assigned to sing-box sockets by auto_redirect, allowing startup DNS and
	// node handshakes while all unrelated original-direction output is blocked.
	fmt.Fprintf(&rules, `table inet %s {
	comment "%s"
	chain forward {
		type filter hook forward priority -1; policy drop;
		%s
		meta l4proto { tcp, udp } th dport 53 drop
		meta l4proto { tcp, udp } ct original proto-dst 53 drop
		%s accept
		%s accept
	}
	chain output {
		type filter hook output priority 300; policy drop;
		oifname "lo" accept
		meta mark 0x%x accept
		ct direction reply accept
		icmpv6 type { nd-router-solicit, nd-router-advert, nd-neighbor-solicit, nd-neighbor-advert } accept
		ip daddr 255.255.255.255 udp sport 68 udp dport 67 accept
		ip6 daddr ff02::1:2 udp sport 546 udp dport 547 accept
		meta l4proto { tcp, udp } th dport 53 drop
		meta l4proto { tcp, udp } ct original proto-dst 53 drop
		%s accept
		%s accept
	}
}
`, transitionGuardTable, transitionGuardOwner, bypass.String(),
		deviceLocalNFTMatch("ip", fakeIPs...), deviceLocalNFTMatch("ip6", fakeIPs...), defaultAutoRedirectMark,
		deviceLocalNFTMatch("ip", fakeIPs...), deviceLocalNFTMatch("ip6", fakeIPs...))
	return rules.String()
}

func (g *nftTransitionGuard) Disable() error {
	return g.disable(false)
}

func (g *nftTransitionGuard) disable(force bool) error {
	if !force {
		if _, err := os.Stat(g.statePath); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	present, _, err := g.inspect()
	if err != nil {
		return err
	}
	if present {
		if _, err := g.run("", "delete", "table", "inet", transitionGuardTable); err != nil {
			return fmt.Errorf("删除 Ackwrap 防泄漏表失败: %w", err)
		}
	}
	if err := os.Remove(g.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
