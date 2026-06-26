package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type RouteRuleHandler struct {
	svc *service.RouteRuleService
}

func NewRouteRuleHandler(svc *service.RouteRuleService) *RouteRuleHandler {
	return &RouteRuleHandler{svc: svc}
}

func (h *RouteRuleHandler) List(c *gin.Context) {
	items, err := h.svc.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *RouteRuleHandler) Create(c *gin.Context) {
	var req model.RouteRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.Create(&req)
	if err != nil {
		if errors.Is(err, service.ErrSystemRouteRuleProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_RULE_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_CREATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RouteRuleHandler) Update(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	var req model.RouteRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.Update(id, &req)
	if err != nil {
		if errors.Is(err, service.ErrSystemRouteRuleProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_RULE_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RouteRuleHandler) Delete(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	resp, err := h.svc.Delete(id)
	if err != nil {
		if errors.Is(err, service.ErrSystemRouteRuleProtected) {
			c.JSON(http.StatusForbidden, model.ErrorResponse{Error: model.APIError{Code: "SYSTEM_RULE_PROTECTED", Message: err.Error()}})
			return
		}
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_DELETE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) Reorder(c *gin.Context) {
	var req model.RouteRuleReorderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_INVALID", Message: err.Error()}})
		return
	}
	resp, err := h.svc.Reorder(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_REORDER_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) Preview(c *gin.Context) {
	resp, err := h.svc.PreviewWithBaseURL(requestBaseURL(c))
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_PREVIEW_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) ListSubscriptions(c *gin.Context) {
	items, err := h.svc.ListSubscriptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *RouteRuleHandler) CreateSubscription(c *gin.Context) {
	var req model.RouteRuleSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.CreateSubscription(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_CREATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RouteRuleHandler) UpdateSubscription(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	var req model.RouteRuleSubscriptionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.UpdateSubscription(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RouteRuleHandler) DeleteSubscription(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	resp, err := h.svc.DeleteSubscription(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_DELETE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) SyncSubscription(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	resp, err := h.svc.SyncSubscription(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) SyncAllSubscriptions(c *gin.Context) {
	resp, err := h.svc.SyncAllSubscriptions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) SubscriptionContent(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	data, contentType, err := h.svc.SubscriptionContent(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ROUTE_RULE_SUBSCRIPTION_CONTENT_FAILED", Message: err.Error()}})
		return
	}
	c.Data(http.StatusOK, contentType, data)
}

func (h *RouteRuleHandler) ListGeoAssets(c *gin.Context) {
	items, err := h.svc.ListGeoAssets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "GEO_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *RouteRuleHandler) UpdateGeoAsset(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	var req model.GeoAssetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.UpdateGeoAsset(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *RouteRuleHandler) SyncGeoAsset(c *gin.Context) {
	id, ok := parseRouteRuleID(c)
	if !ok {
		return
	}
	resp, err := h.svc.SyncGeoAsset(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) SyncAllGeoAssets(c *gin.Context) {
	resp, err := h.svc.SyncAllGeoAssets()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "GEO_SYNC_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) GeoLookup(c *gin.Context) {
	target := c.Query("target")
	dnsServer := c.Query("dns_server")
	resp, err := h.svc.GeoLookup(target, dnsServer)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_LOOKUP_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) GeoTags(c *gin.Context) {
	assetType := c.DefaultQuery("type", "geosite")
	query := c.Query("q")
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	resp, err := h.svc.GeoTags(assetType, query, limit)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_TAGS_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *RouteRuleHandler) GeoDomains(c *gin.Context) {
	tag := c.Query("tag")
	limit := 1000
	if raw := c.Query("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	offset := 0
	if raw := c.Query("offset"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			offset = parsed
		}
	}
	resp, err := h.svc.GeoDomains(tag, limit, offset)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "GEO_DOMAINS_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseRouteRuleID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ID_INVALID", Message: "invalid id"}})
		return 0, false
	}
	return id, true
}

func requestBaseURL(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + c.Request.Host
}
