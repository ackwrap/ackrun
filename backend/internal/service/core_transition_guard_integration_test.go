//go:build linux

package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/paths"
)

// Run only inside a disposable network namespace:
// unshare -n env ACKWRAP_NFT_GUARD_TEST=1 go test ... -run TestNFTGuardPackets
func TestNFTGuardPackets(t *testing.T) {
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
			for _, diagnostic := range [][]string{{"ip", "-6", "route"}, {"ip", "-6", "neigh"}} {
				if result, readErr := exec.Command(diagnostic[0], diagnostic[1:]...).CombinedOutput(); readErr == nil {
					t.Logf("router %v: %s", diagnostic, result)
				}
			}
			if name == "nsenter" {
				for _, diagnostic := range [][]string{{"ip", "-6", "route"}, {"ip", "-6", "neigh"}} {
					result, _ := exec.Command("nsenter", append(args[:3:3], diagnostic...)...).CombinedOutput()
					t.Logf("peer %v: %s", diagnostic, result)
				}
			}
			t.Fatalf("%s %v: %v %s", name, args, err, output)
		}
	}
	newPeer := func(name, router4, peer4, router6, peer6 string) string {
		t.Helper()
		cmd := exec.Command("unshare", "-n", "sleep", "120")
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		pid := fmt.Sprint(cmd.Process.Pid)
		deadline := time.Now().Add(time.Second)
		for {
			ns, _ := os.Readlink("/proc/" + pid + "/ns/net")
			if ns != "" && ns != self {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("peer namespace did not start")
			}
			time.Sleep(10 * time.Millisecond)
		}
		run("ip", "link", "add", name, "type", "veth", "peer", "name", name+"-peer")
		run("ip", "link", "set", name+"-peer", "netns", pid)
		run("ip", "addr", "add", router4+"/24", "dev", name)
		run("ip", "-6", "addr", "add", router6+"/64", "dev", name, "nodad")
		run("ip", "link", "set", name, "up")
		peer := func(args ...string) { run("nsenter", append([]string{"-t", pid, "-n", "ip"}, args...)...) }
		peer("link", "set", "lo", "up")
		peer("addr", "add", peer4+"/24", "dev", name+"-peer")
		peer("-6", "addr", "add", peer6+"/64", "dev", name+"-peer", "nodad")
		peer("link", "set", name+"-peer", "up")
		peer("route", "add", "default", "via", router4)
		peer("-6", "route", "add", "default", "via", router6)
		return pid
	}
	run("ip", "link", "set", "lo", "up")
	lan := newPeer("guard-lan", "192.0.2.1", "192.0.2.2", "2001:db8:1::1", "2001:db8:1::2")
	wan := newPeer("guard-wan", "198.51.100.1", "198.51.100.2", "2001:db8:2::1", "2001:db8:2::2")
	run("sysctl", "-qw", "net.ipv4.ip_forward=1")
	run("sysctl", "-qw", "net.ipv6.conf.all.forwarding=1")
	const echo = `import socket,sys,threading
tcp=sys.argv[3]=='tcp'
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM if tcp else socket.SOCK_DGRAM)
s.setsockopt(socket.SOL_SOCKET,socket.SO_REUSEADDR,1)
s.bind((sys.argv[1],int(sys.argv[2])))
if tcp:s.listen()
print('ready',flush=True)
def serve(c):
 try:
  while True:
   data=c.recv(1024)
   if not data:break
   c.sendall(data)
 finally:c.close()
while True:
 if tcp:
  c,_=s.accept()
  threading.Thread(target=serve,args=(c,),daemon=True).start()
 else:
  data,peer=s.recvfrom(1024)
  s.sendto(data,peer)
`
	server := func(pid, address string, port ...string) *exec.Cmd {
		t.Helper()
		listenPort := "15153"
		if len(port) > 0 {
			listenPort = port[0]
		}
		network := "udp"
		if len(port) > 1 {
			network = port[1]
		}
		args := []string{"python3", "-u", "-c", echo, address, listenPort, network}
		name := "python3"
		if pid != "" {
			name = "nsenter"
			args = append([]string{"-t", pid, "-n"}, args...)
		} else {
			args = args[1:]
		}
		cmd := exec.Command(name, args...)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		ready := make(chan error, 1)
		go func() { var b [6]byte; _, err := stdout.Read(b[:]); ready <- err }()
		select {
		case err := <-ready:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("echo server did not start")
		}
		return cmd
	}
	server(wan, "198.51.100.2")
	server(wan, "2001:db8:2::2")
	server(wan, "198.51.100.2", "15153", "tcp")
	server(wan, "2001:db8:2::2", "15153", "tcp")
	server("", "192.0.2.1")
	server("", "127.0.0.1")
	// Kernel-generated link-local addresses still perform DAD even when the
	// synthetic global addresses use nodad. Wait before the IPv6 baseline.
	time.Sleep(1500 * time.Millisecond)
	const probe = `import socket,sys
tcp=sys.argv[5]=='tcp'
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM if tcp else socket.SOCK_DGRAM)
s.settimeout(2 if sys.argv[3]=='pass' else 0.5)
if int(sys.argv[2]): s.setsockopt(socket.SOL_SOCKET,36,int(sys.argv[2]))
try:
 if tcp:
  s.connect((sys.argv[1],int(sys.argv[4])))
  s.sendall(b'test')
 else:s.sendto(b'test',(sys.argv[1],int(sys.argv[4])))
 data=s.recv(1024)
 ok=data==b'test'
except OSError: ok=False
sys.exit(0 if ok==(sys.argv[3]=='pass') else 1)
`
	check := func(pid, address, mark, want string, port ...string) {
		t.Helper()
		destPort := "15153"
		if len(port) > 0 {
			destPort = port[0]
		}
		network := "udp"
		if len(port) > 1 {
			network = port[1]
		}
		args := []string{"python3", "-c", probe, address, mark, want, destPort, network}
		if pid == "" {
			run("python3", args[1:]...)
		} else {
			run("nsenter", append([]string{"-t", pid, "-n"}, args...)...)
		}
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "pass")
		check("", address, "0", "pass")
	}
	guard := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()})
	if err := guard.Enable(); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop")
		check("", address, "0", "drop")
		check("", address, "8228", "pass") // sing-box SO_MARK 0x2024
	}
	check(lan, "192.0.2.1", "0", "pass") // local management replies
	check("", "127.0.0.1", "0", "pass")  // runtime API/local rule downloads
	// A new backend recovers the persistent guard rather than opening networking
	// merely because it has no in-memory transition state.
	restarted := newCoreTransitionGuard(&paths.Paths{DataDir: filepath.Dir(guard.(*nftTransitionGuard).statePath)})
	if err := restarted.Disable(); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "pass")
	}
	output, err := exec.Command("nft", "list", "tables").CombinedOutput()
	if err != nil || strings.Contains(string(output), transitionGuardTable) {
		t.Fatal("transition table retained after recovery")
	}
	const establishedProbe = `import socket,sys
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM)
s.settimeout(1)
s.connect((sys.argv[1],15153))
s.sendall(b'before')
assert s.recv(1024)==b'before'
print('ready',flush=True)
sys.stdin.read(1)
try:
 s.sendall(b'after')
 leaked=s.recv(1024)==b'after'
except OSError:leaked=False
sys.exit(1 if leaked else 0)
`
	var established []*exec.Cmd
	var releaseClients []func()
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "pass", "15153", "tcp")
		cmd := exec.Command("nsenter", "-t", lan, "-n", "python3", "-u", "-c", establishedProbe, address)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		stdin, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
		var ready [6]byte
		if _, err := stdout.Read(ready[:]); err != nil || string(ready[:]) != "ready\n" {
			t.Fatalf("existing TCP flow not ready: %v", err)
		}
		established = append(established, cmd)
		releaseClients = append(releaseClients, func() { _, _ = stdin.Write([]byte("x")); _ = stdin.Close() })
	}

	// Persistent device policy survives both DNS listener death and daemon
	// recreation. fw4 reload reads the same file without needing either process.
	deviceGuard := &nftDeviceProtection{path: filepath.Join(t.TempDir(), "guard.nft"), supported: func() bool { return true }, run: func(input string, args ...string) ([]byte, error) {
		cmd := exec.Command("nft", args...)
		cmd.Stdin = strings.NewReader(input)
		return cmd.CombinedOutput()
	}}
	sources := []string{"192.0.2.2/32", "2001:db8:1::2/128"}
	if err := deviceGuard.Apply(sources); err != nil {
		t.Fatal(err)
	}
	for i, cmd := range established {
		releaseClients[i]()
		if err := cmd.Wait(); err != nil {
			t.Fatalf("established TCP leaked after guard installation: %v", err)
		}
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop")
		check(lan, address, "0", "drop", "15153", "tcp")
		check("", address, "0", "pass") // Other router traffic is unaffected.
	}
	check(lan, "192.0.2.1", "0", "pass") // Keep management reachable.
	dns4 := server("", "192.0.2.1", "1054")
	dns6 := server("", "2001:db8:1::1", "1054")
	dnsTCP4 := server("", "192.0.2.1", "1054", "tcp")
	dnsTCP6 := server("", "2001:db8:1::1", "1054", "tcp")
	for _, address := range []string{"192.0.2.1", "198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "pass", "53")
		check(lan, address, "0", "pass", "53", "tcp")
	}
	_ = dns4.Process.Kill()
	_ = dns4.Wait()
	_ = dns6.Process.Kill()
	_ = dns6.Wait()
	_ = dnsTCP4.Process.Kill()
	_ = dnsTCP4.Wait()
	_ = dnsTCP6.Process.Kill()
	_ = dnsTCP6.Wait()
	for _, address := range []string{"192.0.2.1", "198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop", "53")
		check(lan, address, "0", "drop", "53", "tcp")
	}
	// Simulate fw4 boot/reload, even with both daemon and core absent.
	run("nft", "-f", deviceGuard.path)
	newDaemon := &nftDeviceProtection{path: deviceGuard.path, supported: deviceGuard.supported, run: deviceGuard.run}
	if err := newDaemon.Verify(sources); err != nil {
		t.Fatal(err)
	}
	check(lan, "198.51.100.2", "0", "drop")
	check(lan, "2001:db8:2::2", "0", "drop")
	if err := newDaemon.Disable(); err != nil {
		t.Fatal(err)
	}
	check(lan, "198.51.100.2", "0", "pass")
	check(lan, "2001:db8:2::2", "0", "pass")
}
