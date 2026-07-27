package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type AdvancedHandler struct {
	svc *service.AdvancedRoutingService
}

func NewAdvancedHandler(svc *service.AdvancedRoutingService) *AdvancedHandler {
	return &AdvancedHandler{svc: svc}
}

func (h *AdvancedHandler) ListPlatformRoutes(c *gin.Context) {
	items, err := h.svc.ListPlatformRoutes()
	h.respond(c, items, err, "ADVANCED_PLATFORM_ROUTE_LIST_FAILED")
}

func (h *AdvancedHandler) CreatePlatformRoute(c *gin.Context) {
	var request model.PlatformRoute
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.CreatePlatformRoute(request)
	h.respond(c, item, err, "ADVANCED_PLATFORM_ROUTE_CREATE_FAILED")
}

func (h *AdvancedHandler) UpdatePlatformRoute(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request model.PlatformRoute
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.UpdatePlatformRoute(id, request)
	h.respond(c, item, err, "ADVANCED_PLATFORM_ROUTE_UPDATE_FAILED")
}

func (h *AdvancedHandler) DeletePlatformRoute(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.svc.DeletePlatformRoute(id)
	h.respond(c, item, err, "ADVANCED_PLATFORM_ROUTE_DELETE_FAILED")
}

func (h *AdvancedHandler) ReorderPlatformRoutes(c *gin.Context) {
	var request struct {
		IDs []int64 `json:"ids" binding:"required"`
	}
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.ReorderPlatformRoutes(request.IDs)
	h.respond(c, item, err, "ADVANCED_PLATFORM_ROUTE_REORDER_FAILED")
}

func (h *AdvancedHandler) ListSessionLeases(c *gin.Context) {
	items, err := h.svc.ListSessionLeases()
	h.respond(c, items, err, "ADVANCED_SESSION_LEASE_LIST_FAILED")
}

func (h *AdvancedHandler) CreateSessionLease(c *gin.Context) {
	var request model.SessionLease
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.CreateSessionLease(request)
	h.respond(c, item, err, "ADVANCED_SESSION_LEASE_CREATE_FAILED")
}

func (h *AdvancedHandler) UpdateSessionLease(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request model.SessionLease
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.UpdateSessionLease(id, request)
	h.respond(c, item, err, "ADVANCED_SESSION_LEASE_UPDATE_FAILED")
}

func (h *AdvancedHandler) DeleteSessionLease(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	item, err := h.svc.DeleteSessionLease(id)
	h.respond(c, item, err, "ADVANCED_SESSION_LEASE_DELETE_FAILED")
}

func (h *AdvancedHandler) RenewSessionLease(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	var request struct {
		DurationMinutes int `json:"duration_minutes" binding:"required"`
	}
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.RenewSessionLease(id, request.DurationMinutes)
	h.respond(c, item, err, "ADVANCED_SESSION_LEASE_RENEW_FAILED")
}

func (h *AdvancedHandler) HealthScheduling(c *gin.Context) {
	item, err := h.svc.HealthSnapshot()
	h.respond(c, item, err, "ADVANCED_HEALTH_LIST_FAILED")
}

func (h *AdvancedHandler) RunHealth(c *gin.Context) {
	item, err := h.svc.RunHealth()
	h.respond(c, item, err, "ADVANCED_HEALTH_RUN_FAILED")
}

func (h *AdvancedHandler) ListAccessLogs(c *gin.Context) {
	page, ok := advancedPositiveQuery(c, "page", 1)
	if !ok {
		return
	}
	pageSize, ok := advancedPositiveQuery(c, "page_size", 50)
	if !ok {
		return
	}
	item, err := h.svc.ListAccessLogs(page, pageSize, c.Query("platform"), c.Query("decision"), c.Query("keyword"))
	h.respond(c, item, err, "ADVANCED_ACCESS_LOG_LIST_FAILED")
}

func (h *AdvancedHandler) ClearAccessLogs(c *gin.Context) {
	item, err := h.svc.ClearAccessLogs()
	h.respond(c, item, err, "ADVANCED_ACCESS_LOG_CLEAR_FAILED")
}

func (h *AdvancedHandler) GetSettings(c *gin.Context) {
	item, err := h.svc.GetSettings()
	h.respond(c, item, err, "ADVANCED_SETTINGS_GET_FAILED")
}

func (h *AdvancedHandler) UpdateSettings(c *gin.Context) {
	var request model.AdvancedSettings
	if !bindAdvancedJSON(c, &request) {
		return
	}
	item, err := h.svc.UpdateSettings(&request)
	h.respond(c, item, err, "ADVANCED_SETTINGS_UPDATE_FAILED")
}

func (h *AdvancedHandler) respond(c *gin.Context, value any, err error, code string) {
	if err == nil {
		c.JSON(http.StatusOK, value)
		return
	}
	status := http.StatusInternalServerError
	message := err.Error()
	if errors.Is(err, service.ErrAdvancedInvalid) || errors.Is(err, service.ErrAdvancedApply) {
		status = http.StatusBadRequest
		if errors.Is(err, service.ErrAdvancedApply) {
			message = "应用高级路由运行时失败，请检查核心运行状态与配置"
		}
	} else if errors.Is(err, service.ErrAdvancedNotFound) {
		status = http.StatusNotFound
	}
	c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: message}})
}

func bindAdvancedJSON(c *gin.Context, destination any) bool {
	if err := c.ShouldBindJSON(destination); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ADVANCED_INVALID", Message: err.Error()}})
		return false
	}
	return true
}

func advancedPositiveQuery(c *gin.Context, name string, fallback int) (int, bool) {
	value := c.Query(name)
	if value == "" {
		return fallback, true
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ADVANCED_INVALID", Message: name + " must be a positive integer"}})
		return 0, false
	}
	return parsed, true
}
