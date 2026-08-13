package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/geoquery"
	"github.com/ackwrap/ackrun/internal/httpclient"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/traceroute"
)

const (
	nodeExitIPMaxResponse          = 64 << 10
	nodeExitIPProxyListMaxResponse = 4 << 20
)

var ErrNodeExitIPInvalid = errors.New("invalid node exit IP request")

type nodeExitIPFailure struct {
	details model.NodeExitIPErrorDetails
	message string
	cause   error
}

func (e *nodeExitIPFailure) Error() string {
	return e.message
}

func (e *nodeExitIPFailure) Unwrap() error {
	return e.cause
}

func newNodeExitIPFailure(stage string, reason string, message string, coreStatus int, upstreamStatus int, cause error) error {
	return &nodeExitIPFailure{
		details: model.NodeExitIPErrorDetails{
			Stage: stage, Reason: reason, CoreStatus: coreStatus, UpstreamStatus: upstreamStatus,
		},
		message: message,
		cause:   cause,
	}
}

func ExitIPFailureDetails(err error) *model.NodeExitIPErrorDetails {
	var failure *nodeExitIPFailure
	if !errors.As(err, &failure) {
		return nil
	}
	details := failure.details
	return &details
}

func (svc *NodeService) ExitIP(ctx context.Context, uid string, geoProvider string) (result *model.NodeExitIPResponse, returnErr error) {
	logUID := "unknown"
	defer func() {
		if returnErr == nil {
			return
		}
		details := ExitIPFailureDetails(returnErr)
		if details == nil {
			logging.Error("node.exit_ip", "exit IP check failed uid=%s reason=unclassified", logUID)
			return
		}
		logging.Error(
			"node.exit_ip",
			"exit IP check failed uid=%s stage=%s reason=%s core_status=%d upstream_status=%d",
			logUID, details.Stage, details.Reason, details.CoreStatus, details.UpstreamStatus,
		)
	}()
	resolvedProvider, err := svc.resolveNodeGeoProvider(geoProvider)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNodeExitIPInvalid, err)
	}
	nodes, err := svc.store.ListNodesByUIDs([]string{uid})
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, ErrNodeNotFound
	}
	node := nodes[0]
	logUID = node.UID
	logging.Info("node.exit_ip", "checking exit IP uid=%s", logUID)
	if !node.Enabled {
		return nil, errors.New("节点未启用，无法载入活动配置")
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	nodeIP, err := traceroute.ResolveTarget(lookupCtx, node.Server)
	if err != nil {
		reason := "dns_resolution"
		message := "节点地址解析失败，请检查节点域名和 DNS 设置"
		if isNodeExitIPTimeout(err) {
			reason = "timeout"
			message = "节点地址解析超时，请检查 DNS 和网络连接"
		}
		return nil, newNodeExitIPFailure("node_resolution", reason, message, 0, 0, err)
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

	exitIP, err := svc.lookupNodeExitIP(lookupCtx, selectedTag, nodeIP.To4() == nil)
	if err != nil {
		return nil, fmt.Errorf("通过节点查询出口 IP 失败: %w", err)
	}
	matched := nodeIP.Equal(exitIP)
	response := &model.NodeExitIPResponse{
		UID: node.UID, NodeName: node.Name, NodeIP: nodeIP.String(), ExitIP: exitIP.String(),
		Matched: matched, Resolution: resolution, GeoProvider: resolvedProvider.Key,
	}
	if resolvedProvider.Key != traceroute.DefaultGeoProvider {
		geoCtx, geoCancel := context.WithTimeout(ctx, 10*time.Second)
		geo, geoErr := resolvedProvider.lookup(geoCtx, exitIP)
		geoCancel()
		if geoErr != nil {
			logging.Info("node.exit_ip", "online Geo lookup failed, trying local fallback uid=%s provider=%s", node.UID, resolvedProvider.Key)
			localGeo, localErr := svc.lookupLocalExitGeo(exitIP)
			if localErr != nil {
				response.GeoError = fmt.Sprintf("%s；%v", onlineGeoErrorMessage(geoErr), localErr)
			} else {
				response.Geo = convertTracerouteGeo(localGeo)
				logging.Info("node.exit_ip", "local Geo fallback succeeded uid=%s provider=%s", node.UID, resolvedProvider.Key)
			}
		} else {
			response.Geo = convertTracerouteGeo(geo)
		}
	}
	logging.Info("node.exit_ip", "exit IP check completed uid=%s matched=%v geo_provider=%s geo_success=%v", node.UID, matched, resolvedProvider.Key, response.Geo != nil)
	return response, nil
}

func (svc *NodeService) lookupLocalExitGeo(ip net.IP) (traceroute.GeoData, error) {
	if svc.localGeoLookup != nil {
		return svc.localGeoLookup(ip)
	}
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return traceroute.GeoData{}, errors.New("读取本地 GeoIP 状态失败")
	}
	for _, asset := range assets {
		if asset.Type != "geoip" || strings.TrimSpace(asset.LocalPath) == "" {
			continue
		}
		reader, err := geoquery.OpenGeoIP(asset.LocalPath)
		if err != nil {
			return traceroute.GeoData{}, errors.New("本地 GeoIP 数据库无法读取，请重新同步")
		}
		address, parseErr := netip.ParseAddr(ip.String())
		if parseErr != nil {
			reader.Close()
			return traceroute.GeoData{}, errors.New("本地 GeoIP 查询地址无效")
		}
		countryCode := reader.Lookup(address.Unmap())
		reader.Close()
		if countryCode == "unknown" {
			return traceroute.GeoData{}, errors.New("本地 GeoIP 数据库未匹配到归属")
		}
		return traceroute.GeoDataFromCountryCode(countryCode, "geoip.db（本地回退）"), nil
	}
	return traceroute.GeoData{}, errors.New("本地 GeoIP 数据库不可用，请先在规则管理中同步 GeoIP")
}

func onlineGeoErrorMessage(err error) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return "在线 Geo 查询超时"
	}
	var networkError net.Error
	if errors.As(err, &networkError) && networkError.Timeout() {
		return "在线 Geo 查询超时"
	}
	return "在线 Geo 查询失败"
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
	httpclient.SetBrowserUserAgent(request)
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return "", describeCoreAPITransportFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized {
		return "", newNodeExitIPFailure("core_api", "core_authentication", "sing-box Clash API 鉴权失败，请检查 API Secret", response.StatusCode, 0, nil)
	}
	if response.StatusCode == http.StatusForbidden {
		return "", newNodeExitIPFailure("core_api", "core_access_denied", "sing-box Clash API 拒绝访问，请检查鉴权和本机访问限制", response.StatusCode, 0, nil)
	}
	if response.StatusCode != http.StatusOK {
		return "", newNodeExitIPFailure("core_api", "core_http_status", fmt.Sprintf("读取 sing-box 活动 outbound 返回 HTTP %d", response.StatusCode), response.StatusCode, 0, nil)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nodeExitIPProxyListMaxResponse+1))
	if err != nil {
		return "", newNodeExitIPFailure("core_response", "read_response", "读取 sing-box 活动 outbound 失败", response.StatusCode, 0, err)
	}
	if len(body) > nodeExitIPProxyListMaxResponse {
		return "", newNodeExitIPFailure("core_response", "response_too_large", "sing-box 活动 outbound 响应超过 4 MiB", response.StatusCode, 0, nil)
	}
	var payload struct {
		Proxies map[string]json.RawMessage `json:"proxies"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", newNodeExitIPFailure("core_response", "invalid_response", "无法解析 sing-box 活动 outbound", response.StatusCode, 0, err)
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
		return "", newNodeExitIPFailure("active_outbound", "multiple_matches", "活动配置中存在多个匹配节点 UID 的 outbound，请重新生成并应用配置", response.StatusCode, 0, nil)
	}
	return "", newNodeExitIPFailure("active_outbound", "node_not_loaded", "目标节点未载入活动配置，请重新生成并应用配置", response.StatusCode, 0, nil)
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
	httpclient.SetBrowserUserAgent(request)
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return nil, describeCoreAPITransportFailure(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var payload struct {
			Stage          string `json:"stage"`
			Reason         string `json:"reason"`
			UpstreamStatus int    `json:"upstream_status"`
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, nodeExitIPMaxResponse+1))
		if len(body) <= nodeExitIPMaxResponse {
			_ = json.Unmarshal(body, &payload)
		}
		switch response.StatusCode {
		case http.StatusUnauthorized:
			return nil, newNodeExitIPFailure("core_api", "core_authentication", "sing-box Clash API 鉴权失败，请检查 API Secret", response.StatusCode, 0, nil)
		case http.StatusForbidden:
			return nil, newNodeExitIPFailure("core_api", "core_access_denied", "sing-box Clash API 拒绝访问，请检查鉴权和本机访问限制", response.StatusCode, 0, nil)
		case http.StatusNotFound:
			return nil, newNodeExitIPFailure("core_api", "unsupported_api", "当前核心不支持出口 IP 检测接口，或目标节点未载入活动配置，请更新核心并重新生成配置", response.StatusCode, 0, nil)
		case http.StatusBadGateway, http.StatusServiceUnavailable:
			return nil, describeCoreExitIPFailure(payload.Stage, payload.Reason, response.StatusCode, payload.UpstreamStatus)
		case http.StatusGatewayTimeout:
			return nil, newNodeExitIPFailure("timeout", "timeout", "sing-box 出口 IP 检测超时", response.StatusCode, 0, nil)
		default:
			return nil, newNodeExitIPFailure("core_api", "core_http_status", fmt.Sprintf("sing-box 出口 IP 检测返回 HTTP %d", response.StatusCode), response.StatusCode, 0, nil)
		}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nodeExitIPMaxResponse+1))
	if err != nil {
		return nil, newNodeExitIPFailure("core_response", "read_response", "读取 sing-box 出口 IP 检测响应失败", response.StatusCode, 0, err)
	}
	if len(body) > nodeExitIPMaxResponse {
		return nil, newNodeExitIPFailure("core_response", "response_too_large", "sing-box 出口 IP 检测响应超过 64 KiB", response.StatusCode, 0, nil)
	}
	var payload struct {
		IP string `json:"ip"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, newNodeExitIPFailure("core_response", "invalid_response", "无法解析 sing-box 出口 IP 检测响应", response.StatusCode, 0, err)
	}
	exitIP := net.ParseIP(strings.TrimSpace(payload.IP))
	if exitIP == nil || ipv6 == (exitIP.To4() != nil) {
		return nil, newNodeExitIPFailure("core_response", "invalid_response", "sing-box 出口 IP 检测未返回有效地址", response.StatusCode, 0, nil)
	}
	return exitIP, nil
}

func describeCoreExitIPFailure(stage string, reason string, coreStatus int, upstreamStatus int) error {
	stage = normalizeNodeExitIPStage(stage)
	reason, message := describeNodeExitIPReason(stage, reason, upstreamStatus)
	return newNodeExitIPFailure(stage, reason, message, coreStatus, upstreamStatus, nil)
}

func normalizeNodeExitIPStage(stage string) string {
	switch stage {
	case "outbound_connect", "http_status", "read_response", "response_too_large", "invalid_response", "address_family", "timeout":
		return stage
	default:
		return "unknown"
	}
}

func describeNodeExitIPReason(stage string, reason string, upstreamStatus int) (string, string) {
	switch reason {
	case "dns_resolution":
		return reason, "目标节点无法解析所需域名，请检查节点 DNS 和网络设置"
	case "tls_certificate":
		return reason, "目标节点连接出口 IP 服务时 TLS 证书校验失败，请检查系统时间和证书链"
	case "tls_handshake":
		return reason, "目标节点连接出口 IP 服务时 TLS 握手失败，可能存在协议干扰或 TLS 不兼容"
	case "connection_refused":
		return reason, "目标节点的连接被远端拒绝"
	case "network_unreachable":
		return reason, "目标节点返回网络不可达或无可用路由"
	case "connection_reset":
		return reason, "目标节点的连接被远端重置"
	case "connection_closed":
		return reason, "目标节点的连接被意外关闭"
	case "outbound_authentication":
		return reason, "目标节点认证失败，请检查节点凭据和协议参数"
	case "protocol_handshake":
		return reason, "目标节点协议握手失败，请检查节点协议参数"
	case "timeout":
		return reason, "目标节点出口 IP 检测超时"
	case "upstream_http_status":
		if upstreamStatus >= 100 && upstreamStatus <= 599 {
			return reason, fmt.Sprintf("出口 IP 服务通过该节点返回 HTTP %d", upstreamStatus)
		}
		return reason, "出口 IP 服务通过该节点返回异常 HTTP 状态"
	case "read_response":
		return reason, "读取出口 IP 服务响应失败"
	case "response_too_large":
		return reason, "出口 IP 服务响应超过大小限制"
	case "invalid_response":
		return reason, "出口 IP 服务响应格式无效，未找到有效 IP"
	case "address_family":
		return reason, "出口 IP 服务返回的 IPv4/IPv6 地址族与节点不一致"
	case "connection_failed":
		return reason, "目标节点无法连接出口 IP 服务，核心未识别出更具体的网络错误"
	}
	switch stage {
	case "http_status":
		return "upstream_http_status", "出口 IP 服务通过该节点返回异常 HTTP 状态"
	case "read_response", "response_too_large", "invalid_response", "address_family":
		return describeNodeExitIPReason(stage, stage, upstreamStatus)
	case "timeout":
		return "timeout", "目标节点出口 IP 检测超时"
	default:
		return "connection_failed", "sing-box 无法通过目标节点访问出口 IP 服务，当前核心未返回具体原因"
	}
}

func describeCoreAPITransportFailure(err error) error {
	if isNodeExitIPTimeout(err) {
		return newNodeExitIPFailure("core_api", "timeout", "连接 sing-box Clash API 超时，请确认核心状态和 API 端口", 0, 0, err)
	}
	message := "无法连接 sing-box Clash API，请确认核心正在运行且 API 端口配置正确"
	reason := "core_unavailable"
	if strings.Contains(strings.ToLower(err.Error()), "connection refused") {
		reason = "connection_refused"
		message = "sing-box Clash API 拒绝连接，请确认核心正在运行且 API 端口一致"
	}
	return newNodeExitIPFailure("core_api", reason, message, 0, 0, err)
}

func isNodeExitIPTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkError net.Error
	return errors.As(err, &networkError) && networkError.Timeout()
}

func (svc *NodeService) nodeHTTPClient() *http.Client {
	if svc.httpClient != nil {
		return svc.httpClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}
