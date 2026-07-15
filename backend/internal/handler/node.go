package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type NodeHandler struct {
	svc *service.NodeService
}

func NewNodeHandler(svc *service.NodeService) *NodeHandler {
	return &NodeHandler{svc: svc}
}

func (h *NodeHandler) List(c *gin.Context) {
	req := model.NodeListRequest{
		Keyword: c.Query("keyword"),
		Type:    c.Query("type"),
		Status:  c.Query("status"),
		Limit:   parseQueryInt(c, "limit", 50),
		Offset:  parseQueryInt(c, "offset", 0),
	}
	if subscriptionID := parseQueryInt(c, "subscription_id", 0); subscriptionID > 0 {
		req.SubscriptionID = int64(subscriptionID)
	}
	if value, ok := parseQueryBool(c, "enabled"); ok {
		req.Enabled = &value
	}
	if value, ok := parseQueryBool(c, "preferred"); ok {
		req.Preferred = &value
	}
	resp, err := h.svc.List(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) Facets(c *gin.Context) {
	resp, err := h.svc.Facets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_FACETS_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) Import(c *gin.Context) {
	var req model.NodeImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_IMPORT_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.Import(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_IMPORT_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) ImportPreview(c *gin.Context) {
	var req model.NodeImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_IMPORT_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.ImportPreview(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_IMPORT_PREVIEW_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) SetEnabled(c *gin.Context) {
	h.setNodeBool(c, "enabled")
}

func (h *NodeHandler) SetPreferred(c *gin.Context) {
	h.setNodeBool(c, "preferred")
}

func (h *NodeHandler) TCPing(c *gin.Context) {
	var req model.NodeUIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.TCPing(req.UIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_TCPING_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) ExitIP(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_UID_INVALID", Message: "node uid is required"}})
		return
	}
	response, err := h.svc.ExitIP(c.Request.Context(), uid)
	if err != nil {
		status := http.StatusBadGateway
		code := "NODE_EXIT_IP_FAILED"
		if errors.Is(err, service.ErrNodeNotFound) {
			status = http.StatusNotFound
			code = "NODE_NOT_FOUND"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *NodeHandler) Traceroute(c *gin.Context) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_UID_INVALID", Message: "node uid is required"}})
		return
	}
	var req model.NodeTracerouteStartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_TRACEROUTE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.StartTraceroute(uid, req.TraceID, req.GeoProvider)
	if err != nil {
		status := http.StatusInternalServerError
		code := "NODE_TRACEROUTE_FAILED"
		if errors.Is(err, service.ErrNodeNotFound) {
			status = http.StatusNotFound
			code = "NODE_NOT_FOUND"
		} else if errors.Is(err, service.ErrTracerouteInvalid) {
			status = http.StatusBadRequest
			code = "NODE_TRACEROUTE_INVALID"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

func (h *NodeHandler) CancelTraceroute(c *gin.Context) {
	uid := c.Param("uid")
	traceID := c.Param("traceID")
	if uid == "" || traceID == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_TRACEROUTE_INVALID", Message: "node uid and trace id are required"}})
		return
	}
	resp, err := h.svc.CancelTraceroute(uid, traceID)
	if err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: model.APIError{Code: "NODE_TRACEROUTE_NOT_FOUND", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) AddEmoji(c *gin.Context) {
	var req model.NodeUIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.AddEmoji(req.UIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_EMOJI_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) InferFlag(c *gin.Context) {
	var req model.NodeFlagRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FLAG_INVALID", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, h.svc.InferFlag(req))
}

func (h *NodeHandler) InferFlags(c *gin.Context) {
	var req model.NodeFlagBatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FLAG_INVALID", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, h.svc.InferFlags(req))
}

func (h *NodeHandler) BatchRename(c *gin.Context) {
	var req model.NodeBatchRenameRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.BatchRename(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_RENAME_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) BatchDelete(c *gin.Context) {
	var req model.NodeUIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.BatchDelete(req.UIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_DELETE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *NodeHandler) setNodeBool(c *gin.Context, field string) {
	uid := c.Param("uid")
	if uid == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_UID_INVALID", Message: "node uid is required"}})
		return
	}
	var req model.NodeToggleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_INVALID", Message: err.Error()}})
		return
	}
	var resp *model.ActionResponse
	var err error
	if field == "enabled" {
		resp, err = h.svc.SetEnabled(uid, req.Value)
	} else {
		resp, err = h.svc.SetPreferred(uid, req.Value)
	}
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseQueryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return value
}

func parseQueryBool(c *gin.Context, key string) (bool, bool) {
	value := c.Query(key)
	if value == "" {
		return false, false
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, false
	}
	return parsed, true
}
