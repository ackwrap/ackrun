//go:build linux

package service

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"slices"
	"strings"
)

// A ruleset dump describes flow admission, not the connections already cached
// in a flowtable. In particular, removing a flow-add rule, excluding a source,
// or seeing zero rule counters does not establish that the fast path is empty.
// Only a disjoint address family is a sufficient proof here. MAC identities
// cover both families; an empty source list denotes the legacy whole-TUN guard.
func checkNFTFlowtables(data []byte, sources []string) error {
	v4, v6 := len(sources) == 0, len(sources) == 0
	if len(sources) > 0 {
		values, err := normalizeDeviceSources(sources)
		if err != nil {
			return err
		}
		cidrs, macs := splitDeviceSources(values)
		v4, v6 = len(macs) > 0, len(macs) > 0
		for _, cidr := range cidrs {
			prefix := netip.MustParsePrefix(cidr)
			v4 = v4 || prefix.Addr().Is4()
			v6 = v6 || prefix.Addr().Is6()
		}
	}
	var state struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if json.Unmarshal(data, &state) != nil || state.NFTables == nil {
		return fmt.Errorf("无法解析 flowtable 检查所需的 nftables 状态")
	}
	var conflicts []string
	for _, entry := range state.NFTables {
		raw, exists := entry["flowtable"]
		if !exists {
			continue
		}
		var table struct {
			Family string   `json:"family"`
			Table  string   `json:"table"`
			Name   string   `json:"name"`
			Dev    []string `json:"dev"`
			Flags  []string `json:"flags"`
		}
		if json.Unmarshal(raw, &table) != nil {
			return fmt.Errorf("flowtable 信息不完整，无法确认与保护范围隔离；请关闭流量卸载并重载防火墙后重试")
		}
		if table.Family == "ip" && !v4 || table.Family == "ip6" && !v6 {
			continue
		}
		kind := "软件卸载"
		if slices.Contains(table.Flags, "offload") {
			kind = "已启用硬件卸载"
		} else if table.Family != "ip" && table.Family != "ip6" && table.Family != "inet" {
			kind = "卸载范围未知"
		}
		devices := "接口未知"
		if len(table.Dev) > 0 {
			devices = fmt.Sprintf("接口 %q", table.Dev)
		}
		conflicts = append(conflicts, fmt.Sprintf("%q/%q/%q（%s，%s）", table.Family, table.Table, table.Name, kind, devices))
	}
	if len(conflicts) == 0 {
		return nil
	}
	scope := "指定设备"
	if len(sources) == 0 {
		scope = "当前全局 TUN 保护范围"
	}
	return fmt.Errorf("flowtable %s 无法确认与%s隔离：已有卸载连接可能绕过防泄漏规则，仅修改加入条件或接口排除不足以确认安全；请关闭相关流量卸载并重载防火墙后重试", strings.Join(conflicts, "、"), scope)
}
