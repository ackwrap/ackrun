//go:build linux

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
)

const deviceDNSTable = "ackwrap_device_dns"
const deviceDNSOwner = "Ackwrap device DNS v1"

type nftDeviceDNS struct {
	statePath string
	run       func(string, ...string) ([]byte, error)
}

func newDeviceDNSLifecycle(p *paths.Paths) deviceDNSLifecycle {
	return &nftDeviceDNS{statePath: p.DeviceDNSStatePath(), run: func(input string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), linuxNetworkCommandTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "nft", args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}}
}

func (d *nftDeviceDNS) inspect(activating bool) (bool, error) {
	output, err := d.run("", "-j", "list", "ruleset")
	if err != nil {
		return false, fmt.Errorf("读取设备 DNS 接管状态失败，请确认 nftables 可用: %w", err)
	}
	var state struct {
		NFTables []struct {
			Table     *struct{ Family, Name, Comment string } `json:"table"`
			Flowtable *json.RawMessage                        `json:"flowtable"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(output, &state); err != nil {
		return false, fmt.Errorf("解析设备 DNS 接管状态失败")
	}
	present := false
	for _, entry := range state.NFTables {
		if activating && entry.Flowtable != nil {
			return false, fmt.Errorf("设备规则需要关闭软/硬件流量卸载，请先关闭 flowtable 再启动核心")
		}
		if table := entry.Table; table != nil && table.Family == "inet" && table.Name == deviceDNSTable {
			if table.Comment != deviceDNSOwner {
				return false, fmt.Errorf("设备 DNS 接管表被其他程序占用，拒绝覆盖")
			}
			present = true
		}
	}
	return present, nil
}

func deviceDNSNFTRules(cidrs []string, replace bool, excluded ...string) (string, error) {
	values, err := normalizeDeviceCIDRs(cidrs)
	if err != nil {
		return "", err
	}
	excluded, err = normalizeExcludedInterfaces(excluded)
	if err != nil {
		return "", err
	}
	var rules strings.Builder
	if replace {
		fmt.Fprintf(&rules, "delete table inet %s\n", deviceDNSTable)
	}
	fmt.Fprintf(&rules, "table inet %s {\n comment \"%s\"\n chain prerouting {\n type nat hook prerouting priority -190; policy accept;\n", deviceDNSTable, deviceDNSOwner)
	for _, name := range excluded {
		fmt.Fprintf(&rules, " iifname %q return\n", name)
	}
	// Run after conntrack (-200), before sing-box's redirect and fw4 dstnat.
	// Destination NAT preserves the device source, even for router-local DNS.
	for _, value := range values {
		family := "ip"
		if netip.MustParsePrefix(value).Addr().Is6() {
			family = "ip6"
		}
		for _, protocol := range []string{"tcp", "udp"} {
			fmt.Fprintf(&rules, " %s saddr %s %s dport 53 redirect to :%d\n", family, value, protocol, deviceDNSPort)
		}
	}
	rules.WriteString(" }\n}\n")
	return rules.String(), nil
}

func (d *nftDeviceDNS) Activate(cidrs []string, excluded ...string) error {
	present, err := d.inspect(true)
	if err != nil {
		return err
	}
	rules, err := deviceDNSNFTRules(cidrs, present, excluded...)
	if err != nil {
		return err
	}
	// Record intent first, so process-exit and next-start cleanup can recover
	// if the daemon exits between the atomic nft transaction and completion.
	if err := os.WriteFile(d.statePath, []byte(deviceDNSOwner+"\n"), 0600); err != nil {
		return err
	}
	if _, err := d.run(rules, "-f", "-"); err != nil {
		return fmt.Errorf("安装设备 DNS 接管规则失败: %w", err)
	}
	logging.Info("device_dns.activate", "已为 %d 个设备地址范围接管 TCP/UDP DNS，排除 %d 个接口", len(cidrs), len(excluded))
	return nil
}

func (d *nftDeviceDNS) Restore() error {
	return d.restore(false)
}

func (d *nftDeviceDNS) restore(force bool) error {
	if !force {
		if _, err := os.Stat(d.statePath); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	present, err := d.inspect(false)
	if err != nil {
		return err
	}
	if present {
		if _, err := d.run("", "delete", "table", "inet", deviceDNSTable); err != nil {
			return fmt.Errorf("清理设备 DNS 接管规则失败: %w", err)
		}
	}
	if err := os.Remove(d.statePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	logging.Info("device_dns.restore", "设备 DNS 接管规则已清理")
	return nil
}
