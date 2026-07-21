package service

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestUsesUDPTransport(t *testing.T) {
	for _, nodeType := range []string{"hysteria", "hysteria2", "tuic", "wireguard", " TUIC "} {
		if !usesUDPTransport(nodeType) {
			t.Fatalf("expected %q to use UDP transport", nodeType)
		}
	}
	for _, nodeType := range []string{"vmess", "vless", "trojan", "shadowsocks", "socks"} {
		if usesUDPTransport(nodeType) {
			t.Fatalf("expected %q to keep TCPing", nodeType)
		}
	}
}

func TestURLTestNodeUsesSingboxClashAPI(t *testing.T) {
	node := model.Node{UID: "1234567890abcdef", Name: "UDP Test", Type: "hysteria2"}
	tag := buildNodeOutboundTags([]model.Node{node})[node.UID]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if !strings.HasPrefix(r.URL.Path, "/proxies/"+tag+"/delay") {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("url") != "https://www.gstatic.com/generate_204" {
			t.Fatalf("unexpected test URL: %s", r.URL.Query().Get("url"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"delay":42}`))
	}))
	defer server.Close()

	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	result := svc.urlTestNode(node, tag)
	if !result.Success || result.LatencyMS != 42 || result.Error != "" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestURLTestNodeReportsMissingActiveOutbound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	node := model.Node{UID: "udp-node", Name: "UDP Test", Type: "tuic"}
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	result := svc.urlTestNode(node, "udp-test")
	if result.Success || !strings.Contains(result.Error, "未载入当前配置") {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestTCPingNodeRecordsSubMillisecondSuccess(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	address := listener.Addr().(*net.TCPAddr)

	result := (&NodeService{}).tcpingNode(model.Node{
		UID:        "local-node",
		Server:     address.IP.String(),
		ServerPort: address.Port,
	})
	if !result.Success || result.LatencyMS < 1 {
		t.Fatalf("local TCPing result = %+v, want successful positive latency", result)
	}
}

func TestTCPingNodeUsesResolvedAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()
	port := listener.Addr().(*net.TCPAddr).Port
	svc := &NodeService{
		resolveTCPing: func(context.Context, string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("127.0.0.1")}, nil
		},
	}
	result := svc.tcpingNode(model.Node{UID: "resolved", Server: "not-resolved.invalid", ServerPort: port})
	if !result.Success || result.LatencyMS < 1 {
		t.Fatalf("resolved TCPing result = %+v, want success", result)
	}
}

func TestElapsedLatencyMilliseconds(t *testing.T) {
	tests := []struct {
		name    string
		elapsed time.Duration
		want    int
	}{
		{name: "zero", elapsed: 0, want: 1},
		{name: "sub millisecond", elapsed: 999 * time.Microsecond, want: 1},
		{name: "one millisecond", elapsed: time.Millisecond, want: 1},
		{name: "multiple milliseconds", elapsed: 42 * time.Millisecond, want: 42},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := elapsedLatencyMilliseconds(test.elapsed); got != test.want {
				t.Fatalf("elapsedLatencyMilliseconds(%s) = %d, want %d", test.elapsed, got, test.want)
			}
		})
	}
}
