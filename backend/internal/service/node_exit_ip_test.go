package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

func TestLookupNodeExitIPUsesDedicatedCoreAPI(t *testing.T) {
	var requests int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.EscapedPath() != "/proxies/Node%20A-test/exit-ip" {
			t.Errorf("path = %s", request.URL.EscapedPath())
		}
		switch request.URL.Query().Get("ip_version") {
		case "4":
			_ = json.NewEncoder(writer).Encode(map[string]any{"ip": "203.0.113.8", "ip_version": 4})
		case "6":
			_ = json.NewEncoder(writer).Encode(map[string]any{"ip": "2001:db8::8", "ip_version": 6})
		default:
			writer.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer server.Close()

	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	for _, item := range []struct {
		ipv6 bool
		want string
	}{{want: "203.0.113.8"}, {ipv6: true, want: "2001:db8::8"}} {
		ip, err := svc.lookupNodeExitIP(context.Background(), "Node A-test", item.ipv6)
		if err != nil {
			t.Fatal(err)
		}
		if ip.String() != item.want {
			t.Fatalf("exit IP = %s, want %s", ip, item.want)
		}
	}
	if requests != 2 {
		t.Fatalf("request count = %d, want 2", requests)
	}
}

func TestLookupNodeExitIPReportsUnsupportedCore(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	_, err := svc.lookupNodeExitIP(context.Background(), "node", false)
	if err == nil || !strings.Contains(err.Error(), "更新核心") {
		t.Fatalf("expected core update error, got %v", err)
	}
}

func TestLookupNodeExitIPReportsDetailedCoreFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"message": "Exit IP service returned an invalid response",
			"stage":   "invalid_response",
		})
	}))
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	_, err := svc.lookupNodeExitIP(context.Background(), "node", false)
	if err == nil || !strings.Contains(err.Error(), "响应格式无效") {
		t.Fatalf("expected detailed response error, got %v", err)
	}
}

func TestLookupNodeExitIPRejectsUnexpectedAddressFamily(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]string{"ip": "2001:db8::8"})
	}))
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	if _, err := svc.lookupNodeExitIP(context.Background(), "node", false); err == nil {
		t.Fatal("expected address family error")
	}
}

func TestResolveActiveNodeOutboundTagAfterRename(t *testing.T) {
	node := model.Node{UID: "1234567890abcdef", Name: "Renamed"}
	activeTag := "Original-1234567890abcdef"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"proxies": map[string]any{activeTag: map[string]any{"type": "Socks5"}},
		})
	}))
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	tag, err := svc.resolveActiveNodeOutboundTag(context.Background(), node, "Renamed-1234567890abcdef")
	if err != nil {
		t.Fatal(err)
	}
	if tag != activeTag {
		t.Fatalf("active tag = %q, want %q", tag, activeTag)
	}
}

func TestResolveActiveNodeOutboundTagRejectsShortUIDGuess(t *testing.T) {
	node := model.Node{UID: "1234567890abcdef", Name: "Renamed"}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"proxies": map[string]any{"Other-12345678": map[string]any{"type": "Socks5"}},
		})
	}))
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	if _, err := svc.resolveActiveNodeOutboundTag(context.Background(), node, "Renamed-1234567890abcdef"); err == nil {
		t.Fatal("short UID suffix must not identify an outbound")
	}
}

func TestExitIPUsesNodeOutboundWithoutSelectorMutation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "exit-check", URL: "https://example.com/subscription"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		Name: "Target", Type: "socks", Server: "203.0.113.11", ServerPort: 1080,
		RawJSON: `{"type":"socks","server":"203.0.113.11","server_port":1080}`,
	}}); err != nil {
		t.Fatal(err)
	}
	nodes, err := db.ListEnabledNodes()
	if err != nil || len(nodes) != 1 {
		t.Fatalf("enabled nodes: %d, %v", len(nodes), err)
	}
	expectedTag := buildNodeOutboundTags(nodes)[nodes[0].UID]
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", request.Method)
		}
		if request.URL.Path == "/proxies" {
			_ = json.NewEncoder(writer).Encode(map[string]any{
				"proxies": map[string]any{expectedTag: map[string]any{"type": "Socks5"}},
			})
			return
		}
		wantPath := "/proxies/" + url.PathEscape(expectedTag) + "/exit-ip"
		if request.URL.EscapedPath() != wantPath {
			t.Errorf("path = %s, want %s", request.URL.EscapedPath(), wantPath)
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{"ip": "203.0.113.11", "ip_version": 4})
	}))
	defer server.Close()
	geoFails := false
	geoServer := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if geoFails {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		if request.URL.Query().Get("ip") != "203.0.113.11" {
			t.Errorf("Geo query target = %q", request.URL.Query().Get("ip"))
		}
		_ = json.NewEncoder(writer).Encode(map[string]any{
			"code": 200,
			"data": map[string]any{
				"countryCode": "SG", "country": "Singapore", "city": "Singapore", "isp": "Example ISP",
			},
		})
	}))
	defer geoServer.Close()
	t.Setenv("NEXTTRACE_SONGZIXIAN_IP_BASE", geoServer.URL)

	svc := NewNodeService(db)
	svc.clashBaseURL = server.URL
	svc.httpClient = server.Client()
	response, err := svc.ExitIP(context.Background(), nodes[0].UID, "")
	if err != nil {
		t.Fatal(err)
	}
	if response == nil || !response.Matched || response.GeoProvider != "songzixian" {
		t.Fatalf("response = %+v", response)
	}
	if response.Geo == nil || response.Geo.Country != "新加坡" || response.Geo.Source != "松子 IP" {
		t.Fatalf("Geo response = %+v", response.Geo)
	}

	geoFails = true
	svc.localGeoLookup = func(ip net.IP) (traceroute.GeoData, error) {
		if ip.String() != "203.0.113.11" {
			t.Fatalf("local Geo query target = %s", ip)
		}
		return traceroute.GeoData{Country: "新加坡", Source: "geoip.db（本地回退）"}, nil
	}
	response, err = svc.ExitIP(context.Background(), nodes[0].UID, "")
	if err != nil {
		t.Fatal(err)
	}
	if response.Geo == nil || response.Geo.Source != "geoip.db（本地回退）" || response.GeoError != "" {
		t.Fatalf("local fallback response = %+v", response)
	}
}
