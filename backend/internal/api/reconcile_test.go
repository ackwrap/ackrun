package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
	"github.com/gin-gonic/gin"
)

func TestShouldReconcileRequest(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   bool
	}{
		{http.MethodPut, "/api/v1/settings/dns", true},
		{http.MethodPut, "/api/v1/settings/inbound-mode", false},
		{http.MethodPut, "/api/v1/settings/proxy-mode", false},
		{http.MethodPost, "/api/v1/settings/geoip-providers", false},
		{http.MethodDelete, "/api/v1/settings/connectivity-targets/1", false},
		{http.MethodPost, "/api/v1/settings/node-filters", false},
		{http.MethodDelete, "/api/v1/settings/node-filters/1", false},
		{http.MethodPut, "/api/v1/settings/core-restart", false},
		{http.MethodPost, "/api/v1/nodes/import", true},
		{http.MethodPost, "/api/v1/nodes/import/preview", false},
		{http.MethodPost, "/api/v1/nodes/flag", false},
		{http.MethodPost, "/api/v1/nodes/flags", false},
		{http.MethodPut, "/api/v1/collections/1", true},
		{http.MethodPost, "/api/v1/collections/reorder", true},
		{http.MethodPost, "/api/v1/collections/1/test", false},
		{http.MethodPost, "/api/v1/dns/servers/reorder", true},
		{http.MethodPost, "/api/v1/dns/rules", true},
		{http.MethodPut, "/api/v1/dns/servers/1", true},
		{http.MethodDelete, "/api/v1/dns/rules/1", true},
		{http.MethodDelete, "/api/v1/subscriptions/1", true},
		{http.MethodPost, "/api/v1/subscriptions/1/sync", false},
		{http.MethodPost, "/api/v1/nodes/tcping", false},
		{http.MethodPost, "/api/v1/nodes/uid/exit-ip", false},
		{http.MethodPost, "/api/v1/nodes/uid/traceroute", false},
		{http.MethodPost, "/api/v1/config/generate", false},
		{http.MethodPost, "/api/v1/core/restart", false},
		{http.MethodPost, "/api/v1/rules", false},
		{http.MethodPut, "/api/v1/rules/1", true},
		{http.MethodPut, "/api/v1/settings/traffic-bypass", true},
		{http.MethodGet, "/api/v1/rules", false},
	}

	for _, test := range tests {
		if got := shouldReconcileRequest(test.method, test.path); got != test.want {
			t.Errorf("shouldReconcileRequest(%q, %q) = %v, want %v", test.method, test.path, got, test.want)
		}
	}
}

func TestRouteRuleCreateReconcileDecision(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	routeRuleHandler := handler.NewRouteRuleHandler(service.NewRouteRuleService(db, nil, nil))
	reasons := make([]string, 0, 3)
	router := gin.New()
	v1 := router.Group("/api/v1")
	v1.Use(configMutationMiddlewareWithTrigger(func(reason string) {
		reasons = append(reasons, reason)
	}))
	v1.POST("/rules", routeRuleHandler.Create)
	v1.PUT("/collections/:id", func(c *gin.Context) {
		c.Status(http.StatusOK)
	})

	tests := []struct {
		name          string
		outbound      string
		wantReconcile bool
	}{
		{name: "direct", outbound: "direct", wantReconcile: true},
		{name: "block", outbound: "block", wantReconcile: true},
		{name: "proxy", outbound: "proxy", wantReconcile: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := len(reasons)
			body := []byte(`{"name":"` + test.name + ` rule","enabled":true,"rule_type":"domain","values":["` + test.name + `.example"],"outbound":"` + test.outbound + `"}`)
			request := httptest.NewRequest(http.MethodPost, "/api/v1/rules", bytes.NewReader(body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if got := len(reasons) > before; got != test.wantReconcile {
				t.Fatalf("reconciled = %v, want %v", got, test.wantReconcile)
			}
		})
	}

	request := httptest.NewRequest(http.MethodPut, "/api/v1/collections/1", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("collection status = %d", response.Code)
	}
	if got := reasons[len(reasons)-1]; got != "PUT /api/v1/collections/1" {
		t.Fatalf("last reconcile reason = %q, want collection configuration", got)
	}
}
