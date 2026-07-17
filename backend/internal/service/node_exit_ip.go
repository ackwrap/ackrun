package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

const (
	nodeExitIPMaxResponse          = 64 << 10
	nodeExitIPProxyListMaxResponse = 4 << 20
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
		return nil, errors.New("节点未启用，无法载入活动配置")
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
	selectedTag, err = svc.resolveActiveNodeOutboundTag(lookupCtx, node, selectedTag)
	if err != nil {
		return nil, err
	}

	logging.Info("node.exit_ip", "checking exit IP uid=%s", node.UID)
	exitIP, err := svc.lookupNodeExitIP(lookupCtx, selectedTag, nodeIP.To4() == nil)
	if err != nil {
		logging.Error("node.exit_ip", "exit IP check failed uid=%s", node.UID)
		return nil, fmt.Errorf("通过节点查询出口 IP 失败: %w", err)
	}
	matched := nodeIP.Equal(exitIP)
	logging.Info("node.exit_ip", "exit IP check completed uid=%s matched=%v", node.UID, matched)
	return &model.NodeExitIPResponse{
		UID: node.UID, NodeName: node.Name, NodeIP: nodeIP.String(), ExitIP: exitIP.String(),
		Matched: matched, Resolution: resolution,
	}, nil
}

func (svc *NodeService) resolveActiveNodeOutboundTag(ctx context.Context, node model.Node, expectedTag string) (string, error) {
	baseURL, secret, err := svc.nodeClashAPI()
	if err != nil {
		return "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/proxies", nil)
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
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return "", errors.New("sing-box Clash API 鉴权失败")
	}
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("读取 sing-box 活动 outbound 返回 HTTP %d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nodeExitIPProxyListMaxResponse+1))
	if err != nil {
		return "", errors.New("读取 sing-box 活动 outbound 失败")
	}
	if len(body) > nodeExitIPProxyListMaxResponse {
		return "", errors.New("sing-box 活动 outbound 响应超过 4 MiB")
	}
	var payload struct {
		Proxies map[string]json.RawMessage `json:"proxies"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", errors.New("无法解析 sing-box 活动 outbound")
	}
	if _, loaded := payload.Proxies[expectedTag]; loaded {
		return expectedTag, nil
	}

	candidates := make([]string, 0, 1)
	for tag := range payload.Proxies {
		if strings.HasSuffix(tag, "-"+node.UID) {
			candidates = append(candidates, tag)
		}
	}
	if len(candidates) == 1 {
		return candidates[0], nil
	}
	if len(candidates) > 1 {
		return "", errors.New("活动配置中存在多个匹配节点 UID 的 outbound，请重新生成并应用配置")
	}
	return "", errors.New("目标节点未载入活动配置，请重新生成并应用配置")
}

func (svc *NodeService) lookupNodeExitIP(ctx context.Context, outboundTag string, ipv6 bool) (net.IP, error) {
	baseURL, secret, err := svc.nodeClashAPI()
	if err != nil {
		return nil, err
	}
	ipVersion := 4
	if ipv6 {
		ipVersion = 6
	}
	endpoint := fmt.Sprintf("%s/proxies/%s/exit-ip?ip_version=%d", baseURL, url.PathEscape(outboundTag), ipVersion)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return nil, errors.New("无法连接 sing-box Clash API，请确认核心正在运行")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, errors.New("sing-box Clash API 鉴权失败")
		case http.StatusNotFound:
			return nil, errors.New("当前核心不支持出口 IP 检测接口，或目标节点未载入活动配置，请更新核心并重新生成配置")
		case http.StatusBadGateway, http.StatusServiceUnavailable:
			return nil, errors.New("sing-box 无法通过目标节点访问出口 IP 服务")
		case http.StatusGatewayTimeout:
			return nil, errors.New("sing-box 出口 IP 检测超时")
		default:
			return nil, fmt.Errorf("sing-box 出口 IP 检测返回 HTTP %d", response.StatusCode)
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nodeExitIPMaxResponse+1))
	if err != nil {
		return nil, errors.New("读取 sing-box 出口 IP 检测响应失败")
	}
	if len(body) > nodeExitIPMaxResponse {
		return nil, errors.New("sing-box 出口 IP 检测响应超过 64 KiB")
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, errors.New("无法解析 sing-box 出口 IP 检测响应")
	}
	exitIP := net.ParseIP(strings.TrimSpace(payload.IP))
	if exitIP == nil || ipv6 == (exitIP.To4() != nil) {
		return nil, errors.New("sing-box 出口 IP 检测未返回有效地址")
	}
	return exitIP, nil
}

func (svc *NodeService) nodeHTTPClient() *http.Client {
	if svc.httpClient != nil {
		return svc.httpClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}
