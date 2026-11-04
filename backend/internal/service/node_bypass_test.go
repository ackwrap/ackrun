package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func nodeBypassFixture(t *testing.T) (*store.Store, *ConfigGeneratorService) {
	t.Helper()
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.SetProxyMode("direct"); err != nil {
		t.Fatal(err)
	}
	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{Tag: "bootstrap", ServerType: "udp", Address: "192.0.2.53", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	settings, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Enabled, settings.Final = true, "bootstrap"
	settings.FakeIPInet4Range = "10.66.0.0/24"
	if err := db.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	return db, NewConfigGeneratorService(db, &paths.Paths{DataDir: dir})
}

func addNodeBypassNodes(t *testing.T, db *store.Store, servers ...string) []model.Node {
	t.Helper()
	sub, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic bypass", URL: "https://example.invalid/sub"})
	if err != nil {
		t.Fatal(err)
	}
	var parsed []model.ParsedNode
	for index, server := range servers {
		parsed = append(parsed, model.ParsedNode{
			Name: fmt.Sprintf("synthetic-%d", index), Type: "socks", Server: server, ServerPort: 1080 + index,
			RawJSON: fmt.Sprintf(`{"type":"socks","server":%q,"server_port":%d}`, server, 1080+index),
		})
	}
	if err := db.ReplaceSubscriptionNodes(sub.ID, parsed); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListNodesBySubscription(sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	return nodes
}

func TestResolveNodeServerBypassSnapshot(t *testing.T) {
	db, g := nodeBypassFixture(t)
	nodes := addNodeBypassNodes(t, db, "Node.Example.", "node.example", "192.0.2.25", "2001:db8::25", "hosts.example", "failed.example", "disabled.example")
	for _, node := range nodes {
		if node.Server == "disabled.example" {
			if err := db.SetNodeEnabled(node.UID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := db.CreateDNSHost(&model.DNSHostRequest{Domain: "hosts.example", Addresses: []string{"192.0.2.30", "2001:db8::30"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	calls := map[string]int{}
	g.nodeBypassLookup = func(_ context.Context, domain string) ([]net.IP, error) {
		mu.Lock()
		calls[domain]++
		mu.Unlock()
		if domain == "failed.example" {
			return nil, errors.New("synthetic DNS failure")
		}
		return []net.IP{
			net.ParseIP("192.0.2.25"), net.ParseIP("::ffff:192.0.2.25"), net.ParseIP("2001:db8::25"),
			net.ParseIP("10.66.0.10"), net.ParseIP("198.18.0.10"), net.ParseIP("fc00::10"),
			net.ParseIP("127.0.0.1"), net.ParseIP("ff02::1"), nil,
		}, nil
	}
	got, err := g.resolveNodeServerBypass(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"192.0.2.25/32", "192.0.2.30/32", "2001:db8::25/128", "2001:db8::30/128"}
	if !reflect.DeepEqual(got.ipCIDRs, want) {
		t.Fatalf("real-IP exclusions = %v, want %v", got.ipCIDRs, want)
	}
	if !reflect.DeepEqual(calls, map[string]int{"node.example": 1, "failed.example": 1}) {
		t.Fatalf("lookups did not deduplicate, honor Hosts or skip disabled nodes: %v", calls)
	}
}

func TestNodeServerBypassResolutionIsBounded(t *testing.T) {
	db, g := nodeBypassFixture(t)
	var names []string
	for index := range 24 {
		names = append(names, fmt.Sprintf("node-%d.example", index))
	}
	addNodeBypassNodes(t, db, names...)
	var active, maximum atomic.Int32
	g.nodeBypassLookup = func(ctx context.Context, _ string) ([]net.IP, error) {
		now := active.Add(1)
		defer active.Add(-1)
		for prior := maximum.Load(); prior < now && !maximum.CompareAndSwap(prior, now); prior = maximum.Load() {
		}
		<-ctx.Done()
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	started := time.Now()
	if _, err := g.resolveNodeServerBypass(ctx); err != nil {
		t.Fatal(err)
	}
	if maximum.Load() > nodeBypassWorkers || active.Load() != 0 || time.Since(started) > time.Second {
		t.Fatalf("unbounded resolver: concurrent=%d remaining=%d elapsed=%s", maximum.Load(), active.Load(), time.Since(started))
	}
}

func TestNodeServerBypassGenerationRefreshesAddresses(t *testing.T) {
	db, g := nodeBypassFixture(t)
	nodes := addNodeBypassNodes(t, db, "node.example")
	ip := "192.0.2.25"
	lookupCount := 0
	g.nodeBypassLookup = func(context.Context, string) ([]net.IP, error) {
		lookupCount++
		return []net.IP{net.ParseIP(ip), net.ParseIP("2001:db8::25")}, nil
	}
	generate := func() map[string]interface{} {
		t.Helper()
		result, err := g.GenerateCurrent()
		if err != nil {
			t.Fatal(err)
		}
		return result.Config
	}
	first := generate()
	assertNodeBypassConfig(t, first, "192.0.2.25/32", true)
	ip = "192.0.2.26"
	second := generate()
	assertNodeBypassConfig(t, second, "192.0.2.26/32", true)
	assertNodeBypassConfig(t, second, "192.0.2.25/32", false)
	if lookupCount != 2 {
		t.Fatalf("lookups = %d, want one per complete generation", lookupCount)
	}
	if err := db.SetNodeEnabled(nodes[0].UID, false); err != nil {
		t.Fatal(err)
	}
	third := generate()
	assertNodeBypassConfig(t, third, "192.0.2.26/32", false)
	if lookupCount != 2 {
		t.Fatal("disabled node was resolved")
	}
	for _, raw := range third["dns"].(map[string]interface{})["rules"].([]map[string]interface{}) {
		if stringListContains(raw["domain"], "node.example") {
			t.Fatal("disabled node left a DNS whitelist entry")
		}
	}
}

func assertNodeBypassConfig(t *testing.T, config map[string]interface{}, cidr string, want bool) {
	t.Helper()
	found := false
	for _, raw := range config["inbounds"].([]interface{}) {
		inbound := raw.(map[string]interface{})
		if inbound["type"] == "tun" {
			found = stringListContains(inbound["route_exclude_address"], cidr)
		}
	}
	if found != want {
		t.Fatalf("TUN exclusion for %s = %t, want %t", cidr, found, want)
	}
	found = false
	for _, rule := range config["route"].(map[string]interface{})["rules"].([]map[string]interface{}) {
		if rule["action"] == "bypass" && stringListContains(rule["ip_cidr"], cidr) {
			found = true
		}
	}
	if found != want {
		t.Fatalf("pre-match bypass for %s = %t, want %t", cidr, found, want)
	}
}

func TestNodeServerBypassKeepsDevicePoliciesFirst(t *testing.T) {
	for _, mac := range []bool{false, true} {
		t.Run(fmt.Sprintf("MAC=%t", mac), func(t *testing.T) {
			db, g, devices := devicePolicyFixture(t)
			if mac {
				values := []string{"02:00:00:00:00:25"}
				if _, err := db.UpdateRouteRule(devices[0].ID, &model.RouteRuleRequest{Name: devices[0].Name, RuleType: "source_ip_cidr", Values: values, Enabled: true, Outbound: "proxy"}); err != nil {
					t.Fatal(err)
				}
			}
			addNodeBypassNodes(t, db, "node.example")
			snapshot := &nodeServerBypass{domains: []string{"node.example"}, ipCIDRs: []string{"192.0.2.10/32", "192.0.2.11/32", "192.0.2.25/32", "2001:db8::25/128"}}
			route, err := g.generateRoute("direct", snapshot)
			if err != nil {
				t.Fatal(err)
			}
			lastDevice, firstNode := -1, -1
			for index, rule := range route["rules"].([]map[string]interface{}) {
				if rule["action"] == "route" && (rule["outbound"] == devices[0].Name || rule["outbound"] == devices[1].Name) {
					lastDevice = index
					if rule["network"] != nil {
						t.Fatal("device route excludes ICMP")
					}
				}
				if firstNode == -1 && (stringListContains(rule["domain"], "node.example") || stringListContains(rule["ip_cidr"], "192.0.2.25/32")) {
					firstNode = index
				}
			}
			if lastDevice == -1 || firstNode <= lastDevice {
				t.Fatalf("node exemption (%d) precedes device policy (%d)", firstNode, lastDevice)
			}
			inbounds, err := g.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address, snapshot)
			if err != nil {
				t.Fatal(err)
			}
			for _, raw := range inbounds {
				for _, cidr := range snapshot.ipCIDRs {
					if stringListContains(raw.(map[string]interface{})["route_exclude_address"], cidr) {
						t.Fatal("global TUN exclusion lets protected sources escape before device policy")
					}
				}
			}
			dns := mustGenerateDNS(t, g)
			rules := dns["rules"].([]map[string]interface{})
			lastDevice, firstNode = -1, -1
			for index, rule := range rules {
				if rule["server"] == deviceDNSServerTag(devices[0].ID) || rule["server"] == deviceDNSServerTag(devices[1].ID) {
					lastDevice = index
				}
				if stringListContains(rule["domain"], "node.example") {
					firstNode = index
					break
				}
			}
			if firstNode <= lastDevice || lastDevice == -1 || dns["reverse_mapping"] != true {
				t.Fatalf("device DNS lost precedence: device=%d node=%d", lastDevice, firstNode)
			}
		})
	}
}

func TestNodeServerDNSUsesRealResolverBeforeFakeIP(t *testing.T) {
	db, g := nodeBypassFixture(t)
	addNodeBypassNodes(t, db, "node.example", "hosts.example")
	if _, err := db.CreateDNSHost(&model.DNSHostRequest{Domain: "hosts.example", Addresses: []string{"192.0.2.30"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	dns := mustGenerateDNS(t, g)
	rules := dns["rules"].([]map[string]interface{})
	if !stringListContains(rules[0]["domain"], "hosts.example") || rules[0]["server"] != managedDNSHostsServerTag {
		t.Fatal("node Hosts override was not preserved")
	}
	if !stringListContains(rules[1]["domain"], "node.example") || rules[1]["server"] != "bootstrap" || !reflect.DeepEqual(rules[1]["query_type"], []string{"A", "AAAA"}) {
		t.Fatalf("node DNS does not select real A/AAAA bootstrap: %+v", rules[1])
	}
	if rules[len(rules)-1]["server"] != "fakeip" || !slices.Contains(rules[1]["domain"].([]string), "hosts.example") {
		t.Fatal("ordinary domains lost FakeIP or node whitelist is incomplete")
	}
	if dns["reverse_mapping"] != true {
		t.Fatal("rotating DNS answers cannot match the node domain bypass")
	}
}
