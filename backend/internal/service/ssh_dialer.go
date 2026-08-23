package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/proxy"

	"github.com/ackwrap/ackrun/internal/model"
)

func (svc *SSHHostService) dial(ctx context.Context, host *model.SSHHost, targetAddress string) (net.Conn, error) {
	if host.ConnectionMode == "direct" {
		conn, err := (&net.Dialer{Timeout: sshConnectTimeout}).DialContext(ctx, "tcp", targetAddress)
		if err != nil {
			return nil, classifySSHDialError(ctx, err)
		}
		return conn, nil
	}
	if host.NodeExposureID == nil {
		return nil, sshError("SSH_NODE_EXPOSURE_NOT_FOUND", "SSH 主机未配置节点入口", nil)
	}
	exposure, err := svc.store.GetNodeExposure(*host.NodeExposureID)
	if err != nil {
		return nil, sshError("SSH_NODE_EXPOSURE_NOT_FOUND", "节点入口不存在", err)
	}
	if !exposure.Enabled {
		return nil, sshError("SSH_NODE_EXPOSURE_DISABLED", "节点入口已停用", nil)
	}
	if !exposure.NodeExists || !exposure.NodeEnabled {
		return nil, sshError("SSH_NODE_EXPOSURE_UNAVAILABLE", "节点入口目标不可用", nil)
	}
	if svc.core == nil || !svc.core.IsRunning() {
		return nil, sshError("SSH_CORE_NOT_RUNNING", "sing-box 核心未运行", nil)
	}
	proxyHost, err := localExposureAddress(exposure.Listen)
	if err != nil {
		return nil, sshError("SSH_NODE_EXPOSURE_UNAVAILABLE", "节点入口监听地址不可供后端连接", err)
	}
	proxyAddress := net.JoinHostPort(proxyHost, fmt.Sprintf("%d", exposure.ListenPort))
	switch exposure.InboundType {
	case "socks", "mixed":
		return dialSSHThroughSOCKS(ctx, proxyAddress, targetAddress, exposure.Username, exposure.Password)
	case "http":
		return dialSSHThroughHTTP(ctx, proxyAddress, targetAddress, exposure.Username, exposure.Password)
	default:
		return nil, sshError("SSH_NODE_EXPOSURE_UNAVAILABLE", "节点入口协议不支持 SSH 连接", nil)
	}
}

func dialSSHThroughSOCKS(ctx context.Context, proxyAddress, targetAddress, username, password string) (net.Conn, error) {
	var auth *proxy.Auth
	if username != "" || password != "" {
		auth = &proxy.Auth{User: username, Password: password}
	}
	dialer, err := proxy.SOCKS5("tcp", proxyAddress, auth, &net.Dialer{Timeout: sshConnectTimeout})
	if err != nil {
		return nil, sshError("SSH_PROXY_CONNECT_FAILED", "无法初始化 SOCKS5 节点入口", err)
	}
	contextDialer, ok := dialer.(proxy.ContextDialer)
	if !ok {
		return nil, sshError("SSH_PROXY_CONNECT_FAILED", "SOCKS5 节点入口不支持取消", nil)
	}
	conn, err := contextDialer.DialContext(ctx, "tcp", targetAddress)
	if err != nil {
		return nil, classifySSHProxyError(ctx, err)
	}
	return conn, nil
}

func dialSSHThroughHTTP(ctx context.Context, proxyAddress, targetAddress, username, password string) (net.Conn, error) {
	conn, err := (&net.Dialer{Timeout: sshConnectTimeout}).DialContext(ctx, "tcp", proxyAddress)
	if err != nil {
		return nil, classifySSHProxyError(ctx, err)
	}
	closeOnError := true
	defer func() {
		if closeOnError {
			_ = conn.Close()
		}
	}()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(sshConnectTimeout))
	}
	request := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: targetAddress},
		Host:   targetAddress,
		Header: make(http.Header),
	}
	if username != "" || password != "" {
		request.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(username+":"+password)))
	}
	if err := request.Write(conn); err != nil {
		return nil, classifySSHProxyError(ctx, err)
	}
	reader := bufio.NewReaderSize(conn, 4096)
	response, err := http.ReadResponse(reader, request)
	if err != nil {
		return nil, classifySSHProxyError(ctx, err)
	}
	if response.StatusCode == http.StatusProxyAuthRequired {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, sshError("SSH_PROXY_AUTH_FAILED", "节点入口认证失败", nil)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, sshError("SSH_PROXY_CONNECT_FAILED", "节点入口拒绝 SSH 目标连接", nil)
	}
	_ = conn.SetDeadline(time.Time{})
	closeOnError = false
	if reader.Buffered() > 0 {
		return &bufferedSSHConn{Conn: conn, reader: reader}, nil
	}
	return conn, nil
}

type bufferedSSHConn struct {
	net.Conn
	reader *bufio.Reader
}

func (conn *bufferedSSHConn) Read(content []byte) (int, error) {
	return conn.reader.Read(content)
}

func localExposureAddress(listen string) (string, error) {
	listen = strings.TrimSpace(listen)
	if listen == "localhost" {
		return "127.0.0.1", nil
	}
	ip := net.ParseIP(listen)
	if ip == nil {
		return "", fmt.Errorf("listen address is not an IP address")
	}
	if ip.IsUnspecified() {
		if ip.To4() == nil {
			return "::1", nil
		}
		return "127.0.0.1", nil
	}
	if ip.IsLoopback() {
		return listen, nil
	}
	addresses, err := net.InterfaceAddrs()
	if err != nil {
		return "", err
	}
	for _, address := range addresses {
		localIP, _, parseErr := net.ParseCIDR(address.String())
		if parseErr == nil && localIP.Equal(ip) {
			return listen, nil
		}
	}
	return "", fmt.Errorf("listen address is not local")
}

func classifySSHDialError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return classifySSHConnectError(ctx, err)
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return sshError("SSH_CONNECT_TIMEOUT", "SSH 连接超时", err)
	}
	return sshError("SSH_CONNECT_FAILED", "无法连接 SSH 主机", err)
}

func classifySSHProxyError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return classifySSHConnectError(ctx, err)
	}
	lower := strings.ToLower(err.Error())
	if strings.Contains(lower, "authentication") || strings.Contains(lower, "auth") {
		return sshError("SSH_PROXY_AUTH_FAILED", "节点入口认证失败", err)
	}
	return sshError("SSH_PROXY_CONNECT_FAILED", "无法通过节点入口连接 SSH 主机", err)
}
