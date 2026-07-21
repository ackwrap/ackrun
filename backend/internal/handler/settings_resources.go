package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

func (h *SettingsHandler) ListGeoIPProviders(c *gin.Context) {
	response, err := h.svc.ListGeoIPProviders()
	if err != nil {
		settingsResourceError(c, err, "GEOIP_PROVIDERS_LIST_FAILED")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SettingsHandler) CreateGeoIPProvider(c *gin.Context) {
	var req model.GeoIPProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		settingsResourceError(c, err, "SETTINGS_INVALID")
		return
	}
	item, err := h.svc.CreateGeoIPProvider(&req)
	if err != nil {
		settingsResourceError(c, err, "GEOIP_PROVIDER_CREATE_FAILED")
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *SettingsHandler) UpdateGeoIPProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		settingsResourceError(c, service.ErrSettingsResourceNotFound, "SETTINGS_RESOURCE_NOT_FOUND")
		return
	}
	var req model.GeoIPProviderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		settingsResourceError(c, err, "SETTINGS_INVALID")
		return
	}
	item, err := h.svc.UpdateGeoIPProvider(id, &req)
	if err != nil {
		settingsResourceError(c, err, "GEOIP_PROVIDER_UPDATE_FAILED")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SettingsHandler) DeleteGeoIPProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		settingsResourceError(c, service.ErrSettingsResourceNotFound, "SETTINGS_RESOURCE_NOT_FOUND")
		return
	}
	response, err := h.svc.DeleteGeoIPProvider(id)
	if err != nil {
		settingsResourceError(c, err, "GEOIP_PROVIDER_DELETE_FAILED")
		return
	}
	c.JSON(http.StatusOK, response)
}

func (h *SettingsHandler) ListConnectivityTargets(c *gin.Context) {
	items, err := h.svc.ListConnectivityTargets()
	if err != nil {
		settingsResourceError(c, err, "CONNECTIVITY_TARGETS_LIST_FAILED")
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *SettingsHandler) CreateConnectivityTarget(c *gin.Context) {
	var req model.ConnectivityTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		settingsResourceError(c, err, "SETTINGS_INVALID")
		return
	}
	item, err := h.svc.CreateConnectivityTarget(&req)
	if err != nil {
		settingsResourceError(c, err, "CONNECTIVITY_TARGET_CREATE_FAILED")
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *SettingsHandler) UpdateConnectivityTarget(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		settingsResourceError(c, service.ErrSettingsResourceNotFound, "SETTINGS_RESOURCE_NOT_FOUND")
		return
	}
	var req model.ConnectivityTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		settingsResourceError(c, err, "SETTINGS_INVALID")
		return
	}
	item, err := h.svc.UpdateConnectivityTarget(id, &req)
	if err != nil {
		settingsResourceError(c, err, "CONNECTIVITY_TARGET_UPDATE_FAILED")
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SettingsHandler) DeleteConnectivityTarget(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		settingsResourceError(c, service.ErrSettingsResourceNotFound, "SETTINGS_RESOURCE_NOT_FOUND")
		return
	}
	response, err := h.svc.DeleteConnectivityTarget(id)
	if err != nil {
		settingsResourceError(c, err, "CONNECTIVITY_TARGET_DELETE_FAILED")
		return
	}
	c.JSON(http.StatusOK, response)
}

func settingsResourceError(c *gin.Context, err error, code string) {
	status := http.StatusInternalServerError
	if code == "SETTINGS_INVALID" || errors.Is(err, service.ErrGeoIPProviderInvalid) || errors.Is(err, service.ErrConnectivitySettingsInvalid) {
		status = http.StatusBadRequest
		code = "SETTINGS_INVALID"
	} else if errors.Is(err, service.ErrSettingsResourceNotFound) {
		status = http.StatusNotFound
		code = "SETTINGS_RESOURCE_NOT_FOUND"
	}
	c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
}
