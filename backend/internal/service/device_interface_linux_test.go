//go:build linux

package service

import (
	"errors"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestDeviceInterfaceExclusionsPersistAndSynchronize(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	guard, nft := testDeviceProtection(t)
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = guard
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	enabled := true
	if err := svc.SetGeneralSettings(&model.GeneralSettingsRequest{DeviceProtectionEnabled: &enabled}); err != nil {
		t.Fatal(err)
	}
	sources, err := core.desiredProtectedSources()
	if err != nil {
		t.Fatal(err)
	}
	settings := &model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{
		{Type: "interface", Value: "easytier-tun"},
		{Type: "interface", Value: "wg0"},
	}}
	if err := svc.SetTrafficBypassSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := guard.Verify(sources, []string{"wg0", "easytier-tun"}); err != nil {
		t.Fatal(err)
	}
	if err := g.validateDevicePolicies(); err != nil {
		t.Fatal(err)
	}
	inbounds, err := g.generateInbounds("", 0, defaultTUNIPv4Address, defaultTUNIPv6Address)
	if err != nil {
		t.Fatal(err)
	}
	if got := inbounds[0].(map[string]interface{})["exclude_interface"]; !reflect.DeepEqual(got, []string{"easytier-tun", "wg0"}) {
		t.Fatalf("TUN exclusions: %v", got)
	}
	// The same requested settings repair a prior kernel apply failure.
	nft.count--
	if err := svc.SetTrafficBypassSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := guard.Verify(sources, []string{"easytier-tun", "wg0"}); err != nil {
		t.Fatal(err)
	}
	core.pid, core.cmd = 123, &exec.Cmd{Process: &os.Process{Pid: 123}}
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{}); !errors.Is(err, ErrTrafficBypassSettingsInvalid) {
		t.Fatalf("running interface mutation: %v", err)
	}
	if got, _ := deviceExcludedInterfaces(db); len(got) != 2 {
		t.Fatal("rejected update changed settings")
	}
	core.pid, core.cmd = 0, nil
	if err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{}); err != nil {
		t.Fatal(err)
	}
	if err := guard.Verify(sources, nil); err != nil {
		t.Fatal(err)
	}
	_, _, excluded, err := guard.readPolicy()
	if err != nil || len(excluded) != 0 {
		t.Fatalf("removed exclusions survive reboot: %v %v", excluded, err)
	}
}

func TestDeviceInterfaceExclusionFailureRetainsProtection(t *testing.T) {
	db, g, _ := devicePolicyFixture(t)
	guard, nft := testDeviceProtection(t)
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = guard
	svc := NewSettingsService(db)
	svc.SetModeDependencies(core, g)
	sources, _ := core.desiredProtectedSources()
	if err := guard.Apply(sources, nil); err != nil {
		t.Fatal(err)
	}
	nft.failApply = true
	err := svc.SetTrafficBypassSettings(&model.TrafficBypassSettings{Rules: []model.TrafficBypassRule{{Type: "interface", Value: "easytier-tun"}}})
	if err == nil {
		t.Fatal("firewall failure hidden")
	}
	if excluded, _ := deviceExcludedInterfaces(db); len(excluded) != 0 {
		t.Fatal("failed update changed settings")
	}
	if got, _ := guard.Sources(); !reflect.DeepEqual(got, sources) {
		t.Fatal("failed update lost protection")
	}
}

func TestDeviceInterfaceExclusionsValidateNamesAndOldPolicies(t *testing.T) {
	for _, name := range []string{"", "abcdefghijklmnop", "bad*", "bad\nname", `bad"name`} {
		if _, err := normalizeExcludedInterfaces([]string{name}); err == nil {
			t.Fatalf("accepted interface %q", name)
		}
	}
	guard, nft := testDeviceProtection(t)
	sources := []string{"192.168.50.10/32"}
	old, owner, err := renderDeviceProtectionRules(sources, localDeviceProtectionOwner, nil, "10.77.0.0/24")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard.path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}
	nft.owner, nft.count = owner, 8
	if err := guard.Apply(sources, []string{"easytier-tun"}); err != nil {
		t.Fatal(err)
	}
	_, pools, excluded, err := guard.readPolicy()
	if err != nil || !reflect.DeepEqual(excluded, []string{"easytier-tun"}) || !strings.Contains(strings.Join(pools, ","), "10.77.0.0/24") {
		t.Fatalf("migration lost policy: %v %v %v", pools, excluded, err)
	}
}
