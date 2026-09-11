package api

import (
	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/service"
)

func RegisterSSHMCPRoutes(router *gin.Engine, settings *service.SSHMCPService, hosts *service.SSHHostService) {
	h := handler.NewSSHMCPHandler(settings, hosts)
	router.GET("/api/v1/advanced/ssh/mcp/settings", h.GetSettings)
	router.PUT("/api/v1/advanced/ssh/mcp/settings", h.UpdateSettings)
	for _, endpoint := range []string{service.SSHMCPEndpoint, service.SSHMCPEndpoint + "/"} {
		router.GET(endpoint, h.ServeHTTP)
		router.POST(endpoint, h.ServeHTTP)
		router.DELETE(endpoint, h.ServeHTTP)
	}
}
