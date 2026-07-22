package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type AppUpdateHandler struct {
	service *service.AppUpdateService
}

func NewAppUpdateHandler(updateService *service.AppUpdateService) *AppUpdateHandler {
	return &AppUpdateHandler{service: updateService}
}

func (handler *AppUpdateHandler) Check(c *gin.Context) {
	status, err := handler.service.Check(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, model.ErrorResponse{Error: model.APIError{Code: "APP_UPDATE_CHECK_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, status)
}

func (handler *AppUpdateHandler) Status(c *gin.Context) {
	c.JSON(http.StatusOK, handler.service.InstallStatus())
}

func (handler *AppUpdateHandler) Install(c *gin.Context) {
	response, err := handler.service.Install(c.Request.Context())
	if err != nil {
		status := http.StatusInternalServerError
		code := "APP_UPDATE_INSTALL_FAILED"
		switch {
		case errors.Is(err, service.ErrAppUpdateInProgress):
			status = http.StatusConflict
			code = "APP_UPDATE_IN_PROGRESS"
		case errors.Is(err, service.ErrAppUpdateUnavailable):
			status = http.StatusConflict
			code = "APP_UPDATE_UNAVAILABLE"
		case errors.Is(err, service.ErrAppUpdateUnsupported):
			status = http.StatusBadRequest
			code = "APP_UPDATE_UNSUPPORTED"
		}
		c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
		return
	}
	c.JSON(http.StatusAccepted, response)
}
