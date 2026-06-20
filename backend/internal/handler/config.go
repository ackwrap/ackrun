package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type ConfigHandler struct {
	svc *service.ConfigService
}

func NewConfigHandler(svc *service.ConfigService) *ConfigHandler {
	return &ConfigHandler{svc: svc}
}

func (h *ConfigHandler) GetStatus(c *gin.Context) {
	status, err := h.svc.GetConfigStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *ConfigHandler) ListFiles(c *gin.Context) {
	items, err := h.svc.ListConfigFiles()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_LIST_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *ConfigHandler) GenerateDefault(c *gin.Context) {
	if err := h.svc.GenerateDefault(); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_GENERATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "config generated"})
}

func (h *ConfigHandler) Validate(c *gin.Context) {
	if err := h.svc.Validate(); err != nil {
		status, _ := h.svc.GetConfigStatus()
		fileName := ""
		if status != nil {
			fileName = status.FileName
		}
		c.JSON(http.StatusOK, gin.H{
			"has_config": true,
			"valid":      false,
			"file_name":  fileName,
			"error":      err.Error(),
		})
		return
	}
	status, err := h.svc.GetConfigStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (h *ConfigHandler) UpdateRules(c *gin.Context) {
	resp, err := h.svc.UpdateRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "RULE_UPDATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ConfigHandler) Backup(c *gin.Context) {
	resp, err := h.svc.Backup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_BACKUP_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ConfigHandler) Restore(c *gin.Context) {
	resp, err := h.svc.RestoreLatestBackup()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_RESTORE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
