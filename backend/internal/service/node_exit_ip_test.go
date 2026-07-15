package service

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestParseNodeExitIP(t *testing.T) {
	ip, err := parseNodeExitIP("fl=1\nip=203.0.113.8\nloc=ZZ\n")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "203.0.113.8" {
		t.Fatalf("parsed IP = %s", ip)
	}
	if _, err := parseNodeExitIP("ip=invalid\n"); err == nil {
		t.Fatal("expected invalid exit IP error")
	}
}

func TestNodeExitIPProxyURLReadsActiveMixedInbound(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"inbounds":[{"type":"mixed","tag":"mixed-in","listen":"0.0.0.0","listen_port":2080}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	svc := &NodeService{paths: &paths.Paths{ConfigDir: configDir, ConfigPath: configPath}}
	proxyURL, err := svc.nodeExitIPProxyURL()
	if err != nil {
		t.Fatal(err)
	}
	if proxyURL.String() != "http://127.0.0.1:2080" {
		t.Fatalf("proxy URL = %s", proxyURL)
	}
}

func TestNodeExitIPProxyURLRejectsNonLoopbackInbound(t *testing.T) {
	configDir := t.TempDir()
	configPath := filepath.Join(configDir, "config.json")
	if err := os.WriteFile(configPath, []byte(`{"inbounds":[{"type":"mixed","tag":"mixed-in","listen":"192.0.2.10","listen_port":2080}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	svc := &NodeService{paths: &paths.Paths{ConfigDir: configDir, ConfigPath: configPath}}
	if _, err := svc.nodeExitIPProxyURL(); err == nil {
		t.Fatal("expected non-loopback mixed inbound rejection")
	}
}

func TestSelectNodeCheckOutbound(t *testing.T) {
	current := "old-node"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			_ = json.NewEncoder(writer).Encode(map[string]any{"now": current, "all": []string{"old-node", "new-node"}})
		case http.MethodPut:
			var body map[string]string
			_ = json.NewDecoder(request.Body).Decode(&body)
			current = body["name"]
			writer.WriteHeader(http.StatusNoContent)
		default:
			writer.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()
	svc := &NodeService{clashBaseURL: server.URL, httpClient: server.Client()}
	previous, err := svc.selectNodeCheckOutbound(context.Background(), "new-node")
	if err != nil {
		t.Fatal(err)
	}
	if previous != "old-node" || current != "new-node" {
		t.Fatalf("previous=%q current=%q", previous, current)
	}
}

func TestExitIPRestoresInternalSelector(t *testing.T) {
	for _, restoreFails := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "restore failure"}[restoreFails], func(t *testing.T) {
			db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "exit-check", URL: "https://example.com/subscription"})
			if err != nil {
				t.Fatal(err)
			}
			if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{
				{Name: "Old", Type: "socks", Server: "203.0.113.10", ServerPort: 1080, RawJSON: `{"type":"socks","server":"203.0.113.10","server_port":1080}`},
				{Name: "Target", Type: "socks", Server: "203.0.113.11", ServerPort: 1080, RawJSON: `{"type":"socks","server":"203.0.113.11","server_port":1080}`},
			}); err != nil {
				t.Fatal(err)
			}
			nodes, err := db.ListEnabledNodes()
			if err != nil || len(nodes) != 2 {
				t.Fatalf("enabled nodes: %d, %v", len(nodes), err)
			}
			tags := buildNodeOutboundTags(nodes)
			oldTag, targetTag := tags[nodes[0].UID], tags[nodes[1].UID]
			current := oldTag
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.Method {
				case http.MethodGet:
					_ = json.NewEncoder(writer).Encode(map[string]any{"now": current, "all": []string{oldTag, targetTag}})
				case http.MethodPut:
					var body map[string]string
					_ = json.NewDecoder(request.Body).Decode(&body)
					if restoreFails && body["name"] == oldTag {
						writer.WriteHeader(http.StatusInternalServerError)
						return
					}
					current = body["name"]
					writer.WriteHeader(http.StatusNoContent)
				}
			}))
			defer server.Close()
			configDir := t.TempDir()
			configPath := filepath.Join(configDir, "config.json")
			if err := os.WriteFile(configPath, []byte(`{"inbounds":[{"type":"mixed","tag":"mixed-in","listen":"127.0.0.1","listen_port":2080}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			svc := NewNodeService(db)
			svc.paths = &paths.Paths{ConfigDir: configDir, ConfigPath: configPath}
			svc.clashBaseURL = server.URL
			svc.httpClient = server.Client()
			svc.exitIPLookup = func(context.Context, *url.URL, bool) (net.IP, error) {
				return net.ParseIP(nodes[1].Server), nil
			}
			response, err := svc.ExitIP(context.Background(), nodes[1].UID)
			if restoreFails {
				if err == nil || !strings.Contains(err.Error(), "恢复内部 selector 失败") {
					t.Fatalf("expected restore error, response=%+v err=%v", response, err)
				}
				return
			}
			if err != nil || response == nil || !response.Matched || current != oldTag {
				t.Fatalf("response=%+v err=%v current=%q", response, err, current)
			}
		})
	}
}

func TestReservedInternalSelectorName(t *testing.T) {
	if !IsReservedProxyCollectionName(nodeCheckOutboundTag) || IsSystemProxyCollectionName(nodeCheckOutboundTag) {
		t.Fatal("internal selector name must be reserved for proxy collections")
	}
	request := &model.NodeGroupRequest{Name: nodeCheckOutboundTag, Type: "selector"}
	if _, err := NewNodeGroupService(nil).Create(request); err == nil {
		t.Fatal("internal selector name must be reserved for node groups")
	}
}

func TestExistingInternalSelectorConflictCanBeRenamed(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	collection := &model.ProxyCollection{
		Name: nodeCheckOutboundTag, Type: "selector", SourceType: "manual", NodeUIDs: `["node-uid"]`,
		TestURL: "https://www.gstatic.com/generate_204", TestInterval: 300, Enabled: true,
	}
	if err := db.CreateProxyCollection(collection); err != nil {
		t.Fatal(err)
	}
	err = NewProxyCollectionService(db, nil).Update(collection.ID, model.ProxyCollectionRequest{
		Name: "renamed-user-group", Type: "selector", SourceType: "manual", NodeUIDs: []string{"node-uid"},
		TestURL: "https://www.gstatic.com/generate_204", TestInterval: 300, Enabled: true,
	})
	if err != nil {
		t.Fatalf("rename existing conflicting collection: %v", err)
	}
}
