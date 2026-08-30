package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

const maxAlertRequestBody = 64 << 10

type AlertHandler struct {
	svc *service.AlertService
}

func NewAlertHandler(svc *service.AlertService) *AlertHandler {
	return &AlertHandler{svc: svc}
}

func (h *AlertHandler) ListChannels(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	items, err := h.svc.ListChannels()
	respondAlert(c, items, err, "ALERT_CHANNEL_LIST_FAILED")
}

func (h *AlertHandler) CreateChannel(c *gin.Context) {
	var request model.AlertChannelRequest
	if !bindAlertJSON(c, &request) {
		return
	}
	item, err := h.svc.CreateChannel(request)
	respondAlert(c, item, err, "ALERT_CHANNEL_CREATE_FAILED")
}

func (h *AlertHandler) UpdateChannel(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request model.AlertChannelRequest
	if !bindAlertJSON(c, &request) {
		return
	}
	item, err := h.svc.UpdateChannel(id, request)
	respondAlert(c, item, err, "ALERT_CHANNEL_UPDATE_FAILED")
}

func (h *AlertHandler) DeleteChannel(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.svc.DeleteChannel(id)
	respondAlert(c, item, err, "ALERT_CHANNEL_DELETE_FAILED")
}

func (h *AlertHandler) TestChannel(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	item, err := h.svc.TestChannel(ctx, id)
	respondAlert(c, item, err, "ALERT_CHANNEL_TEST_FAILED")
}

func (h *AlertHandler) ListRules(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	items, err := h.svc.ListRules()
	respondAlert(c, items, err, "ALERT_RULE_LIST_FAILED")
}

func (h *AlertHandler) CreateRule(c *gin.Context) {
	var request model.AlertRuleRequest
	if !bindAlertJSON(c, &request) {
		return
	}
	item, err := h.svc.CreateRule(request)
	respondAlert(c, item, err, "ALERT_RULE_CREATE_FAILED")
}

func (h *AlertHandler) UpdateRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request model.AlertRuleRequest
	if !bindAlertJSON(c, &request) {
		return
	}
	item, err := h.svc.UpdateRule(id, request)
	respondAlert(c, item, err, "ALERT_RULE_UPDATE_FAILED")
}

func (h *AlertHandler) DeleteRule(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.svc.DeleteRule(id)
	respondAlert(c, item, err, "ALERT_RULE_DELETE_FAILED")
}

func (h *AlertHandler) ListDeliveries(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	page, ok := advancedPositiveQuery(c, "page", 1)
	if !ok {
		return
	}
	pageSize, ok := advancedPositiveQuery(c, "page_size", 25)
	if !ok {
		return
	}
	channelID := int64(0)
	if raw := c.Query("channel_id"); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			respondAlert(c, nil, service.ErrAlertInvalid, "ALERT_DELIVERY_LIST_FAILED")
			return
		}
		channelID = parsed
	}
	items, err := h.svc.ListDeliveries(page, pageSize, channelID, c.Query("event_type"), c.Query("status"))
	respondAlert(c, items, err, "ALERT_DELIVERY_LIST_FAILED")
}

func (h *AlertHandler) ClearDeliveries(c *gin.Context) {
	item, err := h.svc.ClearDeliveries()
	respondAlert(c, item, err, "ALERT_DELIVERY_CLEAR_FAILED")
}

func bindAlertJSON(c *gin.Context, destination any) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAlertRequestBody)
	if err := c.ShouldBindJSON(destination); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ALERT_INVALID", Message: err.Error()}})
		return false
	}
	return true
}

func respondAlert(c *gin.Context, value any, err error, fallbackCode string) {
	if err == nil {
		c.JSON(http.StatusOK, value)
		return
	}
	status, code := http.StatusInternalServerError, fallbackCode
	switch {
	case errors.Is(err, service.ErrAlertInvalid):
		status, code = http.StatusBadRequest, "ALERT_INVALID"
	case errors.Is(err, service.ErrAlertNotFound):
		status, code = http.StatusNotFound, "ALERT_NOT_FOUND"
	case errors.Is(err, service.ErrAlertConflict):
		status, code = http.StatusConflict, "ALERT_CONFLICT"
	case errors.Is(err, service.ErrAlertDelivery):
		status, code = http.StatusBadGateway, "ALERT_DELIVERY_FAILED"
	}
	c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
}
