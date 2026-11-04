package service

import (
	"context"
	"encoding/json"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/dns/dnsmessage"
)

// A local authoritative fixture proves both address families without external
// DNS, real node credentials, host routing changes or a TUN device.
func nodeBypassTestDNS(t *testing.T) string {
	t.Helper()
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		buf := make([]byte, 4096)
		for {
			n, peer, err := listener.ReadFrom(buf)
			if err != nil {
				return
			}
			var query dnsmessage.Message
			if query.Unpack(buf[:n]) != nil || len(query.Questions) != 1 {
				continue
			}
			question := query.Questions[0]
			response := dnsmessage.Message{
				Header:    dnsmessage.Header{ID: query.ID, Response: true, Authoritative: true, RecursionAvailable: true},
				Questions: query.Questions,
			}
			header := dnsmessage.ResourceHeader{Name: question.Name, Class: dnsmessage.ClassINET, Type: question.Type, TTL: 30}
			switch question.Type {
			case dnsmessage.TypeA:
				response.Answers = []dnsmessage.Resource{{Header: header, Body: &dnsmessage.AResource{A: [4]byte{192, 0, 2, 25}}}}
			case dnsmessage.TypeAAAA:
				response.Answers = []dnsmessage.Resource{{Header: header, Body: &dnsmessage.AAAAResource{AAAA: netip.MustParseAddr("2001:db8::25").As16()}}}
			}
			packet, err := response.Pack()
			if err == nil {
				_, _ = listener.WriteTo(packet, peer)
			}
		}
	}()
	return listener.LocalAddr().String()
}

func nodeBypassTestResolver(server string) *net.Resolver {
	return &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server)
	}}
}

func TestNodeServerBypassResolvesBothFamilies(t *testing.T) {
	db, g := nodeBypassFixture(t)
	addNodeBypassNodes(t, db, "node.example")
	resolver := nodeBypassTestResolver(nodeBypassTestDNS(t))
	g.nodeBypassLookup = func(ctx context.Context, host string) ([]net.IP, error) { return resolver.LookupIP(ctx, "ip", host) }
	targets, err := g.resolveNodeServerBypass(context.Background())
	if err != nil || !reflect.DeepEqual(targets.ipCIDRs, []string{"192.0.2.25/32", "2001:db8::25/128"}) {
		t.Fatalf("A/AAAA lookup produced %+v, error=%v", targets, err)
	}
}

func TestNodeServerDNSWithAvailableCore(t *testing.T) {
	core := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if core == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN for real core DNS validation")
	}
	db, g := nodeBypassFixture(t)
	addNodeBypassNodes(t, db, "node.example")
	dns := mustGenerateDNS(t, g)
	upstreamHost, upstreamPort, _ := net.SplitHostPort(nodeBypassTestDNS(t))
	port, _ := strconv.Atoi(upstreamPort)
	for _, server := range dns["servers"].([]map[string]interface{}) {
		if server["tag"] == "bootstrap" {
			server["server"], server["server_port"] = upstreamHost, port
		}
	}
	listener, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.LocalAddr().String()
	_, testPort, _ := net.SplitHostPort(address)
	dnsPort, _ := strconv.Atoi(testPort)
	listener.Close()
	bypass, err := g.defaultBypassRules(false)
	if err != nil {
		t.Fatal(err)
	}
	rules := append([]map[string]interface{}{{"inbound": "test-dns", "action": "hijack-dns"}}, bypass...)
	config := map[string]interface{}{
		"log":       map[string]interface{}{"level": "error"},
		"dns":       dns,
		"inbounds":  []map[string]interface{}{{"type": "direct", "tag": "test-dns", "listen": "127.0.0.1", "listen_port": dnsPort}},
		"outbounds": []map[string]interface{}{{"type": "direct", "tag": "direct"}},
		"route": map[string]interface{}{
			"rules": rules, "final": "direct", "default_domain_resolver": g.defaultDomainResolver(),
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	configPath := filepath.Join(dir, "node-dns.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(core, "check", "-c", configPath).CombinedOutput(); err != nil {
		t.Fatalf("core rejected generated DNS/bypass actions: %v: %s", err, out)
	}
	log, err := os.Create(filepath.Join(dir, "core.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer log.Close()
	cmd := exec.Command(core, "run", "-c", configPath)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	resolver := nodeBypassTestResolver(address)
	for _, network := range []string{"ip4", "ip6"} {
		var ips []net.IP
		var err error
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
			ips, err = resolver.LookupIP(ctx, network, "node.example")
			cancel()
			if err == nil && len(ips) > 0 {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		want := "192.0.2.25"
		if network == "ip6" {
			want = "2001:db8::25"
		}
		if err != nil || len(ips) != 1 || !ips[0].Equal(net.ParseIP(want)) {
			t.Fatalf("node %s DNS returned %v, error=%v, want %s", network, ips, err, want)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	ips, err := resolver.LookupIP(ctx, "ip4", "ordinary.example")
	if err != nil || len(ips) != 1 || !netip.MustParsePrefix("10.66.0.0/24").Contains(netip.MustParseAddr(ips[0].String())) {
		t.Fatalf("ordinary DNS no longer uses FakeIP: %v, error=%v", ips, err)
	}
}
