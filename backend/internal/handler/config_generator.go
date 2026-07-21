package handler

import (
	"errors"
	"net/http"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

// ConfigGeneratorHandler 配置生成处理器
type ConfigGeneratorHandler struct {
	service *service.ConfigGeneratorService
}

// NewConfigGeneratorHandler 创建配置生成处理器
func NewConfigGeneratorHandler(service *service.ConfigGeneratorService) *ConfigGeneratorHandler {
	return &ConfigGeneratorHandler{service: service}
}

// Generate 生成配置
func (h *ConfigGeneratorHandler) Generate(c *gin.Context) {
	var req model.ConfigGenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}

	result, err := h.service.Generate(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "GENERATE_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *ConfigGeneratorHandler) GetGenerateRequest(c *gin.Context) {
	request, err := h.service.GetGenerateRequest()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CONFIG_GENERATE_SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, request)
}

// Preview 预览配置
func (h *ConfigGeneratorHandler) Preview(c *gin.Context) {
	defaultOutbound := c.Query("default_outbound")

	config, err := h.service.Preview(defaultOutbound)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "PREVIEW_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, config)
}

// Apply 应用配置
func (h *ConfigGeneratorHandler) Apply(c *gin.Context) {
	var req model.ConfigApplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}

	if err := h.service.Apply(req.FileName, req.RestartCore); err != nil {
		if errors.Is(err, service.ErrInvalidConfigFileName) {
			c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_CONFIG_FILE_NAME", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "APPLY_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "配置已应用"})
}
