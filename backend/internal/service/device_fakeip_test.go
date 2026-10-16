package service

import (
	"encoding/json"
	"net/netip"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func cidrsContainAddress(cidrs []string, address string) bool {
	ip := netip.MustParseAddr(address)
	for _, value := range cidrs {
		if netip.MustParsePrefix(value).Contains(ip) {
			return true
		}
	}
	return false
}

func TestExcludeDeviceFakeIPPreservesRemainingAddresses(t *testing.T) {
	allowed, err := excludeDeviceFakeIPCIDRs([]string{"192.168.1.0/24", "fd00::/120"}, []string{"192.168.1.64/26", "fd00::40/122"})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 256; n++ {
		v4 := netip.AddrFrom4([4]byte{192, 168, 1, byte(n)}).String()
		v6Bytes := [16]byte{0xfd}
		v6Bytes[15] = byte(n)
		v6 := netip.AddrFrom16(v6Bytes).String()
		want := n < 64 || n >= 128
		if cidrsContainAddress(allowed, v4) != want || cidrsContainAddress(allowed, v6) != want {
			t.Fatalf("incorrect address subtraction at offset %d", n)
		}
	}
	all, err := excludeDeviceFakeIPCIDRs(deviceLocalDestinationCIDRs(), []string{"0.0.0.0/0", "::/0"})
	if err != nil || len(all) != 0 {
		t.Fatalf("full-pool subtraction: %v %v", all, err)
	}
}

func TestDeviceFakeIPPoolsNeverBecomeLANOrTUNExclusions(t *testing.T) {
	for _, test := range []struct{ name, v4, v6, fake4, fake6 string }{
		{"defaults", "198.18.0.1/16", "fc00::/18", "198.18.0.2", "fc00::2"},
		{"private", "10.66.0.1/24", "fd66::/64", "10.66.0.2", "fd66::2"},
		{"link-local", "169.254.66.1/24", "fe80:66::/64", "169.254.66.2", "fe80:66::2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			db, g, devices := devicePolicyFixture(t)
			settings, err := db.GetDNSGlobalSettings()
			if err != nil {
				t.Fatal(err)
			}
			settings.FakeIPInet4Range, settings.FakeIPInet6Range = test.v4, test.v6
			if err := db.SetDNSGlobalSettings(settings); err != nil {
				t.Fatal(err)
			}
			var exclusions []model.TrafficBypassRule
			for _, value := range []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "fc00::/7"} {
				exclusions = append(exclusions, model.TrafficBypassRule{Type: "ip_cidr", Value: value})
			}
			if err := db.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: exclusions}); err != nil {
				t.Fatal(err)
			}
			route, err := g.generateRoute("direct")
			if err != nil {
				t.Fatal(err)
			}
			localFound, rejectFound := false, false
			for _, rule := range route["rules"].([]map[string]interface{}) {
				if rule["action"] == "route" && rule["outbound"] == devices[0].Name {
					if !localFound || !rejectFound {
						t.Fatal("fixed outbound precedes LAN handling or unresolved FakeIP rejection")
					}
				}
				cidrs, ok := rule["ip_cidr"].([]string)
				if !ok {
					continue
				}
				if rule["action"] == "bypass" {
					for _, address := range []string{test.fake4, test.fake6, "fc00::2", "198.18.0.2"} {
						if cidrsContainAddress(cidrs, address) {
							t.Fatalf("FakeIP %s bypasses device routing", address)
						}
					}
					if reflect.DeepEqual(rule["source_ip_cidr"], devices[0].Values) {
						localFound = cidrsContainAddress(cidrs, "192.168.1.1") && cidrsContainAddress(cidrs, "fd12:3456::1")
					}
				}
				if rule["action"] == "reject" && reflect.DeepEqual(rule["source_ip_cidr"], devices[0].Values) {
					rejectFound = cidrsContainAddress(cidrs, test.fake4) && cidrsContainAddress(cidrs, test.fake6)
				}
			}
			inbounds, err := g.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
			if err != nil {
				t.Fatal(err)
			}
			for _, inbound := range inbounds {
				item := inbound.(map[string]interface{})
				if item["type"] == "tun" {
					cidrs := item["route_exclude_address"].([]string)
					if cidrsContainAddress(cidrs, test.fake4) || cidrsContainAddress(cidrs, test.fake6) {
						t.Fatal("FakeIP escaped the TUN through a destination exclusion")
					}
					if !cidrsContainAddress(cidrs, "192.168.1.1") {
						t.Fatal("unrelated LAN exclusion was removed")
					}
				}
			}
		})
	}
}

func TestDeviceFakeIPIncludesRunningConfigDuringRangeChange(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	g.paths.ConfigDir = t.TempDir()
	g.paths.ConfigPath = filepath.Join(g.paths.ConfigDir, "active.json")
	data, _ := json.Marshal(map[string]any{"dns": map[string]any{"servers": []any{
		map[string]any{"type": "fakeip", "inet4_range": "10.66.0.1/24", "inet6_range": "fd66::/64"},
	}}})
	if err := os.WriteFile(g.paths.ConfigPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	settings, _ := db.GetDNSGlobalSettings()
	settings.FakeIPInet4Range, settings.FakeIPInet6Range = "10.77.0.1/24", "fd77::/64"
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	core := NewSingboxService(g.paths, nil, nil, db)
	runningPools := []string{"10.88.0.0/24", "fd88::/64"}
	core.runningFakeIPCIDRs.Store(&runningPools)
	g.singbox = core
	fakeIPs, err := g.deviceFakeIPCIDRs()
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"198.18.0.2", "fc00::2", "10.66.0.2", "fd66::2", "10.77.0.2", "fd77::2", "10.88.0.2", "fd88::2"} {
		if !cidrsContainAddress(fakeIPs, address) {
			t.Fatalf("lost current or previous pool for %s", address)
		}
	}
}
