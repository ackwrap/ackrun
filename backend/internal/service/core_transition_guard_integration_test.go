//go:build linux

package service

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
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
	if os.Getenv("ACKWRAP_OPENWRT_INTEGRATION") == "1" {
		requireOpenWrtOffloadIsolation(t)
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
		cmd := exec.Command("unshare", "-n", "sleep", "1800")
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
	lan := newPeer("guard-lan", "192.168.50.1", "192.168.50.2", "fd12:3456::1", "fd12:3456::2")
	wan := newPeer("guard-wan", "198.51.100.1", "198.51.100.2", "2001:db8:2::1", "2001:db8:2::2")
	vpn := newPeer("guard-vpn", "192.168.60.1", "192.168.60.2", "fd60::1", "fd60::2")
	local := newPeer("guard-loc", "10.44.0.1", "10.44.0.2", "fd44:1::1", "fd44:1::2")
	fakePools := []string{"10.66.0.0/24", "fd66::/64"}
	fakeTargets := []string{"198.18.0.2", "fc00::2", "10.66.0.2", "fd66::2"}
	for _, address := range []struct{ router, peer, prefix string }{
		{"198.18.0.1", "198.18.0.2", "/24"}, {"fc00::1", "fc00::2", "/64"},
		{"10.66.0.1", "10.66.0.2", "/24"}, {"fd66::1", "fd66::2", "/64"},
		{"10.77.0.1", "10.77.0.2", "/24"}, {"fd77::1", "fd77::2", "/64"},
	} {
		args := []string{"addr", "add", address.router + address.prefix, "dev", "guard-loc"}
		peerArgs := []string{"addr", "add", address.peer + address.prefix, "dev", "guard-loc-peer"}
		if strings.Contains(address.router, ":") {
			args = append(args, "nodad")
			peerArgs = append(peerArgs, "nodad")
		}
		run("ip", args...)
		run("nsenter", append([]string{"-t", local, "-n", "ip"}, peerArgs...)...)
	}
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
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		server(wan, address, "53", "tcp")
		server(wan, address, "53", "udp")
	}
	for _, address := range []string{"192.168.60.1", "fd60::1"} {
		server("", address, "53", "tcp")
		server("", address, "53", "udp")
	}
	server("", "192.168.50.1")
	server("", "127.0.0.1")
	for _, address := range []string{"10.44.0.2", "fd44:1::2"} {
		for _, protocol := range []string{"tcp", "udp"} {
			server(local, address, "15153", protocol)
			server(local, address, "53", protocol)
		}
	}
	for _, address := range append(fakeTargets, "10.77.0.2", "fd77::2") {
		server(local, address)
	}
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
	checkLocal := func() {
		t.Helper()
		for _, address := range []string{"10.44.0.2", "fd44:1::2"} {
			for _, protocol := range []string{"tcp", "udp"} {
				check(lan, address, "0", "pass", "15153", protocol)
				check("", address, "0", "pass", "15153", protocol)
			}
		}
	}
	checkLocal()
	for _, address := range fakeTargets {
		check(lan, address, "0", "pass")
	}
	checkExcluded := func(want string, routerDNS bool) {
		t.Helper()
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			for _, protocol := range []string{"tcp", "udp"} {
				check(vpn, address, "0", want, "15153", protocol)
				check(vpn, address, "0", want, "53", protocol)
			}
		}
		if routerDNS {
			for _, address := range []string{"192.168.60.1", "fd60::1"} {
				check(vpn, address, "0", want, "53", "tcp")
				check(vpn, address, "0", want, "53", "udp")
			}
		}
	}
	guard := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()})
	if err := guard.Enable(nil, []string{"guard-vpn"}, fakePools...); err != nil {
		t.Fatal(err)
	}
	for _, address := range fakeTargets {
		check(lan, address, "0", "drop")
		check("", address, "0", "drop")
	}
	checkExcluded("pass", true)
	// Removing an exemption must close even connections established above.
	if err := guard.Enable(nil, nil, fakePools...); err != nil {
		t.Fatal(err)
	}
	checkExcluded("drop", false)
	// A subsequent reload retains the old pools as well as the replacement.
	if err := guard.Enable(nil, nil, "10.77.0.0/24", "fd77::/64"); err != nil {
		t.Fatal(err)
	}
	for _, address := range append(fakeTargets, "10.77.0.2", "fd77::2") {
		check(lan, address, "0", "drop")
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop")
		check("", address, "0", "drop")
		check("", address, "8228", "pass") // sing-box SO_MARK 0x2024
	}
	checkLocal()
	for _, address := range []string{"10.44.0.2", "fd44:1::2"} {
		check(lan, address, "0", "drop", "53")
		check("", address, "0", "drop", "53")
		check("", address, "8228", "pass", "53")
	}
	check(lan, "192.168.50.1", "0", "pass") // local management replies
	check("", "127.0.0.1", "0", "pass")     // runtime API/local rule downloads
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
	t.Run("device-scoped-transition-and-flowtable", func(t *testing.T) {
		// A real IPv6 flowtable must coexist with IPv4-only protection. It
		// cannot be declared unrelated once an IPv6 or MAC identity is added.
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(`table ip6 guard_test_offload {
 flowtable ft { hook ingress priority 0; devices = { guard-lan, guard-wan, guard-vpn }; }
 chain forward { type filter hook forward priority 0; policy accept; meta l4proto { tcp, udp } flow add @ft; }
}`)
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("install isolated test flowtable: %v %s", err, data)
		}
		scoped := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()}).(*nftTransitionGuard)
		if err := scoped.Enable([]string{"192.168.50.2"}, nil, fakePools...); err != nil {
			t.Fatal(err)
		}
		check(lan, "198.51.100.2", "0", "drop")
		check(lan, "2001:db8:2::2", "0", "pass") // IP-only does not cover IPv6.
		check(lan, "192.168.60.1", "0", "drop", "53")
		checkExcluded("pass", true)
		check("", "198.51.100.2", "0", "pass") // Router output is unscoped.
		before, err := os.ReadFile(scoped.statePath)
		if err != nil {
			t.Fatal(err)
		}
		for _, values := range [][]string{{"fd12:3456::2"}, {"02:00:00:00:00:02"}} {
			if err := scoped.Enable(values, nil); err == nil || !strings.Contains(err.Error(), "guard_test_offload") {
				t.Fatalf("missing conflicting table diagnostic: %v", err)
			}
		}
		after, err := os.ReadFile(scoped.statePath)
		if err != nil || string(after) != string(before) {
			t.Fatal("failed widening overwrote recovery state")
		}
		if err := scoped.Disable(); err != nil {
			t.Fatal(err)
		}
		run("nft", "delete", "table", "ip6", "guard_test_offload")

		data, err := exec.Command("nsenter", "-t", lan, "-n", "ip", "-j", "link", "show", "guard-lan-peer").Output()
		var links []struct{ Address string }
		if err != nil || json.Unmarshal(data, &links) != nil || len(links) != 1 || links[0].Address == "" {
			t.Fatal("missing test device MAC")
		}
		for _, values := range [][]string{
			{"192.168.50.0/24", "fd12:3456::2"},
			{links[0].Address},
			{"192.168.50.2", links[0].Address},
		} {
			if err := scoped.Enable(values, nil, fakePools...); err != nil {
				t.Fatal(err)
			}
			for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
				for _, protocol := range []string{"tcp", "udp"} {
					check(lan, address, "0", "drop", "15153", protocol)
					check(vpn, address, "0", "pass", "15153", protocol)
				}
			}
			for _, address := range []string{"192.168.60.1", "fd60::1", "10.44.0.2", "fd44:1::2"} {
				check(lan, address, "0", "drop", "53")
			}
			for _, address := range []string{"10.66.0.2", "fd66::2"} {
				check(lan, address, "0", "drop")
			}
			checkLocal()
			check(lan, "192.168.50.1", "0", "pass")
			if err := scoped.Disable(); err != nil {
				t.Fatal(err)
			}
		}
	})
	realOpenWrt := os.Getenv("ACKWRAP_OPENWRT_INTEGRATION") == "1"
	offloadTestName := "fw4-offload-pause-and-restore"
	if realOpenWrt {
		offloadTestName = "openwrt-real-fw4"
	}
	t.Run(offloadTestName, func(t *testing.T) {
		g := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()}).(*nftTransitionGuard)
		if realOpenWrt {
			prepareOpenWrtFW4(t, g)
		} else {
			f := newFW4OffloadFixture(t)
			// UCI is modeled in-memory; exercise fw4's actual atomic table rebuild
			// and old-flow deletion with the real kernel, sockets and nft compiler.
			f.manager.nft = g.run
			// Some host kernels omit CONFIG_NF_CONNTRACK_PROCFS; the cached-flow
			// probe below still verifies deletion with real packets on those hosts.
			if _, err := os.Stat("/proc/net/nf_conntrack"); err == nil {
				f.manager.drained = waitForFlowOffloadDrain
			}
			uciRun := f.manager.run
			reload := func() ([]byte, error) {
				var b strings.Builder
				b.WriteString("add table inet fw4\nflush table inet fw4\n")
				data, err := g.run("", "-j", "list", "ruleset")
				if err != nil {
					return nil, err
				}
				if strings.Contains(string(data), `"flowtable"`) {
					b.WriteString("delete flowtable inet fw4 ft\n")
				}
				if f.options.Software != nil && *f.options.Software == "1" {
					b.WriteString(`table inet fw4 {
	 flowtable ft { hook ingress priority 0; devices = { guard-lan, guard-wan, guard-vpn }; counter; }
	 chain forward { type filter hook forward priority 0; policy accept; meta l4proto { tcp, udp } flow add @ft; }
	}`)
				}
				return g.run(b.String(), "-f", "-")
			}
			f.manager.run = func(input, path string, args ...string) ([]byte, error) {
				if path == fw4CommandPath {
					if args[len(args)-1] == "reload" {
						return reload()
					}
					return nil, nil
				}
				return uciRun(input, path, args...)
			}
			g.offload = f.manager
			if output, err := reload(); err != nil {
				t.Fatalf("test fw4 reload: %v %s", err, output)
			}
		}
		const client = `import socket,sys,time
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM if sys.argv[2]=='tcp' else socket.SOCK_DGRAM)
s.settimeout(1)
s.connect((sys.argv[1],15153))
for i in range(8):
 s.send(b'before'); assert s.recv(1024)==b'before'; time.sleep(.02)
print('ready',flush=True)
sys.stdin.read(1)
s.send(b'cached'); assert s.recv(1024)==b'cached'
print('cached',flush=True)
sys.stdin.read(1)
try:
 s.send(b'after'); leaked=s.recv(1024)==b'after'
except OSError:leaked=False
sys.exit(1 if leaked else 0)
`
		var clients []*exec.Cmd
		var cachedProbes []func()
		var releases []func()
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			for _, protocol := range []string{"tcp", "udp"} {
				cmd := exec.Command("nsenter", "-t", lan, "-n", "python3", "-u", "-c", client, address, protocol)
				out, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				in, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
				var ready [6]byte
				if _, err := out.Read(ready[:]); err != nil || string(ready[:]) != "ready\n" {
					t.Fatal("flow client did not start", err)
				}
				clients = append(clients, cmd)
				cachedProbes = append(cachedProbes, func() {
					_, _ = in.Write([]byte("x"))
					var reply [7]byte
					if _, err := io.ReadFull(out, reply[:]); err != nil || string(reply[:]) != "cached\n" {
						t.Fatalf("connection did not bypass the forward drop through offload: %v", err)
					}
				})
				releases = append(releases, func() { _, _ = in.Write([]byte("x")); _ = in.Close() })
			}
		}
		// A successful echo despite this drop proves that the warmed flows
		// really entered the fast path, rather than only creating a flowtable.
		run("nft", "insert", "rule", "inet", "fw4", "forward", "drop")
		for _, probe := range cachedProbes {
			probe()
		}
		sources := []string(nil)
		if realOpenWrt {
			sources = []string{"192.168.50.2", "fd12:3456::2"}
		}
		if err := g.Enable(sources, []string{"guard-vpn"}, fakePools...); err != nil {
			t.Fatal(err)
		}
		for i, cmd := range clients {
			releases[i]()
			if err := cmd.Wait(); err != nil {
				t.Fatal("cached connection bypassed transition guard", err)
			}
		}
		checkExcluded("pass", true)
		if realOpenWrt {
			verifyOpenWrtPaused(t, g)
			// A real fw4 reload must preserve the independent transition table.
			run(fw4CommandPath, "reload")
			for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
				for _, protocol := range []string{"tcp", "udp"} {
					check(lan, address, "0", "drop", "15153", protocol)
					check(lan, address, "0", "drop", "53", protocol)
					check("", address, "0", "pass", "15153", protocol)
				}
			}
			for _, address := range []string{"192.168.60.1", "fd60::1"} {
				for _, protocol := range []string{"tcp", "udp"} {
					check(lan, address, "0", "drop", "53", protocol)
				}
			}
			for _, address := range fakeTargets {
				check(lan, address, "0", "drop")
			}
			checkLocal()
			check(lan, "192.168.50.1", "0", "pass")
		}
		if err := g.Disable(); err != nil {
			t.Fatal(err)
		}
		data, err := g.run("", "-j", "list", "ruleset")
		if err != nil || !strings.Contains(string(data), `"flowtable"`) {
			t.Fatal("original flowtable was not restored", err)
		}
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			check(lan, address, "0", "pass")
		}
		if realOpenWrt {
			verifyOpenWrtRestored(t, g)
			testOpenWrtOffloadFaults(t, func(want string) {
				for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
					for _, protocol := range []string{"tcp", "udp"} {
						check(lan, address, "0", want, "15153", protocol)
					}
				}
			})
		}
		run("nft", "delete", "table", "inet", "fw4")
	})
	if realOpenWrt {
		return // Other packet scenarios are covered by the existing Linux mode.
	}
	const establishedProbe = `import socket,sys
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_STREAM)
s.settimeout(1)
s.connect((sys.argv[1],int(sys.argv[2])))
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
	for _, target := range []struct{ address, port string }{
		{"198.51.100.2", "15153"}, {"2001:db8:2::2", "15153"},
		{"10.44.0.2", "53"}, {"fd44:1::2", "53"},
	} {
		check(lan, target.address, "0", "pass", target.port, "tcp")
		cmd := exec.Command("nsenter", "-t", lan, "-n", "python3", "-u", "-c", establishedProbe, target.address, target.port)
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
	sources := []string{"192.168.50.2/32", "fd12:3456::2/128", "192.168.60.2/32", "fd60::2/128"}
	// A public destination DNATed to a LAN relay must not inherit the local
	// exception. Exercise both NAT families with actual packets.
	_, err = deviceGuard.run(`table inet guard_test_nat {
	 chain prerouting {
	  type nat hook prerouting priority dstnat; policy accept;
	  ip daddr 203.0.113.9 udp dport 15153 dnat ip to 10.44.0.2
	  ip6 daddr 2001:db8:ff::9 udp dport 15153 dnat ip6 to fd44:1::2
	 }
}`, "-f", "-")
	if err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"203.0.113.9", "2001:db8:ff::9"} {
		check(lan, address, "0", "pass")
	}
	if err := deviceGuard.Apply(sources, nil, fakePools...); err != nil {
		t.Fatal(err)
	}
	for _, address := range fakeTargets {
		check(lan, address, "0", "drop")
	}
	checkLocal()
	for _, address := range []string{"203.0.113.9", "2001:db8:ff::9"} {
		check(lan, address, "0", "drop")
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
	check(lan, "192.168.50.1", "0", "pass") // Keep management reachable.
	dns4 := server("", "192.168.50.1", "1054")
	dns6 := server("", "fd12:3456::1", "1054")
	dnsTCP4 := server("", "192.168.50.1", "1054", "tcp")
	dnsTCP6 := server("", "fd12:3456::1", "1054", "tcp")
	for _, address := range []string{"192.168.50.1", "198.51.100.2", "2001:db8:2::2", "10.44.0.2", "fd44:1::2"} {
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
	checkLocal()
	for _, address := range []string{"192.168.50.1", "198.51.100.2", "2001:db8:2::2", "10.44.0.2", "fd44:1::2"} {
		check(lan, address, "0", "drop", "53")
		check(lan, address, "0", "drop", "53", "tcp")
	}
	// Simulate fw4 boot/reload, even with both daemon and core absent.
	run("nft", "-f", deviceGuard.path)
	newDaemon := &nftDeviceProtection{path: deviceGuard.path, supported: deviceGuard.supported, run: deviceGuard.run}
	if err := newDaemon.Verify(sources, nil); err != nil {
		t.Fatal(err)
	}
	for _, address := range fakeTargets {
		check(lan, address, "0", "drop")
	}
	if err := newDaemon.Apply(sources, nil, "10.77.0.0/24", "fd77::/64"); err != nil {
		t.Fatal(err)
	}
	for _, address := range append(fakeTargets, "10.77.0.2", "fd77::2") {
		check(lan, address, "0", "drop")
	}
	checkLocal()
	// Explicit ingress interfaces bypass both DNS redirect tables and the
	// persistent guard, including public TCP/UDP and router-local DNS.
	excludedDNS := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "excluded-dns"), run: deviceGuard.run}
	if err := excludedDNS.Activate(sources, "guard-vpn"); err != nil {
		t.Fatal(err)
	}
	if err := newDaemon.Apply(sources, []string{"guard-vpn"}); err != nil {
		t.Fatal(err)
	}
	run("nft", "-f", newDaemon.path) // Exemptions survive fw4/daemon reloads.
	if err := newDaemon.Verify(sources, []string{"guard-vpn"}); err != nil {
		t.Fatal(err)
	}
	checkExcluded("pass", true)
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop")
		check(lan, address, "0", "drop", "53")
	}
	for _, address := range fakeTargets {
		check(lan, address, "0", "drop")
	}
	if err := newDaemon.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	checkExcluded("drop", true)
	if err := excludedDNS.Restore(); err != nil {
		t.Fatal(err)
	}
	// Even an empty remaining LAN range must not render an unconditional allow.
	if err := newDaemon.Apply(sources, nil, "0.0.0.0/0", "::/0"); err != nil {
		t.Fatal(err)
	}
	check(lan, "10.44.0.2", "0", "drop")
	check(lan, "fd44:1::2", "0", "drop")
	check(lan, "198.51.100.2", "0", "drop")
	check(lan, "2001:db8:2::2", "0", "drop")
	// Force reset also recovers a damaged policy file and an orphaned device
	// DNS redirect. Ordinary repair must never implicitly release these guards.
	deviceDNS := &nftDeviceDNS{statePath: filepath.Join(t.TempDir(), "device-dns"), run: deviceGuard.run}
	if err := deviceDNS.Activate(sources); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(deviceDNS.statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newDaemon.path, []byte("damaged policy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := resetDeviceProtectionForRepair(&paths.Paths{DBPath: filepath.Join(t.TempDir(), "missing.db")}, newDaemon); err != nil {
		t.Fatal(err)
	}
	if err := deviceDNS.restore(true); err != nil {
		t.Fatal(err)
	}
	transitionPaths := &paths.Paths{DataDir: t.TempDir()}
	transition := newCoreTransitionGuard(transitionPaths)
	if err := transition.Enable(nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(transitionPaths.CoreTransitionGuardPath()); err != nil {
		t.Fatal(err)
	}
	if err := resetPlatformTransitionGuard(transitionPaths); err != nil {
		t.Fatal(err)
	}
	check(lan, "198.51.100.2", "0", "pass")
	check(lan, "2001:db8:2::2", "0", "pass")
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		for _, protocol := range []string{"tcp", "udp"} {
			check(lan, address, "0", "pass", "53", protocol)
		}
	}

	// MAC-only protection remains effective across a changed source address.
	output, err = exec.Command("nsenter", "-t", lan, "-n", "ip", "-j", "link", "show", "guard-lan-peer").Output()
	if err != nil {
		t.Fatal(err)
	}
	var links []struct {
		Address string `json:"address"`
	}
	if json.Unmarshal(output, &links) != nil || len(links) != 1 {
		t.Fatal("missing synthetic peer MAC")
	}
	macSources := []string{links[0].Address}
	// A stale DNAT to another LAN resolver at the same port as our new DNS
	// listener must not inherit the local-network forwarding exception.
	server(local, "10.44.0.2", "1056")
	if _, err := newDaemon.run(`table inet mac_dns_legacy {
 chain prerouting {
 type nat hook prerouting priority dstnat; policy accept;
 ip daddr 10.44.0.2 udp dport 53 dnat ip to 10.44.0.2:1056
 }
}`, "-f", "-"); err != nil {
		t.Fatal(err)
	}
	checkLegacyDNS := func(want string) {
		run("nsenter", "-t", lan, "-n", "python3", "-c", `import socket,sys
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.settimeout(.4)
s.bind(('192.168.50.2',30123));s.sendto(b'legacy-dns',('10.44.0.2',53))
try:r=s.recv(100);received=r==b'legacy-dns'
except OSError:received=False
assert received==(sys.argv[1]=='pass')
`, want)
	}
	checkLegacyDNS("pass")
	if err := newDaemon.Apply(macSources, nil); err != nil {
		t.Fatal(err)
	}
	checkLegacyDNS("drop")
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		for _, protocol := range []string{"tcp", "udp"} {
			check(lan, address, "0", "drop", "15153", protocol)
			check(lan, address, "0", "drop", "53", protocol)
			check(vpn, address, "0", "pass", "15153", protocol)
		}
	}
	check(lan, "192.168.50.1", "0", "pass")
	for _, address := range []string{"192.168.50.1", "fd12:3456::1"} {
		check(lan, address, "0", "drop", "53")
	}
	run("nsenter", "-t", lan, "-n", "ip", "addr", "del", "192.168.50.2/24", "dev", "guard-lan-peer")
	run("nsenter", "-t", lan, "-n", "ip", "addr", "add", "192.168.50.3/24", "dev", "guard-lan-peer")
	run("nsenter", "-t", lan, "-n", "ip", "route", "replace", "default", "via", "192.168.50.1")
	run("nsenter", "-t", lan, "-n", "ip", "-6", "addr", "del", "fd12:3456::2/64", "dev", "guard-lan-peer")
	run("nsenter", "-t", lan, "-n", "ip", "-6", "addr", "add", "fd12:3456::3/64", "dev", "guard-lan-peer", "nodad")
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "drop")
	}
	run("nft", "-f", newDaemon.path)
	if err := newDaemon.Verify(macSources, nil); err != nil {
		t.Fatal(err)
	}
	if binary := os.Getenv("ACKWRAP_SINGBOX_114_BIN"); binary != "" {
		resolver := exec.Command("nsenter", "-t", wan, "-n", "python3", "-u", "-c", `import socket
s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM)
s.bind(('198.51.100.2',15354))
print('ready',flush=True)
while True:
 d,p=s.recvfrom(4096)
 s.sendto(d[:2]+b'\x81\x80'+d[4:6]+b'\x00\x00\x00\x00\x00\x00'+d[12:],p)
`)
		stdout, err := resolver.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := resolver.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = resolver.Process.Kill(); _ = resolver.Wait() })
		var ready [6]byte
		if _, err := stdout.Read(ready[:]); err != nil || string(ready[:]) != "ready\n" {
			t.Fatal("DNS responder not ready")
		}
		run("ip", "route", "replace", "default", "via", "198.51.100.2")
		run("ip", "-6", "route", "replace", "default", "via", "2001:db8:2::2")
		startCore := func(identity []string) *exec.Cmd {
			t.Helper()
			known := deviceSourceRule(identity, map[string]interface{}{})
			known["invert"] = true
			tun := generatedTUNInbound(true, defaultTUNIPv4Address, defaultTUNIPv6Address, nil, nil)
			tun["exclude_mac_address"] = macSources
			config := map[string]interface{}{
				"inbounds":  []map[string]interface{}{tun, {"type": "tproxy", "tag": deviceMACInboundTag, "listen": "::", "listen_port": deviceMACPort}, {"type": "direct", "tag": deviceMACDNSInboundTag, "listen": "::", "listen_port": deviceMACDNSPort}},
				"outbounds": []map[string]interface{}{{"type": "direct", "tag": "direct"}},
				"dns": map[string]interface{}{
					"servers": []map[string]interface{}{{"type": "udp", "tag": "bound", "server": "198.51.100.2", "server_port": 15354}, {"type": "udp", "tag": "ordinary", "server": "198.51.100.2", "server_port": 15355}},
					"rules":   []map[string]interface{}{deviceSourceRule(identity, map[string]interface{}{"server": "bound"}), {"inbound": deviceMACInbounds(), "action": "reject"}},
					"final":   "ordinary",
				},
				"route": map[string]interface{}{"rules": []map[string]interface{}{
					{"type": "logical", "mode": "and", "rules": []map[string]interface{}{{"inbound": deviceMACInbounds()}, known}, "action": "reject"},
					{"inbound": deviceMACDNSInboundTag, "action": "hijack-dns"},
					{"port": 53, "action": "hijack-dns"},
					deviceSourceRule(identity, map[string]interface{}{"action": "route", "outbound": "direct"}),
					{"inbound": deviceMACInboundTag, "action": "reject"},
				}, "final": "direct", "auto_detect_interface": true},
			}
			data, _ := json.Marshal(config)
			path := filepath.Join(t.TempDir(), "core.json")
			if err := os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(binary, "run", "-c", path)
			logFile, err := os.Create(filepath.Join(t.TempDir(), "core.log"))
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stdout, cmd.Stderr = logFile, logFile
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_ = cmd.Process.Kill()
				_ = cmd.Wait()
				_ = logFile.Close()
				if t.Failed() {
					data, _ := os.ReadFile(logFile.Name())
					t.Logf("synthetic core: %s", data)
				}
			})
			time.Sleep(300 * time.Millisecond)
			return cmd
		}
		// Model lookup failure with an identity absent from the core's match
		// set; its ordinary final is deliberately direct to expose fallthrough.
		core := startCore([]string{"02:00:00:00:ff:fe"})
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			for _, protocol := range []string{"tcp", "udp"} {
				check(lan, address, "0", "drop", "15153", protocol)
				check(lan, address, "0", "drop", "53", protocol)
			}
		}
		_ = core.Process.Signal(syscall.SIGTERM)
		_ = core.Wait()
		core = startCore(macSources)
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			for _, protocol := range []string{"tcp", "udp"} {
				check(lan, address, "0", "pass", "15153", protocol)
				check(vpn, address, "0", "pass", "15153", protocol)
			}
		}
		linkData, err := exec.Command("ip", "-j", "-6", "addr", "show", "dev", "guard-lan").Output()
		if err != nil {
			t.Fatal(err)
		}
		var interfaces []struct {
			Addresses []struct{ Local, Scope string } `json:"addr_info"`
		}
		if json.Unmarshal(linkData, &interfaces) != nil {
			t.Fatal("link-local address parse failed")
		}
		dnsTargets := []string{"192.168.50.1", "198.51.100.2", "2001:db8:2::2"}
		for _, iface := range interfaces {
			for _, addr := range iface.Addresses {
				if addr.Scope == "link" {
					dnsTargets = append(dnsTargets, addr.Local+"%guard-lan-peer")
				}
			}
		}
		if len(dnsTargets) != 4 {
			t.Fatal("missing IPv6 link-local DNS test target")
		}
		for _, address := range dnsTargets {
			for _, network := range []string{"tcp", "udp"} {
				run("nsenter", "-t", lan, "-n", "python3", "-c", `import socket,sys,struct
tcp=sys.argv[2]=='tcp'
family=socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET
kind=socket.SOCK_STREAM if tcp else socket.SOCK_DGRAM
s=socket.socket(family,kind);s.settimeout(2)
target=socket.getaddrinfo(sys.argv[1],53,family,kind)[0][4]
q=b'\x12\x34\x01\x00\x00\x01\x00\x00\x00\x00\x00\x00\x07example\x03com\x00\x00\x01\x00\x01'
s.connect(target)
s.sendall((struct.pack('!H',len(q)) if tcp else b'')+q)
if tcp:
 size=struct.unpack('!H',s.recv(2))[0]
 r=s.recv(size)
else:r=s.recv(4096)
assert r[:2]==q[:2] and r[2]&128 and r[3]&15==0,repr(r)
`, address, network)
			}
		}
		_ = core.Process.Kill()
		_ = core.Wait()
		for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
			check(lan, address, "0", "drop")
		}
	}
	if err := newDaemon.Disable(); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"198.51.100.2", "2001:db8:2::2"} {
		check(lan, address, "0", "pass")
	}
}
