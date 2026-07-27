package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
)

func TestLoadOrCreateCoreAPITokenPersistsProtectedSecret(t *testing.T) {
	configDir := t.TempDir()
	runtimePaths := &paths.Paths{ConfigDir: configDir}
	first, err := LoadOrCreateCoreAPIToken(runtimePaths)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadOrCreateCoreAPIToken(runtimePaths)
	if err != nil {
		t.Fatal(err)
	}
	if first == "" || first != second || len(first) != coreAPITokenBytes*2 {
		t.Fatalf("persisted token mismatch: first=%d bytes second=%d bytes", len(first), len(second))
	}
	info, err := os.Stat(filepath.Join(configDir, "core-api-token"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("token permissions = %o", info.Mode().Perm())
		}
	}
}

func TestNodeExposureRuntimeClientSyncsDesiredState(t *testing.T) {
	const secret = "runtime-test-secret"
	var putPayload coreNodeExposureRequest
	putCalls := 0
	deleted := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/health":
			_ = json.NewEncoder(writer).Encode(map[string]string{"status": "ok"})
		case request.Method == http.MethodGet && request.URL.Path == "/api/v1/node-exposures":
			_ = json.NewEncoder(writer).Encode(map[string]interface{}{
				"items": []map[string]string{{"id": "1"}, {"id": "99"}},
			})
		case request.Method == http.MethodPut && request.URL.Path == "/api/v1/node-exposures/1":
			putCalls++
			if err := json.NewDecoder(request.Body).Decode(&putPayload); err != nil {
				t.Errorf("decode PUT: %v", err)
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{"id": "1"})
		case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/api/v1/node-exposures/"):
			deleted = append(deleted, strings.TrimPrefix(request.URL.Path, "/api/v1/node-exposures/"))
			writer.WriteHeader(http.StatusNoContent)
		default:
			http.Error(writer, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := NewNodeExposureRuntimeClient(secret)
	client.baseURL = server.URL + "/api/v1"
	item := model.NodeExposureWithNode{
		NodeExposure: model.NodeExposure{
			ID:          1,
			NodeUID:     "uid-1",
			InboundType: "mixed",
			Listen:      "127.0.0.1",
			ListenPort:  18080,
			Username:    "test-user",
			Password:    "test-password",
			Enabled:     true,
		},
		NodeName:    "test node",
		NodeType:    "socks",
		NodeExists:  true,
		NodeEnabled: true,
	}
	if err := client.Sync([]model.NodeExposureWithNode{item}); err != nil {
		t.Fatal(err)
	}
	if putCalls != 1 || len(deleted) != 1 || deleted[0] != "99" {
		t.Fatalf("PUT calls = %d, deleted = %+v", putCalls, deleted)
	}
	if putPayload.OutboundTag != "test-node-uid-1" {
		t.Fatalf("outbound tag = %q", putPayload.OutboundTag)
	}
	if putPayload.Inbound["type"] != "mixed" || putPayload.Inbound["listen"] != "127.0.0.1" {
		t.Fatalf("inbound = %+v", putPayload.Inbound)
	}
	users, ok := putPayload.Inbound["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("users = %#v", putPayload.Inbound["users"])
	}
}

func TestNodeExposureRuntimeClientRedactsSecretFromErrors(t *testing.T) {
	const secret = "runtime-test-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusInternalServerError)
		_, _ = writer.Write([]byte(`{"error":{"message":"rejected ` + secret + `"}}`))
	}))
	defer server.Close()
	client := NewNodeExposureRuntimeClient(secret)
	client.baseURL = server.URL
	_, err := client.request(http.MethodGet, "/health", nil, http.StatusOK)
	if err == nil || strings.Contains(err.Error(), secret) || !strings.Contains(err.Error(), "[REDACTED]") {
		t.Fatalf("error was not redacted: %v", err)
	}
}

func TestRuntimeRoutingHTTPContract(t *testing.T) {
	const secret = "runtime-routing-secret"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "Bearer "+secret {
			t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
		}
		switch request.URL.Path {
		case "/api/v1/runtime-routing":
			if request.Method == http.MethodGet {
				_ = json.NewEncoder(writer).Encode(RuntimeRoutingConfig{
					Routes: []RuntimeRoute{}, Leases: []RuntimeLease{}, UnhealthyOutbounds: []string{},
					AccessEventsPrivacyMode: model.AdvancedPrivacyStrict,
				})
				return
			}
			var payload RuntimeRoutingConfig
			if request.Method != http.MethodPut || request.Header.Get("Content-Type") != "application/json" {
				t.Fatalf("runtime routing request = %s content-type=%q", request.Method, request.Header.Get("Content-Type"))
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if len(payload.Routes) != 1 || payload.Routes[0].ID != "route-7" {
				t.Fatalf("runtime routing payload = %+v", payload)
			}
			_ = json.NewEncoder(writer).Encode(payload)
		case "/api/v1/access-events":
			if request.Method != http.MethodGet || request.URL.Query().Get("after") != "42" || request.URL.Query().Get("limit") != "100" {
				t.Fatalf("access event request = %s %s", request.Method, request.URL.String())
			}
			_ = json.NewEncoder(writer).Encode(RuntimeAccessEventList{LatestID: 43, Items: []RuntimeAccessEvent{{ID: 43, RouteID: "route-7"}}})
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	client := NewNodeExposureRuntimeClient(secret)
	client.baseURL = server.URL + "/api/v1"
	config := RuntimeRoutingConfig{
		Routes: []RuntimeRoute{{ID: "route-7", OutboundTag: "direct"}}, Leases: []RuntimeLease{}, UnhealthyOutbounds: []string{},
		AccessEventsEnabled: true, AccessEventsPrivacyMode: model.AdvancedPrivacyBalanced,
	}
	if err := client.PutRuntimeRouting(config); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRuntimeRouting(); err != nil {
		t.Fatal(err)
	}
	events, err := client.GetAccessEvents(42, 100)
	if err != nil || events.LatestID != 43 || len(events.Items) != 1 {
		t.Fatalf("events = %+v, err = %v", events, err)
	}
}

func TestRuntimeAPIServiceConfigAndRedaction(t *testing.T) {
	const secret = "runtime-test-secret"
	serviceConfig := singboxRuntimeAPIServiceConfig(secret)
	if serviceConfig["listen"] != "127.0.0.1" || serviceConfig["listen_port"] != 9097 || serviceConfig["secret"] != secret {
		t.Fatalf("service config = %+v", serviceConfig)
	}
	redacted := redactConfigAccessTokens(map[string]interface{}{"service": serviceConfig}).(map[string]interface{})
	redactedService := redacted["service"].(map[string]interface{})
	if redactedService["secret"] != "[REDACTED]" {
		t.Fatalf("redacted service config = %+v", redactedService)
	}
	if serviceConfig["secret"] != secret {
		t.Fatal("redaction mutated the source config")
	}
	encoded, err := json.Marshal(map[string]interface{}{"services": []map[string]interface{}{serviceConfig}})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !runtimeAPIServiceMatches(decoded["services"], secret) || runtimeAPIServiceMatches(decoded["services"], "wrong-secret") {
		t.Fatalf("runtime API service matching failed: %+v", decoded["services"])
	}
}

func TestSingboxRuntimeAPIPortRecognizesEquivalentValues(t *testing.T) {
	for _, value := range []string{"9097", "09097", " 9097 "} {
		if !isSingboxRuntimeAPIPort(value) {
			t.Fatalf("port %q did not conflict", value)
		}
	}
	for _, value := range []string{"", "9096", "invalid"} {
		if isSingboxRuntimeAPIPort(value) {
			t.Fatalf("port %q unexpectedly conflicted", value)
		}
	}
}
