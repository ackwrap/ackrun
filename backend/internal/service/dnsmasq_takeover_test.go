package service

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/paths"
)

type readinessDNSMasqStub struct {
	activationErr error
	activations   int
	onActivate    func()
}

func (stub *readinessDNSMasqStub) Activate() error {
	stub.activations++
	if stub.onActivate != nil {
		stub.onActivate()
	}
	return stub.activationErr
}

func (*readinessDNSMasqStub) Restore() (bool, error) { return false, nil }

func TestDNSMasqReadinessDoesNotSwallowFailures(t *testing.T) {
	probeErr := errors.New("DNS is not ready")
	activationErr := errors.New("cannot apply dnsmasq settings")
	tests := []struct {
		name            string
		probeErr        error
		activationErr   error
		wantErr         error
		wantActivations int
	}{
		{name: "ready", wantActivations: 1},
		{name: "DNS timeout", probeErr: probeErr, wantErr: probeErr},
		{name: "activation failure", activationErr: activationErr, wantErr: activationErr, wantActivations: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			stub := &readinessDNSMasqStub{activationErr: test.activationErr}
			err := activateDNSMasqAfterReady(ctx, stub.Activate, func(context.Context) error { return test.probeErr })
			if !errors.Is(err, test.wantErr) || stub.activations != test.wantActivations {
				t.Fatalf("error = %v, activations = %d; want %v, %d", err, stub.activations, test.wantErr, test.wantActivations)
			}
			if test.probeErr != nil && !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("DNS timeout lost its cause: %v", err)
			}
		})
	}
}

func TestDNSMasqReadinessRejectsMissingLifecycle(t *testing.T) {
	err := (&SingboxService{}).activateDNSMasq(context.Background())
	if err == nil || !strings.Contains(err.Error(), "生命周期服务不可用") {
		t.Fatalf("error = %v, want unavailable lifecycle", err)
	}
}

func TestDNSMasqActivationHoldsNetworkLifecycleLock(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	stub := &readinessDNSMasqStub{onActivate: func() {
		close(started)
		<-release
	}}
	svc := &SingboxService{paths: &paths.Paths{DataDir: t.TempDir()}, dnsmasq: stub}
	result := make(chan error, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		result <- svc.activateDNSMasq(context.Background())
	}()
	defer func() {
		close(release)
		<-done
	}()
	select {
	case <-started:
		if svc.networkLifecycleMu.TryLock() {
			svc.networkLifecycleMu.Unlock()
			t.Fatal("exit cleanup can race DNS activation")
		}
	case err := <-result:
		t.Fatalf("activation ended before readiness: %v", err)
	case <-time.After(time.Second):
		t.Fatal("activation did not start")
	}
}

func TestDNSMasqActivationChecksCancellationAfterNetworkLock(t *testing.T) {
	stub := &readinessDNSMasqStub{}
	svc := &SingboxService{paths: &paths.Paths{DataDir: t.TempDir()}, dnsmasq: stub}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.networkLifecycleMu.Lock()
	result := make(chan error, 1)
	go func() { result <- svc.activateDNSMasq(ctx) }()
	cancel()
	svc.networkLifecycleMu.Unlock()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) || stub.activations != 0 {
			t.Fatalf("error = %v, activations = %d; want canceled without activation", err, stub.activations)
		}
	case <-time.After(time.Second):
		t.Fatal("activation did not honor startup cancellation")
	}
}

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
