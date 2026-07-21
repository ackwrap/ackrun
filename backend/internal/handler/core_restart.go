package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type CoreRestartHandler struct {
	scheduler *service.CoreRestartScheduler
}

func NewCoreRestartHandler(scheduler *service.CoreRestartScheduler) *CoreRestartHandler {
	return &CoreRestartHandler{scheduler: scheduler}
}

func (h *CoreRestartHandler) GetSettings(c *gin.Context) {
	settings, err := h.scheduler.GetSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "CORE_RESTART_SETTINGS_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *CoreRestartHandler) UpdateSettings(c *gin.Context) {
	var settings model.CoreRestartSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}
	updated, err := h.scheduler.UpdateSettings(&settings)
	if err != nil {
		if errors.Is(err, service.ErrInvalidCoreRestartSettings) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "CORE_RESTART_SETTINGS_INVALID", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "CORE_RESTART_SETTINGS_SAVE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, updated)
}
