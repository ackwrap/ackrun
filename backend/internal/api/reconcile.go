package api

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/ackwrap/ackwrap/internal/service"
)

func configMutationMiddleware(reconciler *service.ConfigReconcileService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if reconciler == nil || c.Writer.Status() >= http.StatusBadRequest || !shouldReconcileRequest(c.Request.Method, c.Request.URL.Path) {
			return
		}
		reconciler.Trigger(c.Request.Method + " " + c.Request.URL.Path)
	}
}

func shouldReconcileRequest(method, path string) bool {
	if method == http.MethodGet || strings.HasPrefix(path, "/api/v1/config/") || strings.HasPrefix(path, "/api/v1/core/") {
		return false
	}
	if path == "/api/v1/nodes/flag" || path == "/api/v1/nodes/flags" || path == "/api/v1/nodes/import/preview" {
		return false
	}
	if path == "/api/v1/settings/update" || strings.Contains(path, "/tcping") || strings.Contains(path, "/exit-ip") || strings.Contains(path, "/traceroute") || strings.Contains(path, "/sync") {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/settings/geoip-providers") || strings.HasPrefix(path, "/api/v1/settings/connectivity-targets") {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/settings/node-filters") {
		return false
	}
	if path == "/api/v1/settings/core-restart" {
		return false
	}
	if path == "/api/v1/settings/inbound-mode" || path == "/api/v1/settings/proxy-mode" {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/collections/") && strings.HasSuffix(path, "/test") {
		return false
	}
	if strings.HasPrefix(path, "/api/v1/subscriptions") {
		return method == http.MethodDelete
	}
	configPrefixes := []string{
		"/api/v1/settings/",
		"/api/v1/nodes/",
		"/api/v1/collections",
		"/api/v1/rules",
		"/api/v1/dns/",
		"/api/v1/node-groups",
	}
	for _, prefix := range configPrefixes {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}
