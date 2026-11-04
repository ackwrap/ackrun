package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
)

func (h *SSHHostHandler) GetPrivacySettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	settings, err := h.service.PrivacySettings()
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SSHHostHandler) UpdatePrivacySettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !mcpOriginAllowed(c.Request) {
		writeMCPError(c, http.StatusForbidden, "SSH_PRIVACY_ORIGIN_DENIED", "不允许跨站修改 SSH 地址隐藏设置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
	var request struct {
		HideHostAddress *bool `json:"hide_host_address"`
	}
	if err := c.ShouldBindJSON(&request); err != nil || request.HideHostAddress == nil {
		writeMCPError(c, http.StatusBadRequest, "SSH_PRIVACY_INVALID", "请指定是否隐藏 SSH 主机地址")
		return
	}
	settings, err := h.service.UpdatePrivacySettings(model.SSHPrivacySettings{HideHostAddress: *request.HideHostAddress})
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}
