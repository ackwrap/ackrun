package service

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestDeviceIdentityNormalization(t *testing.T) {
	values, err := normalizeDeviceSources([]string{"02-AA-BB-CC-DD-EE", "192.0.2.10", "2001:db8::10", "02:aa:bb:cc:dd:ee"})
	if err != nil || !reflect.DeepEqual(values, []string{"02:aa:bb:cc:dd:ee", "192.0.2.10/32", "2001:db8::10/128"}) {
		t.Fatalf("identity alternatives: %v, %v", values, err)
	}
	for _, value := range []string{"01:00:00:00:00:01", "00:00:00:00:00:00", "02:00:00:00:00:00:00:01", "02:00:00:00:00:01;accept", "fe80::1"} {
		if _, err := normalizeDeviceSources([]string{value}); err == nil {
			t.Fatalf("accepted invalid identity %q", value)
		}
	}
}

// Evaluate the generated identity tree independently, including its AND/OR
// grouping. An empty identity must not turn into a catch-all whitelist.
func TestDeviceIdentityORAndWhitelist(t *testing.T) {
	var match func(map[string]interface{}, bool, bool, bool, bool) bool
	match = func(rule map[string]interface{}, ip, mac, ingress, domain bool) bool {
		result := true
		if rule["type"] == "logical" {
			result = rule["mode"] != "or"
			for _, child := range rule["rules"].([]map[string]interface{}) {
				hit := match(child, ip, mac, ingress, domain)
				if rule["mode"] == "or" {
					result = result || hit
				} else {
					result = result && hit
				}
			}
		} else {
			if _, ok := rule["source_ip_cidr"]; ok {
				result = result && ip
			}
			if _, ok := rule["source_mac_address"]; ok {
				result = result && mac
			}
			if _, ok := rule["inbound"]; ok {
				result = result && ingress
			}
			if _, ok := rule["domain_suffix"]; ok {
				result = result && domain
			}
		}
		return result
	}
	for _, sources := range [][]string{{"192.0.2.1/32"}, {"02:00:00:00:00:01"}, {"192.0.2.1/32", "02:00:00:00:00:01"}} {
		cidrs, macs := splitDeviceSources(sources)
		rule := deviceSourceRule(sources, map[string]interface{}{"domain_suffix": []string{"example.com"}, "action": "route", "outbound": "direct"})
		for mask := 0; mask < 16; mask++ {
			ip, mac, ingress, domain := mask&1 != 0, mask&2 != 0, mask&4 != 0, mask&8 != 0
			want := (len(cidrs) > 0 && ip || len(macs) > 0 && mac && ingress) && domain
			if got := match(rule, ip, mac, ingress, domain); got != want {
				t.Fatalf("sources %v combination %d: %t != %t", sources, mask, got, want)
			}
		}
	}
}

func TestDeviceMACGeneratedConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("MAC device protection requires Linux TUN/TPROXY")
	}
	db, g, devices := devicePolicyFixture(t)
	settings, err := db.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.DNSMasqTakeoverEnabled = true
	if err := db.SetGeneralSettings(settings); err != nil {
		t.Fatal(err)
	}
	for i, values := range [][]string{{"02:00:00:00:00:01"}, {"192.168.50.20/32", "02:00:00:00:00:02"}} {
		r := devices[i]
		if _, err := db.UpdateRouteRule(r.ID, &model.RouteRuleRequest{Name: r.Name, RuleType: r.RuleType, Values: values, Enabled: true, Outbound: "proxy", DirectDomains: []string{"example.com"}}); err != nil {
			t.Fatal(err)
		}
	}
	if err := g.validateDevicePolicies(); err != nil {
		t.Fatal(err)
	}
	inbounds, err := g.generateInbounds("127.0.0.1", 1080, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	route, err := g.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	rules := route["rules"].([]map[string]interface{})
	first, _ := json.Marshal(rules[1])
	if rules[1]["action"] != "reject" || !strings.Contains(string(first), deviceMACInboundTag) {
		t.Fatal("unknown MAC can reach ordinary rules")
	}
	dns, err := g.generateDNSFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	dnsRules := dns["rules"].([]map[string]interface{})
	if !reflect.DeepEqual(dnsRules[4]["inbound"], deviceMACInbounds()) || dnsRules[4]["action"] != "reject" {
		t.Fatal("unknown MAC DNS falls through")
	}
	outbounds, endpoints, err := g.generateOutbounds(true)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]interface{}{"inbounds": inbounds, "route": route, "dns": dns, "outbounds": outbounds, "endpoints": endpoints, "http_clients": []map[string]interface{}{{"tag": defaultRuleSetHTTPClientTag}}}
	data, _ := json.Marshal(config)
	path := filepath.Join(t.TempDir(), "synthetic.json")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	state, err := readActiveTUNState(path)
	if !state.DNSMasqTakeover {
		t.Fatal("MAC guard disabled dnsmasq takeover detection")
	}
	if err != nil || !reflect.DeepEqual(state.DeviceDNSCIDRs, []string{"192.168.50.20/32"}) {
		t.Fatalf("DNS source readback: %+v, %v", state, err)
	}
	if binary := os.Getenv("ACKWRAP_SINGBOX_114_BIN"); binary != "" {
		if output, err := exec.Command(binary, "check", "-c", path).CombinedOutput(); err != nil {
			t.Fatalf("real core check: %v: %s", err, output)
		}
	}
}
