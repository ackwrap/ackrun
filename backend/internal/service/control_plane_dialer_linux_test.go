//go:build linux

package service

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func useControlPlaneResolverFiles(t *testing.T, files ...string) {
	t.Helper()
	original := controlPlaneResolverFiles
	controlPlaneResolverFiles = files
	t.Cleanup(func() { controlPlaneResolverFiles = original })
}

func TestControlPlaneNameServers(t *testing.T) {
	dir := t.TempDir()
	wan := filepath.Join(dir, "wan.conf")
	system := filepath.Join(dir, "resolv.conf")
	if err := os.WriteFile(wan, []byte("nameserver 192.0.2.53 # WAN\nnameserver 2001:db8::53\nnameserver fe80::53%wan0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(system, []byte("nameserver 127.0.0.1\nnameserver ::1\nnameserver 0.0.0.0\nnameserver ::\nnameserver invalid\nnameserver 224.0.0.1\nnameserver 192.0.2.53\nnameserver [2001:db8::54]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	useControlPlaneResolverFiles(t, filepath.Join(dir, "missing"), wan, system)
	want := []string{"192.0.2.53:53", "[2001:db8::53]:53", "[fe80::53%wan0]:53", "[2001:db8::54]:53"}
	if got := controlPlaneNameServers(); !reflect.DeepEqual(got, want) {
		t.Fatalf("upstreams = %v, want %v", got, want)
	}
}

func TestControlPlaneResolverUsesUpstreamsAndFollowsRenewal(t *testing.T) {
	file := filepath.Join(t.TempDir(), "wan.conf")
	if err := os.WriteFile(file, []byte("nameserver 192.0.2.53\nnameserver 2001:db8::53\n"), 0600); err != nil {
		t.Fatal(err)
	}
	useControlPlaneResolverFiles(t, file)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	wantErr := errors.New("DNS transport failure")
	var got []string
	resolver := controlPlaneResolver(func(gotCtx context.Context, network, address string) (net.Conn, error) {
		if gotCtx != ctx || (network != "udp" && network != "tcp") {
			t.Fatal("resolver lost context or network")
		}
		got = append(got, address)
		return nil, wantErr
	})
	if !resolver.PreferGo {
		t.Fatal("control-plane DNS could escape through the system/cgo resolver")
	}
	for _, network := range []string{"udp", "tcp", "udp"} {
		if _, err := resolver.Dial(ctx, network, "[::1]:53"); !errors.Is(err, wantErr) {
			t.Fatalf("DNS failure = %v", err)
		}
	}
	want := []string{"192.0.2.53:53", "[2001:db8::53]:53", "192.0.2.53:53"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DNS targets = %v, want %v", got, want)
	}
	if err := os.WriteFile(file, []byte("nameserver 192.0.2.54\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, _ = resolver.Dial(ctx, "udp", "127.0.0.1:53")
	if got[len(got)-1] != "192.0.2.54:53" {
		t.Fatal("resolver retained the old WAN upstream")
	}
}

func TestControlPlaneResolverNoUpstreamPreservesSystemDNS(t *testing.T) {
	useControlPlaneResolverFiles(t, filepath.Join(t.TempDir(), "missing"))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	resolver := controlPlaneResolver(func(gotCtx context.Context, network, address string) (net.Conn, error) {
		if network != "tcp" || address != "127.0.0.53:53" {
			t.Fatalf("unexpected resolver fallback: %s %s", network, address)
		}
		return nil, gotCtx.Err()
	})
	if _, err := resolver.Dial(ctx, "tcp", "127.0.0.53:53"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation = %v", err)
	}
}

func TestControlPlaneDialerMarksHTTPAndDNS(t *testing.T) {
	dialer := controlPlaneDialer(7 * time.Second)
	if dialer.Timeout != 7*time.Second || dialer.Control == nil || dialer.Resolver == nil || dialer.Resolver.Dial == nil {
		t.Fatal("control-plane dialer lacks a timeout, socket marking or independent DNS")
	}
	transport := directHTTPTransport()
	defer transport.CloseIdleConnections()
	if transport.DialContext == nil || transport.Proxy != nil || transport == http.DefaultTransport {
		t.Fatal("downloads do not have an independent direct transport")
	}
}
