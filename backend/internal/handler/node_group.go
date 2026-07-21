package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

type NodeGroupHandler struct {
	svc *service.NodeGroupService
}

func NewNodeGroupHandler(svc *service.NodeGroupService) *NodeGroupHandler {
	return &NodeGroupHandler{svc: svc}
}

func (h *NodeGroupHandler) List(c *gin.Context) {
	groups, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUPS_LIST_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, groups)
}

func (h *NodeGroupHandler) Get(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	group, err := h.svc.Get(id)
	if err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUP_NOT_FOUND", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, group)
}

func (h *NodeGroupHandler) Create(c *gin.Context) {
	var req model.NodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	group, err := h.svc.Create(&req)
	if err != nil {
		if isNodeGroupNameConflict(err) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error: model.APIError{Code: "NODE_GROUP_NAME_EXISTS", Message: "节点组名称已存在"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUP_CREATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusCreated, group)
}

func (h *NodeGroupHandler) Update(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req model.NodeGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.Update(id, &req); err != nil {
		if isNodeGroupNameConflict(err) {
			c.JSON(http.StatusConflict, model.ErrorResponse{
				Error: model.APIError{Code: "NODE_GROUP_NAME_EXISTS", Message: "节点组名称已存在"},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUP_UPDATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "Node group updated"})
}

func isNodeGroupNameConflict(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") && strings.Contains(msg, "node_groups") && strings.Contains(msg, "name")
}

func (h *NodeGroupHandler) Delete(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUP_DELETE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "Node group deleted"})
}

func (h *NodeGroupHandler) BatchDelete(c *gin.Context) {
	var req model.NodeGroupIDsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}
	if err := h.svc.BatchDelete(req.IDs); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUP_BATCH_DELETE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "Node groups deleted"})
}

func (h *NodeGroupHandler) Reorder(c *gin.Context) {
	var ids []int64
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.Reorder(ids); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "NODE_GROUPS_REORDER_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "Node groups reordered"})
}

func (h *NodeGroupHandler) PreviewMatches(c *gin.Context) {
	filterProtocols := c.Query("filter_protocols")
	filterSubscriptions := c.Query("filter_subscriptions")
	filterInclude := c.Query("filter_include")
	filterExclude := c.Query("filter_exclude")

	nodes, err := h.svc.PreviewMatches(filterProtocols, filterSubscriptions, filterInclude, filterExclude)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "PREVIEW_FAILED", Message: err.Error()},
		})
		return
	}
	// 返回简化版节点信息
	type simpleNode struct {
		UID              string `json:"uid"`
		Name             string `json:"name"`
		Type             string `json:"type"`
		SubscriptionID   int64  `json:"subscription_id"`
		SubscriptionName string `json:"subscription_name"`
		LatencyMS        int    `json:"latency_ms"`
		Status           string `json:"status"`
	}
	result := []simpleNode{}
	for _, n := range nodes {
		result = append(result, simpleNode{
			UID:              n.UID,
			Name:             n.Name,
			Type:             n.Type,
			SubscriptionID:   n.SubscriptionID,
			SubscriptionName: n.SubscriptionName,
			LatencyMS:        n.LatencyMS,
			Status:           n.Status,
		})
	}
	c.JSON(http.StatusOK, result)
}

func (h *NodeGroupHandler) QuickSetup(c *gin.Context) {
	var req model.NodeGroupQuickSetupRequest
	_ = c.ShouldBindJSON(&req)
	if err := h.svc.QuickSetup(req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "QUICK_SETUP_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "Node groups created"})
}
