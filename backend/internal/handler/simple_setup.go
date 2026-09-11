package handler

import (
	"net/http"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

type SimpleSetupHandler struct{ svc *service.SimpleSetupService }

func NewSimpleSetupHandler(svc *service.SimpleSetupService) *SimpleSetupHandler {
	return &SimpleSetupHandler{svc: svc}
}

func (h *SimpleSetupHandler) Status(c *gin.Context) {
	status, err := h.svc.Status()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "SETUP_STATUS_FAILED", Message: "无法读取配置任务状态"}})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *SimpleSetupHandler) Start(c *gin.Context) {
	var req model.SimpleSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: "请填写订阅地址"}})
		return
	}
	status, err := h.svc.Start(req)
	if err != nil {
		c.JSON(http.StatusConflict, model.ErrorResponse{Error: model.APIError{Code: "SETUP_NOT_READY", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusAccepted, status)
}
