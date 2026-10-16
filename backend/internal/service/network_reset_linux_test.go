//go:build linux

package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNetworkResetReleasesCorruptPolicyAndStaysDisabledAfterRecovery(t *testing.T) {
	db, g, rules := devicePolicyFixture(t)
	g.paths.DBPath = filepath.Join(g.paths.DataDir, "test.db")
	guard, nft := testDeviceProtection(t)
	settings, err := db.GetGeneralSettings()
	if err != nil {
		t.Fatal(err)
	}
	settings.DeviceProtectionEnabled = true
	if err := db.SetGeneralSettings(settings); err != nil {
		t.Fatal(err)
	}
	if err := guard.Apply(rules[0].Values); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guard.path, []byte("damaged policy"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := guard.Disable(); err == nil {
		t.Fatal("ordinary opt-out unexpectedly accepted damaged state")
	}
	if err := resetDeviceProtectionForRepair(g.paths, guard); err != nil {
		t.Fatal(err)
	}
	if nft.owner != "" {
		t.Fatal("reset retained kernel guard")
	}
	if _, err := os.Stat(guard.path); !os.IsNotExist(err) {
		t.Fatal("reset retained boot policy")
	}
	after, err := db.GetGeneralSettings()
	if err != nil || after.DeviceProtectionEnabled || after.AutoStartCore != settings.AutoStartCore || after.GeoSource != settings.GeoSource {
		t.Fatalf("reset settings: %+v %v", after, err)
	}
	remaining, err := enabledDeviceRules(db)
	if err != nil || len(remaining) != len(rules) {
		t.Fatal("reset removed device bindings", err)
	}
	core := NewSingboxService(g.paths, nil, nil, db)
	core.deviceProtection = guard
	nft.failApply = true
	if err := core.restoreDeviceProtection(); err != nil {
		t.Fatal("daemon recovery tried to re-enable protection", err)
	}
}

func TestNetworkResetRetainsGuardWhenSettingCannotBeSaved(t *testing.T) {
	_, g, rules := devicePolicyFixture(t)
	g.paths.DBPath = filepath.Join(g.paths.DataDir, "test.db")
	guard, nft := testDeviceProtection(t)
	if err := guard.Apply(rules[0].Values); err != nil {
		t.Fatal(err)
	}
	p := *g.paths
	p.DBPath = filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(p.DBPath, []byte("damaged database"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := resetDeviceProtectionForRepair(&p, guard); err == nil {
		t.Fatal("hidden database failure")
	}
	if nft.owner == "" {
		t.Fatal("database failure released guard")
	}
	if _, err := guard.Sources(); err != nil {
		t.Fatal("database failure damaged boot policy", err)
	}
}

func TestNetworkResetReleaseFailureCanBeRetried(t *testing.T) {
	_, g, rules := devicePolicyFixture(t)
	g.paths.DBPath = filepath.Join(g.paths.DataDir, "test.db")
	guard, nft := testDeviceProtection(t)
	if err := guard.Apply(rules[0].Values); err != nil {
		t.Fatal(err)
	}
	nft.failDelete = true
	if err := resetDeviceProtectionForRepair(g.paths, guard); err == nil {
		t.Fatal("hidden firewall failure")
	}
	if _, err := os.Stat(guard.path); err != nil {
		t.Fatal("failed release lost recovery policy")
	}
	nft.failDelete = false
	if err := resetDeviceProtectionForRepair(g.paths, guard); err != nil {
		t.Fatal(err)
	}
	if nft.owner != "" {
		t.Fatal("retry did not clear kernel protection")
	}
}
