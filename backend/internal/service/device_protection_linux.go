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

	"github.com/ackwrap/ackrun/internal/paths"
)

const deviceProtectionTable = "ackwrap_device_protection"
const deviceProtectionOwner = "Ackwrap device protection v2"
const legacyDeviceProtectionOwner = "Ackwrap device protection v1"
const deviceProtectionInclude = "/usr/share/nftables.d/ruleset-pre/30-ackwrap-protection.nft"

type nftDeviceProtection struct {
	path      string
	supported func() bool
	bootCheck func() error
	run       func(string, ...string) ([]byte, error)
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
			return fmt.Errorf("请关闭防火墙配置中的软/硬件流量卸载")
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
	values, err := normalizeDeviceCIDRs(values)
	if err != nil {
		return nil, err
	}
	slices.Sort(values)
	return values, nil
}

func deviceProtectionRules(values []string, fakeIPs ...string) (string, string, error) {
	return renderDeviceProtectionRules(values, deviceProtectionOwner, fakeIPs...)
}

// v1 is retained only to validate and safely upgrade already-installed policies.
func renderDeviceProtectionRules(values []string, version string, fakeIPs ...string) (string, string, error) {
	values, err := protectionSources(values)
	if err != nil {
		return "", "", err
	}
	data, _ := json.Marshal(values)
	fakeIPs, err = normalizeDeviceFakeIPCIDRs(fakeIPs...)
	if err != nil {
		return "", "", err
	}
	fakeData, _ := json.Marshal(fakeIPs)
	digestData := data
	if version == deviceProtectionOwner {
		digestData = append(append([]byte{}, data...), fakeData...)
	}
	owner := fmt.Sprintf("%s %x", version, sha256.Sum256(digestData))
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n# sources %s\n", version, data)
	if version == deviceProtectionOwner {
		fmt.Fprintf(&b, "# fakeip %s\n", fakeData)
	}
	// Idempotent in a single nft transaction, including fw4 reloads and boots.
	fmt.Fprintf(&b, "add table inet %s\ndelete table inet %s\ntable inet %s {\n comment %q\n", deviceProtectionTable, deviceProtectionTable, deviceProtectionTable, owner)
	b.WriteString(" chain forward {\n type filter hook forward priority -5; policy accept;\n")
	for _, value := range values {
		family := "ip"
		if netip.MustParsePrefix(value).Addr().Is6() {
			family = "ip6"
		}
		if version == deviceProtectionOwner {
			// Existing conntrack DNS may predate our redirect. Never exempt it
			// just because the old resolver is on the LAN.
			fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } th dport 53 counter drop\n", family, value)
			fmt.Fprintf(&b, " %s saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 counter drop\n", family, value)
			fmt.Fprintf(&b, " %s saddr %s %s counter return\n", family, value, deviceLocalNFTMatch(family, fakeIPs...))
		}
		fmt.Fprintf(&b, " %s saddr %s oifname != \"tun0\" counter drop\n", family, value)
	}
	b.WriteString(" }\n chain input {\n type filter hook input priority -5; policy accept;\n")
	for _, value := range values {
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
	for _, value := range values {
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
	sources, _, err := g.readPolicy()
	return sources, err
}

func (g *nftDeviceProtection) FakeIPCIDRs() ([]string, error) {
	_, fakeIPs, err := g.readPolicy()
	return fakeIPs, err
}

func (g *nftDeviceProtection) readPolicy() ([]string, []string, error) {
	data, err := os.ReadFile(g.path)
	if os.IsNotExist(err) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	lines := strings.SplitN(string(data), "\n", 4)
	if len(lines) != 4 || (lines[0] != "# "+deviceProtectionOwner && lines[0] != "# "+legacyDeviceProtectionOwner) || !strings.HasPrefix(lines[1], "# sources ") {
		return nil, nil, fmt.Errorf("设备防泄漏持久文件损坏，拒绝解除保护")
	}
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimPrefix(lines[1], "# sources ")), &values); err != nil {
		return nil, nil, fmt.Errorf("设备防泄漏来源记录无效")
	}
	var fakeIPs []string
	if lines[0] == "# "+deviceProtectionOwner {
		if !strings.HasPrefix(lines[2], "# fakeip ") || json.Unmarshal([]byte(strings.TrimPrefix(lines[2], "# fakeip ")), &fakeIPs) != nil {
			return nil, nil, fmt.Errorf("设备防泄漏 FakeIP 记录无效")
		}
	}
	rules, _, err := renderDeviceProtectionRules(values, strings.TrimPrefix(lines[0], "# "), fakeIPs...)
	if err != nil {
		return nil, nil, err
	}
	if !bytes.Equal(data, []byte(rules)) {
		return nil, nil, fmt.Errorf("设备防泄漏持久规则与来源记录不一致")
	}
	values, err = protectionSources(values)
	return values, fakeIPs, err
}

func (g *nftDeviceProtection) inspect(activating bool) (string, int, error) {
	data, err := g.run("", "-j", "list", "ruleset")
	if err != nil {
		return "", 0, fmt.Errorf("无法读取设备防泄漏规则，请检查 nftables 和权限: %w", err)
	}
	var state struct {
		NFTables []struct {
			Table     *struct{ Family, Name, Comment string } `json:"table"`
			Rule      *struct{ Family, Table string }         `json:"rule"`
			Flowtable *json.RawMessage                        `json:"flowtable"`
		} `json:"nftables"`
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return "", 0, fmt.Errorf("无法解析设备防泄漏规则")
	}
	owner, count := "", 0
	for _, entry := range state.NFTables {
		if activating && entry.Flowtable != nil {
			return "", 0, fmt.Errorf("请先关闭软/硬件流量卸载并重载防火墙")
		}
		if table := entry.Table; table != nil && table.Family == "inet" && table.Name == deviceProtectionTable {
			if !strings.HasPrefix(table.Comment, deviceProtectionOwner+" ") && !strings.HasPrefix(table.Comment, legacyDeviceProtectionOwner+" ") {
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

func (g *nftDeviceProtection) Verify(values []string, fakeIPs ...string) error {
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
	previous, err := g.FakeIPCIDRs()
	if err != nil {
		return err
	}
	rules, expected, err := deviceProtectionRules(values, append(previous, fakeIPs...)...)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(g.path)
	if err != nil || !bytes.Equal(data, []byte(rules)) {
		return fmt.Errorf("设备防泄漏持久规则尚未同步")
	}
	owner, count, err := g.inspect(true)
	if err != nil {
		return err
	}
	if owner != expected || count != len(values)*8 {
		return fmt.Errorf("设备防泄漏内核规则缺失或不完整，请停止核心后重新启用保护")
	}
	return nil
}

func (g *nftDeviceProtection) Apply(values []string, fakeIPs ...string) error {
	if !g.Supported() {
		return fmt.Errorf("设备防泄漏需要 OpenWrt fw4 及新版 Ackwrap 防火墙加载文件")
	}
	if g.bootCheck != nil {
		if err := g.bootCheck(); err != nil {
			return err
		}
	}
	if _, _, err := g.inspect(true); err != nil {
		return err
	}
	previous, err := g.FakeIPCIDRs()
	if err != nil {
		return err
	}
	fakeIPs = append(previous, fakeIPs...)
	rules, _, err := deviceProtectionRules(values, fakeIPs...)
	if err != nil {
		return err
	}
	if _, err := g.run(rules, "-c", "-f", "-"); err != nil {
		return fmt.Errorf("设备防泄漏规则校验失败: %w", err)
	}
	// Durable intent precedes activation. A failed apply retains the intended
	// boot policy, and must never be reported as an enabled/healthy switch.
	if err := writeProtectionFile(g.path, []byte(rules)); err != nil {
		return err
	}
	if _, err := g.run(rules, "-f", "-"); err != nil {
		return fmt.Errorf("设备防泄漏规则安装失败，已保留持久阻断记录: %w", err)
	}
	return g.Verify(values, fakeIPs...)
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
