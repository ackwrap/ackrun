package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type RuntimeHandler struct {
	svc *service.RuntimeService
}

func NewRuntimeHandler(svc *service.RuntimeService) *RuntimeHandler {
	return &RuntimeHandler{svc: svc}
}

func (h *RuntimeHandler) GetStatus(c *gin.Context) {
	resp, err := h.svc.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "RUNTIME_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
