package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type DashboardHandler struct {
	service *service.DashboardService
}

func NewDashboardHandler(dashboardService *service.DashboardService) *DashboardHandler {
	return &DashboardHandler{service: dashboardService}
}

func (handler *DashboardHandler) List(c *gin.Context) {
	items, err := handler.service.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "DASHBOARD_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (handler *DashboardHandler) CheckUpdates(c *gin.Context) {
	items, err := handler.service.CheckUpdates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: model.APIError{Code: "DASHBOARD_CHECK_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (handler *DashboardHandler) Install(c *gin.Context) {
	item, err := handler.service.Install(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		code := "DASHBOARD_INSTALL_FAILED"
		if errors.Is(err, service.ErrDashboardNotFound) {
			status = http.StatusNotFound
			code = "DASHBOARD_NOT_FOUND"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (handler *DashboardHandler) Delete(c *gin.Context) {
	response, err := handler.service.Delete(c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		code := "DASHBOARD_DELETE_FAILED"
		switch {
		case errors.Is(err, service.ErrDashboardNotFound), errors.Is(err, service.ErrDashboardNotInstalled):
			status = http.StatusNotFound
			code = "DASHBOARD_NOT_FOUND"
		case errors.Is(err, service.ErrDashboardInUse):
			status = http.StatusConflict
			code = "DASHBOARD_IN_USE"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, response)
}
