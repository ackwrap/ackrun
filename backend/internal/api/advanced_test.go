package api

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRegisterAdvancedRoutesContract(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1")
	registerAdvancedRoutes(group, handler.NewAdvancedHandler(service.NewAdvancedRoutingService(db, nil, nil, nil, "test-secret")))
	want := map[string]bool{
		"GET /api/v1/advanced/platform-routes":           false,
		"POST /api/v1/advanced/platform-routes":          false,
		"PUT /api/v1/advanced/platform-routes/:id":       false,
		"DELETE /api/v1/advanced/platform-routes/:id":    false,
		"POST /api/v1/advanced/platform-routes/reorder":  false,
		"GET /api/v1/advanced/session-leases":            false,
		"POST /api/v1/advanced/session-leases":           false,
		"PUT /api/v1/advanced/session-leases/:id":        false,
		"DELETE /api/v1/advanced/session-leases/:id":     false,
		"POST /api/v1/advanced/session-leases/:id/renew": false,
		"GET /api/v1/advanced/health-scheduling":         false,
		"POST /api/v1/advanced/health-scheduling/run":    false,
		"GET /api/v1/advanced/access-logs":               false,
		"DELETE /api/v1/advanced/access-logs":            false,
		"GET /api/v1/advanced/settings":                  false,
		"PUT /api/v1/advanced/settings":                  false,
	}
	for _, route := range router.Routes() {
		key := route.Method + " " + route.Path
		if _, exists := want[key]; exists {
			want[key] = true
		}
	}
	for route, found := range want {
		if !found {
			t.Errorf("route not registered: %s", route)
		}
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/advanced/access-logs?page=0", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("invalid pagination status = %d body=%s", recorder.Code, recorder.Body.String())
	}
}
