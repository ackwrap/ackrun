package service

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const invertedAutoRedirectRules = `9000: from all fwmark 0x2024 goto 9002
9001: from all fwmark 0x2023 lookup 2022
9002: from all nop
32768: not from all fwmark 0x2024 lookup 2022
`

func TestInvertedAutoRedirectOwnershipRoundTrip(t *testing.T) {
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
			pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
				Required: true, ExpectIPv4: family.ipv4, ExpectIPv6: family.ipv6, NFTTableAbsent: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := parseIPRuleSnapshot("1: from all lookup 300\n" + invertedAutoRedirectRules)
			var ipv4, ipv6 ipRuleSnapshot
			if family.ipv4 {
				ipv4 = snapshot
			}
			if family.ipv6 {
				ipv6 = snapshot
			}
			ready, complete, err := derivePendingSingboxOwnership(pending, ipv4, ipv6, "42", true)
			if err != nil || !complete || ready.Phase != singboxOwnershipReady {
				t.Fatalf("inverted ownership = %+v, complete=%t, err=%v", ready, complete, err)
			}
			if !snapshot.hasSingboxAutoRedirectSignature() {
				t.Fatal("inverted fallback did not form a complete signature")
			}
			wantIDs := []string{"fallback_inverted", "input_mark", "output_mark", "redirect_nop"}
			for _, owned := range []struct {
				expected bool
				tables   []string
				rules    []string
			}{
				{family.ipv4, ready.IPv4Tables, ready.IPv4Rules},
				{family.ipv6, ready.IPv6Tables, ready.IPv6Rules},
			} {
				if !owned.expected {
					continue
				}
				if !reflect.DeepEqual(owned.tables, []string{"300"}) || !reflect.DeepEqual(owned.rules, wantIDs) {
					t.Fatalf("owned resources = %v, %v", owned.tables, owned.rules)
				}
				if err := validateManagedIPRuleDeletions(snapshot, owned.rules); err != nil {
					t.Fatalf("recorded inverted rules cannot be cleaned: %v", err)
				}
			}
			path := filepath.Join(t.TempDir(), "ownership.json")
			if err := writeSingboxRouteTableState(path, ready); err != nil {
				t.Fatal(err)
			}
			stored, present, err := readSingboxRouteTableState(path)
			if err != nil || !present || !reflect.DeepEqual(stored, ready) {
				t.Fatalf("persisted ownership = %+v, present=%t, err=%v", stored, present, err)
			}
		})
	}
}

func TestInvertedAutoRedirectOwnershipRejectsConflicts(t *testing.T) {
	for _, test := range []struct {
		name  string
		rules string
	}{
		{"wrong mark", strings.ReplaceAll(invertedAutoRedirectRules, "not from all fwmark 0x2024", "not from all fwmark 0x2025")},
		{"wrong table", strings.ReplaceAll(invertedAutoRedirectRules, "not from all fwmark 0x2024 lookup 2022", "not from all fwmark 0x2024 lookup 200")},
		{"missing inversion", strings.ReplaceAll(invertedAutoRedirectRules, "not from all", "from all")},
		{"unexpected mask", strings.ReplaceAll(invertedAutoRedirectRules, "not from all fwmark 0x2024", "not from all fwmark 0x2024/0xffff")},
		{"shared priority", invertedAutoRedirectRules + "32768: from all lookup 200\n"},
		{"duplicate fallback", invertedAutoRedirectRules + "32768: not from all fwmark 0x2024 lookup 2022\n"},
		{"mixed profile", strings.ReplaceAll(maskedAutoRedirectRules, "not from all fwmark 0x2024/0x2027 lookup 2022", "not from all fwmark 0x2024 lookup 2022")},
	} {
		t.Run(test.name, func(t *testing.T) {
			pending, err := pendingSingboxRouteTableState(platformNetworkBaseline{
				Required: true, ExpectIPv4: true, NFTTableAbsent: true,
			})
			if err != nil {
				t.Fatal(err)
			}
			snapshot := parseIPRuleSnapshot("1: from all lookup 300\n" + test.rules)
			if _, complete, err := derivePendingSingboxOwnership(pending, snapshot, ipRuleSnapshot{}, "42", true); err == nil && complete {
				t.Fatal("ambiguous or changed rule was accepted")
			}
		})
	}
}

func TestInvertedFallbackCleanupRejectsReplacement(t *testing.T) {
	for _, replacement := range []string{
		"from all lookup 2022",
		"from all fwmark 0x2024 lookup 2022",
		"not from all fwmark 0x2025 lookup 2022",
		"not from all fwmark 0x2024/0x2027 lookup 2022",
	} {
		snapshot := parseIPRuleSnapshot("32768: " + replacement)
		if err := validateManagedIPRuleDeletions(snapshot, []string{"fallback_inverted"}); err == nil {
			t.Fatalf("replacement was authorized for deletion: %s", replacement)
		}
	}
}
