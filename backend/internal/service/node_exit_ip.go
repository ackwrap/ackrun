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

	"github.com/ackwrap/ackwrap/internal/geoquery"
	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/traceroute"
)

const (
	nodeExitIPMaxResponse          = 64 << 10
	nodeExitIPProxyListMaxResponse = 4 << 20
)

var ErrNodeExitIPInvalid = errors.New("invalid node exit IP request")

func (svc *NodeService) ExitIP(ctx context.Context, uid string, geoProvider string) (*model.NodeExitIPResponse, error) {
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
		logging.Error("node.exit_ip", "exit IP check failed uid=%s error=%v", node.UID, err)
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
	if secret != "" {
		request.Header.Set("Authorization", "Bearer "+secret)
	}
	response, err := svc.nodeHTTPClient().Do(request)
	if err != nil {
		return "", errors.New("无法连接 sing-box，请确认核心正在运行")
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
		return nil, errors.New("无法连接 sing-box，请确认核心正在运行")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		var payload struct {
			Message string `json:"message"`
			Stage   string `json:"stage"`
		}
		body, _ := io.ReadAll(io.LimitReader(response.Body, nodeExitIPMaxResponse+1))
		if len(body) <= nodeExitIPMaxResponse {
			_ = json.Unmarshal(body, &payload)
		}
		switch response.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, errors.New("sing-box Clash API 鉴权失败")
		case http.StatusNotFound:
			return nil, errors.New("当前核心不支持出口 IP 检测接口，或目标节点未载入活动配置，请更新核心并重新生成配置")
		case http.StatusBadGateway, http.StatusServiceUnavailable:
			return nil, describeCoreExitIPFailure(payload.Stage, payload.Message)
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

func describeCoreExitIPFailure(stage string, message string) error {
	switch stage {
	case "outbound_connect":
		return errors.New("目标节点无法连接出口 IP 服务，可能是节点不可用、网络被阻断或 TLS 握手失败")
	case "http_status":
		if message != "" {
			return fmt.Errorf("出口 IP 服务返回异常状态: %s", message)
		}
		return errors.New("出口 IP 服务返回异常 HTTP 状态")
	case "read_response":
		return errors.New("读取出口 IP 服务响应失败")
	case "response_too_large":
		return errors.New("出口 IP 服务响应超过大小限制")
	case "invalid_response":
		return errors.New("出口 IP 服务响应格式无效，未找到有效 IP")
	case "address_family":
		return errors.New("出口 IP 服务返回的 IPv4/IPv6 地址族与节点不一致")
	default:
		return errors.New("sing-box 无法通过目标节点访问出口 IP 服务")
	}
}

func (svc *NodeService) nodeHTTPClient() *http.Client {
	if svc.httpClient != nil {
		return svc.httpClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}
