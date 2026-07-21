package handler

import (
	"net/http"
	"strconv"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
	"github.com/gin-gonic/gin"
)

type DNSHandler struct {
	svc *service.DNSService
}

func NewDNSHandler(svc *service.DNSService) *DNSHandler {
	return &DNSHandler{svc: svc}
}

// DNS Servers

func (h *DNSHandler) ListDNSServers(c *gin.Context) {
	servers, err := h.svc.ListDNSServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_LIST_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (h *DNSHandler) GetDNSServer(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	server, err := h.svc.GetDNSServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_SERVER_NOT_FOUND", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, server)
}

func (h *DNSHandler) CreateDNSServer(c *gin.Context) {
	var req model.DNSServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	server, err := h.svc.CreateDNSServer(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_SERVER_CREATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusCreated, server)
}

func (h *DNSHandler) UpdateDNSServer(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req model.DNSServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.UpdateDNSServer(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_SERVER_UPDATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS server updated"})
}

func (h *DNSHandler) DeleteDNSServer(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DeleteDNSServer(id); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_SERVER_DELETE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS server deleted"})
}

func (h *DNSHandler) ReorderDNSServers(c *gin.Context) {
	var ids []int64
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
		return
	}
	if err := h.svc.ReorderDNSServers(ids); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "DNS_SERVERS_REORDER_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS servers reordered"})
}

// DNS Rules

func (h *DNSHandler) ListDNSRules(c *gin.Context) {
	rules, err := h.svc.ListDNSRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULES_LIST_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func (h *DNSHandler) GetDNSRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	rule, err := h.svc.GetDNSRule(id)
	if err != nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULE_NOT_FOUND", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *DNSHandler) CreateDNSRule(c *gin.Context) {
	var req model.DNSRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	rule, err := h.svc.CreateDNSRule(&req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULE_CREATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusCreated, rule)
}

func (h *DNSHandler) UpdateDNSRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	var req model.DNSRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.UpdateDNSRule(id, &req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULE_UPDATE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS rule updated"})
}

func (h *DNSHandler) DeleteDNSRule(c *gin.Context) {
	id, _ := strconv.ParseInt(c.Param("id"), 10, 64)
	if err := h.svc.DeleteDNSRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULE_DELETE_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS rule deleted"})
}

func (h *DNSHandler) ReorderDNSRules(c *gin.Context) {
	var ids []int64
	if err := c.ShouldBindJSON(&ids); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.ReorderDNSRules(ids); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_RULES_REORDER_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS rules reordered"})
}

// DNS Global Settings

func (h *DNSHandler) GetDNSGlobalSettings(c *gin.Context) {
	settings, err := h.svc.GetDNSGlobalSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_GLOBAL_SETTINGS_GET_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *DNSHandler) SetDNSGlobalSettings(c *gin.Context) {
	var req model.DNSGlobalSettings
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{
			Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()},
		})
		return
	}

	if err := h.svc.SetDNSGlobalSettings(&req); err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "DNS_GLOBAL_SETTINGS_SET_FAILED", Message: err.Error()},
		})
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS global settings updated"})
}
