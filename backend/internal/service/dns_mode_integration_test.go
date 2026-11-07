package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

// Exercises generated DNS/routing with only loopback sockets, no TUN interface,
// external DNS, real nodes, or changes to the machine's routing/firewall.
func TestDNSModeSwitchWithAvailableCore(t *testing.T) {
	core := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if core == "" || runtime.GOOS == "windows" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN on Unix for graceful mode-switch validation")
	}
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.SetProxyMode("rule"); err != nil {
		t.Fatal(err)
	}
	host, portText, _ := net.SplitHostPort(nodeBypassTestDNS(t))
	port, _ := strconv.Atoi(portText)
	if _, err := db.CreateDNSServer(&model.DNSServerRequest{
		Tag: "test-dns", Enabled: true, ServerType: "udp", Address: net.JoinHostPort(host, portText),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.CreateRouteRule(&model.RouteRuleRequest{
		Name: "domain-route", Enabled: true, RuleType: "geosite", Values: []string{"test"}, Outbound: "direct",
	}); err != nil {
		t.Fatal(err)
	}
	g := NewConfigGeneratorService(db, &paths.Paths{DataDir: dir})
	dnsService := NewDNSService(db, nil)
	settings, err := dnsService.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.Final = "test-dns"
	proxy := func(tag string) map[string]interface{} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
			_ = rw.Flush()
			if _, err := rw.ReadString('\n'); err == nil {
				fmt.Fprintln(rw, tag)
				_ = rw.Flush()
			}
		}))
		t.Cleanup(server.Close)
		_, text, _ := net.SplitHostPort(server.Listener.Addr().String())
		port, _ := strconv.Atoi(text)
		return map[string]interface{}{"type": "http", "tag": tag, "server": "127.0.0.1", "server_port": port}
	}
	outbounds := []map[string]interface{}{proxy("domain-route"), proxy(SystemGlobalDirectRouteRuleName)}
	freePort := func() int {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port
	}
	mixedPort, dnsPort := freePort(), freePort()
	mixedAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(mixedPort))
	dnsAddress := net.JoinHostPort("127.0.0.1", strconv.Itoa(dnsPort))
	start := func(fakeIP bool) func() {
		t.Helper()
		settings.FakeIPEnabled = fakeIP
		settings.ReverseMapping = false // redir-host must enable it automatically.
		if err := dnsService.SetDNSGlobalSettings(settings); err != nil {
			t.Fatal(err)
		}
		dns := mustGenerateDNS(t, g)
		for _, server := range dns["servers"].([]map[string]interface{}) {
			if server["tag"] == "test-dns" {
				server["server"], server["server_port"] = host, port
			}
		}
		route, err := g.generateRoute("direct")
		if err != nil {
			t.Fatal(err)
		}
		route["find_process"] = false
		delete(route, "default_http_client")
		delete(route, "auto_detect_interface")
		// Replace remote GeoSite data, including the system ad rule, with local
		// fixtures. The generated route actions and their order remain unchanged.
		for _, ruleSet := range route["rule_set"].([]map[string]interface{}) {
			tag := ruleSet["tag"]
			domain := "unused-fixture.invalid"
			if tag == "geosite-test" {
				domain = "service.example"
			}
			for key := range ruleSet {
				delete(ruleSet, key)
			}
			ruleSet["tag"], ruleSet["type"] = tag, "inline"
			ruleSet["rules"] = []map[string]interface{}{{"domain": []string{domain}}}
		}
		route["rules"] = append([]map[string]interface{}{{"inbound": "test-dns-in", "action": "hijack-dns"}}, route["rules"].([]map[string]interface{})...)
		config := map[string]interface{}{
			"log": map[string]interface{}{"level": "error"}, "dns": dns, "route": route, "outbounds": outbounds,
			"inbounds": []map[string]interface{}{
				{"type": "mixed", "tag": "test-mixed", "listen": "127.0.0.1", "listen_port": mixedPort},
				{"type": "direct", "tag": "test-dns-in", "listen": "127.0.0.1", "listen_port": dnsPort},
			},
			"experimental": map[string]interface{}{"cache_file": g.cacheFileConfig(&model.ExperimentalSettingsResponse{}, true)},
		}
		data, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		configPath := filepath.Join(dir, dnsModeName(fakeIP)+".json")
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command(core, "check", "-c", configPath).CombinedOutput(); err != nil {
			t.Fatalf("core rejected generated mode: %v: %s", err, output)
		}
		log, err := os.Create(filepath.Join(dir, dnsModeName(fakeIP)+".log"))
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(core, "run", "-c", configPath)
		cmd.Stdout, cmd.Stderr = log, log
		if err := cmd.Start(); err != nil {
			log.Close()
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		stopped := false
		stop := func() {
			if stopped {
				return
			}
			stopped = true
			_ = cmd.Process.Signal(os.Interrupt)
			select {
			case err := <-done:
				if err != nil {
					t.Errorf("core stopped with error: %v", err)
				}
			case <-time.After(5 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Error("core did not stop gracefully")
			}
			log.Close()
			if t.Failed() {
				data, _ := os.ReadFile(log.Name())
				t.Logf("synthetic mode-switch core log: %s", data)
			}
		}
		t.Cleanup(stop)
		deadline := time.Now().Add(5 * time.Second)
		for {
			conn, err := net.DialTimeout("tcp4", mixedAddress, 100*time.Millisecond)
			if err == nil {
				conn.Close()
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("core did not open its loopback listener")
			}
			time.Sleep(20 * time.Millisecond)
		}
		return stop
	}
	lookup := func(network string) net.IP {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		ips, err := nodeBypassTestResolver(dnsAddress).LookupIP(ctx, network, "service.example")
		if err != nil || len(ips) != 1 {
			t.Fatalf("DNS %s: %v, %v", network, ips, err)
		}
		return ips[0]
	}
	request := func(target net.IP, want string) {
		t.Helper()
		conn, err := net.DialTimeout("tcp4", mixedAddress, time.Second)
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(4 * time.Second))
		address := net.JoinHostPort(target.String(), "443")
		fmt.Fprintf(conn, "CONNECT %s HTTP/1.1\r\nHost: %s\r\n\r\n", address, address)
		rw := bufio.NewReader(conn)
		if response, err := http.ReadResponse(rw, nil); err != nil || response.StatusCode != 200 {
			t.Fatalf("CONNECT failed: %v, %v", response, err)
		}
		// No HTTP Host or TLS SNI: only DNS/FakeIP mapping can supply the domain.
		fmt.Fprint(conn, "opaque-payload\n")
		response, err := rw.ReadString('\n')
		if err != nil || strings.TrimSpace(response) != want {
			t.Fatalf("mapped connection got %q, %v; want %s", response, err, want)
		}
		_, _ = io.Copy(io.Discard, rw)
	}
	stop := start(true)
	old4, old6 := lookup("ip4"), lookup("ip6")
	if !netip.MustParsePrefix("198.18.0.0/16").Contains(netip.MustParseAddr(old4.String())) || !netip.MustParsePrefix("fc00::/18").Contains(netip.MustParseAddr(old6.String())) {
		t.Fatalf("fake-ip mode returned non-FakeIP answers: %s, %s", old4, old6)
	}
	request(old4, "domain-route")
	stop()
	stop = start(false)
	request(old4, "domain-route") // Old client cache survives the mode switch.
	request(old6, "domain-route")
	real4, real6 := lookup("ip4"), lookup("ip6")
	if !real4.Equal(net.ParseIP("192.0.2.25")) || !real6.Equal(net.ParseIP("2001:db8::25")) {
		t.Fatalf("redir-host returned virtual answers: %s, %s", real4, real6)
	}
	request(real4, "domain-route") // Reverse mapping preserves GeoSite without sniff data.
	request(net.ParseIP("192.0.2.26"), SystemGlobalDirectRouteRuleName)
	stop()
	stop = start(true)
	request(old4, "domain-route")
	if current := lookup("ip4"); !current.Equal(old4) {
		t.Fatalf("switching back lost stable FakeIP mapping: %s != %s", current, old4)
	}
	stop()
}
