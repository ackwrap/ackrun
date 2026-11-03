package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRegisterNodeExposureRoutesContract(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	registerNodeExposureRoutes(
		group,
		handler.NewNodeExposureHandler(service.NewNodeExposureService(db)),
	)
	want := map[string]bool{
		"GET /api/v1/advanced/node-exposures":           false,
		"GET /api/v1/advanced/node-exposures/:id/share": false,
		"POST /api/v1/advanced/node-exposures":          false,
		"PUT /api/v1/advanced/node-exposures/:id":       false,
		"DELETE /api/v1/advanced/node-exposures/:id":    false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = true
		} else {
			t.Errorf("unexpected route registered: %s", key)
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("route not registered: %s", route)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/advanced/node-exposures", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list bindings status = %d body=%s", recorder.Code, recorder.Body.String())
	}

	subscription, err := db.CreateSubscription(&model.SubscriptionRequest{Name: "synthetic", URL: "https://example.invalid/test"})
	if err != nil {
		t.Fatal(err)
	}
	item := &model.NodeExposure{
		Name: "synthetic", SubscriptionID: subscription.ID, NodeUID: "synthetic",
		InboundType: "mixed", Listen: "127.0.0.1", ListenPort: 18080,
		Username: "synthetic", Password: "synthetic-secret",
	}
	if err := db.CreateNodeExposure(item); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id   string
		want int
	}{
		{strconv.FormatInt(item.ID, 10), http.StatusOK},
		{"0", http.StatusBadRequest},
		{"invalid", http.StatusBadRequest},
		{"9999", http.StatusNotFound},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/advanced/node-exposures/"+tc.id+"/share", nil))
		if response.Code != tc.want || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("share response: status=%d cache=%q", response.Code, response.Header().Get("Cache-Control"))
		}
		if tc.want == http.StatusOK {
			var shared model.NodeShareResponse
			if err := json.Unmarshal(response.Body.Bytes(), &shared); err != nil {
				t.Fatal(err)
			}
			if !strings.HasPrefix(shared.URI, "socks5://") {
				t.Fatal("share endpoint did not return a proxy link")
			}
		}
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/advanced/node-exposures", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("list bindings status = %d", response.Code)
	}
	if strings.Contains(response.Body.String(), item.Password) {
		t.Fatal("list endpoint exposed the share password")
	}
}
