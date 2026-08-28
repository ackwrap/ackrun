package handler

import (
	"errors"
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

// DNS Hosts

func (h *DNSHandler) ListDNSHosts(c *gin.Context) {
	items, err := h.svc.ListDNSHosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: model.APIError{Code: "DNS_HOSTS_LIST_FAILED", Message: err.Error()}})
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *DNSHandler) GetDNSHost(c *gin.Context) {
	id, ok := parseDNSHostID(c)
	if !ok {
		return
	}
	item, err := h.svc.GetDNSHost(id)
	if err != nil {
		writeDNSHostError(c, "DNS_HOST_GET_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DNSHandler) CreateDNSHost(c *gin.Context) {
	var req model.DNSHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "DNS_HOST_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.CreateDNSHost(&req)
	if err != nil {
		writeDNSHostError(c, "DNS_HOST_CREATE_FAILED", err)
		return
	}
	c.JSON(http.StatusCreated, item)
}

func (h *DNSHandler) UpdateDNSHost(c *gin.Context) {
	id, ok := parseDNSHostID(c)
	if !ok {
		return
	}
	var req model.DNSHostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "DNS_HOST_INVALID", Message: err.Error()}})
		return
	}
	item, err := h.svc.UpdateDNSHost(id, &req)
	if err != nil {
		writeDNSHostError(c, "DNS_HOST_UPDATE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *DNSHandler) DeleteDNSHost(c *gin.Context) {
	id, ok := parseDNSHostID(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteDNSHost(id); err != nil {
		writeDNSHostError(c, "DNS_HOST_DELETE_FAILED", err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "DNS Hosts mapping deleted"})
}

func parseDNSHostID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "ID_INVALID", Message: "invalid id"}})
		return 0, false
	}
	return id, true
}

func writeDNSHostError(c *gin.Context, fallbackCode string, err error) {
	status, code := http.StatusInternalServerError, fallbackCode
	switch {
	case errors.Is(err, service.ErrDNSHostInvalid):
		status, code = http.StatusBadRequest, "DNS_HOST_INVALID"
	case errors.Is(err, service.ErrDNSHostNotFound):
		status, code = http.StatusNotFound, "DNS_HOST_NOT_FOUND"
	case errors.Is(err, service.ErrDNSHostDomainConflict):
		status, code = http.StatusConflict, "DNS_HOST_DOMAIN_CONFLICT"
	}
	c.JSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: err.Error()}})
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
