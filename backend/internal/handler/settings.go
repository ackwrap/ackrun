package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type SettingsHandler struct {
	svc *service.SettingsService
}

func NewSettingsHandler(svc *service.SettingsService) *SettingsHandler {
	return &SettingsHandler{svc: svc}
}

func (h *SettingsHandler) GetUpdateSettings(c *gin.Context) {
	resp, err := h.svc.GetUpdateSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetUpdateSettings(c *gin.Context) {
	var req model.UpdateSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetUpdateSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_SAVE_FAILED", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "settings updated"})
}

func (h *SettingsHandler) GetTrafficBypassSettings(c *gin.Context) {
	settings, err := h.svc.GetTrafficBypassSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "TRAFFIC_BYPASS_SETTINGS_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SettingsHandler) SetTrafficBypassSettings(c *gin.Context) {
	var settings model.TrafficBypassSettings
	if err := c.ShouldBindJSON(&settings); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "TRAFFIC_BYPASS_SETTINGS_INVALID", Message: err.Error()}})
		return
	}
	if err := h.svc.SetTrafficBypassSettings(&settings); err != nil {
		status := http.StatusInternalServerError
		code := "TRAFFIC_BYPASS_SETTINGS_SAVE_FAILED"
		if errors.Is(err, service.ErrTrafficBypassSettingsInvalid) {
			status = http.StatusBadRequest
			code = "TRAFFIC_BYPASS_SETTINGS_INVALID"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "traffic bypass settings updated"})
}

func (h *SettingsHandler) GetLogSettings(c *gin.Context) {
	resp, err := h.svc.GetLogSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetLogSettings(c *gin.Context) {
	var req model.LogSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetLogSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_SAVE_FAILED", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "log settings updated"})
}

func (h *SettingsHandler) GetConnectivitySettings(c *gin.Context) {
	resp, err := h.svc.GetConnectivitySettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetConnectivitySettings(c *gin.Context) {
	var req model.ConnectivitySettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}
	if err := h.svc.SetConnectivitySettings(&req); err != nil {
		status := http.StatusInternalServerError
		code := "SETTINGS_SAVE_FAILED"
		if errors.Is(err, service.ErrConnectivitySettingsInvalid) {
			status = http.StatusBadRequest
			code = "SETTINGS_INVALID"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "connectivity settings updated"})
}

func (h *SettingsHandler) GetNTPSettings(c *gin.Context) {
	resp, err := h.svc.GetNTPSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetNTPSettings(c *gin.Context) {
	var req model.NTPSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetNTPSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_SAVE_FAILED", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "ntp settings updated"})
}

func (h *SettingsHandler) GetDNSSettings(c *gin.Context) {
	resp, err := h.svc.GetDNSSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetDNSSettings(c *gin.Context) {
	var req model.DNSSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetDNSSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_SAVE_FAILED", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "dns settings updated"})
}

func (h *SettingsHandler) GetInboundMode(c *gin.Context) {
	mode := h.svc.GetInboundMode()
	c.JSON(http.StatusOK, gin.H{"mode": mode})
}

func (h *SettingsHandler) SetInboundMode(c *gin.Context) {
	var req struct {
		Mode string `json:"mode" binding:"required,oneof=tun mixed tun_mixed"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_MODE", Message: "mode must be one of: tun, mixed, tun_mixed"},
		})
		return
	}

	if err := h.svc.SetInboundMode(req.Mode); err != nil {
		status := http.StatusInternalServerError
		code := "SETTINGS_SAVE_FAILED"
		if errors.Is(err, service.ErrModeChangeWhileRunning) {
			status = http.StatusConflict
			code = "MODE_CHANGE_NOT_ALLOWED"
		}
		c.JSON(status, model.ErrorResponse{
			Error: model.APIError{Code: code, Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "inbound mode updated"})
}

func (h *SettingsHandler) GetProxyMode(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"mode": h.svc.GetProxyMode()})
}

func (h *SettingsHandler) SetProxyMode(c *gin.Context) {
	var req struct {
		Mode string `json:"mode" binding:"required,oneof=rule global direct"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_MODE", Message: "mode must be one of: rule, global, direct"},
		})
		return
	}
	if err := h.svc.SetProxyMode(req.Mode); err != nil {
		status := http.StatusInternalServerError
		code := "SETTINGS_SAVE_FAILED"
		if errors.Is(err, service.ErrModeChangeWhileRunning) {
			status = http.StatusConflict
			code = "MODE_CHANGE_NOT_ALLOWED"
		}
		c.JSON(status, model.ErrorResponse{
			Error: model.APIError{Code: code, Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "proxy mode updated"})
}

func (h *SettingsHandler) GetExperimentalSettings(c *gin.Context) {
	resp, err := h.svc.GetExperimentalSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *SettingsHandler) SetExperimentalSettings(c *gin.Context) {
	var req model.ExperimentalSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_INVALID", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetExperimentalSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "SETTINGS_SAVE_FAILED", Message: err.Error()},
		})
		return
	}

	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "experimental settings updated"})
}

func (h *SettingsHandler) ListNodeFilters(c *gin.Context) {
	items, err := h.svc.ListNodeFilters()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *SettingsHandler) CreateNodeFilter(c *gin.Context) {
	var req model.NodeFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.CreateNodeFilter(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_CREATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SettingsHandler) UpdateNodeFilter(c *gin.Context) {
	id, ok := parseFilterID(c)
	if !ok {
		return
	}
	var req model.NodeFilterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.UpdateNodeFilter(id, &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_UPDATE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SettingsHandler) DeleteNodeFilter(c *gin.Context) {
	id, ok := parseFilterID(c)
	if !ok {
		return
	}
	resp, err := h.svc.DeleteNodeFilter(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "NODE_FILTER_DELETE_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func parseFilterID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ID_INVALID", Message: "invalid id"}})
		return 0, false
	}
	return id, true
}
