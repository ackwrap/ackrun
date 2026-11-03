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
}
