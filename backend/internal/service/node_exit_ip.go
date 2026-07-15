package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

const (
	nodeExitIPv4CheckURL  = "https://1.1.1.1/cdn-cgi/trace"
	nodeExitIPv6CheckURL  = "https://[2606:4700:4700::1111]/cdn-cgi/trace"
	nodeExitIPMaxResponse = 64 << 10
)

func (svc *NodeService) ExitIP(ctx context.Context, uid string) (*model.NodeExitIPResponse, error) {
	nodes, err := svc.store.ListNodesByUIDs([]string{uid})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNodeNotFound
	}
	node := nodes[0]
	if !node.Enabled {
		return nil, errors.New("节点未启用，无法载入出口检测 selector")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	nodeIP, err := traceroute.ResolveTarget(lookupCtx, node.Server)
	if err != nil {
		return nil, fmt.Errorf("解析节点地址失败: %w", err)
	}
	resolution := "literal"
	if net.ParseIP(strings.TrimSpace(node.Server)) == nil {
		resolution = "alidns_doh"
	}

	enabledNodes, err := svc.store.ListEnabledNodes()
	if err != nil {
		return nil, err
	}
	selectedTag := buildNodeOutboundTags(enabledNodes)[node.UID]
	if selectedTag == "" {
		return nil, errors.New("节点未载入活动配置")
	}
	proxyURL, err := svc.nodeExitIPProxyURL()
	if err != nil {
		return nil, err
	}

	svc.exitIPMu.Lock()
	defer svc.exitIPMu.Unlock()
	logging.Info("node.exit_ip", "checking exit IP uid=%s", node.UID)
	previous, err := svc.selectNodeCheckOutbound(lookupCtx, selectedTag)
	if err != nil {
		return nil, err
	}
	lookup := lookupNodeExitIP
	if svc.exitIPLookup != nil {
		lookup = svc.exitIPLookup
	}
	exitIP, lookupErr := lookup(lookupCtx, proxyURL, nodeIP.To4() == nil)
	var restoreErr error
	if previous != "" && previous != selectedTag {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 5*time.Second)
		restoreErr = svc.setNodeCheckOutbound(restoreCtx, previous)
		restoreCancel()
	}
	if restoreErr != nil {
		logging.Error("node.exit_ip", "restore internal selector failed: %v", restoreErr)
		return nil, errors.New("出口检测完成，但恢复内部 selector 失败，请重启核心恢复状态")
	}
	if lookupErr != nil {
		return nil, fmt.Errorf("通过节点查询出口 IP 失败: %w", lookupErr)
	}
	matched := nodeIP.Equal(exitIP)
	logging.Info("node.exit_ip", "exit IP check completed uid=%s matched=%v", node.UID, matched)
	return &model.NodeExitIPResponse{
		UID: node.UID, NodeName: node.Name, NodeIP: nodeIP.String(), ExitIP: exitIP.String(),
		Matched: matched, Resolution: resolution,
	}, nil
}

func (svc *NodeService) nodeExitIPProxyURL() (*url.URL, error) {
	if svc.paths == nil {
		return nil, errors.New("出口检测路径服务不可用")
	}
	configPath, ok, err := svc.paths.ActiveConfigPath()
	if err != nil {
		return nil, errors.New("读取活动配置失败")
	}
	if !ok {
		return nil, errors.New("活动配置不存在")
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		return nil, errors.New("读取活动配置失败")
	}
	var config struct {
		Inbounds []struct {
			Type       string `json:"type"`
			Tag        string `json:"tag"`
			Listen     string `json:"listen"`
			ListenPort int    `json:"listen_port"`
		} `json:"inbounds"`
	}
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, errors.New("活动配置格式无效")
	}
	for _, inbound := range config.Inbounds {
		if inbound.Type != "mixed" || inbound.Tag != "mixed-in" || inbound.ListenPort <= 0 {
			continue
		}
		host := strings.TrimSpace(inbound.Listen)
		switch host {
		case "", "0.0.0.0":
			host = "127.0.0.1"
		case "::", "[::]":
			host = "::1"
		}
		host = strings.Trim(host, "[]")
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return nil, errors.New("活动 mixed 入站必须监听本机回环地址才能执行出口检测")
		}
		return &url.URL{Scheme: "http", Host: net.JoinHostPort(host, fmt.Sprintf("%d", inbound.ListenPort))}, nil
	}
	return nil, errors.New("活动配置未启用 mixed 入站，出口 IP 检测需要 mixed 或 tun_mixed 模式")
}

func (svc *NodeService) selectNodeCheckOutbound(ctx context.Context, selectedTag string) (string, error) {
	baseURL, secret, err := svc.nodeClashAPI()
	if err != nil {
		return "", err
	}
	endpoint := baseURL + "/proxies/" + url.PathEscape(nodeCheckOutboundTag)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return "", errors.New("无法连接 sing-box Clash API，请确认核心正在运行")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("活动配置没有出口检测 selector，请重新生成配置并重启核心")
	}
	var group struct {
		Now string   `json:"now"`
		All []string `json:"all"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, nodeExitIPMaxResponse)).Decode(&group); err != nil {
		return "", errors.New("无法解析出口检测 selector 状态")
	}
	if !containsString(group.All, selectedTag) {
		return "", errors.New("目标节点未载入活动配置，请重新生成配置并重启核心")
	}
	if group.Now != selectedTag {
		if err := svc.setNodeCheckOutbound(ctx, selectedTag); err != nil {
			return "", err
		}
	}
	return group.Now, nil
}

func (svc *NodeService) setNodeCheckOutbound(ctx context.Context, selectedTag string) error {
	baseURL, secret, err := svc.nodeClashAPI()
	if err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"name": selectedTag})
	endpoint := baseURL + "/proxies/" + url.PathEscape(nodeCheckOutboundTag)
	request, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return errors.New("无法更新出口检测 selector")
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("出口检测 selector 返回 HTTP %d", response.StatusCode)
	}
	return nil
}

func (svc *NodeService) nodeHTTPClient() *http.Client {
	if svc.httpClient != nil {
		return svc.httpClient
	}
	return &http.Client{Timeout: 5 * time.Second}
}

func lookupNodeExitIP(ctx context.Context, proxyURL *url.URL, ipv6 bool) (net.IP, error) {
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		TLSHandshakeTimeout:   5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second,
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Transport: transport,
		Timeout:   12 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	checkURL := nodeExitIPv4CheckURL
	if ipv6 {
		checkURL = nodeExitIPv6CheckURL
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, checkURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Ackwrap/1")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("出口 IP 服务返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nodeExitIPMaxResponse+1))
	if err != nil {
		return nil, err
	}
	if len(body) > nodeExitIPMaxResponse {
		return nil, errors.New("出口 IP 响应超过 64 KiB")
	}
	return parseNodeExitIP(string(body))
}

func parseNodeExitIP(body string) (net.IP, error) {
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && key == "ip" {
			if ip := net.ParseIP(strings.TrimSpace(value)); ip != nil {
				return ip, nil
			}
		}
	}
	return nil, errors.New("出口 IP 服务未返回有效地址")
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
