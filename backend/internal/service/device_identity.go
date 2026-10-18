package service

import (
	"fmt"
	"net"
	"strings"
)

const deviceMACInboundTag = "ackwrap-device-mac-in"
const deviceMACPort = 1055
const deviceMACDNSInboundTag = "ackwrap-device-mac-dns-in"
const deviceMACDNSPort = 1056

func deviceMACInbounds() []string { return []string{deviceMACInboundTag, deviceMACDNSInboundTag} }

// Values retain the existing device-rule storage format. Each entry is an
// identity alternative, never an additional constraint on another entry.
func normalizeDeviceSources(values []string) ([]string, error) {
	if len(values) == 0 || len(values) > 128 {
		return nil, fmt.Errorf("设备 MAC / 地址数量必须在 1 到 128 之间")
	}
	var result []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if mac, err := net.ParseMAC(value); err == nil {
			if len(mac) != 6 || mac[0]&1 != 0 || mac.String() == "00:00:00:00:00:00" {
				return nil, fmt.Errorf("设备 MAC 必须是非零的 48 位单播地址")
			}
			result = appendUniqueStrings(result, mac.String())
			continue
		}
		cidrs, err := normalizeDeviceCIDRs([]string{value})
		if err != nil {
			return nil, fmt.Errorf("设备来源必须是有效的 MAC、IP 或 CIDR 网段")
		}
		result = appendUniqueStrings(result, cidrs...)
	}
	return result, nil
}

func splitDeviceSources(values []string) (cidrs, macs []string) {
	for _, value := range values {
		if mac, err := net.ParseMAC(value); err == nil && len(mac) == 6 {
			macs = append(macs, mac.String())
		} else {
			cidrs = append(cidrs, value)
		}
	}
	return
}

// Keep destination/interface restrictions outside the identity OR. A MAC
// alternative is only trusted on the firewall's dedicated transparent ingress.
func deviceSourceRule(values []string, conditions map[string]interface{}) map[string]interface{} {
	cidrs, macs := splitDeviceSources(values)
	if len(macs) == 0 {
		conditions["source_ip_cidr"] = cidrs
		return conditions
	}
	identity := map[string]interface{}{"inbound": deviceMACInbounds(), "source_mac_address": macs}
	if len(cidrs) > 0 {
		identity = map[string]interface{}{"type": "logical", "mode": "or", "rules": []map[string]interface{}{
			{"source_ip_cidr": cidrs}, identity,
		}}
	}
	match := map[string]interface{}{}
	action := map[string]interface{}{"type": "logical", "mode": "and"}
	for key, value := range conditions {
		switch key {
		case "action", "outbound", "server":
			action[key] = value
		default:
			match[key] = value
		}
	}
	rules := []map[string]interface{}{identity}
	if len(match) > 0 {
		rules = append(rules, match)
	}
	action["rules"] = rules
	return action
}
