package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackrun/internal/model"
)

const maxSSHSingboxDeployRequestBody = 32 << 10

func (h *SSHHostHandler) InstallSingbox(c *gin.Context) {
	hostID, ok := parseSSHID(c)
	if !ok {
		return
	}
	result, err := h.service.InstallSingbox(c.Request.Context(), hostID)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}

func (h *SSHHostHandler) DeploySingbox(c *gin.Context) {
	hostID, ok := parseSSHID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxSSHSingboxDeployRequestBody)
	var request model.SSHSingboxDeployRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeSSHInvalidRequest(c, err)
		return
	}
	result, err := h.service.DeploySingbox(c.Request.Context(), hostID, request)
	if err != nil {
		writeSSHError(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, result)
}
