package service

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestParseIPRuleSnapshotRecognizesSingboxAutoRedirect(t *testing.T) {
	snapshot := parseIPRuleSnapshot(`0: from all lookup local
1: from all lookup 3324141050
9000: from all fwmark 0x2024 goto 9002
9001: from all fwmark 0x2023 lookup 2022
9002: from all nop
32766: from all lookup main
32767: from all lookup default
32768: from all lookup 2022
`)

	if !snapshot.hasSingboxAutoRedirectSignature() {
		t.Fatal("expected sing-box auto-redirect rule signature")
	}
	if tables := snapshot.priorityOneLookupTables(); !reflect.DeepEqual(tables, []string{"3324141050"}) {
		t.Fatalf("redirect route tables = %v", tables)
	}
}

func TestParseIPRuleSnapshotRejectsPartialSignature(t *testing.T) {
	snapshot := parseIPRuleSnapshot(`9000: from all fwmark 0x2024 goto 9002
9001: from all fwmark 0x2023 lookup 2022
`)

	if snapshot.hasSingboxAutoRedirectSignature() {
		t.Fatal("partial rule set must not identify sing-box ownership")
	}
}

func TestPriorityOneLookupTablesOnlyReturnsNumericLookups(t *testing.T) {
	snapshot := parseIPRuleSnapshot(`1: from all lookup local
1: from all lookup 4294967295
1: from all lookup 100
1: from 192.0.2.1 lookup 300
1: from all lookup 2022
2: from all lookup 200
`)

	if tables := snapshot.priorityOneLookupTables(); !reflect.DeepEqual(tables, []string{"100", "300", "4294967295"}) {
		t.Fatalf("redirect route tables = %v", tables)
	}
	if tables := snapshot.exactPriorityOneLookupTables(); !reflect.DeepEqual(tables, []string{"100", "4294967295"}) {
		t.Fatalf("exact sing-tun-shaped route tables = %v", tables)
	}
}

func TestRuleSignaturesDoNotEstablishCleanupOwnership(t *testing.T) {
	complete := parseIPRuleSnapshot(`9000: from all fwmark 0x2024 goto 9002
9001: from all fwmark 0x2023 lookup 2022
9002: from all nop
32768: from all lookup 2022`)
	if !complete.hasSingboxAutoRedirectSignature() {
		t.Fatal("test setup must contain the complete managed rule signature")
	}
	if _, ready, err := derivePendingSingboxOwnership(singboxRouteTableState{}, complete, ipRuleSnapshot{}, "", false); err == nil || ready {
		t.Fatal("rule signatures without valid pending ownership state must fail closed")
	}
}

func TestAddedManagedIPRuleIDsProtectsBaselineRules(t *testing.T) {
	current := []string{"output_mark", "input_mark", "redirect_nop", "fallback"}
	baseline := []string{"input_mark", "fallback"}
	want := []string{"output_mark", "redirect_nop"}
	if got := addedManagedIPRuleIDs(current, baseline); !reflect.DeepEqual(got, want) {
		t.Fatalf("owned managed rules = %v, want %v", got, want)
	}
}

func TestManagedIPRuleIDsRequireExactRuleLines(t *testing.T) {
	snapshot := parseIPRuleSnapshot(`9000: from all fwmark 0x2024/0xffffffff goto 9002
9001: from all fwmark 0x2023 lookup 2022
9002: from all nop
32768: from all lookup 2022`)
	want := []string{"input_mark", "redirect_nop", "fallback"}
	if got := snapshot.managedRuleIDs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("managed rule IDs = %v, want %v", got, want)
	}
}

func TestSingboxTUNPriorityConflictsCoverCoreCleanupRange(t *testing.T) {
	snapshot := parseIPRuleSnapshot(`8999: from all lookup 99
9000: from all lookup 100
9005: from all lookup 105
9010: from all lookup 110
9011: from all lookup 111
32768: from all lookup 200`)
	want := []int{9000, 9005, 9010, 32768}
	if got := snapshot.singboxTUNPriorityConflicts(); !reflect.DeepEqual(got, want) {
		t.Fatalf("sing-box TUN priority conflicts = %v, want %v", got, want)
	}
}

func TestValidateManagedIPRuleDeletionsRejectsSharedPriority(t *testing.T) {
	shared := parseIPRuleSnapshot(`9002: from all lookup 100
9002: from all nop`)
	if err := validateManagedIPRuleDeletions(shared, []string{"redirect_nop"}); err == nil {
		t.Fatal("shared managed rule priority must fail closed")
	}
	onlyManaged := parseIPRuleSnapshot("9002: from all nop")
	if err := validateManagedIPRuleDeletions(onlyManaged, []string{"redirect_nop"}); err != nil {
		t.Fatalf("exclusive managed rule rejected: %v", err)
	}
	replaced := parseIPRuleSnapshot("9002: from all lookup 100")
	if err := validateManagedIPRuleDeletions(replaced, []string{"redirect_nop"}); err == nil {
		t.Fatal("a replaced managed rule must fail closed")
	}
}

func TestValidatePriorityOneRuleDeletionsRejectsChangedSelector(t *testing.T) {
	snapshot := parseIPRuleSnapshot("1: from 192.0.2.1 lookup 100")
	if err := validatePriorityOneRuleDeletions(snapshot, []string{"100"}); err == nil {
		t.Fatal("a changed priority-1 selector must fail closed")
	}
	if err := validatePriorityOneRuleDeletions(parseIPRuleSnapshot("1: from all lookup 100"), []string{"100"}); err != nil {
		t.Fatalf("exact priority-1 rule rejected: %v", err)
	}
}

func TestManagedNopRuleUsesExactDeleteAction(t *testing.T) {
	spec, ok := managedIPRuleSpecByID("redirect_nop")
	if !ok || !reflect.DeepEqual(spec.delete, []string{"rule", "del", "priority", "9002", "from", "all", "type", "nop"}) {
		t.Fatalf("managed nop delete arguments = %v", spec.delete)
	}
}

func TestSharedPriorityOneLookupTablesProtectsExternalTables(t *testing.T) {
	ipv4 := parseIPRuleSnapshot("1: from all lookup 100\n1: from all lookup 3324141050")
	ipv6 := parseIPRuleSnapshot("1: from all lookup 3324141050\n1: from all lookup 200")
	if tables := sharedPriorityOneLookupTables(ipv4, ipv6); !reflect.DeepEqual(tables, []string{"3324141050"}) {
		t.Fatalf("shared priority-one tables = %v", tables)
	}
}

func TestPriorityOneLookupTableMatchIsExact(t *testing.T) {
	snapshot := parseIPRuleSnapshot("1: from all lookup 1000\n")
	if snapshot.hasPriorityOneLookupTable("100") {
		t.Fatal("route table 100 must not match table 1000")
	}
	if !snapshot.hasPriorityOneLookupTable("1000") {
		t.Fatal("expected exact priority-one route table match")
	}
}

func TestSingboxRedirectRouteTableRequiresOnlyLoopbackLocalRoutes(t *testing.T) {
	if !isSingboxRedirectRouteTable("local 127.0.0.1 dev br-lan scope host\nlocal 127.0.0.1 dev eth0 scope host\n", false) {
		t.Fatal("expected sing-tun IPv4 redirect route table")
	}
	if !isSingboxRedirectRouteTable("local ::1 dev br-lan metric 1024 pref medium\n", true) {
		t.Fatal("expected sing-tun IPv6 redirect route table")
	}
	if isSingboxRedirectRouteTable("default via 192.0.2.1 dev eth0\n", false) {
		t.Fatal("external route table must not match sing-tun ownership")
	}
	if !isSafeOwnedRedirectRouteTable("", false) {
		t.Fatal("an already emptied owned route table must be safe to finish cleaning")
	}
	if isSafeOwnedRedirectRouteTable("local 127.0.0.1 dev eth0 scope host\ndefault via 192.0.2.1 dev eth0\n", false) {
		t.Fatal("an owned table reused for external routes must fail closed")
	}
}

func TestParseRouteTableSnapshotFindsOnlyNumericTables(t *testing.T) {
	snapshot := parseRouteTableSnapshot(`local 127.0.0.1 dev lo table 2751636479 proto kernel scope host src 127.0.0.1
default via 192.0.2.1 dev eth0 table main
local ::1 dev lo table 2751636479 proto kernel metric 0 pref medium
local 192.0.2.2 dev eth0 table 100 proto kernel scope host
`)

	if !snapshot.has("2751636479") || !snapshot.has("100") {
		t.Fatalf("numeric route tables were not detected: %v", snapshot)
	}
	if snapshot.has("main") || snapshot.has("718677876") {
		t.Fatalf("unexpected route table detected: %v", snapshot)
	}
}

func TestPlanRouteTableCleanupSkipsMissingRecordedTable(t *testing.T) {
	rules := parseIPRuleSnapshot("1: from all lookup 718677876")
	actions := planRouteTableCleanup(rules, routeTableSnapshot{}, []string{"718677876"})
	want := []routeTableCleanupAction{{table: "718677876", deleteRule: true, flushTable: false}}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("cleanup actions = %#v, want %#v", actions, want)
	}
}

func TestPlanRouteTableCleanupFlushesOnlyRecordedExistingTables(t *testing.T) {
	rules := parseIPRuleSnapshot("1: from all lookup 100")
	existing := routeTableSnapshot{"100": {}, "200": {}}
	actions := planRouteTableCleanup(rules, existing, []string{"100", "300"})
	want := []routeTableCleanupAction{
		{table: "100", deleteRule: true, flushTable: true},
		{table: "300", deleteRule: false, flushTable: false},
	}
	if !reflect.DeepEqual(actions, want) {
		t.Fatalf("cleanup actions = %#v, want %#v", actions, want)
	}
}

func TestHasSingboxNFTTableMatchesExactTable(t *testing.T) {
	if !hasSingboxNFTTable("table inet fw4\ntable   inet   sing-box\n") {
		t.Fatal("expected exact sing-box nftables table")
	}
	if hasSingboxNFTTable("table inet fw4\ntable inet sing-box-backup\n") {
		t.Fatal("must not match similarly named nftables table")
	}
}

func TestParseSingboxNFTTableHandle(t *testing.T) {
	handle, ok := parseSingboxNFTTableHandle("table inet sing-box { # handle 73\n}")
	if !ok || handle != "73" {
		t.Fatalf("nftables handle = %q, ok=%t", handle, ok)
	}
	if _, ok := parseSingboxNFTTableHandle("table inet sing-box-backup { # handle 73\n}"); ok {
		t.Fatal("must not read a similarly named nftables table handle")
	}
}

func TestSingboxNFTTableOwnershipRequiresMatchingRecordedHandle(t *testing.T) {
	if !ownsSingboxNFTTable("73", "73", true) {
		t.Fatal("matching persisted nftables handle must establish ownership")
	}
	for _, test := range []struct {
		name           string
		recordedHandle string
		currentHandle  string
		tablePresent   bool
	}{
		{name: "missing persisted handle", currentHandle: "73", tablePresent: true},
		{name: "different current table", recordedHandle: "72", currentHandle: "73", tablePresent: true},
		{name: "table absent", recordedHandle: "73", currentHandle: "73"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if ownsSingboxNFTTable(test.recordedHandle, test.currentHandle, test.tablePresent) {
				t.Fatal("unverified nftables table must not establish ownership")
			}
		})
	}
}

func TestValidateRecordedNFTTableRejectsReplacement(t *testing.T) {
	if err := validateRecordedNFTTable("72", "73", true); err == nil {
		t.Fatal("replaced nftables handle must fail closed")
	}
	if err := validateRecordedNFTTable("72", "", false); err != nil {
		t.Fatalf("an already absent recorded nftables table should be safe: %v", err)
	}
}

func TestSingboxPendingRouteTableStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
	pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
		Required: true, ExpectIPv4: true, ExpectIPv6: true, NFTTableAbsent: true,
		IPv4Tables: []string{"3324141050", "100", "100"}, IPv6Tables: []string{"200"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSingboxRouteTableState(path, pending); err != nil {
		t.Fatal(err)
	}
	state, present, err := readSingboxRouteTableState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !present || state.Version != singboxRouteTableStateVersion || state.Phase != singboxOwnershipPending ||
		!reflect.DeepEqual(state.BaselineIPv4Tables, []string{"100", "3324141050"}) || !reflect.DeepEqual(state.BaselineIPv6Tables, []string{"200"}) {
		t.Fatalf("route-table state = %+v, present=%t", state, present)
	}
	if info, err := os.Stat(path); err != nil {
		t.Fatal(err)
	} else if mode := info.Mode().Perm(); runtime.GOOS != "windows" && mode != 0600 {
		t.Fatalf("route-table state mode = %o, want 600", mode)
	}
}

func TestDerivePendingSingboxOwnershipUsesPerFamilyBaselines(t *testing.T) {
	pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
		Required: true, ExpectIPv4: true, ExpectIPv6: true, NFTTableAbsent: true,
		IPv4Tables: []string{"100"}, IPv6Tables: []string{"200"},
	})
	if err != nil {
		t.Fatal(err)
	}
	managed := "9000: from all fwmark 0x2024 goto 9002\n9001: from all fwmark 0x2023 lookup 2022\n9002: from all nop\n32768: from all lookup 2022\n"
	ipv4 := parseIPRuleSnapshot("1: from all lookup 100\n1: from all lookup 300\n" + managed)
	ipv6 := parseIPRuleSnapshot("1: from all lookup 200\n1: from all lookup 300\n" + managed)
	ready, complete, err := derivePendingSingboxOwnership(pending, ipv4, ipv6, "42", true)
	if err != nil {
		t.Fatal(err)
	}
	if !complete || ready.Phase != singboxOwnershipReady || !reflect.DeepEqual(ready.IPv4Tables, []string{"300"}) ||
		!reflect.DeepEqual(ready.IPv6Tables, []string{"300"}) || ready.NFTTableHandle != "42" {
		t.Fatalf("derived ownership = %+v, complete=%t", ready, complete)
	}
}

func TestPendingOwnershipNormalizationRejectsReadyResources(t *testing.T) {
	_, err := normalizeSingboxRouteTableState(singboxRouteTableState{
		Version: singboxRouteTableStateVersion, Phase: singboxOwnershipPending,
		ExpectIPv4: true, NFTTableAbsent: true, IPv4Tables: []string{"100"},
	})
	if err == nil {
		t.Fatal("pending ownership with ready resources must be rejected")
	}
}

func TestReadyOwnershipNormalizationRejectsBaselineOverlap(t *testing.T) {
	_, err := normalizeSingboxRouteTableState(singboxRouteTableState{
		Version: singboxRouteTableStateVersion, Phase: singboxOwnershipReady,
		ExpectIPv4: true, NFTTableAbsent: true, NFTTableHandle: "42",
		BaselineIPv4Tables: []string{"100"}, IPv4Tables: []string{"100"},
		IPv4Rules: []string{"output_mark", "input_mark", "redirect_nop", "fallback"},
	})
	if err == nil {
		t.Fatal("ready ownership overlapping its baseline must be rejected")
	}
}

func TestDiscardPendingOwnershipDoesNotRemoveReadyState(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
	pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
		Required: true, ExpectIPv4: true, NFTTableAbsent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := writeSingboxRouteTableState(path, pending); err != nil {
		t.Fatal(err)
	}
	if err := discardPendingSingboxRouteTableState(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("pending state still exists: %v", err)
	}

	ready := pending
	ready.Phase = singboxOwnershipReady
	ready.NFTTableHandle = "42"
	ready.IPv4Tables = []string{"100"}
	ready.IPv4Rules = []string{"output_mark", "input_mark", "redirect_nop", "fallback"}
	if err := writeSingboxRouteTableState(path, ready); err != nil {
		t.Fatal(err)
	}
	if err := discardPendingSingboxRouteTableState(path); err == nil {
		t.Fatal("ready state must not be discarded as pending")
	}
	if _, present, err := readSingboxRouteTableState(path); err != nil || !present {
		t.Fatalf("ready state was lost: present=%t err=%v", present, err)
	}
}

func TestReadSingboxRouteTableStateRejectsInvalidID(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"phase":"pending","expect_ipv4":true,"nft_table_absent":true,"baseline_ipv4_tables":["2022"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := readSingboxRouteTableState(path); err == nil || !present {
		t.Fatalf("invalid route-table state error = %v, present=%t", err, present)
	}
}

func TestReadSingboxRouteTableStateRejectsInvalidNFTHandle(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
	if err := os.WriteFile(path, []byte(`{"version":2,"phase":"ready","expect_ipv4":true,"nft_table_absent":true,"ipv4_tables":["100"],"ipv4_rules":["output_mark","input_mark","redirect_nop","fallback"],"nft_table_handle":"invalid"}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := readSingboxRouteTableState(path); err == nil || !present {
		t.Fatalf("invalid nftables handle error = %v, present=%t", err, present)
	}
}

func TestReadSingboxRouteTableStateRejectsMissingVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
	if err := os.WriteFile(path, []byte(`{"tables":["100"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, present, err := readSingboxRouteTableState(path); err == nil || !present {
		t.Fatalf("unversioned ownership state error = %v, present=%t", err, present)
	}
}

func TestReadSingboxRouteTableStateRejectsUnsupportedOrUnknownFormat(t *testing.T) {
	for _, data := range []string{
		`{"version":3,"phase":"pending","expect_ipv4":true,"nft_table_absent":true}`,
		`{"version":2,"phase":"pending","expect_ipv4":true,"nft_table_absent":true,"unknown":true}`,
	} {
		path := filepath.Join(t.TempDir(), ".singbox-route-tables.json")
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if _, present, err := readSingboxRouteTableState(path); err == nil || !present {
			t.Fatalf("unsupported ownership state error = %v, present=%t", err, present)
		}
	}
}
