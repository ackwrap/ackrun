package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"

	"github.com/ackwrap/ackrun/internal/logging"
)

func forceCleanupSingboxState(statePath, includePath string, command func(string, ...string) ([]byte, error)) (platformCleanupResult, error) {
	result := platformCleanupResult{}
	var failures []error
	run := func(action, name string, args ...string) {
		_, err := command(name, args...)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", action, err))
			logging.Error("network.repair", "强制清理命令失败: action=%s error=%v", action, err)
			return
		}
		result.Cleaned = true
	}
	state, _, stateErr := readSingboxRouteTableState(statePath)
	if stateErr != nil {
		logging.Info("network.repair", "强制清理忽略不可用的网络所有权记录: %v", stateErr)
	}
	stalePath := includePath + ".ackwrap-stale"
	markerPresent := false
	if _, err := os.Lstat(includePath); err == nil {
		markerPresent = true
		if _, staleErr := os.Lstat(stalePath); staleErr == nil {
			err = os.Remove(includePath)
		} else if os.IsNotExist(staleErr) {
			err = os.Rename(includePath, stalePath)
		} else {
			err = staleErr
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("disable sing-box fw4 include: %w", err))
		} else {
			result.Cleaned = true
		}
	} else if !os.IsNotExist(err) {
		failures = append(failures, fmt.Errorf("inspect sing-box fw4 include: %w", err))
	}
	if _, err := os.Lstat(stalePath); err == nil {
		markerPresent = true
	} else if !os.IsNotExist(err) {
		failures = append(failures, fmt.Errorf("inspect sing-box cleanup marker: %w", err))
	}
	output, err := command("nft", "list", "tables")
	if err != nil {
		failures = append(failures, fmt.Errorf("list nftables tables: %w", err))
	} else if hasSingboxNFTTable(string(output)) {
		run("delete sing-box nftables table", "nft", "delete", "table", "inet", "sing-box")
	}
	if _, err := command("fw4", "reload"); err != nil {
		if markerPresent || !errors.Is(err, exec.ErrNotFound) {
			failures = append(failures, fmt.Errorf("reload fw4: %w", err))
		}
	} else {
		result.Cleaned = true
	}
	for _, ipv6 := range []bool{false, true} {
		prefix := []string{}
		owned, baseline := state.IPv4Tables, state.BaselineIPv4Tables
		if ipv6 {
			prefix = append(prefix, "-6")
			owned, baseline = state.IPv6Tables, state.BaselineIPv6Tables
		}
		output, err := command("ip", append(append([]string{}, prefix...), "rule", "show")...)
		if err != nil {
			failures = append(failures, fmt.Errorf("list rules (ipv6=%t): %w", ipv6, err))
			continue
		}
		rules := parseIPRuleSnapshot(string(output))
		for _, priority := range rules.singboxTUNPriorityConflicts() {
			for range rules.lines[priority] {
				run("delete reserved sing-box rule", "ip", append(append([]string{}, prefix...), "rule", "del", "priority", strconv.Itoa(priority))...)
			}
		}
		output, err = command("ip", append(append([]string{}, prefix...), "-o", "route", "show", "table", "all")...)
		if err != nil {
			failures = append(failures, fmt.Errorf("list route tables (ipv6=%t): %w", ipv6, err))
			continue
		}
		existing := parseRouteTableSnapshot(string(output))
		if existing.has(singboxRouteTable) {
			run("flush sing-box TUN route table", "ip", append(append([]string{}, prefix...), "route", "flush", "table", singboxRouteTable)...)
		}
		candidates := append(rules.exactPriorityOneLookupTables(), owned...)
		sort.Strings(candidates)
		previous := ""
		for _, table := range candidates {
			if table == previous {
				continue
			}
			previous = table
			tableID, err := strconv.ParseUint(table, 10, 32)
			if err != nil || tableID == 0 || tableID == 253 || tableID == 254 || tableID == 255 || tableID == defaultIPRoute2TableIndex || stringSlicesOverlap([]string{table}, baseline) {
				continue
			}
			recorded := stringSlicesOverlap([]string{table}, owned)
			if existing.has(table) {
				output, err := command("ip", append(append([]string{}, prefix...), "route", "show", "table", table)...)
				if err != nil {
					failures = append(failures, fmt.Errorf("inspect redirect route table %s: %w", table, err))
					continue
				}
				if !isSingboxRedirectRouteTable(string(output), ipv6) && !(recorded && isSafeOwnedRedirectRouteTable(string(output), ipv6)) {
					continue
				}
			} else if !recorded {
				continue
			}
			for _, line := range rules.lines[1] {
				if line == "from all lookup "+table {
					run("delete sing-box redirect rule", "ip", append(append([]string{}, prefix...), "rule", "del", "priority", "1", "from", "all", "lookup", table)...)
				}
			}
			if existing.has(table) {
				run("flush sing-box redirect route table", "ip", append(append([]string{}, prefix...), "route", "flush", "table", table)...)
			}
		}
	}
	if len(failures) == 0 {
		for _, path := range []string{stalePath, statePath} {
			if err := os.Remove(path); err == nil {
				result.Cleaned = true
			} else if !os.IsNotExist(err) {
				failures = append(failures, fmt.Errorf("remove sing-box recovery state: %w", err))
			}
		}
	}
	return result, errors.Join(failures...)
}
