package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type CoreHandler struct {
	singbox *service.SingboxService
}

func NewCoreHandler(s *service.SingboxService) *CoreHandler {
	return &CoreHandler{singbox: s}
}

func (h *CoreHandler) Start(c *gin.Context) {
	resp, err := h.singbox.Start()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_START_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) Stop(c *gin.Context) {
	resp, err := h.singbox.Stop()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_STOP_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) Restart(c *gin.Context) {
	resp, err := h.singbox.Restart()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_RESTART_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) ReloadConfig(c *gin.Context) {
	resp, err := h.singbox.ReloadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_RELOAD_CONFIG_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
