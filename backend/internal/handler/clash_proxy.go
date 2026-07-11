package handler

import (
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/service"
	"github.com/gin-gonic/gin"
)

type ClashProxyHandler struct {
	settingsSvc *service.SettingsService
}

func NewClashProxyHandler(settingsSvc *service.SettingsService) *ClashProxyHandler {
	return &ClashProxyHandler{
		settingsSvc: settingsSvc,
	}
}

func (h *ClashProxyHandler) targetBase() (string, string) {
	port := "9090"
	secret := ""
	if h.settingsSvc != nil {
		if settings, err := h.settingsSvc.GetExperimentalSettings(); err == nil && settings != nil {
			if settings.ClashAPIPort != "" {
				port = settings.ClashAPIPort
			}
			secret = settings.ClashAPISecret
		}
	}
	return "http://127.0.0.1:" + port, secret
}

// Proxy 代理所有 Clash API 请求
func (h *ClashProxyHandler) Proxy(c *gin.Context) {
	if strings.EqualFold(c.GetHeader("Upgrade"), "websocket") {
		h.ProxyWebSocket(c)
		return
	}

	// 构建目标 URL
	targetPath := strings.TrimPrefix(c.Request.URL.Path, "/api/v1/clash")
	if targetPath == "" {
		targetPath = "/"
	}

	clashURL, secret := h.targetBase()
	targetURL, err := url.Parse(clashURL + targetPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CLASH_PROXY_ERROR", Message: "Invalid Clash API URL"},
		})
		return
	}

	// 复制查询参数
	targetURL.RawQuery = c.Request.URL.RawQuery

	// 创建代理请求
	proxyReq, err := http.NewRequest(c.Request.Method, targetURL.String(), c.Request.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error: model.APIError{Code: "CLASH_PROXY_ERROR", Message: "Failed to create proxy request"},
		})
		return
	}

	// 复制请求头
	for key, values := range c.Request.Header {
		for _, value := range values {
			proxyReq.Header.Add(key, value)
		}
	}

	// 添加 Clash API 认证
	if secret != "" {
		proxyReq.Header.Set("Authorization", "Bearer "+secret)
	}

	// 设置超时
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// 发送请求
	resp, err := client.Do(proxyReq)
	if err != nil {
		c.JSON(http.StatusBadGateway, model.ErrorResponse{
			Error: model.APIError{Code: "CLASH_PROXY_ERROR", Message: "Failed to connect to Clash API (" + clashURL + "): " + err.Error()},
		})
		return
	}
	defer resp.Body.Close()

	// 复制响应头
	for key, values := range resp.Header {
		for _, value := range values {
			c.Writer.Header().Add(key, value)
		}
	}

	// 设置状态码
	c.Status(resp.StatusCode)

	// 复制响应体
	io.Copy(c.Writer, resp.Body)
}

// ProxyWebSocket 代理 WebSocket 连接
func (h *ClashProxyHandler) ProxyWebSocket(c *gin.Context) {
	// WebSocket 代理使用反向代理
	clashURL, secret := h.targetBase()
	targetURL, _ := url.Parse(clashURL)

	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	proxy.ModifyResponse = func(resp *http.Response) error {
		return nil
	}

	// 重写请求路径
	originalPath := c.Request.URL.Path
	c.Request.URL.Path = strings.TrimPrefix(originalPath, "/api/v1/clash")
	if c.Request.URL.Path == "" {
		c.Request.URL.Path = "/"
	}

	// 添加认证
	if secret != "" {
		c.Request.Header.Set("Authorization", "Bearer "+secret)
	}

	proxy.ServeHTTP(c.Writer, c.Request)
}

// GetClashStatus 获取 Clash API 状态
func (h *ClashProxyHandler) GetClashStatus(c *gin.Context) {
	client := &http.Client{Timeout: 5 * time.Second}
	clashURL, secret := h.targetBase()
	req, _ := http.NewRequest("GET", clashURL+"/version", nil)
	if secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"connected": false,
			"error":     err.Error(),
			"target":    clashURL,
		})
		return
	}
	defer resp.Body.Close()

	c.JSON(http.StatusOK, gin.H{
		"connected": resp.StatusCode == http.StatusOK,
		"status":    resp.StatusCode,
		"target":    clashURL,
	})
}
