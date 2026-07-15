package traceroute

import (
	"context"
	"encoding/binary"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNormalizedOptionsUsesDefaults(t *testing.T) {
	options := normalizedOptions(Options{})
	if options.Queries != DefaultQueries || options.MaxHops != DefaultMaxHops {
		t.Fatalf("unexpected count defaults: %+v", options)
	}
	if options.Timeout != DefaultTimeout || options.RDNSTimeout != DefaultRDNSTimeout {
		t.Fatalf("unexpected timeout defaults: %+v", options)
	}
	if options.GeoProvider != DefaultGeoProvider {
		t.Fatalf("unexpected Geo provider default: %+v", options)
	}
}

func TestNormalizedOptionsKeepsValues(t *testing.T) {
	options := normalizedOptions(Options{
		Queries:       1,
		MaxHops:       8,
		Timeout:       2 * time.Second,
		ProbeInterval: 10 * time.Millisecond,
		TTLInterval:   20 * time.Millisecond,
		RDNSTimeout:   30 * time.Millisecond,
	})
	if options.Queries != 1 || options.MaxHops != 8 || options.Timeout != 2*time.Second {
		t.Fatalf("options were replaced: %+v", options)
	}
}

func TestEmbeddedIPv4EchoSequence(t *testing.T) {
	destination := net.ParseIP("203.0.113.10").To4()
	packet := make([]byte, 28)
	packet[0] = 0x45
	packet[9] = 1
	copy(packet[16:20], destination)
	packet[20] = byte(8)
	binary.BigEndian.PutUint16(packet[24:26], 4321)
	binary.BigEndian.PutUint16(packet[26:28], 77)

	seq, ok := embeddedEchoSequence(packet, true, destination, 4321)
	if !ok || seq != 77 {
		t.Fatalf("unexpected embedded echo result: seq=%d ok=%v", seq, ok)
	}
}

func TestCanonicalIP(t *testing.T) {
	if got := canonicalIP(net.ParseIP("192.0.2.1")); len(got) != net.IPv4len {
		t.Fatalf("expected canonical IPv4, got %v", got)
	}
	if got := canonicalIP(net.ParseIP("2001:db8::1")); len(got) != net.IPv6len {
		t.Fatalf("expected canonical IPv6, got %v", got)
	}
}

func TestResolveTargetIPDoesNotNeedDNS(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ip, err := ResolveTarget(ctx, "203.0.113.7")
	if err != nil || ip.String() != "203.0.113.7" {
		t.Fatalf("ResolveTarget() = %v, %v", ip, err)
	}
}

func TestResolveTargetWithAliDoH(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("name") != "node.example" || request.URL.Query().Get("type") != "1" {
			t.Fatalf("unexpected AliDNS query: %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/dns-json")
		_, _ = writer.Write([]byte(`{"Status":0,"Answer":[{"type":5,"data":"alias.example."},{"type":1,"data":"203.0.113.9"}]}`))
	}))
	defer server.Close()

	ip, err := resolveTargetWithAliDoH(context.Background(), server.Client(), server.URL, "node.example", 1)
	if err != nil {
		t.Fatal(err)
	}
	if ip == nil || ip.String() != "203.0.113.9" {
		t.Fatalf("unexpected resolved IP: %v", ip)
	}
}

func TestNextEchoIDIsUnique(t *testing.T) {
	first := nextEchoID()
	second := nextEchoID()
	if first <= 0 || first > 0xffff || second <= 0 || second > 0xffff || first == second {
		t.Fatalf("unexpected echo IDs: %d %d", first, second)
	}
}
