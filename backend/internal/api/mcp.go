package api

import (
	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/service"
)

func RegisterMCPRoutes(router *gin.Engine, settings *service.MCPService, apiToken string) {
	h := handler.NewMCPHandler(settings, router, apiToken)
	router.GET("/api/v1/mcp/settings", h.GetSettings)
	router.PUT("/api/v1/mcp/settings", h.UpdateSettings)
	for _, endpoint := range []string{service.MCPEndpoint, service.MCPEndpoint + "/"} {
		router.GET(endpoint, h.ServeHTTP)
		router.POST(endpoint, h.ServeHTTP)
		router.DELETE(endpoint, h.ServeHTTP)
	}
}
