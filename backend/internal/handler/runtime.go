package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
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

func (h *RuntimeHandler) GetSummary(c *gin.Context) {
	resp, err := h.svc.GetSummary()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SUMMARY_ERROR", Message: "加载流量与订阅概览失败"},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
