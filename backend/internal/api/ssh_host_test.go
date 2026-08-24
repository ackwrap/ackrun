package api

import (
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestRegisterSSHHostRoutesContract(t *testing.T) {
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := service.NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerSSHHostRoutes(router.Group("/api/v1"), handler.NewSSHHostHandler(svc))
	want := map[string]bool{
		"GET /api/v1/advanced/ssh/hosts":                       false,
		"POST /api/v1/advanced/ssh/hosts":                      false,
		"POST /api/v1/advanced/ssh/hosts/import":               false,
		"POST /api/v1/advanced/ssh/hosts/share":                false,
		"GET /api/v1/advanced/ssh/hosts/:id":                   false,
		"PUT /api/v1/advanced/ssh/hosts/:id":                   false,
		"DELETE /api/v1/advanced/ssh/hosts/:id":                false,
		"POST /api/v1/advanced/ssh/hosts/:id/share":            false,
		"POST /api/v1/advanced/ssh/hosts/:id/test":             false,
		"POST /api/v1/advanced/ssh/hosts/:id/sessions":         false,
		"DELETE /api/v1/advanced/ssh/sessions/:sessionID":      false,
		"GET /api/v1/advanced/ssh/sessions/:sessionID/monitor": false,
		"GET /api/v1/advanced/ssh/sessions/:sessionID/details": false,
		"GET /api/v1/advanced/ssh/hosts/:id/host-key":          false,
		"POST /api/v1/advanced/ssh/hosts/:id/host-key/trust":   false,
		"POST /api/v1/advanced/ssh/hosts/:id/host-key/rotate":  false,
		"DELETE /api/v1/advanced/ssh/hosts/:id/host-key":       false,
		"GET /api/v1/advanced/ssh/credentials":                 false,
		"POST /api/v1/advanced/ssh/credentials":                false,
		"PUT /api/v1/advanced/ssh/credentials/:id":             false,
		"DELETE /api/v1/advanced/ssh/credentials/:id":          false,
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
}
