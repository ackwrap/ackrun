package handler

import (
	"context"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/mcpserver"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/service"
)

type SSHMCPHandler struct {
	service   *service.SSHMCPService
	hosts     *service.SSHHostService
	transport http.Handler
	requests  chan struct{}
}

func NewSSHMCPHandler(settings *service.SSHMCPService, hosts *service.SSHHostService) *SSHMCPHandler {
	server := mcpserver.NewSSHServer(hosts)
	transport := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	return &SSHMCPHandler{service: settings, hosts: hosts, transport: transport, requests: make(chan struct{}, 16)}
}

func (h *SSHMCPHandler) GetSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	settings, err := h.service.Settings()
	if err != nil {
		writeMCPError(c, http.StatusInternalServerError, "SSH_MCP_SETTINGS_FAILED", "读取 MCP 配置失败")
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SSHMCPHandler) UpdateSettings(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if !mcpOriginAllowed(c.Request) {
		writeMCPError(c, http.StatusForbidden, "SSH_MCP_ORIGIN_DENIED", "不允许跨站修改 MCP 配置")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	var request model.SSHMCPSettingsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeMCPError(c, http.StatusBadRequest, "SSH_MCP_INVALID", "MCP 配置格式无效")
		return
	}
	settings, err := h.service.UpdateSettings(request)
	if err != nil {
		writeMCPError(c, http.StatusBadRequest, "SSH_MCP_SETTINGS_FAILED", err.Error())
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *SSHMCPHandler) ServeHTTP(c *gin.Context) {
	h.withAuthorizedRequest(c, func() {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 2<<20)
		h.transport.ServeHTTP(c.Writer, c.Request)
	})
}

func (h *SSHMCPHandler) withAuthorizedRequest(c *gin.Context, run func()) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	peer, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil || !mcpLANAddress(peer) || !mcpHostAllowed(c.Request.Host) || !mcpOriginAllowed(c.Request) {
		writeMCPError(c, http.StatusForbidden, "SSH_MCP_LAN_ONLY", "MCP 仅允许通过本机或 LAN IP 访问")
		return
	}
	for _, header := range []string{"Forwarded", "X-Forwarded-For", "X-Real-IP"} {
		if len(c.Request.Header.Values(header)) != 0 {
			writeMCPError(c, http.StatusForbidden, "SSH_MCP_PROXY_DENIED", "请从 LAN 直接连接 MCP 地址")
			return
		}
	}
	if c.Request.URL.RawQuery != "" {
		writeMCPError(c, http.StatusBadRequest, "SSH_MCP_INVALID", "MCP 地址不接受查询参数，Token 请使用 Authorization 请求头")
		return
	}
	var token string
	authorization := strings.Fields(c.GetHeader("Authorization"))
	if len(c.Request.Header.Values("Authorization")) == 1 && len(authorization) == 2 && strings.EqualFold(authorization[0], "Bearer") {
		token = authorization[1]
	}
	enabled, authenticated, err := h.service.Authenticate(token)
	if err != nil {
		writeMCPError(c, http.StatusServiceUnavailable, "SSH_MCP_UNAVAILABLE", "MCP 配置暂不可用")
		return
	}
	if !enabled {
		writeMCPError(c, http.StatusForbidden, "SSH_MCP_DISABLED", "SSH MCP 尚未启用")
		return
	}
	if !authenticated {
		c.Header("WWW-Authenticate", `Bearer realm="ackwrap-ssh-mcp"`)
		writeMCPError(c, http.StatusUnauthorized, "SSH_MCP_UNAUTHORIZED", "需要有效的 MCP Token")
		return
	}
	select {
	case h.requests <- struct{}{}:
		defer func() { <-h.requests }()
	default:
		writeMCPError(c, http.StatusTooManyRequests, "SSH_MCP_BUSY", "MCP 并发请求已达上限，请稍后重试")
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), service.SSHMCPTransferTimeout)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	run()
}

func mcpLANAddress(host string) bool {
	address, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}
	address = address.Unmap()
	return address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast()
}

func mcpHostAllowed(authority string) bool {
	host := authority
	if value, _, err := net.SplitHostPort(authority); err == nil {
		host = value
	}
	return strings.EqualFold(host, "localhost") || mcpLANAddress(strings.Trim(host, "[]"))
}

func mcpOriginAllowed(request *http.Request) bool {
	origins := request.Header.Values("Origin")
	if len(origins) == 0 {
		return true
	}
	if len(origins) != 1 {
		return false
	}
	origin, err := url.Parse(origins[0])
	scheme := "http"
	if request.TLS != nil {
		scheme = "https"
	}
	return err == nil && origin.Scheme == scheme && strings.EqualFold(origin.Host, request.Host) && origin.User == nil && origin.Path == "" && origin.RawQuery == "" && origin.Fragment == ""
}

func writeMCPError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, model.ErrorResponse{Error: model.APIError{Code: code, Message: message}})
}
