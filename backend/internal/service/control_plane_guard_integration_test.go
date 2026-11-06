//go:build linux

package service

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

// Run only in a disposable network namespace:
// unshare -n env ACKWRAP_NFT_GUARD_TEST=1 go test ./internal/service -run '^TestControlPlaneGuardPackets$'
func TestControlPlaneGuardPackets(t *testing.T) {
	if os.Getenv("ACKWRAP_NFT_GUARD_TEST") != "1" {
		t.Skip("requires root, nft/ip/python3 and an isolated network namespace")
	}
	self, err := os.Readlink("/proc/self/ns/net")
	if err != nil {
		t.Fatal(err)
	}
	host, err := os.Readlink("/proc/1/ns/net")
	if err != nil || self == host {
		t.Fatal("refusing to alter the host network namespace")
	}
	run := func(name string, args ...string) {
		t.Helper()
		if output, err := exec.Command(name, args...).CombinedOutput(); err != nil {
			t.Fatalf("%s %v: %v %s", name, args, err, output)
		}
	}
	peer := exec.Command("unshare", "-n", "sleep", "120")
	if err := peer.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = peer.Process.Kill(); _ = peer.Wait() })
	pid := fmt.Sprint(peer.Process.Pid)
	deadline := time.Now().Add(time.Second)
	for {
		ns, _ := os.Readlink("/proc/" + pid + "/ns/net")
		if ns != "" && ns != self {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("upstream namespace did not start")
		}
		time.Sleep(10 * time.Millisecond)
	}
	run("ip", "link", "set", "lo", "up")
	run("ip", "link", "add", "guard-wan", "type", "veth", "peer", "name", "guard-wan-peer")
	run("ip", "link", "set", "guard-wan-peer", "netns", pid)
	for _, side := range []struct{ pid, iface, ipv4, ipv6 string }{
		{"", "guard-wan", "198.51.100.1/24", "2001:db8:2::1/64"},
		{pid, "guard-wan-peer", "198.51.100.2/24", "2001:db8:2::2/64"},
	} {
		ip := func(args ...string) {
			if side.pid == "" {
				run("ip", args...)
			} else {
				run("nsenter", append([]string{"-t", side.pid, "-n", "ip"}, args...)...)
			}
		}
		ip("addr", "add", side.ipv4, "dev", side.iface)
		ip("-6", "addr", "add", side.ipv6, "dev", side.iface, "nodad")
		ip("link", "set", side.iface, "up")
		ip("link", "set", "lo", "up")
	}
	guard := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()})
	if err := guard.Enable(nil, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := guard.Disable(); err != nil {
			t.Error(err)
		}
	})
	testControlPlaneDownloadsUnderGuard(t, pid)
}

// Runs with the real global transition guard installed and no core running.
func testControlPlaneDownloadsUnderGuard(t *testing.T, wanPID string) {
	t.Helper()
	if output, err := exec.Command("nsenter", "-t", wanPID, "-n", "ip", "addr", "add", "198.51.100.3/24", "dev", "guard-wan-peer").CombinedOutput(); err != nil {
		t.Fatalf("add synthetic DNS address: %v %s", err, output)
	}
	dir := t.TempDir()
	payload := testBinaryRuleSet(t, 1)
	payloadPath := filepath.Join(dir, "rules.srs")
	if err := os.WriteFile(payloadPath, payload, 0600); err != nil {
		t.Fatal(err)
	}
	const serve = `import http.server,socket,struct,sys,threading
payload=open(sys.argv[1],'rb').read()
class Handler(http.server.BaseHTTPRequestHandler):
 def do_GET(self):
  if self.path=='/failed':
   self.send_error(503);return
  self.send_response(200);self.end_headers();self.wfile.write(payload)
 def log_message(self,*args):pass
http4=http.server.ThreadingHTTPServer(('198.51.100.2',18080),Handler)
class HTTP6(http.server.ThreadingHTTPServer):address_family=socket.AF_INET6
http6=HTTP6(('2001:db8:2::2',18080),Handler)
def reply(data,tcp):
 end=12
 while data[end]:end+=data[end]+1
 end+=1
 kind=struct.unpack('!H',data[end:end+2])[0]
 question=data[12:end+4]
 if not tcp and b'tcp' in question:
  return data[:2]+struct.pack('!HHHHH',0x8380,1,0,0,0)+question
 addr=socket.inet_aton('198.51.100.2') if kind==1 else socket.inet_pton(socket.AF_INET6,'2001:db8:2::2')
 answer=b'\xc0\x0c'+struct.pack('!HHIH',kind,1,60,len(addr))+addr
 return data[:2]+struct.pack('!HHHHH',0x8180,1,1,0,0)+question+answer
udp=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);udp.bind(('198.51.100.3',53))
tcp=socket.socket();tcp.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1);tcp.bind(('198.51.100.3',53));tcp.listen()
def udp_loop():
 while True:
  data,peer=udp.recvfrom(4096);udp.sendto(reply(data,False),peer)
def exact(c,n):
 data=b''
 while len(data)<n:
  part=c.recv(n-len(data))
  if not part:raise EOFError()
  data+=part
 return data
def tcp_request(c):
 try:
  data=exact(c,struct.unpack('!H',exact(c,2))[0]);answer=reply(data,True)
  c.sendall(struct.pack('!H',len(answer))+answer)
 finally:c.close()
def tcp_loop():
 while True:
  c,_=tcp.accept();threading.Thread(target=tcp_request,args=(c,),daemon=True).start()
threading.Thread(target=udp_loop,daemon=True).start()
threading.Thread(target=tcp_loop,daemon=True).start()
threading.Thread(target=http6.serve_forever,daemon=True).start()
print('ready',flush=True)
http4.serve_forever()
`
	cmd := exec.Command("nsenter", "-t", wanPID, "-n", "python3", "-u", "-c", serve, payloadPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(stdout).ReadString('\n')
		if err == nil && strings.TrimSpace(line) != "ready" {
			err = fmt.Errorf("unexpected readiness: %q", line)
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			waitErr := cmd.Wait()
			t.Fatalf("synthetic upstream: %v (%v): %s", err, waitErr, stderr.String())
		}
	case <-time.After(3 * time.Second):
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatalf("synthetic upstream did not start: %s", stderr.String())
	}
	resolverFile := filepath.Join(dir, "resolv.conf.auto")
	if err := os.WriteFile(resolverFile, []byte("nameserver 127.0.0.1\nnameserver ::1\nnameserver 198.51.100.3\n"), 0600); err != nil {
		t.Fatal(err)
	}
	useControlPlaneResolverFiles(t, resolverFile)
	plain := http.DefaultTransport.(*http.Transport).Clone()
	plain.Proxy = nil
	defer plain.CloseIdleConnections()
	if resp, err := (&http.Client{Timeout: 300 * time.Millisecond, Transport: plain}).Get("http://198.51.100.2:18080/rules.srs"); err == nil {
		resp.Body.Close()
		t.Fatal("global protection allowed unmarked router HTTP")
	}
	transport := directHTTPTransport()
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 3 * time.Second, Transport: transport}
	for _, name := range []string{"udp.ackwrap.invalid", "tcp.ackwrap.invalid", "198.51.100.2", "[2001:db8:2::2]"} {
		resp, err := client.Get("http://" + name + ":18080/rules.srs")
		if err != nil {
			t.Fatalf("controlled HTTP/DNS blocked by guard (%s): %v", name, err)
		}
		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil || !bytes.Equal(data, payload) {
			t.Fatalf("controlled download: err=%v data=%x", err, data)
		}
	}
	db, err := store.Open(filepath.Join(dir, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := newTestRouteRuleService(t, db)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	data, _, err := svc.generatedGeoRuleSetContentContext(ctx, "geosite-test", "http://tcp.ackwrap.invalid:18080/rules.srs")
	if err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("cold Geo rule cache could not download under protection: %v", err)
	}
	if cached, err := os.ReadFile(filepath.Join(svc.paths.RulesDir, "geo", "geosite-test.srs")); err != nil || !bytes.Equal(cached, payload) {
		t.Fatalf("cold Geo cache not populated: %v", err)
	}
	if data, err := fetchRouteRuleSubscriptionContent("http://udp.ackwrap.invalid:18080/rules.srs", false); err != nil || !bytes.Equal(data, payload) {
		t.Fatalf("rule subscription could not download under protection: %v", err)
	}
	if _, _, err := svc.generatedGeoRuleSetContentContext(ctx, "geosite-failed", "http://udp.ackwrap.invalid:18080/failed"); err == nil {
		t.Fatal("upstream failure was not reported")
	}
	if _, err := os.Stat(filepath.Join(svc.paths.RulesDir, "geo", "geosite-failed.srs")); !os.IsNotExist(err) {
		t.Fatalf("failed download left a cache: %v", err)
	}
	// Success and failure must not remove the guard or exempt other root sockets.
	if resp, err := (&http.Client{Timeout: 300 * time.Millisecond, Transport: plain}).Get("http://198.51.100.2:18080/rules.srs"); err == nil {
		resp.Body.Close()
		t.Fatal("controlled download unlocked unrelated router traffic")
	}
}
