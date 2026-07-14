package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
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
