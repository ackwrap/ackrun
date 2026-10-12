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
	const echo = `import socket,sys
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_DGRAM)
s.bind((sys.argv[1],15153))
print('ready',flush=True)
while True:
 data,peer=s.recvfrom(1024)
 s.sendto(data,peer)
`
	server := func(pid, address string) {
		t.Helper()
		args := []string{"python3", "-u", "-c", echo, address}
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
	}
	server(wan, "198.51.100.2")
	server(wan, "2001:db8:2::2")
	server("", "192.0.2.1")
	server("", "127.0.0.1")
	// Kernel-generated link-local addresses still perform DAD even when the
	// synthetic global addresses use nodad. Wait before the IPv6 baseline.
	time.Sleep(1500 * time.Millisecond)
	const probe = `import socket,sys
s=socket.socket(socket.AF_INET6 if ':' in sys.argv[1] else socket.AF_INET,socket.SOCK_DGRAM)
s.settimeout(2 if sys.argv[3]=='pass' else 0.5)
if int(sys.argv[2]): s.setsockopt(socket.SOL_SOCKET,36,int(sys.argv[2]))
try:
 s.sendto(b'test',(sys.argv[1],15153))
 data=s.recv(1024)
 ok=data==b'test'
except OSError: ok=False
sys.exit(0 if ok==(sys.argv[3]=='pass') else 1)
`
	check := func(pid, address, mark, want string) {
		t.Helper()
		args := []string{"python3", "-c", probe, address, mark, want}
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
}
