package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *SSHHostHandler) GetSessionMonitor(c *gin.Context) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	result, err := h.service.GetSessionMonitor(c.Request.Context(), sessionID, token)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}
