//go:build linux

package service

import (
	"strings"
	"testing"
)

func TestFlowtableIsolationRequiresProof(t *testing.T) {
	for _, test := range []struct {
		name, family string
		sources      []string
		blocked      bool
	}{
		{"global-ip", "ip", nil, true},
		{"global-ip6", "ip6", nil, true},
		{"v4-shared", "inet", []string{"192.168.50.2"}, true},
		{"v4-overlap", "ip", []string{"192.168.50.2/32"}, true},
		{"v4-disjoint", "ip6", []string{"192.168.50.0/24"}, false},
		{"v6-disjoint", "ip", []string{"fd12:3456::2"}, false},
		{"v6-overlap", "ip6", []string{"fd12:3456::2"}, true},
		{"mac-covers-v4", "ip", []string{"02:00:00:00:00:02"}, true},
		{"mac-covers-v6", "ip6", []string{"02:00:00:00:00:02"}, true},
		{"mixed-or", "ip6", []string{"192.168.50.2", "02:00:00:00:00:02"}, true},
		{"unknown-family", "", []string{"192.168.50.2"}, true},
		{"invalid-source", "ip6", []string{"bad"}, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// No flow-add rule is deliberately present: historic cached flows can
			// outlive their admission rule, so that is never an exemption.
			data := []byte(`{"nftables":[{"flowtable":{"family":"` + test.family + `","table":"fw4","name":"ft","dev":["lan1","eth1"],"flags":["offload"]}}]}`)
			err := checkNFTFlowtables(data, test.sources)
			if (err != nil) != test.blocked {
				t.Fatalf("blocked=%t error=%v", test.blocked, err)
			}
			if err != nil && test.name != "invalid-source" {
				for _, want := range []string{"fw4", "ft", "lan1", "eth1", "硬件", "已有卸载连接", "重载防火墙"} {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("missing diagnostic %q: %v", want, err)
					}
				}
			}
		})
	}
}

func TestFlowtableMalformedOrMultipleTables(t *testing.T) {
	for _, data := range []string{
		`invalid`, `{}`, `null`, `{"nftables":null}`,
		`{"nftables":[{"flowtable":null}]}`,
		`{"nftables":[{"flowtable":{"family":"ip6"}},{"flowtable":{"family":"ip"}}]}`,
	} {
		if err := checkNFTFlowtables([]byte(data), []string{"192.168.50.2"}); err == nil {
			t.Fatalf("accepted unprovable isolation: %s", data)
		}
	}
	if err := checkNFTFlowtables([]byte(`{"nftables":[]}`), nil); err != nil {
		t.Fatal(err)
	}
	err := checkNFTFlowtables([]byte(`{"nftables":[{"flowtable":{"family":"inet","table":"fw4","name":"ft","dev":["lan1"]}}]}`), nil)
	if err == nil || !strings.Contains(err.Error(), "软件卸载") || !strings.Contains(err.Error(), "全局 TUN") {
		t.Fatalf("missing global/software diagnostic: %v", err)
	}
}
