//go:build linux

package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	linuxNetworkCommandTimeout = 3 * time.Second
	linuxFW4CommandTimeout     = 15 * time.Second
	ownershipRecordTimeout     = 30 * time.Second
	ownershipRecordInterval    = 100 * time.Millisecond
)

func cleanupPlatformSingboxState(statePath string) (platformCleanupResult, error) {
	markerPresent, err := pathExists(singboxOpenWrtNFTInclude)
	if err != nil {
		return platformCleanupResult{}, err
	}
	staleMarkerPresent, err := pathExists(singboxStaleNFTMarker)
	if err != nil {
		return platformCleanupResult{}, err
	}
	ownershipState, statePresent, stateErr := readSingboxRouteTableState(statePath)
	if stateErr != nil {
		return platformCleanupResult{}, fmt.Errorf("sing-box ownership-state error: %w", stateErr)
	}
	if (markerPresent || staleMarkerPresent) && !statePresent {
		return platformCleanupResult{}, fmt.Errorf("sing-box ownership-state error: fw4 marker exists without valid network ownership state")
	}

	running, err := linuxSingboxProcessRunning()
	if err != nil {
		return platformCleanupResult{}, err
	}
	if running {
		return platformCleanupResult{ProcessRunning: true}, nil
	}
	if !statePresent {
		return platformCleanupResult{}, nil
	}
	if markerPresent && staleMarkerPresent {
		return platformCleanupResult{}, fmt.Errorf("sing-box ownership-state error: current and stale fw4 markers both exist")
	}
	if markerPresent || staleMarkerPresent {
		markerPath := singboxOpenWrtNFTInclude
		if staleMarkerPresent {
			markerPath = singboxStaleNFTMarker
		}
		if err := validateRecordedNFTInclude(markerPath, ownershipState.NFTIncludeIdentity); err != nil {
			return platformCleanupResult{}, err
		}
	}

	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return platformCleanupResult{}, fmt.Errorf("find ip command: %w", err)
	}
	nftPath, err := exec.LookPath("nft")
	if err != nil {
		return platformCleanupResult{}, fmt.Errorf("find nft command: %w", err)
	}
	ipv4Rules, ipv6Rules, err := readPlatformRuleSnapshots(ipPath, ownershipState.ExpectIPv4, ownershipState.ExpectIPv6)
	if err != nil {
		return platformCleanupResult{}, err
	}
	ipv4Tables, ipv6Tables, err := readPlatformRouteTableSnapshots(ipPath, ownershipState.ExpectIPv4, ownershipState.ExpectIPv6)
	if err != nil {
		return platformCleanupResult{}, err
	}
	currentNFTHandle, nftTablePresent, err := currentSingboxNFTTableHandle(nftPath)
	if err != nil {
		return platformCleanupResult{}, err
	}

	effectiveState := ownershipState
	if ownershipState.Phase == singboxOwnershipPending {
		effectiveState, _, err = derivePendingSingboxOwnership(ownershipState, ipv4Rules, ipv6Rules, currentNFTHandle, nftTablePresent)
		if err != nil {
			return platformCleanupResult{}, fmt.Errorf("derive pending sing-box ownership: %w", err)
		}
	}
	if err := validateRecordedNFTTable(effectiveState.NFTTableHandle, currentNFTHandle, nftTablePresent); err != nil {
		return platformCleanupResult{}, err
	}
	if err := validateOwnedRouteTables(ipPath, ipv4Tables, ipv6Tables, effectiveState); err != nil {
		return platformCleanupResult{}, err
	}
	if err := validatePriorityOneRuleDeletions(ipv4Rules, effectiveState.IPv4Tables); err != nil {
		return platformCleanupResult{}, fmt.Errorf("validate owned IPv4 priority-1 rules: %w", err)
	}
	if err := validatePriorityOneRuleDeletions(ipv6Rules, effectiveState.IPv6Tables); err != nil {
		return platformCleanupResult{}, fmt.Errorf("validate owned IPv6 priority-1 rules: %w", err)
	}
	if err := validateManagedIPRuleDeletions(ipv4Rules, effectiveState.IPv4Rules); err != nil {
		return platformCleanupResult{}, fmt.Errorf("validate owned IPv4 rules: %w", err)
	}
	if err := validateManagedIPRuleDeletions(ipv6Rules, effectiveState.IPv6Rules); err != nil {
		return platformCleanupResult{}, fmt.Errorf("validate owned IPv6 rules: %w", err)
	}

	var fw4Path string
	if markerPresent || staleMarkerPresent {
		fw4Path, err = exec.LookPath("fw4")
		if err != nil {
			return platformCleanupResult{}, fmt.Errorf("find fw4 command: %w", err)
		}
	}

	var cleanupErrors []error
	if markerPresent {
		if err := os.Rename(singboxOpenWrtNFTInclude, singboxStaleNFTMarker); err != nil {
			return platformCleanupResult{}, fmt.Errorf("disable sing-box nftables include: %w", err)
		}
		staleMarkerPresent = true
		if err := validateRecordedNFTInclude(singboxStaleNFTMarker, ownershipState.NFTIncludeIdentity); err != nil {
			return platformCleanupResult{}, err
		}
	}
	if ownsSingboxNFTTable(effectiveState.NFTTableHandle, currentNFTHandle, nftTablePresent) {
		currentNFTHandle, nftTablePresent, err = currentSingboxNFTTableHandle(nftPath)
		if err != nil {
			return platformCleanupResult{}, err
		}
		if err := validateRecordedNFTTable(effectiveState.NFTTableHandle, currentNFTHandle, nftTablePresent); err != nil {
			return platformCleanupResult{}, err
		}
		if output, commandErr := runLinuxNetworkCommand(linuxNetworkCommandTimeout, nftPath, "delete", "table", "inet", "sing-box"); commandErr != nil {
			cleanupErrors = append(cleanupErrors, cleanupCommandError("delete sing-box nftables table", output, commandErr))
		}
	}
	if staleMarkerPresent {
		if output, commandErr := runLinuxNetworkCommand(linuxFW4CommandTimeout, fw4Path, "reload"); commandErr != nil {
			cleanupErrors = append(cleanupErrors, cleanupCommandError("reload fw4", output, commandErr))
		}
	}
	cleanupErrors = append(cleanupErrors, cleanupIPRuleSnapshot(ipPath, false, ipv4Rules, ipv4Tables, effectiveState.IPv4Tables, effectiveState.IPv4Rules)...)
	cleanupErrors = append(cleanupErrors, cleanupIPRuleSnapshot(ipPath, true, ipv6Rules, ipv6Tables, effectiveState.IPv6Tables, effectiveState.IPv6Rules)...)
	if len(cleanupErrors) == 0 && staleMarkerPresent {
		if err := os.Remove(singboxStaleNFTMarker); err != nil && !os.IsNotExist(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove sing-box cleanup marker: %w", err))
		}
	}
	if len(cleanupErrors) == 0 {
		if err := removeSingboxRouteTableState(statePath); err != nil {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("remove route-table ownership state: %w", err))
		}
	}
	return platformCleanupResult{Cleaned: true}, errors.Join(cleanupErrors...)
}

func linuxSingboxProcessRunning() (bool, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false, fmt.Errorf("read /proc: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		commPath := filepath.Join("/proc", entry.Name(), "comm")
		name, err := os.ReadFile(commPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return false, fmt.Errorf("read process identity %s: %w", commPath, err)
		}
		if strings.TrimSpace(string(name)) == "sing-box" {
			return true, nil
		}
	}
	return false, nil
}

func snapshotPlatformPriorityOneTables(tunState activeTUNState) (platformNetworkBaseline, error) {
	if !tunState.ManagesRoutes {
		return platformNetworkBaseline{}, nil
	}
	if err := validateLinuxTUNCompatibility(tunState); err != nil {
		return platformNetworkBaseline{}, err
	}
	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return platformNetworkBaseline{}, fmt.Errorf("find ip command for TUN ownership preflight: %w", err)
	}
	nftPath, err := exec.LookPath("nft")
	if err != nil {
		return platformNetworkBaseline{}, fmt.Errorf("find nft command for auto-redirect ownership preflight: %w", err)
	}
	markerPresent, err := pathExists(singboxOpenWrtNFTInclude)
	if err != nil {
		return platformNetworkBaseline{}, err
	}
	staleMarkerPresent, err := pathExists(singboxStaleNFTMarker)
	if err != nil {
		return platformNetworkBaseline{}, err
	}
	if markerPresent || staleMarkerPresent {
		return platformNetworkBaseline{}, fmt.Errorf("auto-redirect ownership preflight found an existing current or stale fw4 include path")
	}
	ipv4Rules, ipv6Rules, err := readPlatformRuleSnapshots(ipPath, tunState.ExpectedIPv4, tunState.ExpectedIPv6)
	if err != nil {
		return platformNetworkBaseline{}, err
	}
	if tunState.ExpectedIPv4 {
		if conflicts := ipv4Rules.singboxTUNPriorityConflicts(); len(conflicts) > 0 {
			return platformNetworkBaseline{}, fmt.Errorf("IPv4 policy rule priorities reserved by sing-box TUN are already in use: %v", conflicts)
		}
	}
	if tunState.ExpectedIPv6 {
		if conflicts := ipv6Rules.singboxTUNPriorityConflicts(); len(conflicts) > 0 {
			return platformNetworkBaseline{}, fmt.Errorf("IPv6 policy rule priorities reserved by sing-box TUN are already in use: %v", conflicts)
		}
	}
	_, nftPresent, err := currentSingboxNFTTableHandle(nftPath)
	if err != nil {
		return platformNetworkBaseline{}, err
	}
	if nftPresent {
		return platformNetworkBaseline{}, fmt.Errorf("auto-redirect ownership preflight found pre-existing nftables table inet sing-box")
	}
	return platformNetworkBaseline{
		Required:       true,
		IPv4Tables:     ipv4Rules.priorityOneLookupTables(),
		IPv6Tables:     ipv6Rules.priorityOneLookupTables(),
		IPv4Rules:      ipv4Rules.managedRuleIDs(),
		IPv6Rules:      ipv6Rules.managedRuleIDs(),
		ExpectIPv4:     tunState.ExpectedIPv4,
		ExpectIPv6:     tunState.ExpectedIPv6,
		NFTTableAbsent: true,
	}, nil
}

func recordPlatformSingboxRouteTables(statePath string, processExited <-chan struct{}) error {
	state, present, err := readSingboxRouteTableState(statePath)
	if err != nil {
		return err
	}
	if !present || state.Phase != singboxOwnershipPending {
		return fmt.Errorf("pending sing-box network ownership state is unavailable")
	}
	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return fmt.Errorf("find ip command while recording network ownership: %w", err)
	}
	nftPath, err := exec.LookPath("nft")
	if err != nil {
		return fmt.Errorf("find nft command while recording network ownership: %w", err)
	}
	deadline := time.NewTimer(ownershipRecordTimeout)
	defer deadline.Stop()
	var lastObservationErr error
	for {
		select {
		case <-processExited:
			return fmt.Errorf("sing-box exited before network ownership could be recorded")
		default:
		}
		ipv4Rules, ipv6Rules, err := readPlatformRuleSnapshots(ipPath, state.ExpectIPv4, state.ExpectIPv6)
		if err != nil {
			return err
		}
		nftHandle, nftPresent, err := currentSingboxNFTTableHandle(nftPath)
		if err != nil {
			return err
		}
		readyState, ready, err := derivePendingSingboxOwnership(state, ipv4Rules, ipv6Rules, nftHandle, nftPresent)
		if err != nil {
			// sing-box installs IPv4 and IPv6 policy rules in several steps. A
			// reserved priority can therefore contain a transitional rule while
			// the process is still starting; only treat it as a conflict if it
			// remains until the observation deadline.
			lastObservationErr = err
		} else if ready {
			includeIdentity, includePresent, err := currentSingboxNFTIncludeIdentity(singboxOpenWrtNFTInclude)
			if err != nil {
				return err
			}
			if includePresent {
				readyState.NFTIncludeIdentity = includeIdentity
			}
			if err := promotePendingSingboxRouteTableState(statePath, state, readyState); err != nil {
				return fmt.Errorf("record ready sing-box network ownership: %w", err)
			}
			return nil
		} else {
			lastObservationErr = nil
		}
		select {
		case <-processExited:
			return fmt.Errorf("sing-box exited before network ownership could be recorded")
		case <-deadline.C:
			if lastObservationErr != nil {
				return fmt.Errorf("timed out recording sing-box network ownership: %w", lastObservationErr)
			}
			return fmt.Errorf("timed out recording sing-box network ownership")
		case <-time.After(ownershipRecordInterval):
		}
	}
}

func validateRecordedNFTInclude(path, recordedIdentity string) error {
	if recordedIdentity == "" {
		return fmt.Errorf("sing-box ownership-state error: fw4 include exists without a recorded identity")
	}
	currentIdentity, present, err := currentSingboxNFTIncludeIdentity(path)
	if err != nil {
		return err
	}
	if !present {
		return fmt.Errorf("sing-box ownership-state error: recorded fw4 include disappeared")
	}
	if currentIdentity != recordedIdentity {
		return fmt.Errorf("sing-box ownership-state error: recorded fw4 include was replaced")
	}
	return nil
}

func currentSingboxNFTIncludeIdentity(path string) (string, bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("inspect fw4 include identity: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", false, fmt.Errorf("inspect fw4 include identity: unsupported file metadata")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", false, fmt.Errorf("read fw4 include identity: %w", err)
	}
	digest := sha256.Sum256(content)
	return fmt.Sprintf("%d:%d:%d:%x", stat.Dev, stat.Ino, info.Size(), digest), true, nil
}

func currentSingboxNFTTableHandle(nftPath string) (string, bool, error) {
	tablesOutput, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, nftPath, "list", "tables")
	if err != nil {
		return "", false, cleanupCommandError("list nftables tables", tablesOutput, err)
	}
	if !hasSingboxNFTTable(string(tablesOutput)) {
		return "", false, nil
	}
	tableOutput, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, nftPath, "-a", "list", "table", "inet", "sing-box")
	if err != nil {
		return "", true, cleanupCommandError("inspect sing-box nftables table", tableOutput, err)
	}
	handle, ok := parseSingboxNFTTableHandle(string(tableOutput))
	if !ok {
		return "", true, fmt.Errorf("inspect sing-box nftables table: handle not found")
	}
	return handle, true, nil
}

func readPlatformRuleSnapshots(ipPath string, expectIPv4, expectIPv6 bool) (ipRuleSnapshot, ipRuleSnapshot, error) {
	ipv4Rules := ipRuleSnapshot{lines: make(map[int][]string)}
	ipv6Rules := ipRuleSnapshot{lines: make(map[int][]string)}
	if expectIPv4 {
		output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, "rule", "show")
		if err != nil {
			return ipRuleSnapshot{}, ipRuleSnapshot{}, cleanupCommandError("list IPv4 rules", output, err)
		}
		ipv4Rules = parseIPRuleSnapshot(string(output))
	}
	if expectIPv6 {
		output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, "-6", "rule", "show")
		if err != nil {
			return ipRuleSnapshot{}, ipRuleSnapshot{}, cleanupCommandError("list IPv6 rules", output, err)
		}
		ipv6Rules = parseIPRuleSnapshot(string(output))
	}
	return ipv4Rules, ipv6Rules, nil
}

func readPlatformRouteTableSnapshots(ipPath string, expectIPv4, expectIPv6 bool) (routeTableSnapshot, routeTableSnapshot, error) {
	ipv4Tables := make(routeTableSnapshot)
	ipv6Tables := make(routeTableSnapshot)
	if expectIPv4 {
		output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, "-o", "route", "show", "table", "all")
		if err != nil {
			return nil, nil, cleanupCommandError("list IPv4 route tables", output, err)
		}
		ipv4Tables = parseRouteTableSnapshot(string(output))
	}
	if expectIPv6 {
		output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, "-6", "-o", "route", "show", "table", "all")
		if err != nil {
			return nil, nil, cleanupCommandError("list IPv6 route tables", output, err)
		}
		ipv6Tables = parseRouteTableSnapshot(string(output))
	}
	return ipv4Tables, ipv6Tables, nil
}

func validateOwnedRouteTables(ipPath string, ipv4Tables, ipv6Tables routeTableSnapshot, state singboxRouteTableState) error {
	var validationErrors []error
	validate := func(ipv6 bool, tables []string, existing routeTableSnapshot) {
		for _, table := range tables {
			if !existing.has(table) {
				continue
			}
			if err := inspectOwnedRouteTable(ipPath, ipv6, table); err != nil {
				validationErrors = append(validationErrors, err)
			}
		}
	}
	if state.ExpectIPv4 {
		validate(false, state.IPv4Tables, ipv4Tables)
	}
	if state.ExpectIPv6 {
		validate(true, state.IPv6Tables, ipv6Tables)
	}
	return errors.Join(validationErrors...)
}

func inspectOwnedRouteTable(ipPath string, ipv6 bool, table string) error {
	args := []string{"route", "show", "table", table}
	familyName := "IPv4"
	if ipv6 {
		args = append([]string{"-6"}, args...)
		familyName = "IPv6"
	}
	output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, args...)
	if err != nil {
		return cleanupCommandError("inspect owned "+familyName+" redirect route table", output, err)
	}
	if !isSafeOwnedRedirectRouteTable(string(output), ipv6) {
		return fmt.Errorf("owned %s redirect route table %s contains unrecognized routes", familyName, table)
	}
	return nil
}

func cleanupIPRuleSnapshot(ipPath string, ipv6 bool, rules ipRuleSnapshot, existingTables routeTableSnapshot, redirectTables, managedRuleIDs []string) []error {
	var cleanupErrors []error
	prefix := make([]string, 0, 1)
	familyName := "IPv4"
	if ipv6 {
		prefix = append(prefix, "-6")
		familyName = "IPv6"
	}
	run := func(action string, args ...string) {
		commandArgs := append(append([]string{}, prefix...), args...)
		if output, err := runLinuxNetworkCommand(linuxNetworkCommandTimeout, ipPath, commandArgs...); err != nil {
			cleanupErrors = append(cleanupErrors, cleanupCommandError(action, output, err))
		}
	}

	for _, action := range planRouteTableCleanup(rules, existingTables, redirectTables) {
		if action.deleteRule {
			run("delete "+familyName+" redirect rule", "rule", "del", "priority", "1", "from", "all", "lookup", action.table)
		}
		if action.flushTable {
			if err := inspectOwnedRouteTable(ipPath, ipv6, action.table); err != nil {
				cleanupErrors = append(cleanupErrors, err)
				continue
			}
			run("flush "+familyName+" redirect route table", "route", "flush", "table", action.table)
		}
	}
	for _, ruleID := range managedRuleIDs {
		spec, ok := managedIPRuleSpecByID(ruleID)
		if !ok || !rules.hasExactRule(spec.priority, spec.line) {
			continue
		}
		run("delete "+familyName+" managed "+ruleID+" rule", spec.delete...)
	}
	return cleanupErrors
}

func runLinuxNetworkCommand(timeout time.Duration, path string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return exec.CommandContext(ctx, path, args...).CombinedOutput()
}

func syncOwnershipStateParentDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return fmt.Errorf("open route-table ownership state directory: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("sync route-table ownership state directory: %w", err)
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("inspect %s: %w", path, err)
}

func cleanupCommandError(action string, output []byte, err error) error {
	details := strings.Join(strings.Fields(string(output)), " ")
	if len(details) > 256 {
		details = details[:256]
	}
	if details == "" {
		return fmt.Errorf("%s: %w", action, err)
	}
	return fmt.Errorf("%s: %w: %s", action, err, details)
}
