package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
)

type InstallerHandler struct {
	svc *service.InstallerService
}

func NewInstallerHandler(svc *service.InstallerService) *InstallerHandler {
	return &InstallerHandler{svc: svc}
}

func (h *InstallerHandler) GetStatus(c *gin.Context) {
	resp, err := h.svc.GetStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "INSTALLER_ERROR", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *InstallerHandler) Install(c *gin.Context) {
	resp, err := h.svc.Install()
	if err != nil {
		c.JSON(http.StatusConflict, model.ErrorResponse{
			Error: model.APIError{Code: "INSTALL_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
