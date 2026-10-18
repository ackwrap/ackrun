//go:build linux

package service

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

type protectionNFTStub struct {
	owner      string
	count      int
	offload    bool
	flowtables []map[string]any
	failApply  bool
	failDelete bool
	objects    []map[string]any
}

func (n *protectionNFTStub) run(input string, args ...string) ([]byte, error) {
	switch strings.Join(args, " ") {
	case "-j list ruleset":
		entries := []map[string]interface{}{}
		if n.owner != "" {
			entries = append(entries, map[string]interface{}{"table": map[string]string{"family": "inet", "name": deviceProtectionTable, "comment": n.owner}})
			for i := 0; i < n.count; i++ {
				entries = append(entries, map[string]interface{}{"rule": map[string]string{"family": "inet", "table": deviceProtectionTable}})
			}
		}
		if n.offload {
			entries = append(entries, map[string]interface{}{"flowtable": map[string]string{"name": "ft"}})
		}
		for _, table := range n.flowtables {
			entries = append(entries, map[string]any{"flowtable": table})
		}
		return json.Marshal(map[string]interface{}{"nftables": entries})
	case "-c -f -":
		return nil, nil
	case "-j list table inet " + deviceProtectionTable:
		return json.Marshal(map[string]any{"nftables": n.objects})
	case "-j -e -f -":
		if n.failApply {
			return nil, errors.New("apply rejected")
		}
		for _, line := range strings.Split(input, "\n") {
			if strings.HasPrefix(line, " comment ") {
				_ = json.Unmarshal([]byte(strings.TrimPrefix(line, " comment ")), &n.owner)
			}
		}
		n.count = strings.Count(input, "counter drop") + strings.Count(input, "counter return") + strings.Count(input, "redirect to")
		n.objects = []map[string]any{{"table": map[string]any{"family": "inet", "name": deviceProtectionTable, "comment": n.owner}}}
		chain := ""
		for _, line := range strings.Split(input, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "chain ") {
				chain = strings.Fields(line)[1]
				n.objects = append(n.objects, map[string]any{"chain": map[string]any{"family": "inet", "table": deviceProtectionTable, "name": chain}})
			} else if strings.Contains(line, "counter ") || strings.Contains(line, "redirect to") {
				// Lifecycle tests only need distinguishable statements. Packet tests
				// below use the real nft compiler and JSON expressions.
				n.objects = append(n.objects, map[string]any{"rule": map[string]any{"family": "inet", "table": deviceProtectionTable, "chain": chain, "expr": []any{map[string]any{"synthetic_statement": line}}}})
			}
		}
		echo := make([]map[string]any, 0, len(n.objects))
		for _, object := range n.objects {
			echo = append(echo, map[string]any{"add": object})
		}
		return json.Marshal(map[string]any{"nftables": echo})
	case "delete table inet " + deviceProtectionTable:
		if n.failDelete {
			return nil, errors.New("delete rejected")
		}
		n.owner, n.count = "", 0
		return nil, nil
	default:
		return nil, errors.New("unexpected nft call")
	}
}

func testDeviceProtection(t *testing.T) (*nftDeviceProtection, *protectionNFTStub) {
	t.Helper()
	n := &protectionNFTStub{}
	return &nftDeviceProtection{path: filepath.Join(t.TempDir(), "guard.nft"), supported: func() bool { return true }, run: n.run}, n
}

func TestDeviceProtectionDurablePolicyAndExplicitRelease(t *testing.T) {
	g, n := testDeviceProtection(t)
	sources := []string{"192.168.50.10/32", "fd12:3456::10/128"}
	if err := g.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	// A new daemon sees the same policy; losing process memory does not unblock.
	recovered := &nftDeviceProtection{path: g.path, supported: g.supported, run: n.run}
	got, err := recovered.Sources()
	if err != nil || !reflect.DeepEqual(got, sources) {
		t.Fatalf("recovery: %v %v", got, err)
	}
	if err := recovered.Verify(sources, nil); err != nil {
		t.Fatal(err)
	}
	n.count--
	if err := recovered.Verify(sources, nil); err == nil {
		t.Fatal("missing kernel rule reported protected")
	}
	if err := recovered.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	n.failDelete = true
	if err := recovered.Disable(); err == nil {
		t.Fatal("hidden delete failure")
	}
	if _, err := os.Stat(g.path); err != nil {
		t.Fatal("failed disable lost boot protection")
	}
	n.failDelete = false
	n.offload = true // Explicit release must remain possible after external changes.
	if err := recovered.Disable(); err != nil {
		t.Fatal(err)
	}
	if n.owner != "" {
		t.Fatal("guard retained after explicit disable")
	}
	if _, err := os.Stat(g.path); !os.IsNotExist(err) {
		t.Fatal("boot protection retained")
	}
}

func TestDeviceProtectionUpgradesLegacyPolicyWithoutReleasingSources(t *testing.T) {
	g, n := testDeviceProtection(t)
	sources := []string{"192.168.50.10/32", "fd12:3456::10/128"}
	legacy, owner, err := renderDeviceProtectionRules(sources, legacyDeviceProtectionOwner, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(g.path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	n.owner, n.count = owner, len(sources)*5
	if got, err := g.Sources(); err != nil || !reflect.DeepEqual(got, sources) {
		t.Fatalf("cannot recover v0.3.0 sources: %v %v", got, err)
	}
	if err := g.Verify(sources, nil); err == nil {
		t.Fatal("old policy incorrectly reported as the new local-access policy")
	}
	if err := g.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(n.owner, deviceProtectionOwner+" ") {
		t.Fatal("kernel policy not upgraded")
	}
	data, err := os.ReadFile(g.path)
	if err != nil || !strings.HasPrefix(string(data), "# "+deviceProtectionOwner+"\n") {
		t.Fatal("boot policy not upgraded", err)
	}
	// Local access must not weaken the ownership/content check on disk.
	if err := os.WriteFile(g.path, append(data, []byte("flush ruleset\n")...), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Sources(); err == nil {
		t.Fatal("modified policy was accepted")
	}
}

func TestDeviceProtectionRejectsIncompatibleOrFailedActivation(t *testing.T) {
	for _, scenario := range []string{"foreign", "offload", "boot", "apply", "invalid", "corrupt"} {
		t.Run(scenario, func(t *testing.T) {
			g, n := testDeviceProtection(t)
			values := []string{"192.168.50.10/32"}
			switch scenario {
			case "foreign":
				n.owner = "unrelated"
			case "offload":
				n.offload = true
			case "boot":
				g.bootCheck = func() error { return errors.New("fw4 include disabled") }
			case "apply":
				n.failApply = true
			case "invalid":
				values = []string{"bad; flush ruleset"}
			case "corrupt":
				if err := os.WriteFile(g.path, []byte("foreign file"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := g.Apply(values, nil); err == nil {
				t.Fatal("invalid activation accepted")
			}
			if scenario == "apply" {
				if _, err := g.Sources(); err != nil {
					t.Fatal("lost durable recovery intent", err)
				}
			}
		})
	}
}

func TestDeviceProtectionSettingsModesAndMutationLifecycle(t *testing.T) {
	db, g, rules := devicePolicyFixture(t)
	firewall, n := testDeviceProtection(t)
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = firewall
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	enabled, disabled := true, false
	settings, err := svc.GetGeneralSettings()
	if err != nil || settings.DeviceProtectionEnabled || settings.DeviceProtectionStatus != "disabled" {
		t.Fatalf("default protection: %+v %v", settings, err)
	}
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	settings, err = svc.GetGeneralSettings()
	if err != nil || !settings.DeviceProtectionEnabled || settings.DeviceProtectionStatus != "blocked" {
		t.Fatalf("settings: %+v %v", settings, err)
	}
	if err := svc.SetProxyMode("direct"); err == nil {
		t.Fatal("direct mode escaped protection")
	}
	if err := svc.SetInboundMode("mixed"); err == nil {
		t.Fatal("mixed-only mode escaped protection")
	}
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{AutoStartCore: &disabled}); err != nil {
		t.Fatal(err)
	}
	settings, _ = db.GetGeneralSettings()
	if !settings.DeviceProtectionEnabled {
		t.Fatal("unrelated setting disabled protection")
	}
	// Routine recovery re-installs the union instead of deleting the policy.
	n.owner = ""
	if err := core.restoreDeviceProtection(); err != nil {
		t.Fatal(err)
	}
	routes := NewRouteRuleService(db, g.paths, nil)
	routes.SetDeviceProtectionCore(core)
	// Running-core checks must reject both settings and device-set mutations.
	core.pid, core.cmd = 123, &exec.Cmd{Process: &os.Process{Pid: 123}}
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &disabled}); err == nil {
		t.Fatal("protection disabled while core was running")
	}
	if _, err := routes.Delete(rules[1].ID); err == nil {
		t.Fatal("device removed while core was running")
	}
	core.pid, core.cmd = 0, nil
	if _, err := routes.Delete(rules[1].ID); err != nil {
		t.Fatal(err)
	}
	sources, err := firewall.Sources()
	if err != nil || len(sources) != 2 {
		t.Fatalf("device set not synchronized: %v %v", sources, err)
	}
	if _, err := routes.Delete(rules[0].ID); err == nil {
		t.Fatal("last protected rule removed without opt-out")
	}
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	settings, err = svc.GetGeneralSettings()
	if err != nil || settings.DeviceProtectionStatus != "disabled" || n.owner != "" {
		t.Fatalf("disable: %+v %v", settings, err)
	}
}

func TestDeviceProtectionInterruptedReleaseReportsKernelResidue(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	firewall, n := testDeviceProtection(t)
	if err := firewall.Apply([]string{"192.168.50.10/32"}, nil); err != nil {
		t.Fatal(err)
	}
	// Simulate interruption after removing boot intent but before kernel release.
	if err := os.Remove(firewall.path); err != nil {
		t.Fatal(err)
	}
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = firewall
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	settings, err := svc.GetGeneralSettings()
	if err != nil || settings.DeviceProtectionEnabled || settings.DeviceProtectionStatus != "error" {
		t.Fatalf("kernel residue hidden: %+v %v", settings, err)
	}
	disabled := false
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &disabled}); err != nil {
		t.Fatal(err)
	}
	if n.owner != "" {
		t.Fatal("explicit retry retained kernel rules")
	}
}

func TestDeviceProtectionActivationFailureDoesNotReportEnabled(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	firewall, n := testDeviceProtection(t)
	n.failApply = true
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = firewall
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	enabled := true
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &enabled}); err == nil {
		t.Fatal("apply failure hidden")
	}
	settings, err := svc.GetGeneralSettings()
	if err != nil || settings.DeviceProtectionEnabled || settings.DeviceProtectionStatus != "error" {
		t.Fatalf("false success: %+v %v", settings, err)
	}
}

func TestDeviceProtectionValidatesManagedConfigBeforeStart(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	g.dnsmasqSupported = platformSupportsDNSMasqTakeover
	firewall, _ := testDeviceProtection(t)
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = firewall
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	enabled := true
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	route, err := g.generateRoute("direct")
	if err != nil {
		t.Fatal(err)
	}
	dns, err := g.generateDNSFromDatabase()
	if err != nil {
		t.Fatal(err)
	}
	outbounds, _, err := g.generateOutbounds(true)
	if err != nil {
		t.Fatal(err)
	}
	inbounds, err := g.generateInbounds("127.0.0.1", 7890, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	config := map[string]interface{}{"route": route, "dns": dns, "outbounds": outbounds, "inbounds": inbounds}
	path := filepath.Join(t.TempDir(), "active.json")
	write := func() {
		t.Helper()
		data, _ := json.Marshal(config)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write()
	if err := core.validateProtectedConfig(path); err != nil {
		t.Fatal(err)
	}
	tun := inbounds[0].(map[string]interface{})
	tun["exclude_interface"] = []string{"easytier-tun"}
	write()
	if err := core.validateProtectedConfig(path); err == nil || !strings.Contains(err.Error(), "接口排除") {
		t.Fatalf("unapproved config exemption accepted: %v", err)
	}
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "interface", Value: "easytier-tun"}}}); err != nil {
		t.Fatal(err)
	}
	if err := core.validateProtectedConfig(path); err != nil {
		t.Fatalf("explicit interface exemption rejected: %v", err)
	}
	route["rules"] = []interface{}{}
	write()
	if err := core.validateProtectedConfig(path); err == nil {
		t.Fatal("old config without device bindings was accepted")
	}
}
