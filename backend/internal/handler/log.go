package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/service"
)

type LogHandler struct {
	coreLogs *service.CoreLogService
}

func NewLogHandler(coreLogs *service.CoreLogService) *LogHandler {
	return &LogHandler{coreLogs: coreLogs}
}

func (h *LogHandler) ListCore(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	c.JSON(http.StatusOK, h.coreLogs.List(limit))
}

func (h *LogHandler) ClearCore(c *gin.Context) {
	h.coreLogs.Clear()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "core logs cleared"})
}

func (h *LogHandler) ListTool(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	c.JSON(http.StatusOK, logging.ListToolLogs(limit))
}

func (h *LogHandler) ClearTool(c *gin.Context) {
	logging.ClearToolLogs()
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "tool logs cleared"})
}
