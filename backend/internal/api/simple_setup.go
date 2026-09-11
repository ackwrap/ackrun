package api

import (
	"net/http"
	"strings"

	"github.com/ackwrap/ackrun/internal/handler"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

func RegisterSimpleSetupRoutes(r *gin.Engine, svc *service.SimpleSetupService) {
	h := handler.NewSimpleSetupHandler(svc)
	r.GET("/api/v1/setup", h.Status)
	r.POST("/api/v1/setup", h.Start)
}

func SimpleSetupMutationGuard(svc *service.SimpleSetupService) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.HasPrefix(c.Request.URL.Path, "/api/v1/") || c.Request.URL.Path == "/api/v1/setup" || c.Request.Method == http.MethodGet || c.Request.Method == http.MethodHead || c.Request.Method == http.MethodOptions {
			c.Next()
			return
		}
		release, err := svc.HoldMutation()
		if err != nil {
			c.AbortWithStatusJSON(http.StatusConflict, model.ErrorResponse{Error: model.APIError{Code: "SETUP_BUSY", Message: err.Error()}})
			return
		}
		defer release()
		c.Next()
	}
}
