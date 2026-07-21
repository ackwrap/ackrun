package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type CoreHandler struct {
	singbox *service.SingboxService
}

func NewCoreHandler(s *service.SingboxService) *CoreHandler {
	return &CoreHandler{singbox: s}
}

func (h *CoreHandler) Start(c *gin.Context) {
	resp, err := h.singbox.Start()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_START_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) Stop(c *gin.Context) {
	resp, err := h.singbox.Stop()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_STOP_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) Restart(c *gin.Context) {
	resp, err := h.singbox.Restart()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_RESTART_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) ReloadConfig(c *gin.Context) {
	resp, err := h.singbox.ReloadConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_RELOAD_CONFIG_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) CloseConnections(c *gin.Context) {
	resp, err := h.singbox.CloseConnections()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_CLOSE_CONNECTIONS_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) FlushCoreDNS(c *gin.Context) {
	resp, err := h.singbox.FlushCoreDNS()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_FLUSH_CORE_DNS_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) FlushFakeIP(c *gin.Context) {
	resp, err := h.singbox.FlushFakeIP()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_FLUSH_FAKEIP_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) NetworkCheck(c *gin.Context) {
	resp, err := h.singbox.NetworkCheck()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_NETWORK_CHECK_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) Diagnostics(c *gin.Context) {
	resp, err := h.singbox.Diagnostics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_DIAGNOSTICS_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) ResetFirewall(c *gin.Context) {
	resp, err := h.singbox.ResetFirewall()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_RESET_FIREWALL_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) FlushDNS(c *gin.Context) {
	resp, err := h.singbox.FlushDNS()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_FLUSH_DNS_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *CoreHandler) CheckUpdate(c *gin.Context) {
	resp, err := h.singbox.CheckUpdate()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CORE_CHECK_UPDATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
