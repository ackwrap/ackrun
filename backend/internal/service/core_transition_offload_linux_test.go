//go:build linux

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func offloadOption(value string) *string { return &value }

type fw4OffloadFixture struct {
	options                                          fw4OffloadOptions
	staged                                           map[string]fw4OffloadOptions
	flow, guard, device, foreign, supported, changes bool
	fail                                             string
	calls                                            []string
	manager                                          *fw4OffloadPause
}

func newFW4OffloadFixture(t *testing.T) *fw4OffloadFixture {
	t.Helper()
	f := &fw4OffloadFixture{options: fw4OffloadOptions{Section: "cfg012345", Software: offloadOption("1")}, staged: map[string]fw4OffloadOptions{}, flow: true, supported: true}
	f.manager = &fw4OffloadPause{path: filepath.Join(t.TempDir(), "offload.json"), supported: func() bool { return f.supported }, nft: f.nft,
		drained: func() error {
			f.calls = append(f.calls, "drain")
			if f.fail == "drain" {
				return errors.New("old flows remain")
			}
			return nil
		}}
	f.manager.run = func(_ string, path string, args ...string) ([]byte, error) {
		if path == fw4CommandPath {
			action := args[len(args)-1]
			f.calls = append(f.calls, action)
			if f.fail == action {
				return nil, errors.New("fw4 failed")
			}
			if action == "reload" {
				f.flow = f.options.Software != nil && *f.options.Software == "1"
				if f.flow && f.guard {
					t.Fatal("offload restored before removing transition guard")
				}
			}
			return nil, nil
		}
		if path != openWrtUCIPath {
			t.Fatalf("unexpected command %s %v", path, args)
		}
		switch strings.Join(args, " ") {
		case "-q changes firewall":
			if f.changes {
				return []byte("firewall.other=value"), nil
			}
			return nil, nil
		case "-q -X show firewall":
			text := "firewall." + f.options.Section + "=defaults\nfirewall.zone_lan=zone\nfirewall.zone_lan.input='ACCEPT'\n"
			if f.options.Software != nil {
				text += "firewall." + f.options.Section + ".flow_offloading='" + *f.options.Software + "'\n"
			}
			if f.options.Hardware != nil {
				text += "firewall." + f.options.Section + ".flow_offloading_hw='" + *f.options.Hardware + "'\n"
			}
			return []byte(text), nil
		}
		if len(args) != 5 || args[0] != "-q" || args[1] != "-t" {
			t.Fatalf("UCI mutation without private delta dir: %v", args)
		}
		if _, err := os.Stat(f.manager.path); err != nil {
			t.Fatal("mutated settings before persisting recovery intent")
		}
		dir, action, key := args[2], args[3], args[4]
		f.calls = append(f.calls, action)
		if f.fail == action {
			return nil, errors.New("uci failed")
		}
		staged, ok := f.staged[dir]
		if !ok {
			staged = f.options
		}
		if action == "commit" {
			if key != "firewall" {
				t.Fatal("committed unrelated UCI package")
			}
			f.options = staged
			return nil, nil
		}
		name, value, _ := strings.Cut(key, "=")
		var target **string
		switch name {
		case "firewall." + f.options.Section + ".flow_offloading":
			target = &staged.Software
		case "firewall." + f.options.Section + ".flow_offloading_hw":
			target = &staged.Hardware
		default:
			t.Fatal("mutated unrelated UCI option")
		}
		if action == "delete" {
			*target = nil
		} else if action == "set" {
			*target = offloadOption(value)
		} else {
			t.Fatal("unexpected UCI action")
		}
		f.staged[dir] = staged
		return nil, nil
	}
	return f
}

func (f *fw4OffloadFixture) nft(input string, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "-j list ruleset":
		objects := []any{}
		if f.flow {
			table := "fw4"
			if f.foreign {
				table = "foreign"
			}
			objects = append(objects, map[string]any{"flowtable": map[string]any{"family": "inet", "table": table, "name": "ft", "hook": "ingress"}})
		}
		if f.guard {
			objects = append(objects, map[string]any{"table": map[string]any{"family": "inet", "name": transitionGuardTable, "comment": transitionGuardOwner}})
		}
		if f.device {
			objects = append(objects, map[string]any{"table": map[string]any{"family": "inet", "name": deviceProtectionTable}})
		}
		return json.Marshal(map[string]any{"nftables": objects})
	case "-f -":
		f.calls = append(f.calls, "guard")
		if f.fail == "guard" {
			return nil, errors.New("nft failed")
		}
		if f.flow {
			return nil, errors.New("guard installed with offload still active")
		}
		f.guard = true
		return nil, nil
	case "delete table inet " + transitionGuardTable:
		f.calls = append(f.calls, "unguard")
		if f.fail == "unguard" {
			return nil, errors.New("delete failed")
		}
		f.guard = false
		return nil, nil
	}
	return nil, fmt.Errorf("unexpected nft call %v", args)
}

func (f *fw4OffloadFixture) guardInstance(t *testing.T) *nftTransitionGuard {
	t.Helper()
	return &nftTransitionGuard{statePath: filepath.Join(filepath.Dir(f.manager.path), "guard"), run: f.nft, offload: f.manager}
}

func TestFW4OffloadTransitionRoundTrip(t *testing.T) {
	for _, hardware := range []string{"absent", "0", "1"} {
		t.Run(hardware, func(t *testing.T) {
			f := newFW4OffloadFixture(t)
			if hardware != "absent" {
				f.options.Hardware = offloadOption(hardware)
			}
			original := f.options
			g := f.guardInstance(t)
			if err := g.Enable(nil, nil); err != nil {
				t.Fatal(err)
			}
			if f.flow || !f.guard {
				t.Fatal("core may not stop before fast path is drained and guard installed")
			}
			if strings.Join(f.calls[len(f.calls)-2:], ",") != "drain,guard" {
				t.Fatal("guard did not follow drain", f.calls)
			}
			// Repeated preparation and a replacement daemon retain the original
			// settings, not the temporarily disabled snapshot.
			restarted := f.guardInstance(t)
			if err := restarted.Enable(nil, nil); err != nil {
				t.Fatal(err)
			}
			if err := restarted.Disable(); err != nil {
				t.Fatal(err)
			}
			if f.guard || !f.flow || !reflect.DeepEqual(f.options, original) {
				t.Fatalf("not restored: %+v", f.options)
			}
			if _, err := os.Stat(f.manager.path); !os.IsNotExist(err) {
				t.Fatal("recovery record retained")
			}
		})
	}
}

func TestFW4OffloadRejectsConcurrentPendingEdits(t *testing.T) {
	f := newFW4OffloadFixture(t)
	run := f.manager.run
	f.manager.run = func(input, path string, args ...string) ([]byte, error) {
		output, err := run(input, path, args...)
		if path == openWrtUCIPath && len(args) == 5 && args[3] == "set" {
			f.changes = true // A different UCI client edits during preparation.
		}
		return output, err
	}
	if err := f.guardInstance(t).Enable(nil, nil); err == nil || errors.Is(err, errTransitionProtectionUnsupported) {
		t.Fatalf("concurrent change was ignored: %v", err)
	}
	if *f.options.Software != "1" || slices.Contains(f.calls, "commit") || slices.Contains(f.calls, "reload") {
		t.Fatal("committed shared pending edits or reloaded firewall", f.calls)
	}
}

func TestFW4OffloadPendingCannotFallBackToUnguardedTransition(t *testing.T) {
	for _, scenario := range []string{"unsupported", "foreign", "corrupt", "missing-nft"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFW4OffloadFixture(t)
			g := f.guardInstance(t)
			if err := g.Enable(nil, nil); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "unsupported":
				f.supported = false
			case "foreign":
				f.foreign, f.flow = true, true
			case "corrupt":
				if err := os.WriteFile(f.manager.path, []byte(`{"version":99}`), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing-nft":
				g.run = func(string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }
			}
			if err := g.Enable(nil, nil); err == nil || errors.Is(err, errTransitionProtectionUnsupported) {
				t.Fatalf("pending protection may not be skipped: %v", err)
			}
			if !f.guard || *f.options.Software != "0" {
				t.Fatal("pending protection was released")
			}
		})
	}
}

func TestTransitionMissingNFTIsOptionalWithoutPendingState(t *testing.T) {
	g := &nftTransitionGuard{statePath: filepath.Join(t.TempDir(), "guard"), run: func(string, ...string) ([]byte, error) { return nil, exec.ErrNotFound }}
	if err := g.Enable(nil, nil); !errors.Is(err, errTransitionProtectionUnsupported) {
		t.Fatalf("missing nft blocked optional protection: %v", err)
	}
}

func TestFW4OffloadUnsupportedIsNonMutating(t *testing.T) {
	for _, scenario := range []string{"unsupported", "foreign", "changes", "check"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFW4OffloadFixture(t)
			switch scenario {
			case "unsupported":
				f.supported = false
			case "foreign":
				f.foreign = true
			case "changes":
				f.changes = true
			case "check":
				f.fail = "check"
			}
			err := f.guardInstance(t).Enable(nil, nil)
			if !errors.Is(err, errTransitionProtectionUnsupported) {
				t.Fatalf("unsupported result: %v", err)
			}
			if !f.flow || f.guard {
				t.Fatal("unsupported environment was modified")
			}
			if _, err := os.Stat(f.manager.path); !os.IsNotExist(err) {
				t.Fatal("unsupported operation persisted intent")
			}
		})
	}
}

func TestFW4OffloadFailuresKeepRecoveryIntent(t *testing.T) {
	for _, failure := range []string{"set", "commit", "reload", "drain", "guard"} {
		t.Run(failure, func(t *testing.T) {
			f := newFW4OffloadFixture(t)
			f.fail = failure
			g := f.guardInstance(t)
			err := g.Enable(nil, nil)
			if err == nil || errors.Is(err, errTransitionProtectionUnsupported) {
				t.Fatalf("activation failure skipped: %v", err)
			}
			if f.guard {
				t.Fatal("failed activation marked guarded")
			}
			if _, err := os.Stat(f.manager.path); err != nil {
				t.Fatal("lost original settings", err)
			}
			f.fail = ""
			if err := g.Disable(); err != nil {
				t.Fatal(err)
			}
			if !f.flow || *f.options.Software != "1" {
				t.Fatal("could not recover partial preparation")
			}
		})
	}
}

func TestFW4OffloadRestoreHonorsDeviceProtectionAndExternalChanges(t *testing.T) {
	for _, scenario := range []string{"device", "external", "reload", "unguard"} {
		t.Run(scenario, func(t *testing.T) {
			f := newFW4OffloadFixture(t)
			g := f.guardInstance(t)
			if err := g.Enable(nil, nil); err != nil {
				t.Fatal(err)
			}
			switch scenario {
			case "device":
				f.device = true
			case "external":
				f.options.Hardware = offloadOption("1")
			default:
				f.fail = scenario
			}
			err := g.Disable()
			if (err != nil) != (scenario != "device") {
				t.Fatalf("restore error: %v", err)
			}
			if _, err := os.Stat(f.manager.path); err != nil {
				t.Fatal("lost retry record", err)
			}
			if scenario == "external" && *f.options.Hardware != "1" {
				t.Fatal("overwrote user changes")
			}
			if scenario == "unguard" && (!f.guard || f.flow) {
				t.Fatal("restored acceleration despite guard deletion failure")
			}
			if scenario == "device" {
				if f.flow {
					t.Fatal("restored offload over permanent device protection")
				}
				f.device = false
				if err := g.Disable(); err != nil {
					t.Fatal(err)
				}
				if !f.flow {
					t.Fatal("did not restore after device policy removed")
				}
			}
		})
	}
}

func TestOffloadSnapshotAndConnectionStatus(t *testing.T) {
	for _, input := range []string{"", "firewall.a=defaults\nfirewall.b=defaults", "firewall.a=defaults\nfirewall.a.flow_offloading='invalid'", "firewall.a=defaults\nfirewall.a.flow_offloading='1' '0'"} {
		if _, err := parseFW4OffloadOptions(input); err == nil {
			t.Fatalf("accepted ambiguous UCI input %q", input)
		}
	}
	for _, test := range []struct {
		data string
		want bool
	}{{"ipv4 tcp ESTABLISHED [ASSURED]\n", false}, {"ipv6 udp [OFFLOAD]\n", true}, {"ipv4 tcp [HW_OFFLOAD]\n", true}} {
		path := filepath.Join(t.TempDir(), "conntrack")
		if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
			t.Fatal(err)
		}
		got, err := hasOffloadedConnections(path)
		if err != nil || got != test.want {
			t.Fatalf("status=%t error=%v", got, err)
		}
	}
}
