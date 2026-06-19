package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type SubscriptionHandler struct {
	svc *service.SubscriptionService
}

func NewSubscriptionHandler(svc *service.SubscriptionService) *SubscriptionHandler {
	return &SubscriptionHandler{svc: svc}
}

func (h *SubscriptionHandler) List(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_ERROR", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *SubscriptionHandler) UserAgentOptions(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.UserAgentOptions())
}

func (h *SubscriptionHandler) Create(c *gin.Context) {
	var req model.SubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.Create(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_CREATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SubscriptionHandler) Update(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var req model.SubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.Update(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SubscriptionHandler) Delete(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Delete(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_DELETE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SubscriptionHandler) Sync(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Sync(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SubscriptionHandler) SyncAll(c *gin.Context) {
	resp, err := h.svc.SyncAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "SUBSCRIPTION_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ID_INVALID", Message: "invalid id"}})
		return 0, false
	}
	return id, true
}
