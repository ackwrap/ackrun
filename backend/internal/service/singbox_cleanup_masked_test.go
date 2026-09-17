package service

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const maskedAutoRedirectRules = `9000: from all fwmark 0x2024/0x2027 goto 9002
9001: from all fwmark 0x2023/0x2027 lookup 2022
9002: from all nop
32768: not from all fwmark 0x2024/0x2027 lookup 2022
`

func TestMaskedAutoRedirectOwnershipRoundTrip(t *testing.T) {
	for _, family := range []struct {
		name string
		ipv4 bool
		ipv6 bool
	}{
		{name: "IPv4", ipv4: true},
		{name: "IPv6", ipv6: true},
		{name: "dual stack", ipv4: true, ipv6: true},
	} {
		t.Run(family.name, func(t *testing.T) {
			baseline := platformNetworkBaseline{
				Required: true, ExpectIPv4: family.ipv4, ExpectIPv6: family.ipv6, NFTTableAbsent: true,
			}
			var ipv4, ipv6 ipRuleSnapshot
			if family.ipv4 {
				baseline.IPv4Tables = []string{"100"}
				ipv4 = parseIPRuleSnapshot("1: from all lookup 100\n1: from all lookup 300\n" + maskedAutoRedirectRules)
			}
			if family.ipv6 {
				baseline.IPv6Tables = []string{"200"}
				ipv6 = parseIPRuleSnapshot("1: from all lookup 200\n1: from all lookup 300\n" + maskedAutoRedirectRules)
			}
			pending, err := pendingSingboxRouteTableState(baseline)
			if err != nil {
				t.Fatal(err)
			}
			ready, complete, err := derivePendingSingboxOwnership(pending, ipv4, ipv6, "42", true)
			if err != nil || !complete || ready.Phase != singboxOwnershipReady {
				t.Fatalf("masked ownership = %+v, complete=%t, err=%v", ready, complete, err)
			}
			wantIDs := []string{"fallback_masked", "input_mark_masked", "output_mark_masked", "redirect_nop"}
			for _, current := range []struct {
				expected bool
				snapshot ipRuleSnapshot
				tables   []string
				rules    []string
			}{
				{family.ipv4, ipv4, ready.IPv4Tables, ready.IPv4Rules},
				{family.ipv6, ipv6, ready.IPv6Tables, ready.IPv6Rules},
			} {
				if !current.expected {
					continue
				}
				if !reflect.DeepEqual(current.tables, []string{"300"}) || !reflect.DeepEqual(current.rules, wantIDs) {
					t.Fatalf("owned resources = %v, %v", current.tables, current.rules)
				}
				if !current.snapshot.hasSingboxAutoRedirectSignature() {
					t.Fatal("masked rules must form a complete auto-redirect signature")
				}
				if err := validateManagedIPRuleDeletions(current.snapshot, current.rules); err != nil {
					t.Fatalf("recorded masked rules cannot be cleaned: %v", err)
				}
			}
			path := filepath.Join(t.TempDir(), "ownership.json")
			if err := writeSingboxRouteTableState(path, ready); err != nil {
				t.Fatal(err)
			}
			restored, present, err := readSingboxRouteTableState(path)
			if err != nil || !present || !reflect.DeepEqual(restored, ready) {
				t.Fatalf("persisted masked ownership = %+v, present=%t, err=%v", restored, present, err)
			}
		})
	}
}

func TestMaskedAutoRedirectOwnershipRejectsConflicts(t *testing.T) {
	for _, test := range []struct {
		name     string
		rules    string
		baseline []string
	}{
		{name: "unknown mask", rules: strings.ReplaceAll(maskedAutoRedirectRules, "0x2027", "0xffff")},
		{name: "foreign shared priority", rules: maskedAutoRedirectRules + "9000: from all lookup 100\n"},
		{name: "duplicate rule", rules: maskedAutoRedirectRules + "9001: from all fwmark 0x2023/0x2027 lookup 2022\n"},
		{name: "replaced baseline format", rules: maskedAutoRedirectRules, baseline: []string{"output_mark"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
				Required: true, ExpectIPv4: true, NFTTableAbsent: true, IPv4Rules: test.baseline,
			})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := parseIPRuleSnapshot("1: from all lookup 300\n" + test.rules)
			if _, ready, err := derivePendingSingboxOwnership(pending, snapshot, ipRuleSnapshot{}, "42", true); err == nil || ready {
				t.Fatalf("conflicting ownership accepted: ready=%t err=%v", ready, err)
			}
		})
	}
}

func TestAutoRedirectOwnershipRequiresConsistentRuleFormat(t *testing.T) {
	pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
		Required: true, ExpectIPv4: true, NFTTableAbsent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	mixed := strings.ReplaceAll(maskedAutoRedirectRules, "not from all fwmark 0x2024/0x2027 lookup 2022", "from all lookup 2022")
	snapshot := parseIPRuleSnapshot("1: from all lookup 300\n" + mixed)
	state, ready, err := derivePendingSingboxOwnership(pending, snapshot, ipRuleSnapshot{}, "42", true)
	if err != nil || ready || snapshot.hasSingboxAutoRedirectSignature() {
		t.Fatalf("mixed formats must stay pending: ready=%t err=%v", ready, err)
	}
	state.Phase = singboxOwnershipReady
	if _, err := normalizeSingboxRouteTableState(state); err == nil {
		t.Fatal("mixed formats must not be accepted as persisted ready ownership")
	}
}

func TestMaskedRuleCleanupRejectsFormatReplacement(t *testing.T) {
	for _, test := range []struct {
		id   string
		line string
	}{
		{"output_mark", "9000: from all fwmark 0x2024/0x2027 goto 9002"},
		{"output_mark_masked", "9000: from all fwmark 0x2024 goto 9002"},
		{"input_mark_masked", "9001: from all fwmark 0x2023/0xffff lookup 2022"},
		{"fallback_masked", "32768: from all fwmark 0x2024/0x2027 lookup 2022"},
	} {
		t.Run(test.id, func(t *testing.T) {
			if err := validateManagedIPRuleDeletions(parseIPRuleSnapshot(test.line), []string{test.id}); err == nil {
				t.Fatal("changed mask or selector must fail cleanup ownership validation")
			}
		})
	}
}
