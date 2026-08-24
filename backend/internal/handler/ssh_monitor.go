package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *SSHHostHandler) GetSessionMonitor(c *gin.Context) {
	h.getSessionSnapshot(c, false)
}

func (h *SSHHostHandler) GetSessionDetails(c *gin.Context) {
	h.getSessionSnapshot(c, true)
}

func (h *SSHHostHandler) getSessionSnapshot(c *gin.Context, details bool) {
	sessionID, token, ok := parseSSHSFTPSession(c)
	if !ok {
		return
	}
	var result any
	var err error
	if details {
		result, err = h.service.GetSessionDetails(c.Request.Context(), sessionID, token)
	} else {
		result, err = h.service.GetSessionMonitor(c.Request.Context(), sessionID, token)
	}
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}
