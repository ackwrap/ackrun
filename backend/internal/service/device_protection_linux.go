//go:build linux

package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/paths"
)

const deviceProtectionTable = "ackwrap_device_protection"
const deviceProtectionOwner = "Ackwrap device protection v3"
const localDeviceProtectionOwner = "Ackwrap device protection v2"
const legacyDeviceProtectionOwner = "Ackwrap device protection v1"
const deviceProtectionInclude = "/usr/share/nftables.d/ruleset-pre/30-ackwrap-protection.nft"

type nftDeviceProtection struct {
	path      string
	supported func() bool
	bootCheck func() error
	run       func(string, ...string) ([]byte, error)
	ipRun     func(...string) ([]byte, error)
}

func newDeviceProtection(p *paths.Paths) deviceProtectionFirewall {
	return &nftDeviceProtection{path: p.DeviceProtectionRulesPath(), supported: func() bool {
		data, err := os.ReadFile(deviceProtectionInclude)
		return platformSupportsDNSMasqTakeover() && err == nil && strings.Contains(string(data), `include "/etc/ackwrap-protection/*.nft"`)
	}, bootCheck: func() error {
		ctx, cancel := context.WithTimeout(context.Background(), linuxNetworkCommandTimeout)
		defer cancel()
		data, err := exec.CommandContext(ctx, "fw4", "print").CombinedOutput()
		if err != nil {
			return fmt.Errorf("无法检查 fw4 开机加载配置: %w", err)
		}
		if !strings.Contains(string(data), `include "`+deviceProtectionInclude+`"`) {
			return fmt.Errorf("fw4 未加载设备防泄漏文件，请启用防火墙自动 includes")
		}
		if strings.Contains(string(data), "flowtable ") {
			return fmt.Errorf("fw4 开机配置含 flowtable 流量卸载，无法确认与指定设备隔离；请关闭相关软/硬件流量卸载并重载防火墙")
		}
		return nil
	}, run: func(input string, args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), linuxNetworkCommandTimeout)
		defer cancel()
		cmd := exec.CommandContext(ctx, "nft", args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}}
}

func (g *nftDeviceProtection) Supported() bool { return g.supported() }

func protectionSources(values []string) ([]string, error) {
	values, err := normalizeDeviceSources(values)
	if err != nil {
		return nil, err
	}
	slices.Sort(values)
	return values, nil
}

func deviceProtectionRules(values, excluded []string, fakeIPs ...string) (string, string, error) {
	return renderDeviceProtectionRules(values, deviceProtectionOwner, excluded, fakeIPs...)
}

// Previous formats are retained to validate and safely upgrade installed policies.
func renderDeviceProtectionRules(values []string, version string, excluded []string, fakeIPs ...string) (string, string, error) {
	values, err := protectionSources(values)
	if err != nil {
		return "", "", err
	}
	excluded, err = normalizeExcludedInterfaces(excluded)
	if err != nil {
		return "", "", err
	}
	interfaceData, _ := json.Marshal(excluded)
	data, _ := json.Marshal(values)
	fakeIPs, err = normalizeDeviceFakeIPCIDRs(fakeIPs...)
	if err != nil {
		return "", "", err
	}
	fakeData, _ := json.Marshal(fakeIPs)
	digestData := data
	if version != legacyDeviceProtectionOwner {
		digestData = append(append([]byte{}, data...), fakeData...)
	}
	if version == deviceProtectionOwner {
		digestData = append(digestData, interfaceData...)
	}
	owner := fmt.Sprintf("%s %x", version, sha256.Sum256(digestData))
	cidrs, macs := splitDeviceSources(values)
	if len(macs) > 0 && (version != deviceProtectionOwner || len(excluded) > 0) {
		return "", "", fmt.Errorf("MAC 防泄漏不支持旧格式或接口排除")
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n# sources %s\n", version, data)
	if version != legacyDeviceProtectionOwner {
		fmt.Fprintf(&b, "# fakeip %s\n", fakeData)
	}
	if version == deviceProtectionOwner {
		fmt.Fprintf(&b, "# interfaces %s\n", interfaceData)
	}
	// Idempotent in a single nft transaction, including fw4 reloads and boots.
	fmt.Fprintf(&b, "add table inet %s\ndelete table inet %s\ntable inet %s {\n comment %q\n", deviceProtectionTable, deviceProtectionTable, deviceProtectionTable, owner)
	if len(macs) > 0 {
		b.WriteString(deviceMACNFTChain(macs, fakeIPs))
	}
	b.WriteString(" chain forward {\n type filter hook forward priority -5; policy accept;\n")
	if version == deviceProtectionOwner {
		for _, name := range excluded {
			fmt.Fprintf(&b, " iifname %q counter return\n", name)
			fmt.Fprintf(&b, " oifname %q ct direction reply counter return\n", name)
		}
	}
	for _, mac := range macs {
		// Port 1056 alone does not prove a local redirect: an older DNAT may
		// point at a LAN resolver using that port. DNS is never forward-exempt.
		fmt.Fprintf(&b, " ether saddr %s meta l4proto { tcp, udp } th dport 53 counter drop\n", mac)
		fmt.Fprintf(&b, " ether saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 counter drop\n", mac)
		for _, family := range []string{"ip", "ip6"} {
			fmt.Fprintf(&b, " ether saddr %s %s counter return\n", mac, deviceLocalNFTMatch(family, fakeIPs...))
		}
		fmt.Fprintf(&b, " ether saddr %s counter drop\n", mac)
	}
	for _, value := range cidrs {
		family := "ip"
		if netip.MustParsePrefix(value).Addr().Is6() {
			family = "ip6"
		}
		if version != legacyDeviceProtectionOwner {
			// Existing conntrack DNS may predate our redirect. Never exempt it
			// just because the old resolver is on the LAN.
			fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } th dport 53 counter drop\n", family, value)
			fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 counter drop\n", family, value)
			fmt.Fprintf(&b, " %s saddr %s %s counter return\n", family, value, deviceLocalNFTMatch(family, fakeIPs...))
		}
		fmt.Fprintf(&b, " %s saddr %s oifname != \"tun0\" counter drop\n", family, value)
	}
	b.WriteString(" }\n chain input {\n type filter hook input priority -5; policy accept;\n")
	if len(macs) > 0 {
		fmt.Fprintf(&b, " meta mark %s counter return\n", deviceMACMark)
		for _, mac := range macs {
			fmt.Fprintf(&b, " ether saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 th dport %d counter return\n", mac, deviceMACDNSPort)
		}
		fmt.Fprintf(&b, " meta l4proto { tcp, udp } th dport { %d, %d } counter drop\n", deviceMACPort, deviceMACDNSPort)
	}
	if version == deviceProtectionOwner {
		for _, name := range excluded {
			fmt.Fprintf(&b, " iifname %q counter return\n", name)
		}
	}
	for _, value := range cidrs {
		family := "ip"
		if netip.MustParsePrefix(value).Addr().Is6() {
			family = "ip6"
		}
		// Also reject established DNS flows created before takeover. Existing
		// conntrack NAT mappings are not rewritten by a new prerouting NAT rule.
		fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } th dport 53 counter drop\n", family, value)
		fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 th dport != %d counter drop\n", family, value, deviceDNSPort)
	}
	b.WriteString(" }\n chain prerouting {\n type nat hook prerouting priority -191; policy accept;\n")
	if len(macs) > 0 {
		fmt.Fprintf(&b, " meta mark %s counter return\n", deviceMACMark)
		for _, mac := range macs {
			for _, protocol := range []string{"tcp", "udp"} {
				fmt.Fprintf(&b, " ether saddr %s %s dport 53 redirect to :%d\n", mac, protocol, deviceMACDNSPort)
			}
		}
	}
	if version == deviceProtectionOwner {
		for _, name := range excluded {
			fmt.Fprintf(&b, " iifname %q counter return\n", name)
		}
	}
	for _, value := range cidrs {
		family := "ip"
		if netip.MustParsePrefix(value).Addr().Is6() {
			family = "ip6"
		}
		for _, protocol := range []string{"tcp", "udp"} {
			fmt.Fprintf(&b, " %s saddr %s %s dport 53 redirect to :%d\n", family, value, protocol, deviceDNSPort)
		}
	}
	b.WriteString(" }\n}\n")
	return b.String(), owner, nil
}

func (g *nftDeviceProtection) Sources() ([]string, error) {
	sources, _, _, err := g.readPolicy()
	return sources, err
}

func (g *nftDeviceProtection) FakeIPCIDRs() ([]string, error) {
	_, fakeIPs, _, err := g.readPolicy()
	return fakeIPs, err
}

func (g *nftDeviceProtection) readPolicy() ([]string, []string, []string, error) {
	data, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		return nil, nil, nil, nil
	}
	if err != nil {
		return nil, nil, nil, err
	}
	lines := strings.SplitN(string(data), "\n", 5)
	if len(lines) != 5 || !strings.HasPrefix(lines[1], "# sources ") {
		return nil, nil, nil, fmt.Errorf("设备防泄漏持久文件损坏，拒绝解除保护")
	}
	version := strings.TrimPrefix(lines[0], "# ")
	if version != deviceProtectionOwner && version != localDeviceProtectionOwner && version != legacyDeviceProtectionOwner {
		return nil, nil, nil, fmt.Errorf("设备防泄漏持久文件版本无效")
	}
	var values, fakeIPs, excluded []string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "# sources ")), &values); err != nil {
		return nil, nil, nil, fmt.Errorf("设备防泄漏来源记录无效")
	}
	if version != legacyDeviceProtectionOwner {
		if !strings.HasPrefix(lines[2], "# fakeip ") || json.Unmarshal([]byte(strings.TrimPrefix(lines[2], "# fakeip ")), &fakeIPs) != nil {
			return nil, nil, nil, fmt.Errorf("设备防泄漏 FakeIP 记录无效")
		}
	}
	if version == deviceProtectionOwner {
		if !strings.HasPrefix(lines[3], "# interfaces ") || json.Unmarshal([]byte(strings.TrimPrefix(lines[3], "# interfaces ")), &excluded) != nil {
			return nil, nil, nil, fmt.Errorf("设备防泄漏接口排除记录无效")
		}
	}
	rules, _, err := renderDeviceProtectionRules(values, version, excluded, fakeIPs...)
	if err != nil {
		return nil, nil, nil, err
	}
	if !bytes.Equal(data, []byte(rules)) {
		return nil, nil, nil, fmt.Errorf("设备防泄漏持久规则与来源记录不一致")
	}
	values, err = protectionSources(values)
	return values, fakeIPs, excluded, err
}

func (g *nftDeviceProtection) inspect(activating bool, sources ...string) (string, int, error) {
	data, err := g.run("", "-j", "list", "ruleset")
	if err != nil {
		return "", 0, fmt.Errorf("无法读取设备防泄漏规则，请检查 nftables 和权限: %w", err)
	}
	var state struct {
		NFTables []struct {
			Table *struct{ Family, Name, Comment string } `json:"table"`
			Rule  *struct{ Family, Table string }         `json:"rule"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", 0, fmt.Errorf("无法解析设备防泄漏规则")
	}
	if activating {
		if err := checkNFTFlowtables(data, sources); err != nil {
			return "", 0, err
		}
	}
	owner, count := "", 0
	for _, entry := range state.NFTables {
		if table := entry.Table; table != nil && table.Family == "inet" && table.Name == deviceProtectionTable {
			if !strings.HasPrefix(table.Comment, deviceProtectionOwner+" ") && !strings.HasPrefix(table.Comment, localDeviceProtectionOwner+" ") && !strings.HasPrefix(table.Comment, legacyDeviceProtectionOwner+" ") {
				return "", 0, fmt.Errorf("设备防泄漏表被其他程序占用")
			}
			owner = table.Comment
		}
		if rule := entry.Rule; rule != nil && rule.Family == "inet" && rule.Table == deviceProtectionTable {
			count++
		}
	}
	return owner, count, nil
}

func (g *nftDeviceProtection) VerifyDisabled() error {
	if !g.Supported() {
		return nil
	}
	owner, _, err := g.inspect(false)
	if err != nil {
		return err
	}
	if owner != "" {
		return fmt.Errorf("内核中仍有设备防泄漏规则，请重新启用或解除残留保护")
	}
	return nil
}

func (g *nftDeviceProtection) Verify(values, excluded []string, fakeIPs ...string) error {
	var err error
	excluded, err = normalizeExcludedInterfaces(excluded)
	if err != nil {
		return err
	}
	values, err = protectionSources(values)
	if err != nil {
		return err
	}
	if !g.Supported() {
		return fmt.Errorf("设备防泄漏需要 OpenWrt fw4 及新版 Ackwrap 防火墙加载文件")
	}
	if g.bootCheck != nil {
		if err := g.bootCheck(); err != nil {
			return err
		}
	}
	previous, err := g.FakeIPCIDRs()
	if err != nil {
		return err
	}
	rules, expected, err := deviceProtectionRules(values, excluded, append(previous, fakeIPs...)...)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(g.path)
	if err != nil || !bytes.Equal(data, []byte(rules)) {
		return fmt.Errorf("设备防泄漏持久规则尚未同步")
	}
	owner, count, err := g.inspect(true, values...)
	if err != nil {
		return err
	}
	_, macs := splitDeviceSources(values)
	if owner != expected || (len(macs) == 0 && count != len(values)*8+len(excluded)*4) {
		return fmt.Errorf("设备防泄漏内核规则缺失或不完整，请停止核心后重新启用保护")
	}
	if len(macs) > 0 {
		if err := g.macRoutes("verify"); err != nil {
			return err
		}
	}
	return g.verifyKernelPolicy(expected)
}

func (g *nftDeviceProtection) Apply(values, excluded []string, fakeIPs ...string) error {
	values, err := protectionSources(values)
	if err != nil {
		return err
	}
	if !g.Supported() {
		return fmt.Errorf("设备防泄漏需要 OpenWrt fw4 及新版 Ackwrap 防火墙加载文件")
	}
	if g.bootCheck != nil {
		if err := g.bootCheck(); err != nil {
			return err
		}
	}
	if _, _, err := g.inspect(true, values...); err != nil {
		return err
	}
	previous, err := g.FakeIPCIDRs()
	if err != nil {
		return err
	}
	fakeIPs = append(previous, fakeIPs...)
	rules, owner, err := deviceProtectionRules(values, excluded, fakeIPs...)
	if err != nil {
		return err
	}
	if _, err := g.run(rules, "-c", "-f", "-"); err != nil {
		if _, macs := splitDeviceSources(values); len(macs) > 0 {
			return fmt.Errorf("MAC 设备入口规则校验失败，请确认 nft TProxy 内核支持可用（OpenWrt: kmod-nft-tproxy）: %w", err)
		}
		return fmt.Errorf("设备防泄漏规则校验失败: %w", err)
	}
	// Durable intent precedes activation. A failed apply retains the intended
	// boot policy, and must never be reported as an enabled/healthy switch.
	if err := writeProtectionFile(g.path, []byte(rules)); err != nil {
		return err
	}
	echo, err := g.run(rules, "-j", "-e", "-f", "-")
	if err != nil {
		return fmt.Errorf("设备防泄漏规则安装失败，已保留持久阻断记录: %w", err)
	}
	if err := g.recordKernelPolicy(echo, owner); err != nil {
		return fmt.Errorf("设备防泄漏已安装，但记录内核规则校验基线失败: %w", err)
	}
	if _, macs := splitDeviceSources(values); len(macs) > 0 {
		if err := g.macRoutes("apply"); err != nil {
			return err
		}
		logging.Info("device_protection.mac", "已安装 %d 个 MAC 的独立流量与 DNS 入口，未识别身份保持拒绝", len(macs))
	} else if err := g.macRoutes("remove"); err != nil {
		return err
	}
	return g.Verify(values, excluded, fakeIPs...)
}

func (g *nftDeviceProtection) Disable() error {
	if _, err := g.Sources(); err != nil {
		return err
	}
	return g.ForceDisable()
}

// Explicit recovery can remove a damaged policy file, but still checks the
// kernel table's ownership before touching any firewall state.
func (g *nftDeviceProtection) ForceDisable() error {
	owner, _, err := g.inspect(false)
	if err != nil {
		return err
	}
	previous, readErr := os.ReadFile(g.path)
	if readErr != nil && !os.IsNotExist(readErr) {
		return readErr
	}
	// Keep the packet guard installed if policy-route cleanup refuses a foreign
	// replacement or fails partway through. Missing routes only cause blocking.
	if err := g.macRoutes("remove"); err != nil {
		return err
	}
	// Remove boot intent first. If interrupted, the running kernel keeps its
	// old guard until an explicit retry; normal lifecycle cleanup never removes it.
	if err := os.Remove(g.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if readErr == nil {
		dir, err := os.Open(filepath.Dir(g.path))
		if err == nil {
			err = dir.Sync()
			dir.Close()
		}
		if err != nil {
			return errors.Join(err, writeProtectionFile(g.path, previous))
		}
	}
	if owner != "" {
		if _, err := g.run("", "delete", "table", "inet", deviceProtectionTable); err != nil {
			var restoreErr error
			if len(previous) > 0 {
				restoreErr = writeProtectionFile(g.path, previous)
			}
			return errors.Join(fmt.Errorf("解除设备防泄漏失败，请重试: %w", err), restoreErr)
		}
	}
	return nil
}

func writeProtectionFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".guard-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
