package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const (
	singboxOpenWrtNFTInclude      = "/etc/nftables.d/0-sing-box-auto-redirect.nft"
	singboxStaleNFTMarker         = singboxOpenWrtNFTInclude + ".ackwrap-stale"
	singboxRouteTable             = "2022"
	singboxRouteTableStateVersion = 2
	singboxOwnershipPending       = "pending"
	singboxOwnershipReady         = "ready"
	defaultIPRoute2TableIndex     = 2022
	defaultIPRoute2RuleIndex      = 9000
	defaultFallbackRuleIndex      = 32768
	defaultAutoRedirectInputMark  = 0x2023
)

var (
	ipRuleLinePattern     = regexp.MustCompile(`^\s*(\d+):\s*(.+)$`)
	nftTableHandlePattern = regexp.MustCompile(`(?m)table\s+inet\s+sing-box\s*\{\s*#\s*handle\s+(\d+)`)
)

type platformCleanupResult struct {
	ProcessRunning bool
	Cleaned        bool
}

type singboxRouteTableState struct {
	Version            int      `json:"version"`
	Phase              string   `json:"phase"`
	ExpectIPv4         bool     `json:"expect_ipv4"`
	ExpectIPv6         bool     `json:"expect_ipv6"`
	NFTTableAbsent     bool     `json:"nft_table_absent"`
	BaselineIPv4Tables []string `json:"baseline_ipv4_tables,omitempty"`
	BaselineIPv6Tables []string `json:"baseline_ipv6_tables,omitempty"`
	BaselineIPv4Rules  []string `json:"baseline_ipv4_rules,omitempty"`
	BaselineIPv6Rules  []string `json:"baseline_ipv6_rules,omitempty"`
	IPv4Tables         []string `json:"ipv4_tables,omitempty"`
	IPv6Tables         []string `json:"ipv6_tables,omitempty"`
	NFTTableHandle     string   `json:"nft_table_handle,omitempty"`
	NFTIncludeIdentity string   `json:"nft_include_identity,omitempty"`
	IPv4Rules          []string `json:"ipv4_rules,omitempty"`
	IPv6Rules          []string `json:"ipv6_rules,omitempty"`
}

type platformNetworkBaseline struct {
	Required       bool
	IPv4Tables     []string
	IPv6Tables     []string
	IPv4Rules      []string
	IPv6Rules      []string
	ExpectIPv4     bool
	ExpectIPv6     bool
	NFTTableAbsent bool
}

type managedIPRuleSpec struct {
	id       string
	priority int
	line     string
	delete   []string
}

var managedIPRuleSpecs = []managedIPRuleSpec{
	{id: "output_mark", priority: 9000, line: "from all fwmark 0x2024 goto 9002", delete: []string{"rule", "del", "priority", "9000", "from", "all", "fwmark", "0x2024", "goto", "9002"}},
	{id: "input_mark", priority: 9001, line: "from all fwmark 0x2023 lookup 2022", delete: []string{"rule", "del", "priority", "9001", "from", "all", "fwmark", "0x2023", "lookup", singboxRouteTable}},
	{id: "redirect_nop", priority: 9002, line: "from all nop", delete: []string{"rule", "del", "priority", "9002", "from", "all", "type", "nop"}},
	{id: "fallback", priority: 32768, line: "from all lookup 2022", delete: []string{"rule", "del", "priority", "32768", "from", "all", "lookup", singboxRouteTable}},
}

type ipRuleSnapshot struct {
	lines map[int][]string
}

type routeTableSnapshot map[string]struct{}

func parseRouteTableSnapshot(output string) routeTableSnapshot {
	snapshot := make(routeTableSnapshot)
	for line := range strings.SplitSeq(output, "\n") {
		fields := strings.Fields(line)
		for index := 0; index+1 < len(fields); index++ {
			if fields[index] != "table" {
				continue
			}
			table := fields[index+1]
			if value, err := strconv.ParseUint(table, 10, 32); err == nil && value != 0 {
				snapshot[table] = struct{}{}
			}
		}
	}
	return snapshot
}

func (snapshot routeTableSnapshot) has(table string) bool {
	_, exists := snapshot[table]
	return exists
}

type routeTableCleanupAction struct {
	table      string
	deleteRule bool
	flushTable bool
}

func planRouteTableCleanup(rules ipRuleSnapshot, existing routeTableSnapshot, recorded []string) []routeTableCleanupAction {
	actions := make([]routeTableCleanupAction, 0, len(recorded))
	for _, table := range recorded {
		actions = append(actions, routeTableCleanupAction{
			table:      table,
			deleteRule: rules.hasExactPriorityOneLookupTable(table),
			flushTable: existing.has(table),
		})
	}
	return actions
}

func parseIPRuleSnapshot(output string) ipRuleSnapshot {
	snapshot := ipRuleSnapshot{lines: make(map[int][]string)}
	for line := range strings.SplitSeq(output, "\n") {
		matches := ipRuleLinePattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}
		priority, err := strconv.Atoi(matches[1])
		if err != nil {
			continue
		}
		snapshot.lines[priority] = append(snapshot.lines[priority], strings.Join(strings.Fields(matches[2]), " "))
	}
	return snapshot
}

func (snapshot ipRuleSnapshot) hasExactRule(priority int, line string) bool {
	for _, candidate := range snapshot.lines[priority] {
		if candidate == line {
			return true
		}
	}
	return false
}

func (snapshot ipRuleSnapshot) managedRuleIDs() []string {
	result := make([]string, 0, len(managedIPRuleSpecs))
	for _, spec := range managedIPRuleSpecs {
		if snapshot.hasExactRule(spec.priority, spec.line) {
			result = append(result, spec.id)
		}
	}
	return result
}

func (snapshot ipRuleSnapshot) singboxTUNPriorityConflicts() []int {
	var conflicts []int
	for priority, lines := range snapshot.lines {
		if len(lines) > 0 && (priority >= 9000 && priority <= 9010 || priority == 32768) {
			conflicts = append(conflicts, priority)
		}
	}
	sort.Ints(conflicts)
	return conflicts
}

func (snapshot ipRuleSnapshot) hasSingboxAutoRedirectSignature() bool {
	return len(snapshot.managedRuleIDs()) == len(managedIPRuleSpecs)
}

func (snapshot ipRuleSnapshot) priorityOneLookupTables() []string {
	tables := make(map[string]struct{})
	for _, line := range snapshot.lines[1] {
		fields := strings.Fields(line)
		for index := 0; index+1 < len(fields); index++ {
			if fields[index] != "lookup" {
				continue
			}
			if table, err := strconv.ParseUint(fields[index+1], 10, 32); err == nil && table != 0 && fields[index+1] != singboxRouteTable {
				tables[fields[index+1]] = struct{}{}
			}
		}
	}
	result := make([]string, 0, len(tables))
	for table := range tables {
		result = append(result, table)
	}
	sort.Strings(result)
	return result
}

func (snapshot ipRuleSnapshot) exactPriorityOneLookupTables() []string {
	var tables []string
	for _, table := range snapshot.priorityOneLookupTables() {
		if snapshot.hasExactPriorityOneLookupTable(table) {
			tables = append(tables, table)
		}
	}
	return tables
}

func (snapshot ipRuleSnapshot) hasPriorityOneLookupTable(table string) bool {
	for _, candidate := range snapshot.priorityOneLookupTables() {
		if candidate == table {
			return true
		}
	}
	return false
}

func (snapshot ipRuleSnapshot) hasExactPriorityOneLookupTable(table string) bool {
	return snapshot.hasExactRule(1, "from all lookup "+table)
}

func (snapshot ipRuleSnapshot) hasLookupTable(table string) bool {
	for _, lines := range snapshot.lines {
		for _, line := range lines {
			fields := strings.Fields(line)
			for index := 0; index+1 < len(fields); index++ {
				if fields[index] == "lookup" && fields[index+1] == table {
					return true
				}
			}
		}
	}
	return false
}

func sharedPriorityOneLookupTables(ipv4, ipv6 ipRuleSnapshot) []string {
	ipv6Tables := make(map[string]struct{})
	for _, table := range ipv6.priorityOneLookupTables() {
		ipv6Tables[table] = struct{}{}
	}
	shared := make([]string, 0)
	for _, table := range ipv4.priorityOneLookupTables() {
		if _, exists := ipv6Tables[table]; exists {
			shared = append(shared, table)
		}
	}
	return shared
}

func priorityOneLookupTablesForFamilies(ipv4, ipv6 ipRuleSnapshot, expectIPv4, expectIPv6 bool) []string {
	if expectIPv4 && expectIPv6 {
		return sharedPriorityOneLookupTables(ipv4, ipv6)
	}
	if expectIPv4 {
		return ipv4.priorityOneLookupTables()
	}
	if expectIPv6 {
		return ipv6.priorityOneLookupTables()
	}
	return nil
}

func isSingboxRedirectRouteTable(output string, ipv6 bool) bool {
	prefix := "local 127.0.0.1 dev "
	if ipv6 {
		prefix = "local ::1 dev "
	}
	found := false
	for line := range strings.SplitSeq(output, "\n") {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, prefix) || (!ipv6 && !strings.Contains(line, " scope host")) {
			return false
		}
		found = true
	}
	return found
}

func isSafeOwnedRedirectRouteTable(output string, ipv6 bool) bool {
	return strings.TrimSpace(output) == "" || isSingboxRedirectRouteTable(output, ipv6)
}

func normalizeRouteTableIDs(tables []string) ([]string, error) {
	seen := make(map[string]struct{})
	normalized := make([]string, 0, len(tables))
	for _, table := range tables {
		table = strings.TrimSpace(table)
		value, err := strconv.ParseUint(table, 10, 32)
		if err != nil || value == 0 || table == singboxRouteTable {
			return nil, fmt.Errorf("invalid sing-box route table ID %q", table)
		}
		if _, exists := seen[table]; exists {
			continue
		}
		seen[table] = struct{}{}
		normalized = append(normalized, table)
	}
	sort.Strings(normalized)
	return normalized, nil
}

func normalizeSingboxRouteTableState(state singboxRouteTableState) (singboxRouteTableState, error) {
	if state.Version != singboxRouteTableStateVersion {
		return singboxRouteTableState{}, fmt.Errorf("unsupported sing-box network ownership state version %d", state.Version)
	}
	if state.Phase != singboxOwnershipPending && state.Phase != singboxOwnershipReady {
		return singboxRouteTableState{}, fmt.Errorf("invalid sing-box network ownership phase %q", state.Phase)
	}
	if !state.ExpectIPv4 && !state.ExpectIPv6 {
		return singboxRouteTableState{}, fmt.Errorf("sing-box network ownership state has no expected address family")
	}
	if !state.NFTTableAbsent {
		return singboxRouteTableState{}, fmt.Errorf("sing-box network ownership state did not record an absent nftables namespace")
	}
	var err error
	state.BaselineIPv4Tables, err = normalizeRouteTableIDs(state.BaselineIPv4Tables)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.BaselineIPv6Tables, err = normalizeRouteTableIDs(state.BaselineIPv6Tables)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.IPv4Tables, err = normalizeRouteTableIDs(state.IPv4Tables)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.IPv6Tables, err = normalizeRouteTableIDs(state.IPv6Tables)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.NFTTableHandle = strings.TrimSpace(state.NFTTableHandle)
	state.NFTIncludeIdentity = strings.TrimSpace(state.NFTIncludeIdentity)
	if state.NFTTableHandle != "" {
		handle, err := strconv.ParseUint(state.NFTTableHandle, 10, 64)
		if err != nil || handle == 0 {
			return singboxRouteTableState{}, fmt.Errorf("invalid sing-box nftables table handle %q", state.NFTTableHandle)
		}
	}
	state.BaselineIPv4Rules, err = normalizeManagedIPRuleIDs(state.BaselineIPv4Rules)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.BaselineIPv6Rules, err = normalizeManagedIPRuleIDs(state.BaselineIPv6Rules)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.IPv4Rules, err = normalizeManagedIPRuleIDs(state.IPv4Rules)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	state.IPv6Rules, err = normalizeManagedIPRuleIDs(state.IPv6Rules)
	if err != nil {
		return singboxRouteTableState{}, err
	}
	if !state.ExpectIPv4 && (len(state.BaselineIPv4Tables) > 0 || len(state.BaselineIPv4Rules) > 0 || len(state.IPv4Tables) > 0 || len(state.IPv4Rules) > 0) {
		return singboxRouteTableState{}, fmt.Errorf("sing-box network ownership state records unexpected IPv4 resources")
	}
	if !state.ExpectIPv6 && (len(state.BaselineIPv6Tables) > 0 || len(state.BaselineIPv6Rules) > 0 || len(state.IPv6Tables) > 0 || len(state.IPv6Rules) > 0) {
		return singboxRouteTableState{}, fmt.Errorf("sing-box network ownership state records unexpected IPv6 resources")
	}
	if state.Phase == singboxOwnershipPending {
		if len(state.IPv4Tables) > 0 || len(state.IPv6Tables) > 0 || state.NFTTableHandle != "" || len(state.IPv4Rules) > 0 || len(state.IPv6Rules) > 0 {
			return singboxRouteTableState{}, fmt.Errorf("pending sing-box network ownership state contains ready resources")
		}
		return state, nil
	}
	if stringSlicesOverlap(state.BaselineIPv4Tables, state.IPv4Tables) || stringSlicesOverlap(state.BaselineIPv6Tables, state.IPv6Tables) ||
		stringSlicesOverlap(state.BaselineIPv4Rules, state.IPv4Rules) || stringSlicesOverlap(state.BaselineIPv6Rules, state.IPv6Rules) {
		return singboxRouteTableState{}, fmt.Errorf("ready sing-box network ownership state overlaps its pre-start baseline")
	}
	if state.NFTTableHandle == "" {
		return singboxRouteTableState{}, fmt.Errorf("ready sing-box network ownership state has no nftables table handle")
	}
	if state.ExpectIPv4 && (len(state.IPv4Tables) != 1 || !hasAllManagedIPRuleIDs(state.IPv4Rules)) {
		return singboxRouteTableState{}, fmt.Errorf("ready sing-box network ownership state has incomplete IPv4 resources")
	}
	if state.ExpectIPv6 && (len(state.IPv6Tables) != 1 || !hasAllManagedIPRuleIDs(state.IPv6Rules)) {
		return singboxRouteTableState{}, fmt.Errorf("ready sing-box network ownership state has incomplete IPv6 resources")
	}
	return state, nil
}

func stringSlicesOverlap(left, right []string) bool {
	values := make(map[string]struct{}, len(left))
	for _, value := range left {
		values[value] = struct{}{}
	}
	for _, value := range right {
		if _, exists := values[value]; exists {
			return true
		}
	}
	return false
}

func hasAllManagedIPRuleIDs(ruleIDs []string) bool {
	return len(ruleIDs) == len(managedIPRuleSpecs)
}

func normalizeManagedIPRuleIDs(ruleIDs []string) ([]string, error) {
	valid := make(map[string]struct{}, len(managedIPRuleSpecs))
	for _, spec := range managedIPRuleSpecs {
		valid[spec.id] = struct{}{}
	}
	seen := make(map[string]struct{}, len(ruleIDs))
	result := make([]string, 0, len(ruleIDs))
	for _, ruleID := range ruleIDs {
		if _, ok := valid[ruleID]; !ok {
			return nil, fmt.Errorf("invalid managed sing-box rule ID %q", ruleID)
		}
		if _, ok := seen[ruleID]; ok {
			continue
		}
		seen[ruleID] = struct{}{}
		result = append(result, ruleID)
	}
	sort.Strings(result)
	return result, nil
}

func addedManagedIPRuleIDs(current, baseline []string) []string {
	baselineSet := make(map[string]struct{}, len(baseline))
	for _, ruleID := range baseline {
		baselineSet[ruleID] = struct{}{}
	}
	result := make([]string, 0, len(current))
	for _, ruleID := range current {
		if _, existed := baselineSet[ruleID]; !existed {
			result = append(result, ruleID)
		}
	}
	return result
}

func managedIPRuleSpecByID(ruleID string) (managedIPRuleSpec, bool) {
	for _, spec := range managedIPRuleSpecs {
		if spec.id == ruleID {
			return spec, true
		}
	}
	return managedIPRuleSpec{}, false
}

func validateManagedIPRuleDeletions(snapshot ipRuleSnapshot, ruleIDs []string) error {
	for _, ruleID := range ruleIDs {
		spec, ok := managedIPRuleSpecByID(ruleID)
		if !ok {
			continue
		}
		if !snapshot.hasExactRule(spec.priority, spec.line) {
			if len(snapshot.lines[spec.priority]) > 0 {
				return fmt.Errorf("managed sing-box rule at priority %d was replaced", spec.priority)
			}
			continue
		}
		if len(snapshot.lines[spec.priority]) != 1 {
			return fmt.Errorf("managed sing-box rule priority %d is shared", spec.priority)
		}
	}
	return nil
}

func validatePriorityOneRuleDeletions(snapshot ipRuleSnapshot, tables []string) error {
	for _, table := range tables {
		if snapshot.hasExactPriorityOneLookupTable(table) {
			continue
		}
		if snapshot.hasLookupTable(table) {
			return fmt.Errorf("managed sing-box priority-1 rule for table %s was replaced", table)
		}
	}
	return nil
}

func pendingSingboxRouteTableState(baseline platformNetworkBaseline) (singboxRouteTableState, error) {
	if !baseline.Required {
		return singboxRouteTableState{}, fmt.Errorf("sing-box network ownership is not required")
	}
	return normalizeSingboxRouteTableState(singboxRouteTableState{
		Version:            singboxRouteTableStateVersion,
		Phase:              singboxOwnershipPending,
		ExpectIPv4:         baseline.ExpectIPv4,
		ExpectIPv6:         baseline.ExpectIPv6,
		NFTTableAbsent:     baseline.NFTTableAbsent,
		BaselineIPv4Tables: baseline.IPv4Tables,
		BaselineIPv6Tables: baseline.IPv6Tables,
		BaselineIPv4Rules:  baseline.IPv4Rules,
		BaselineIPv6Rules:  baseline.IPv6Rules,
	})
}

func addedRouteTableIDs(current, baseline []string) []string {
	baselineSet := make(map[string]struct{}, len(baseline))
	for _, table := range baseline {
		baselineSet[table] = struct{}{}
	}
	var result []string
	for _, table := range current {
		if _, existed := baselineSet[table]; !existed {
			result = append(result, table)
		}
	}
	return result
}

func derivePendingSingboxOwnership(state singboxRouteTableState, ipv4, ipv6 ipRuleSnapshot, nftHandle string, nftPresent bool) (singboxRouteTableState, bool, error) {
	state, err := normalizeSingboxRouteTableState(state)
	if err != nil {
		return singboxRouteTableState{}, false, err
	}
	if state.Phase != singboxOwnershipPending {
		return singboxRouteTableState{}, false, fmt.Errorf("sing-box network ownership state is not pending")
	}
	deriveFamily := func(expected bool, snapshot ipRuleSnapshot, baselineTables, baselineRules []string) ([]string, []string, error) {
		if !expected {
			return nil, nil, nil
		}
		for _, spec := range managedIPRuleSpecs {
			baselineHasRule := false
			for _, ruleID := range baselineRules {
				baselineHasRule = baselineHasRule || ruleID == spec.id
			}
			if !baselineHasRule && len(snapshot.lines[spec.priority]) > 0 && !snapshot.hasExactRule(spec.priority, spec.line) {
				return nil, nil, fmt.Errorf("reserved sing-box rule priority %d contains an unexpected replacement", spec.priority)
			}
		}
		tables := addedRouteTableIDs(snapshot.exactPriorityOneLookupTables(), baselineTables)
		if len(tables) > 1 {
			return nil, nil, fmt.Errorf("multiple post-baseline sing-box priority-1 route tables are ambiguous: %v", tables)
		}
		return tables, addedManagedIPRuleIDs(snapshot.managedRuleIDs(), baselineRules), nil
	}
	state.IPv4Tables, state.IPv4Rules, err = deriveFamily(state.ExpectIPv4, ipv4, state.BaselineIPv4Tables, state.BaselineIPv4Rules)
	if err != nil {
		return singboxRouteTableState{}, false, fmt.Errorf("derive IPv4 ownership: %w", err)
	}
	state.IPv6Tables, state.IPv6Rules, err = deriveFamily(state.ExpectIPv6, ipv6, state.BaselineIPv6Tables, state.BaselineIPv6Rules)
	if err != nil {
		return singboxRouteTableState{}, false, fmt.Errorf("derive IPv6 ownership: %w", err)
	}
	if state.NFTTableAbsent && nftPresent {
		state.NFTTableHandle = nftHandle
	}
	ready := (!state.ExpectIPv4 || len(state.IPv4Tables) == 1 && hasAllManagedIPRuleIDs(state.IPv4Rules)) &&
		(!state.ExpectIPv6 || len(state.IPv6Tables) == 1 && hasAllManagedIPRuleIDs(state.IPv6Rules)) && state.NFTTableHandle != ""
	if ready {
		state.Phase = singboxOwnershipReady
		state, err = normalizeSingboxRouteTableState(state)
		if err != nil {
			return singboxRouteTableState{}, false, err
		}
	}
	return state, ready, nil
}

func readSingboxRouteTableState(path string) (singboxRouteTableState, bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return singboxRouteTableState{}, false, nil
	}
	if err != nil {
		return singboxRouteTableState{}, false, fmt.Errorf("read route-table ownership state: %w", err)
	}
	var state singboxRouteTableState
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&state); err != nil {
		return singboxRouteTableState{}, true, fmt.Errorf("parse route-table ownership state: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return singboxRouteTableState{}, true, fmt.Errorf("parse route-table ownership state: trailing data")
	}
	state, err = normalizeSingboxRouteTableState(state)
	if err != nil {
		return singboxRouteTableState{}, true, err
	}
	return state, true, nil
}

func writeSingboxRouteTableState(path string, state singboxRouteTableState) error {
	state.Version = singboxRouteTableStateVersion
	state, err := normalizeSingboxRouteTableState(state)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".singbox-route-tables-*.tmp")
	if err != nil {
		return fmt.Errorf("create route-table ownership state: %w", err)
	}
	temporaryPath := file.Name()
	defer os.Remove(temporaryPath)
	if err := file.Chmod(0600); err != nil {
		file.Close()
		return fmt.Errorf("protect route-table ownership state: %w", err)
	}
	if err := json.NewEncoder(file).Encode(state); err != nil {
		file.Close()
		return fmt.Errorf("write route-table ownership state: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return fmt.Errorf("sync route-table ownership state: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close route-table ownership state: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("apply route-table ownership state: %w", err)
	}
	return syncOwnershipStateParentDirectory(path)
}

func removeSingboxRouteTableState(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncOwnershipStateParentDirectory(path)
}

func discardPendingSingboxRouteTableState(path string) error {
	state, present, err := readSingboxRouteTableState(path)
	if err != nil {
		return err
	}
	if !present {
		return nil
	}
	if state.Phase != singboxOwnershipPending {
		return fmt.Errorf("refusing to discard non-pending sing-box network ownership state")
	}
	if err := removeSingboxRouteTableState(path); err != nil {
		return fmt.Errorf("discard pending sing-box network ownership state: %w", err)
	}
	return nil
}

func promotePendingSingboxRouteTableState(path string, pending, ready singboxRouteTableState) error {
	if err := writeSingboxRouteTableState(path, ready); err != nil {
		rollbackErr := writeSingboxRouteTableState(path, pending)
		if rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore pending sing-box network ownership state: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func hasSingboxNFTTable(output string) bool {
	for line := range strings.SplitSeq(output, "\n") {
		if strings.Join(strings.Fields(line), " ") == "table inet sing-box" {
			return true
		}
	}
	return false
}

func parseSingboxNFTTableHandle(output string) (string, bool) {
	matches := nftTableHandlePattern.FindStringSubmatch(output)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}

func ownsSingboxNFTTable(recordedHandle, currentHandle string, tablePresent bool) bool {
	return tablePresent && recordedHandle != "" && recordedHandle == currentHandle
}

func validateRecordedNFTTable(recordedHandle, currentHandle string, tablePresent bool) error {
	if recordedHandle != "" && tablePresent && recordedHandle != currentHandle {
		return fmt.Errorf("managed sing-box nftables table handle was replaced (recorded=%s current=%s)", recordedHandle, currentHandle)
	}
	return nil
}
