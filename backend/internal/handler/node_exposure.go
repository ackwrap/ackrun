package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

type NodeExposureHandler struct {
	service *service.NodeExposureService
}

func NewNodeExposureHandler(service *service.NodeExposureService) *NodeExposureHandler {
	return &NodeExposureHandler{service: service}
}

func (h *NodeExposureHandler) List(c *gin.Context) {
	items, err := h.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_EXPOSURE_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *NodeExposureHandler) Create(c *gin.Context) {
	var req model.NodeExposureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}
	item, err := h.service.Create(req)
	if err != nil {
		writeNodeExposureError(c, "NODE_EXPOSURE_CREATE_FAILED", err)
		return
	}
	c.Set(ConfigReconcileContextKey, false)
	c.JSON(http.StatusOK, item)
}

func (h *NodeExposureHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的节点暴露 ID"}})
		return
	}
	var req model.NodeExposureRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}
	item, err := h.service.Update(id, req)
	if err != nil {
		writeNodeExposureError(c, "NODE_EXPOSURE_UPDATE_FAILED", err)
		return
	}
	c.Set(ConfigReconcileContextKey, false)
	c.JSON(http.StatusOK, item)
}

func (h *NodeExposureHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的节点暴露 ID"}})
		return
	}
	if err := h.service.Delete(id); err != nil {
		writeNodeExposureError(c, "NODE_EXPOSURE_DELETE_FAILED", err)
		return
	}
	c.Set(ConfigReconcileContextKey, false)
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "节点暴露已删除"})
}

func writeNodeExposureError(c *gin.Context, fallbackCode string, err error) {
	status, code := http.StatusInternalServerError, fallbackCode
	switch {
	case errors.Is(err, service.ErrNodeExposureNotFound):
		status, code = http.StatusNotFound, "NODE_EXPOSURE_NOT_FOUND"
	case errors.Is(err, service.ErrNodeExposureConflict):
		status, code = http.StatusConflict, "NODE_EXPOSURE_CONFLICT"
	case errors.Is(err, service.ErrNodeExposureInvalid):
		status, code = http.StatusBadRequest, "NODE_EXPOSURE_INVALID"
	case errors.Is(err, service.ErrNodeExposureApply):
		status, code = http.StatusInternalServerError, "NODE_EXPOSURE_APPLY_FAILED"
	}
	c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
}
