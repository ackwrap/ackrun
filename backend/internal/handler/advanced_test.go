package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/service"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestAdvancedHandlerSettingsAndValidation(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	handler := NewAdvancedHandler(service.NewAdvancedRoutingService(db, nil, nil, nil, "test-secret"))
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/settings", handler.GetSettings)
	router.POST("/routes", handler.CreatePlatformRoute)

	settings := httptest.NewRecorder()
	router.ServeHTTP(settings, httptest.NewRequest(http.MethodGet, "/settings", nil))
	if settings.Code != http.StatusOK || !strings.Contains(settings.Body.String(), `"routing_enabled":false`) || !strings.Contains(settings.Body.String(), `"access_logs_enabled":false`) {
		t.Fatalf("settings response = %d %s", settings.Code, settings.Body.String())
	}
	invalid := httptest.NewRecorder()
	router.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/routes", strings.NewReader(`{"name":"","platform":"x","target_type":"direct"}`)))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "ADVANCED_PLATFORM_ROUTE_CREATE_FAILED") {
		t.Fatalf("invalid response = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestAdvancedHandlerRedactsRuntimeApplyError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := &AdvancedHandler{}
	router := gin.New()
	router.GET("/runtime-error", func(c *gin.Context) {
		handler.respond(c, nil, fmt.Errorf("%w: outbound not found: password=sentinel", service.ErrAdvancedApply), "ADVANCED_APPLY_FAILED")
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/runtime-error", nil))
	if response.Code != http.StatusBadRequest || strings.Contains(response.Body.String(), "sentinel") || strings.Contains(response.Body.String(), "outbound not found") {
		t.Fatalf("runtime error was not redacted: %d %s", response.Code, response.Body.String())
	}
	if !strings.Contains(response.Body.String(), "ADVANCED_APPLY_FAILED") {
		t.Fatalf("runtime error code was lost: %s", response.Body.String())
	}
}
