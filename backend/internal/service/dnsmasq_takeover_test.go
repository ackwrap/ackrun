package service

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDNSMasqSnapshotPreservesManagedOptions(t *testing.T) {
	output := strings.Join([]string{
		"dhcp.cfg01411c=dnsmasq",
		"dhcp.cfg01411c.noresolv='0'",
		"dhcp.cfg01411c.server='192.168.3.1' '/corp.example/172.16.1.1'",
		"dhcp.cfgdead=dnsmasq",
		"dhcp.cfgdead.disabled='1'",
	}, "\n")
	snapshot, err := parseDNSMasqSnapshot(output)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Section != "cfg01411c" || !snapshot.NoResolvExists || snapshot.NoResolv != "0" || !snapshot.ServersExist || !slicesEqual(snapshot.Servers, []string{"192.168.3.1", "/corp.example/172.16.1.1"}) {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestParseDNSMasqSnapshotRejectsMultipleEnabledInstances(t *testing.T) {
	_, err := parseDNSMasqSnapshot("dhcp.first=dnsmasq\ndhcp.second=dnsmasq\n")
	if err == nil || !strings.Contains(err.Error(), "2 个") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseUCIWordsHandlesQuotedLists(t *testing.T) {
	values, err := parseUCIWords(`'first' 'second'\''s'`)
	if err != nil {
		t.Fatal(err)
	}
	if !slicesEqual(values, []string{"first", "second's"}) {
		t.Fatalf("values = %#v", values)
	}
}

func TestDNSMasqTakeoverStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".dnsmasq-takeover.json")
	state := dnsmasqTakeoverState{
		Version:  dnsmasqStateVersion,
		Phase:    "active",
		Original: dnsmasqOptionsSnapshot{Section: "cfg01411c", ServersExist: true, Servers: []string{"192.168.3.1"}},
		Managed:  managedDNSMasqSnapshot("cfg01411c"),
	}
	if err := writeDNSMasqTakeoverState(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, exists, err := readDNSMasqTakeoverState(path)
	if err != nil || !exists {
		t.Fatalf("read state = %+v, %t, %v", loaded, exists, err)
	}
	if !dnsmasqSnapshotsEqual(loaded.Original, state.Original) || !dnsmasqSnapshotsEqual(loaded.Managed, state.Managed) {
		t.Fatalf("loaded state = %+v", loaded)
	}
}
