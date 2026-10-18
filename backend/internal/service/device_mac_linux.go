//go:build linux

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// This mark is reserved for the dedicated device ingress, not a sing-box mark.
const deviceMACMark = "0xace1"
const deviceMACTable = "2024"
const deviceMACPriority = "8999"

func (g *nftDeviceProtection) runIP(args ...string) ([]byte, error) {
	if g.ipRun != nil {
		return g.ipRun(args...)
	}
	ctx, cancel := context.WithTimeout(context.Background(), linuxNetworkCommandTimeout)
	defer cancel()
	return exec.CommandContext(ctx, "ip", args...).CombinedOutput()
}

// Inspect only our reserved priority/table. Exact content and a durable intent
// record are both required before adopting or deleting an existing object.
func (g *nftDeviceProtection) macRoutes(operation string) error {
	install := operation != "remove"
	recordPath := g.path + ".mac-routes"
	const owner = "Ackwrap device MAC routes v1\n"
	data, err := os.ReadFile(recordPath)
	owned := err == nil && string(data) == owner
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && !owned {
		return fmt.Errorf("MAC 入口路由所有权记录无效")
	}
	if !install && !owned {
		return nil
	}
	type state struct {
		family      string
		rule, route bool
	}
	var states []state
	for _, family := range []string{"-4", "-6"} {
		s := state{family: family}
		for _, kind := range []string{"rule", "route"} {
			args := []string{family, "-j", kind, "show"}
			if kind == "route" {
				args = append(args, "table", "all")
			}
			output, err := g.runIP(args...)
			if err != nil {
				return fmt.Errorf("读取 MAC 入口路由失败（需要支持 JSON 的 ip-full）: %w", err)
			}
			var entries []map[string]interface{}
			if json.Unmarshal(output, &entries) != nil {
				return fmt.Errorf("MAC 入口路由状态无效")
			}
			for _, entry := range entries {
				if kind == "rule" {
					priority := fmt.Sprint(entry["priority"])
					if priority != deviceMACPriority && fmt.Sprint(entry["table"]) != deviceMACTable {
						continue
					}
					if !owned || s.rule || priority != deviceMACPriority || fmt.Sprint(entry["table"]) != deviceMACTable || entry["src"] != "all" || entry["fwmark"] != deviceMACMark {
						return fmt.Errorf("MAC 入口路由规则优先级或表已被占用")
					}
					for key, value := range entry {
						switch key {
						case "priority", "table", "src", "fwmark":
						case "fwmask":
							if value != "0xffffffff" {
								return fmt.Errorf("MAC 入口路由掩码已改变")
							}
						default:
							return fmt.Errorf("MAC 入口路由规则包含意外条件")
						}
					}
					s.rule = true
				} else {
					if fmt.Sprint(entry["table"]) != deviceMACTable {
						continue
					}
					if !owned || s.route || entry["type"] != "local" || entry["dst"] != "default" || entry["dev"] != "lo" {
						return fmt.Errorf("MAC 入口路由表已被占用或改变")
					}
					for key, value := range entry {
						switch key {
						case "table", "type", "dst", "dev":
						case "protocol":
							if value != "static" {
								return fmt.Errorf("MAC 入口路由协议已改变")
							}
						case "scope":
							if value != "host" || family != "-4" {
								return fmt.Errorf("MAC 入口路由范围已改变")
							}
						case "metric":
							if fmt.Sprint(value) != "1024" || family != "-6" {
								return fmt.Errorf("MAC 入口路由度量已改变")
							}
						case "flags":
							if flags, ok := value.([]interface{}); !ok || len(flags) != 0 {
								return fmt.Errorf("MAC 入口路由标志已改变")
							}
						case "pref":
							if value != "medium" || family != "-6" {
								return fmt.Errorf("MAC 入口路由偏好已改变")
							}
						default:
							return fmt.Errorf("MAC 入口路由包含意外属性")
						}
					}
					s.route = true
				}
			}
		}
		states = append(states, s)
	}
	if operation == "verify" {
		for _, s := range states {
			if !s.route || !s.rule {
				return fmt.Errorf("MAC 入口路由缺失，设备保持阻断，请停止核心后重新启用保护")
			}
		}
		return nil
	}
	if install && !owned {
		if err := writeProtectionFile(recordPath, []byte(owner)); err != nil {
			return err
		}
	}
	for _, s := range states {
		operation := "add"
		if !install {
			operation = "del"
		}
		commands := [][]string{}
		if install != s.route {
			commands = append(commands, []string{s.family, "route", operation, "local", "default", "dev", "lo", "table", deviceMACTable, "proto", "static"})
		}
		if install != s.rule {
			commands = append(commands, []string{s.family, "rule", operation, "pref", deviceMACPriority, "fwmark", deviceMACMark, "lookup", deviceMACTable})
		}
		for _, args := range commands {
			if _, err := g.runIP(args...); err != nil {
				return fmt.Errorf("更新 MAC 入口路由失败，保留阻断与恢复记录: %w", err)
			}
		}
	}
	if !install {
		return os.Remove(recordPath)
	}
	return nil
}

func deviceMACNFTChain(macs, fakeIPs []string) string {
	b := new(strings.Builder)
	// Run before the core's prematch and redirect. Failing TPROXY falls through
	// to drop: neither a missing socket nor a missing policy route permits WAN.
	b.WriteString(" chain mac_ingress {\n type filter hook prerouting priority -192; policy accept;\n")
	renderDeviceMACCapture(b, macs, fakeIPs, true)
	b.WriteString(" }\n")
	// sing-tun overwrites UDP marks after dstnat even for excluded MACs.
	// Reassert the transparent socket and policy-route mark afterwards.
	b.WriteString(" chain mac_route {\n type filter hook prerouting priority -90; policy accept;\n")
	renderDeviceMACCapture(b, macs, fakeIPs, false)
	b.WriteString(" }\n")
	return b.String()
}

func renderDeviceMACCapture(b *strings.Builder, macs, fakeIPs []string, beforeNAT bool) {
	for _, mac := range macs {
		// A normal DNS listener preserves link-local IPv6 reply scope, unlike
		// transparent UDP's original-destination reply socket in the pinned core.
		fmt.Fprintf(b, " ether saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 th dport %d counter return\n", mac, deviceMACDNSPort)
		if beforeNAT {
			fmt.Fprintf(b, " ether saddr %s meta l4proto { tcp, udp } th dport 53 counter return\n", mac)
		} else {
			fmt.Fprintf(b, " ether saddr %s meta l4proto { tcp, udp } th dport 53 counter drop\n", mac)
		}
		fmt.Fprintf(b, " ether saddr %s meta l4proto { tcp, udp } ct original proto-dst 53 counter drop\n", mac)
		fmt.Fprintf(b, " ether saddr %s ip6 daddr { fe80::/10, ff02::/16 } icmpv6 type { 133, 134, 135, 136 } counter return\n", mac)
		for _, family := range []string{"ip", "ip6"} {
			fmt.Fprintf(b, " ether saddr %s %s counter return\n", mac, deviceLocalNFTMatch(family, fakeIPs...))
		}
		fmt.Fprintf(b, " ether saddr %s meta l4proto { tcp, udp } tproxy to :%d meta mark set %s accept\n", mac, deviceMACPort, deviceMACMark)
		fmt.Fprintf(b, " ether saddr %s counter drop\n", mac)
	}
}
