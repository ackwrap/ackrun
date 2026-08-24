package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

const (
	maxSSHCredentialRequestBody = 512 << 10
	maxSSHHostRequestBody       = 64 << 10
	maxSSHHostImportRequestBody = 2 << 20
)

type SSHHostHandler struct {
	service *service.SSHHostService
}

func NewSSHHostHandler(service *service.SSHHostService) *SSHHostHandler {
	return &SSHHostHandler{service: service}
}

func (h *SSHHostHandler) ListHosts(c *gin.Context) {
	items, err := h.service.ListHosts()
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *SSHHostHandler) GetHost(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	item, err := h.service.GetHost(id)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) CreateHost(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHHostRequestBody)
	var request model.SSHHostRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	item, err := h.service.CreateHost(request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) UpdateHost(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHHostRequestBody)
	var request model.SSHHostRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	item, err := h.service.UpdateHost(id, request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) DeleteHost(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteHost(id); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SSH host deleted"})
}

func (h *SSHHostHandler) ShareHost(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHHostRequestBody)
	var request model.SSHHostShareRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.ShareHost(id, request.Password)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) ShareHosts(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHHostRequestBody)
	var request model.SSHHostsShareRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.ShareHosts(request.HostIDs, request.Password)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) ImportHost(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHHostImportRequestBody)
	var request model.SSHHostImportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.ImportHost(request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) TestHost(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	result, err := h.service.TestHost(c.Request.Context(), id)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) CreateSession(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	var request model.SSHSessionCreateRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.CreateSession(c.Request.Context(), id, request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) DeleteSession(c *gin.Context) {
	sessionID := c.Param("sessionID")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "SSH_SESSION_INVALID", Message: "SSH 会话 ID 无效"}})
		return
	}
	if err := h.service.CloseSession(sessionID); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SSH session closed"})
}

func (h *SSHHostHandler) GetHostKey(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	item, err := h.service.GetHostKey(id)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	if item == nil {
		c.JSON(http.StatusNotFound, model.ErrorResponse{Error: model.APIError{Code: "SSH_HOST_KEY_NOT_TRUSTED", Message: "主机尚未信任 Host Key"}})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) TrustHostKey(c *gin.Context) {
	h.updateHostKey(c, false)
}

func (h *SSHHostHandler) RotateHostKey(c *gin.Context) {
	h.updateHostKey(c, true)
}

func (h *SSHHostHandler) updateHostKey(c *gin.Context, rotate bool) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	var request model.SSHHostKeyTrustRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	item, err := h.service.TrustHostKey(id, request, rotate)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) DeleteHostKey(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteHostKey(id); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SSH host key trust deleted"})
}

func (h *SSHHostHandler) ListCredentials(c *gin.Context) {
	items, err := h.service.ListCredentials()
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, items)
}

func (h *SSHHostHandler) CreateCredential(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHCredentialRequestBody)
	var request model.SSHCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	item, err := h.service.CreateCredential(request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) UpdateCredential(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHCredentialRequestBody)
	var request model.SSHCredentialRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	item, err := h.service.UpdateCredential(id, request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h *SSHHostHandler) DeleteCredential(c *gin.Context) {
	id, ok := parseSSHID(c)
	if !ok {
		return
	}
	if err := h.service.DeleteCredential(id); err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, model.ActionResponse{Success: true, Message: "SSH credential deleted"})
}

func parseSSHID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_ID", Message: "无效的 SSH 资源 ID"}})
		return 0, false
	}
	return id, true
}

func writeSSHInvalidRequest(c *gin.Context, err error) {
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		c.JSON(http.StatusRequestEntityTooLarge, model.ErrorResponse{Error: model.APIError{Code: "SSH_REQUEST_TOO_LARGE", Message: "SSH 请求体过大"}})
		return
	}
	c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: model.APIError{Code: "INVALID_REQUEST", Message: err.Error()}})
}

func writeSSHError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	apiError := model.APIError{Code: "SSH_INTERNAL_ERROR", Message: "SSH 操作失败"}
	var serviceError *service.SSHServiceError
	if !errors.As(err, &serviceError) {
		c.JSON(status, model.ErrorResponse{Error: apiError})
		return
	}
	apiError.Code, apiError.Message, apiError.Details = serviceError.Code, serviceError.Message, serviceError.Details
	switch serviceError.Code {
	case "SSH_HOST_NOT_FOUND", "SSH_CREDENTIAL_NOT_FOUND", "SSH_NODE_EXPOSURE_NOT_FOUND", "SSH_SFTP_NOT_FOUND":
		status = http.StatusNotFound
	case "SSH_SESSION_NOT_FOUND":
		status = http.StatusNotFound
	case "SSH_HOST_KEY_UNKNOWN", "SSH_HOST_KEY_CHANGED", "SSH_HOST_KEY_CHALLENGE_INVALID",
		"SSH_HOST_KEY_ALREADY_TRUSTED", "SSH_HOST_KEY_NOT_TRUSTED", "SSH_RESOURCE_IN_USE", "SSH_NAME_CONFLICT",
		"SSH_SFTP_EXISTS", "SSH_SFTP_CONFLICT":
		status = http.StatusConflict
	case "SSH_SFTP_TEXT_TOO_LARGE":
		status = http.StatusRequestEntityTooLarge
	case "SSH_SHARE_TOO_LARGE":
		status = http.StatusRequestEntityTooLarge
	case "SSH_SFTP_BINARY":
		status = http.StatusUnprocessableEntity
	case "SSH_CONNECT_TIMEOUT":
		status = http.StatusGatewayTimeout
	case "SSH_SESSION_LIMIT":
		status = http.StatusTooManyRequests
	case "SSH_SFTP_PERMISSION_DENIED":
		status = http.StatusForbidden
	case "SSH_HOST_DISABLED", "SSH_NODE_EXPOSURE_DISABLED", "SSH_NODE_EXPOSURE_UNAVAILABLE",
		"SSH_CORE_NOT_RUNNING", "SSH_PROXY_CONNECT_FAILED", "SSH_PROXY_AUTH_FAILED",
		"SSH_CONNECT_FAILED", "SSH_CONNECT_CANCELLED", "SSH_AUTH_FAILED", "SSH_HANDSHAKE_FAILED",
		"SSH_SESSION_OPEN_FAILED", "SSH_SESSION_OPEN_TIMEOUT", "SSH_SFTP_UNAVAILABLE",
		"SSH_SFTP_UNSUPPORTED", "SSH_SFTP_FAILED":
		status = http.StatusBadGateway
	case "SSH_HOST_INVALID", "SSH_CREDENTIAL_INVALID", "SSH_REFERENCE_INVALID", "SSH_SFTP_INVALID_PATH",
		"SSH_SHARE_INVALID", "SSH_SHARE_PASSWORD_INVALID", "SSH_SHARE_DECRYPT_FAILED":
		status = http.StatusBadRequest
	}
	c.JSON(status, model.ErrorResponse{Error: apiError})
}
