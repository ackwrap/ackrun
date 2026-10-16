package service

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

// Uses only loopback listeners and synthetic endpoints; no TUN or host routing changes.
func TestDeviceDirectDomainsWithAvailableCore(t *testing.T) {
	core := os.Getenv("ACKWRAP_SINGBOX_114_BIN")
	if core == "" {
		t.Skip("set ACKWRAP_SINGBOX_114_BIN for real traffic validation")
	}
	db, g, devices := devicePolicyFixture(t)
	setDeviceDirectDomains(t, db, devices[0], "allowed.example")
	storedRules, err := db.ListRouteRules()
	if err != nil {
		t.Fatal(err)
	}
	for _, rule := range storedRules {
		if rule.SystemKey == SystemRuleAdBlockKey {
			if _, err := db.UpdateRouteRule(rule.ID, &model.RouteRuleRequest{Name: rule.Name, RuleType: rule.RuleType, Values: rule.Values, Outbound: rule.Outbound}); err != nil {
				t.Fatal(err)
			}
		}
	}
	global, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	global.FakeIPEnabled = true
	if err := db.SetDNSGlobalSettings(global); err != nil {
		t.Fatal(err)
	}
	dns, err := g.generateDNSFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	route, err := g.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	// Substitute only source addresses so generated production policy can run without root.
	for _, rules := range [][]map[string]interface{}{route["rules"].([]map[string]interface{}), dns["rules"].([]map[string]interface{})} {
		for _, rule := range rules {
			for i, device := range devices {
				if reflect.DeepEqual(rule["source_ip_cidr"], device.Values) {
					rule["source_ip_cidr"] = []string{fmt.Sprintf("127.0.0.%d/32", i+2)}
				}
			}
		}
	}
	route["find_process"] = false
	delete(route, "default_http_client") // This fixture has no remote rule sets.
	route["rules"] = append([]map[string]interface{}{{"inbound": "test-dns", "action": "hijack-dns"}}, route["rules"].([]map[string]interface{})...)
	route["final"] = "direct"

	// One local destination and two distinguishable CONNECT proxies prove outbound selection.
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "direct") }))
	defer destination.Close()
	_, destinationPort, _ := net.SplitHostPort(destination.Listener.Addr().String())
	outbounds := []map[string]interface{}{{"type": "direct", "tag": "direct"}, {"type": "selector", "tag": "proxy", "outbounds": []string{devices[0].Name}}}
	for _, device := range devices {
		marker := device.Name
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect {
				http.Error(w, "CONNECT required", 400)
				return
			}
			conn, rw, err := w.(http.Hijacker).Hijack()
			if err != nil {
				return
			}
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
			fmt.Fprint(rw, "HTTP/1.1 200 Connection Established\r\n\r\n")
			_ = rw.Flush()
			if req, err := http.ReadRequest(rw.Reader); err == nil {
				_ = req.Body.Close()
				fmt.Fprintf(rw, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(marker), marker)
				_ = rw.Flush()
			}
		}))
		t.Cleanup(upstream.Close)
		_, port, _ := net.SplitHostPort(upstream.Listener.Addr().String())
		portNumber, _ := strconv.Atoi(port)
		outbounds = append(outbounds, map[string]interface{}{"type": "http", "tag": device.Name, "server": "127.0.0.1", "server_port": portNumber})
	}
	// Synthetic DNS returns loopback for A and an empty answer for AAAA.
	resolver, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer resolver.Close()
	go func() {
		buf := make([]byte, 2048)
		for {
			n, peer, err := resolver.ReadFrom(buf)
			if err != nil {
				return
			}
			if n < 17 {
				continue
			}
			response := append([]byte(nil), buf[:n]...)
			response[2], response[3] = 0x81, 0x80
			for i := 6; i < 12; i++ {
				response[i] = 0
			}
			if binary.BigEndian.Uint16(response[n-4:n-2]) == 1 {
				response[7] = 1
				response = append(response, 0xc0, 0x0c, 0, 1, 0, 1, 0, 0, 0, 60, 0, 4, 127, 0, 0, 1)
			}
			_, _ = resolver.WriteTo(response, peer)
		}
	}()
	for _, server := range dns["servers"].([]map[string]interface{}) {
		if server["type"] == "fakeip" {
			continue
		}
		tag, detour := server["tag"], server["detour"]
		for key := range server {
			delete(server, key)
		}
		server["tag"], server["type"], server["server"], server["server_port"] = tag, "udp", "127.0.0.1", resolver.LocalAddr().(*net.UDPAddr).Port
		if detour != nil {
			server["detour"] = detour
		}
	}
	freePort := func() int {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		defer listener.Close()
		return listener.Addr().(*net.TCPAddr).Port
	}
	mixedPort, dnsPort := freePort(), freePort()
	config := map[string]interface{}{
		"log": map[string]interface{}{"level": "debug", "timestamp": false},
		"dns": dns, "route": route, "outbounds": outbounds,
		"inbounds": []map[string]interface{}{
			{"type": "mixed", "tag": "test-mixed", "listen": "127.0.0.1", "listen_port": mixedPort},
			{"type": "direct", "tag": "test-dns", "listen": "127.0.0.1", "listen_port": dnsPort},
		},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(t.TempDir(), "test.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "core.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	cmd := exec.Command(core, "run", "-c", configPath)
	cmd.Stdout, cmd.Stderr = logFile, logFile
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			data, _ := os.ReadFile(logPath)
			t.Logf("synthetic core log: %s", data)
		}
	})
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(mixedPort))
	deadline := time.Now().Add(5 * time.Second)
	for {
		conn, err := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("core listener not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	request := func(source, target, host string) (string, error) {
		dialer := net.Dialer{Timeout: time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(source)}}
		conn, err := dialer.Dial("tcp4", address)
		if err != nil {
			return "", err
		}
		defer conn.Close()
		_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
		rw := bufio.NewReader(conn)
		fmt.Fprintf(conn, "CONNECT %s:%s HTTP/1.1\r\nHost: %s:%s\r\n\r\n", target, destinationPort, target, destinationPort)
		response, err := http.ReadResponse(rw, nil)
		if err != nil {
			return "", err
		}
		if response.StatusCode != 200 {
			return "", fmt.Errorf("CONNECT status %d", response.StatusCode)
		}
		fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\n\r\n", host)
		response, err = http.ReadResponse(rw, nil)
		if err != nil {
			return "", err
		}
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		return string(body), err
	}
	for _, test := range []struct{ source, host, want string }{
		{"127.0.0.2", "allowed.example", "direct"},
		{"127.0.0.2", "sub.allowed.example", "direct"},
		{"127.0.0.2", "notallowed.example", "Device A"},
		{"127.0.0.3", "allowed.example", "Device B"},
		{"127.0.0.2", "127.0.0.1", "Device A"},
	} {
		t.Run(test.source+"/"+test.host, func(t *testing.T) {
			got, err := request(test.source, "127.0.0.1", test.host)
			if err != nil || got != test.want {
				t.Fatalf("got %q %v, want %q", got, err, test.want)
			}
		})
	}
	// Seed a cached FakeIP as an unbound device, then use it from the bound device.
	query := []byte{0, 1, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0}
	for _, label := range strings.Split("allowed.example", ".") {
		query = append(query, byte(len(label)))
		query = append(query, label...)
	}
	query = append(query, 0, 0, 1, 0, 1)
	boundDNS, err := net.DialUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.2")}, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: dnsPort})
	if err != nil {
		t.Fatal(err)
	}
	defer boundDNS.Close()
	_ = boundDNS.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := boundDNS.Write(query); err != nil {
		t.Fatal(err)
	}
	realAnswer := make([]byte, 2048)
	realSize, err := boundDNS.Read(realAnswer)
	if err != nil || realSize < 16 || realAnswer[7] != 1 || !net.IP(realAnswer[realSize-4:realSize]).Equal(net.IPv4(127, 0, 0, 1)) {
		t.Fatalf("whitelist DNS did not query direct upstream: len=%d err=%v", realSize, err)
	}
	dnsConn, err := net.Dial("udp4", net.JoinHostPort("127.0.0.1", strconv.Itoa(dnsPort)))
	if err != nil {
		t.Fatal(err)
	}
	defer dnsConn.Close()
	_ = dnsConn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := dnsConn.Write(query); err != nil {
		t.Fatal(err)
	}
	response := make([]byte, 2048)
	n, err := dnsConn.Read(response)
	if err != nil || n < 16 || response[7] != 1 {
		t.Fatalf("FakeIP answer: len=%d err=%v", n, err)
	}
	fake := net.IP(response[n-4 : n]).String()
	if !strings.HasPrefix(fake, "198.18.") {
		t.Fatalf("expected FakeIP, got %s", fake)
	}
	if got, err := request("127.0.0.2", fake, "allowed.example"); err != nil || got != "direct" {
		t.Fatalf("cached FakeIP whitelist got %q %v", got, err)
	}
	if got, err := request("127.0.0.2", "198.19.255.254", "allowed.example"); err == nil {
		t.Fatalf("unmapped FakeIP accepted: %q", got)
	}
}
