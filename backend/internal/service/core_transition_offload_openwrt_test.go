//go:build linux

package service

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/paths"
)

// These helpers extend TestNFTGuardPackets, reusing its peers, echo servers,
// cached TCP/UDP sockets and fast-path proof. All command results come from the
// installed OpenWrt tools. Fault hooks change real inputs or kill a real child;
// they never manufacture successful UCI, fw4, nft or conntrack responses.
func requireOpenWrtOffloadIsolation(t *testing.T) {
	t.Helper()
	if os.Getenv("ACKWRAP_OPENWRT_INTEGRATION") != "1" {
		t.Fatal("real OpenWrt mode must be explicitly enabled")
	}
	if os.Geteuid() != 0 {
		t.Fatal("requires root in a disposable OpenWrt VM")
	}
	for _, kind := range []string{"net", "mnt"} {
		self, err := os.Readlink("/proc/self/ns/" + kind)
		host, hostErr := os.Readlink("/proc/1/ns/" + kind)
		if err != nil || hostErr != nil || self == host {
			t.Fatalf("requires a private %s namespace", kind)
		}
	}
	work := os.Getenv("ACKWRAP_OPENWRT_WORK")
	if !filepath.IsAbs(work) {
		t.Fatal("missing isolated work directory")
	}
	for mounted, original := range map[string]string{"/etc/config": work + "/config", "/tmp": work + "/tmp"} {
		a, err := os.Stat(mounted)
		b, otherErr := os.Stat(original)
		if err != nil || otherErr != nil || !os.SameFile(a, b) {
			t.Fatalf("%s is not the disposable bind mount", mounted)
		}
	}
	for _, path := range []string{openWrtReleasePath, openWrtUCIPath, fw4CommandPath, "/proc/net/nf_conntrack"} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("required real OpenWrt capability %s: %v", path, err)
		}
	}
}

func openWrtCommand(t *testing.T, name string, args ...string) []byte {
	t.Helper()
	out, err := runDNSMasqCommand("", name, args...)
	if err != nil {
		t.Fatalf("real command %s %v failed: %v\n%s", name, args, err, out)
	}
	return out
}

func openWrtSnapshot(t *testing.T, label string, guards ...*nftTransitionGuard) {
	t.Helper()
	label = strings.NewReplacer("/", "_", " ", "_").Replace(t.Name() + "-" + label)
	root := filepath.Join(os.Getenv("ACKWRAP_OPENWRT_WORK"), "evidence", label)
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	for name, args := range map[string][]string{
		"nft.json":     {"nft", "-j", "list", "ruleset"},
		"firewall.uci": {openWrtUCIPath, "export", "firewall"},
		"pending.uci":  {openWrtUCIPath, "changes", "firewall"},
	} {
		out, err := runDNSMasqCommand("", args[0], args[1:]...)
		if err != nil {
			t.Errorf("capture %s: %v", name, err)
		}
		if err := os.WriteFile(filepath.Join(root, name), out, 0600); err != nil {
			t.Error(err)
		}
	}
	data, err := os.ReadFile("/proc/net/nf_conntrack")
	if err != nil {
		t.Error(err)
	} else if err := os.WriteFile(filepath.Join(root, "conntrack.txt"), data, 0600); err != nil {
		t.Error(err)
	}
	for _, guard := range guards {
		for _, path := range []string{guard.statePath, guard.offload.path} {
			data, err := os.ReadFile(path)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				t.Error(err)
				continue
			}
			if err := os.WriteFile(filepath.Join(root, filepath.Base(path)), data, 0600); err != nil {
				t.Error(err)
			}
		}
	}
}

func openWrtUnrelated(t *testing.T) string {
	t.Helper()
	var lines []string
	for line := range strings.SplitSeq(string(openWrtCommand(t, openWrtUCIPath, "export", "firewall")), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "option flow_offloading ") &&
			!strings.HasPrefix(strings.TrimSpace(line), "option flow_offloading_hw ") {
			lines = append(lines, line)
		}
	}
	return strings.Join(lines, "\n")
}

func prepareOpenWrtFW4(t *testing.T, g *nftTransitionGuard) {
	t.Helper()
	requireOpenWrtOffloadIsolation(t)
	// Explicit devices avoid depending on netifd state from the VM's management
	// interface. ubusd and /sys also belong to the isolated fixture.
	const config = `config defaults
 option input 'ACCEPT'
 option output 'ACCEPT'
 option forward 'ACCEPT'
 option synflood_protect '1'
 option auto_includes '0'
 option flow_offloading '1'
 option flow_offloading_hw '0'
config zone 'fixture'
 option name 'fixture'
 list device 'guard-lan'
 list device 'guard-wan'
 list device 'guard-vpn'
 list device 'guard-loc'
 option input 'ACCEPT'
 option output 'ACCEPT'
 option forward 'ACCEPT'
`
	if err := os.WriteFile("/etc/config/firewall", []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	openWrtCommand(t, fw4CommandPath, "start")
	openWrtCommand(t, fw4CommandPath, "check")
	if !g.offload.supported() {
		t.Fatal("production offload manager rejected this OpenWrt fixture")
	}
	original, err := g.offload.current()
	if err != nil || original.Software == nil || *original.Software != "1" {
		t.Fatalf("real UCI snapshot: %+v %v", original, err)
	}
	unrelated := openWrtUnrelated(t)
	// fw4 reload must neither remove foreign tables nor change unrelated UCI.
	openWrtCommand(t, "nft", "add", "table", "inet", "ackrun_it_sentinel")
	t.Cleanup(func() {
		openWrtSnapshot(t, "final", g)
		if got := openWrtUnrelated(t); got != unrelated {
			t.Errorf("unrelated firewall configuration changed:\n%s", got)
		}
		openWrtCommand(t, "nft", "list", "table", "inet", "ackrun_it_sentinel")
		openWrtCommand(t, "nft", "delete", "table", "inet", "ackrun_it_sentinel")
	})
	openWrtSnapshot(t, "baseline")
}

func verifyOpenWrtPaused(t *testing.T, g *nftTransitionGuard) {
	t.Helper()
	record, exists, err := g.offload.readRecord()
	if err != nil || !exists {
		t.Fatalf("missing durable original settings: %v", err)
	}
	actual, err := g.offload.current()
	if err != nil || !reflect.DeepEqual(actual, pausedOffloadOptions(record.Original)) {
		t.Fatalf("offload was not paused: %+v %v", actual, err)
	}
	data := openWrtCommand(t, "nft", "-j", "list", "ruleset")
	if err := checkNFTFlowtables(data, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitForFlowOffloadDrain(); err != nil {
		t.Fatal(err)
	}
	openWrtSnapshot(t, "paused", g)
}

func verifyOpenWrtRestored(t *testing.T, g *nftTransitionGuard) {
	t.Helper()
	for _, path := range []string{g.statePath, g.offload.path} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("recovery state not removed: %s: %v", path, err)
		}
	}
	current, err := g.offload.current()
	if err != nil || current.Software == nil || *current.Software != "1" || current.Hardware == nil || *current.Hardware != "0" {
		t.Fatalf("original options not restored: %+v %v", current, err)
	}
	data := openWrtCommand(t, "nft", "-j", "list", "ruleset")
	if !standardFW4Flowtables(data) || !bytes.Contains(data, []byte(`"flowtable"`)) {
		t.Fatal("real fw4 did not recreate its flowtable")
	}
	openWrtSnapshot(t, "restored", g)
}

func testOpenWrtOffloadFaults(t *testing.T, packets func(string)) {
	t.Helper()
	for _, scenario := range []string{"pending-before", "pending-during", "committed-unrelated", "committed-offload", "reload-pause", "reload-restore", "corrupt-record", "permanent-policy"} {
		t.Run(scenario, func(t *testing.T) {
			g := newCoreTransitionGuard(&paths.Paths{DataDir: t.TempDir()}).(*nftTransitionGuard)
			original, err := g.offload.current()
			if err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile("/etc/config/firewall")
			if err != nil {
				t.Fatal(err)
			}
			unrelated := openWrtUnrelated(t)
			// Restore only our disposable fixture between cases, never a live
			// firewall. Capture failures before cleanup removes fault inputs.
			t.Cleanup(func() {
				openWrtSnapshot(t, "result", g)
				openWrtCommand(t, openWrtUCIPath, "revert", "firewall")
				if err := os.WriteFile("/etc/config/firewall", before, 0600); err != nil {
					t.Error(err)
				}
				for _, table := range []string{transitionGuardTable, deviceProtectionTable} {
					_, _ = g.run("", "delete", "table", "inet", table)
				}
				openWrtCommand(t, fw4CommandPath, "reload")
			})
			var pending []byte
			setPending := func() {
				openWrtCommand(t, openWrtUCIPath, "set", "firewall.fixture.input=DROP")
				pending = openWrtCommand(t, openWrtUCIPath, "changes", "firewall")
			}
			if scenario == "pending-before" {
				setPending()
			}
			injected := false
			realRun := func(input, path string, args ...string) ([]byte, error) {
				out, err := runDNSMasqCommand(input, path, args...)
				if err != nil {
					t.Logf("real command %s %v: %v\n%s", path, args, err, out)
				}
				return out, err
			}
			g.offload.run = func(input, path string, args ...string) ([]byte, error) {
				out, err := realRun(input, path, args...)
				if !injected && err == nil && path == openWrtUCIPath && len(args) == 5 && args[3] == "set" {
					switch scenario {
					case "pending-during":
						setPending()
					case "committed-unrelated":
						openWrtCommand(t, openWrtUCIPath, "set", "firewall.@defaults[0].synflood_protect=0")
						openWrtCommand(t, openWrtUCIPath, "commit", "firewall")
						unrelated = openWrtUnrelated(t)
					case "committed-offload":
						openWrtCommand(t, openWrtUCIPath, "set", "firewall.@defaults[0].flow_offloading_hw=1")
						openWrtCommand(t, openWrtUCIPath, "commit", "firewall")
					}
					injected = true
				}
				return out, err
			}
			// Inject a genuine nft parser error after fw4 check, immediately
			// before the requested reload. No synthetic command error is used.
			badReload := func() {
				bad := filepath.Join(os.Getenv("ACKWRAP_OPENWRT_WORK"), "invalid.nft")
				if err := os.WriteFile(bad, []byte("deliberately invalid nft syntax\n"), 0600); err != nil {
					t.Fatal(err)
				}
				for _, value := range []string{"firewall.it_failure=include", "firewall.it_failure.type=nftables", "firewall.it_failure.position=table-pre", "firewall.it_failure.path=" + bad} {
					openWrtCommand(t, openWrtUCIPath, "set", value)
				}
				openWrtCommand(t, openWrtUCIPath, "commit", "firewall")
			}
			if scenario == "reload-pause" {
				g.offload.run = func(input, path string, args ...string) ([]byte, error) {
					if path == fw4CommandPath && args[len(args)-1] == "reload" {
						badReload()
					}
					return realRun(input, path, args...)
				}
			}
			err = g.Enable([]string{"192.168.50.2", "fd12:3456::2"}, nil)
			t.Logf("Enable result: %v", err)
			wantFailure := scenario == "pending-before" || scenario == "pending-during" || scenario == "committed-offload" || scenario == "reload-pause"
			if (err != nil) != wantFailure {
				t.Fatalf("Enable: %v (want failure %t)", err, wantFailure)
			}
			if wantFailure {
				if scenario != "pending-before" && errors.Is(err, errTransitionProtectionUnsupported) {
					t.Fatal("pending recovery was downgraded to optional protection")
				}
				if scenario == "pending-before" || scenario == "pending-during" {
					after, readErr := os.ReadFile("/etc/config/firewall")
					if readErr != nil || !bytes.Equal(before, after) {
						t.Fatal("committed somebody else's pending edits")
					}
					if !bytes.Equal(openWrtCommand(t, openWrtUCIPath, "changes", "firewall"), pending) {
						t.Fatal("changed somebody else's pending edits")
					}
				}
				if scenario == "committed-offload" && strings.TrimSpace(string(openWrtCommand(t, openWrtUCIPath, "get", "firewall.@defaults[0].flow_offloading_hw"))) != "1" {
					t.Fatal("overwrote concurrent offload change")
				}
				if scenario == "reload-pause" {
					if _, exists, err := g.offload.readRecord(); err != nil || !exists {
						t.Fatal("lost recovery intent after reload failure", err)
					}
					openWrtCommand(t, openWrtUCIPath, "delete", "firewall.it_failure")
					openWrtCommand(t, openWrtUCIPath, "commit", "firewall")
					g.offload.run = realRun
					if err := g.Enable([]string{"192.168.50.2", "fd12:3456::2"}, nil); err != nil {
						t.Fatal("retry pause", err)
					}
					verifyOpenWrtPaused(t, g)
					packets("drop")
					if err := g.Disable(); err != nil {
						t.Fatal(err)
					}
					verifyOpenWrtRestored(t, g)
				}
				return
			}
			verifyOpenWrtPaused(t, g)
			packets("drop")
			if scenario == "committed-unrelated" {
				if strings.TrimSpace(string(openWrtCommand(t, openWrtUCIPath, "get", "firewall.@defaults[0].synflood_protect"))) != "0" {
					t.Fatal("overwrote committed unrelated edit")
				}
				if openWrtUnrelated(t) != unrelated {
					t.Fatal("pause overwrote unrelated options")
				}
			}
			if scenario == "corrupt-record" {
				record, err := os.ReadFile(g.offload.path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(g.offload.path, []byte(`{"version":99}`), 0600); err != nil {
					t.Fatal(err)
				}
				if err := g.Disable(); err == nil {
					t.Fatal("accepted corrupt recovery record")
				}
				present, _, err := g.inspect()
				if err != nil || !present {
					t.Fatal("removed guard before detecting corrupt recovery state", err)
				}
				packets("drop")
				if err := os.WriteFile(g.offload.path, record, 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "permanent-policy" {
				// Use the existing production renderer, not a stand-in table.
				rules, _, err := deviceProtectionRules([]string{"192.168.50.2", "fd12:3456::2"}, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := g.run(rules, "-f", "-"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "reload-restore" {
				badReload()
				if err := g.Disable(); err == nil {
					t.Fatal("real fw4 reload unexpectedly accepted invalid nft input")
				}
				if _, exists, err := g.offload.readRecord(); err != nil || !exists {
					t.Fatal("lost recovery intent on restore failure", err)
				}
				openWrtCommand(t, openWrtUCIPath, "delete", "firewall.it_failure")
				openWrtCommand(t, openWrtUCIPath, "commit", "firewall")
			}
			if err := g.Disable(); err != nil {
				t.Fatal(err)
			}
			if scenario == "permanent-policy" {
				verifyOpenWrtPaused(t, g)
				openWrtCommand(t, fw4CommandPath, "reload")
				packets("drop")
				openWrtCommand(t, "nft", "delete", "table", "inet", deviceProtectionTable)
				if err := g.Disable(); err != nil {
					t.Fatal(err)
				}
			}
			verifyOpenWrtRestored(t, g)
			actual, err := g.offload.current()
			if err != nil || !reflect.DeepEqual(actual, original) || openWrtUnrelated(t) != unrelated {
				t.Fatal("restore overwrote original offload or unrelated options", err)
			}
			packets("pass")
		})
	}
	for _, checkpoint := range []string{"intent", "commit", "reload", "guard"} {
		t.Run("kill-"+checkpoint, func(t *testing.T) { testOpenWrtOffloadCrash(t, checkpoint, packets) })
	}
}

// Child uses the same test executable and production managers. Parent SIGKILLs
// it at a command boundary, then recovers using a fresh instance and the same
// durable directory. No graceful cleanup runs in the killed process.
func TestOpenWrtOffloadCrashChild(t *testing.T) {
	if os.Getenv("ACKWRAP_OPENWRT_CRASH_CHILD") != "1" {
		t.Skip("started only by the isolated OpenWrt integration test")
	}
	requireOpenWrtOffloadIsolation(t)
	g := newCoreTransitionGuard(&paths.Paths{DataDir: os.Getenv("ACKWRAP_OPENWRT_STATE")}).(*nftTransitionGuard)
	checkpoint := os.Getenv("ACKWRAP_OPENWRT_CHECKPOINT")
	stop := func() {
		if err := os.WriteFile(filepath.Join(filepath.Dir(g.statePath), "checkpoint"), []byte(checkpoint), 0600); err != nil {
			t.Fatal(err)
		}
		for {
			time.Sleep(time.Hour)
		}
	}
	realRun := g.offload.run
	g.offload.run = func(input, path string, args ...string) ([]byte, error) {
		if checkpoint == "intent" && path == openWrtUCIPath && len(args) == 5 && args[3] == "set" {
			stop()
		}
		out, err := realRun(input, path, args...)
		if err == nil && ((checkpoint == "commit" && path == openWrtUCIPath && len(args) == 5 && args[3] == "commit") ||
			(checkpoint == "reload" && path == fw4CommandPath && args[len(args)-1] == "reload")) {
			stop()
		}
		return out, err
	}
	if err := g.Enable([]string{"192.168.50.2", "fd12:3456::2"}, nil); err != nil {
		t.Fatal(err)
	}
	if checkpoint == "guard" {
		stop()
	}
	t.Fatal("did not reach requested crash checkpoint")
}

func testOpenWrtOffloadCrash(t *testing.T, checkpoint string, packets func(string)) {
	t.Helper()
	root := t.TempDir()
	g := newCoreTransitionGuard(&paths.Paths{DataDir: root}).(*nftTransitionGuard)
	before, err := os.ReadFile("/etc/config/firewall")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		openWrtSnapshot(t, "result", g)
		openWrtCommand(t, openWrtUCIPath, "revert", "firewall")
		if err := os.WriteFile("/etc/config/firewall", before, 0600); err != nil {
			t.Error(err)
		}
		_, _ = g.run("", "delete", "table", "inet", transitionGuardTable)
		openWrtCommand(t, fw4CommandPath, "restart")
	})
	cmd := exec.Command(os.Args[0], "-test.run=^TestOpenWrtOffloadCrashChild$", "-test.v", "-test.timeout=40s")
	cmd.Env = append(os.Environ(), "ACKWRAP_OPENWRT_CRASH_CHILD=1", "ACKWRAP_OPENWRT_STATE="+root, "ACKWRAP_OPENWRT_CHECKPOINT="+checkpoint)
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		if !waited {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "checkpoint")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			waited = true
			t.Fatalf("child did not reach %s: %s", checkpoint, output.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("child was not killed")
	}
	waited = true
	if _, exists, err := g.offload.readRecord(); err != nil || !exists {
		t.Fatal("crash lost recovery record", err)
	}
	if checkpoint == "guard" {
		packets("drop")
	}
	if got := openWrtCommand(t, openWrtUCIPath, "changes", "firewall"); len(got) != 0 {
		t.Fatalf("crash left shared UCI deltas: %s", got)
	}
	if err := g.Enable([]string{"192.168.50.2", "fd12:3456::2"}, nil); err != nil {
		t.Fatal("recover pause", err)
	}
	verifyOpenWrtPaused(t, g)
	packets("drop")
	if err := g.Disable(); err != nil {
		t.Fatal("recover restore", err)
	}
	verifyOpenWrtRestored(t, g)
	packets("pass")
}
