package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
	"github.com/gin-gonic/gin"
)

// ProxyCollectionHandler 代理集合处理器
type ProxyCollectionHandler struct {
	service *service.ProxyCollectionService
}

// NewProxyCollectionHandler 创建代理集合处理器
func NewProxyCollectionHandler(service *service.ProxyCollectionService) *ProxyCollectionHandler {
	return &ProxyCollectionHandler{service: service}
}

// Create 创建代理集合
func (h *ProxyCollectionHandler) Create(c *gin.Context) {
	var req model.ProxyCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}

	result, err := h.service.Create(req)
	if err != nil {
		if errors.Is(err, service.ErrSystemProxyCollectionProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_COLLECTION_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "CREATE_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Get 获取代理集合
func (h *ProxyCollectionHandler) Get(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的集合 ID"}})
		return
	}

	result, err := h.service.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: model.APIError{Code: "NOT_FOUND", Message: "集合不存在"}})
		return
	}

	c.JSON(http.StatusOK, result)
}

// List 列出所有代理集合
func (h *ProxyCollectionHandler) List(c *gin.Context) {
	result, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "LIST_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, result)
}

// Update 更新代理集合
func (h *ProxyCollectionHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的集合 ID"}})
		return
	}

	var req model.ProxyCollectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}

	if err := h.service.Update(id, req); err != nil {
		if errors.Is(err, service.ErrSystemProxyCollectionProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_COLLECTION_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "UPDATE_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "代理集合已更新"})
}

// Delete 删除代理集合
func (h *ProxyCollectionHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的集合 ID"}})
		return
	}

	if err := h.service.Delete(id); err != nil {
		if errors.Is(err, service.ErrSystemProxyCollectionProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_COLLECTION_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "DELETE_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "代理集合已删除"})
}

// ToggleEnabled 切换启用状态
func (h *ProxyCollectionHandler) ToggleEnabled(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的集合 ID"}})
		return
	}

	if err := h.service.ToggleEnabled(id); err != nil {
		if errors.Is(err, service.ErrSystemProxyCollectionProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_COLLECTION_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "TOGGLE_FAILED", Message: err.Error()}})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "状态已更新"})
}

func (h *ProxyCollectionHandler) Test(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的集合 ID"}})
		return
	}
	result, err := h.service.Test(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "COLLECTION_TEST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, result)
}
