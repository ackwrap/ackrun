package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

var outboundTagUnsafePattern = regexp.MustCompile(`[^A-Za-z0-9_.\-\p{Han}]+`)

var ErrInvalidConfigFileName = errors.New("配置文件名无效")

const (
	defaultRuleSetHTTPClientTag = "ackwrap-rule-set-direct"
	defaultTUNIPv4Address       = "172.254.0.1/30"
	defaultTUNIPv6Address       = "fdfe:dcba:9876::1/126"
	defaultAutoRedirectMark     = 0x2024
	legacyDefaultTUNIPv4Address = "172.19.0.1/30"
)

// ConfigGeneratorService 配置生成服务
type ConfigGeneratorService struct {
	store    *store.Store
	paths    *paths.Paths
	singbox  *SingboxService
	configMu sync.Mutex
}

// NewConfigGeneratorService 创建配置生成服务
func NewConfigGeneratorService(store *store.Store, paths *paths.Paths, singbox ...*SingboxService) *ConfigGeneratorService {
	var sb *SingboxService
	if len(singbox) > 0 {
		sb = singbox[0]
	}
	return &ConfigGeneratorService{
		store:   store,
		paths:   paths,
		singbox: sb,
	}
}

// Generate 生成完整配置
func (s *ConfigGeneratorService) Generate(req *model.ConfigGenerateRequest) (*model.ConfigGenerateResponse, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	result, err := s.generateLocked(req)
	if err != nil {
		return nil, err
	}
	if result.Valid {
		if err := s.store.SetConfigGenerateRequest(req); err != nil {
			return nil, fmt.Errorf("保存配置生成参数失败: %w", err)
		}
		logSettings, err := s.store.GetLogSettings()
		if err != nil {
			return nil, fmt.Errorf("读取日志设置失败: %w", err)
		}
		if err := s.store.SetLogSettings(&model.LogSettings{Level: req.LogLevel, Timestamp: logSettings.Timestamp}); err != nil {
			return nil, fmt.Errorf("保存日志设置失败: %w", err)
		}
	}
	return result, nil
}

// GenerateCurrent 使用最近一次校验通过的参数生成配置。
func (s *ConfigGeneratorService) GenerateCurrent() (*model.ConfigGenerateResponse, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return s.generateCurrentLocked()
}

// GetGenerateRequest 返回最近一次校验通过的生成参数。
func (s *ConfigGeneratorService) GetGenerateRequest() (*model.ConfigGenerateRequest, error) {
	return s.previewRequest("")
}

// ReconcileCurrent 在同一临界区内生成并应用配置，避免临时文件被并发请求替换。
func (s *ConfigGeneratorService) ReconcileCurrent() (*model.ConfigGenerateResponse, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	result, err := s.generateCurrentLocked()
	if err != nil || !result.Valid {
		return result, err
	}
	if err := s.applyLocked("", true); err != nil {
		return result, err
	}
	return result, nil
}

func (s *ConfigGeneratorService) generateCurrentLocked() (*model.ConfigGenerateResponse, error) {
	req, err := s.store.GetConfigGenerateRequest()
	if err != nil {
		return nil, fmt.Errorf("读取配置生成参数失败: %w", err)
	}
	if req == nil {
		req = &model.ConfigGenerateRequest{
			DefaultOutbound: "direct",
			InboundListen:   "0.0.0.0",
			InboundPort:     model.DefaultMixedInboundPort,
			TUNIPv4Address:  defaultTUNIPv4Address,
			TUNIPv6Address:  defaultTUNIPv6Address,
			LogLevel:        s.store.GetLogLevel(),
		}
	}
	return s.generateLocked(req)
}

func (s *ConfigGeneratorService) generateLocked(req *model.ConfigGenerateRequest) (*model.ConfigGenerateResponse, error) {
	return s.generateLockedTo(req, filepath.Join(s.paths.DataDir, "config.tmp.json"))
}

func (s *ConfigGeneratorService) generateLockedTo(req *model.ConfigGenerateRequest, tmpPath string) (*model.ConfigGenerateResponse, error) {
	req.LogLevel = strings.ToLower(strings.TrimSpace(req.LogLevel))
	if req.LogLevel == "" {
		req.LogLevel = s.store.GetLogLevel()
	}
	var err error
	req.TUNIPv4Address, req.TUNIPv6Address, err = normalizeTUNAddresses(req.TUNIPv4Address, req.TUNIPv6Address)
	if err != nil {
		return nil, err
	}

	logging.Info("config_generator.generate", "开始生成配置，默认出站: %s", req.DefaultOutbound)

	// 1. 生成 inbounds
	inbounds, err := s.generateInbounds(req.InboundListen, req.InboundPort, req.TUNIPv4Address, req.TUNIPv6Address)
	if err != nil {
		return nil, fmt.Errorf("生成 inbounds 失败: %w", err)
	}

	// 2. 生成 outbounds / endpoints
	outbounds, endpoints, err := s.generateOutbounds()
	if err != nil {
		return nil, fmt.Errorf("生成 outbounds 失败: %w", err)
	}

	// 3. 生成 route
	route, err := s.generateRoute(req.DefaultOutbound)
	if err != nil {
		return nil, fmt.Errorf("生成 route 失败: %w", err)
	}

	logLevel := req.LogLevel

	// 4. 获取实验性功能设置
	expSettings, _ := s.store.GetExperimentalSettings()
	if expSettings == nil {
		expSettings = &model.ExperimentalSettingsResponse{
			ClashAPIEnabled:  true,
			ClashAPIPort:     "9090",
			CacheFileEnabled: true,
		}
	}

	// 5. 合并完整配置
	config := map[string]interface{}{
		"log": map[string]interface{}{
			"level":     logLevel,
			"timestamp": s.store.GetLogTimestamp(),
		},
		"http_clients": []map[string]interface{}{
			{"tag": defaultRuleSetHTTPClientTag},
		},
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}
	if len(endpoints) > 0 {
		config["endpoints"] = endpoints
	}

	dnsGlobalSettings, _ := s.effectiveDNSGlobalSettings()
	if dnsGlobalSettings != nil && dnsGlobalSettings.Enabled {
		if dns := s.generateDNSFromDatabase(); len(dns) > 0 {
			config["dns"] = dns
		}
	}

	// 6. 添加实验性功能配置
	experimental := map[string]interface{}{}

	// Clash API - 必须支持，强制启用
	port := expSettings.ClashAPIPort
	if port == "" {
		port = "9090"
	}
	// 使用 proxy.mode 作为 Clash API 的默认模式，与控制面板保持一致
	proxyMode := s.store.GetProxyMode()
	clashAPI := map[string]interface{}{
		"external_controller": fmt.Sprintf("127.0.0.1:%s", port),
		"default_mode":        proxyMode,
		// 安全设置：禁止外部 CORS 访问（所有请求通过 Ackwrap 后端代理）
		"access_control_allow_origin":          []string{},
		"access_control_allow_private_network": false,
	}
	if expSettings.ClashAPISecret != "" {
		clashAPI["secret"] = expSettings.ClashAPISecret
	}
	if expSettings.ClashAPIExternalUI != "" {
		clashAPI["external_ui"] = expSettings.ClashAPIExternalUI
	}
	if expSettings.ClashAPIExternalUIDownloadURL != "" {
		clashAPI["external_ui_download_url"] = expSettings.ClashAPIExternalUIDownloadURL
	}
	experimental["clash_api"] = clashAPI
	logging.Info("config_generator.experimental", "Clash API 已强制启用: 127.0.0.1:%s, 模式: %s", port, proxyMode)

	// 缓存文件（全局，独立于 clash_api）
	if expSettings.CacheFileEnabled {
		cacheFile := map[string]interface{}{
			"enabled":      true,
			"path":         "cache.db",
			"cache_id":     "default",
			"store_fakeip": expSettings.CacheFileStoreFakeIP,
			"store_dns":    expSettings.CacheFileStoreDNS,
		}
		experimental["cache_file"] = cacheFile
		logging.Info("config_generator.experimental", "启用缓存文件: store_fakeip=%t, store_dns=%t", expSettings.CacheFileStoreFakeIP, expSettings.CacheFileStoreDNS)
	}

	config["experimental"] = experimental

	// 7. 添加 NTP 配置
	ntpSettings, _ := s.store.GetNTPSettings()
	if ntpSettings == nil {
		ntpSettings = &model.NTPSettingsResponse{
			Enabled: true, Server: "time.apple.com", ServerPort: 123,
			Interval: "30m", Detour: "direct",
		}
	}
	if ntpSettings.Enabled {
		ntpConfig := map[string]interface{}{
			"enabled":         true,
			"server":          ntpSettings.Server,
			"server_port":     ntpSettings.ServerPort,
			"interval":        ntpSettings.Interval,
			"write_to_system": false,
		}
		if ntpSettings.Detour != "" && ntpSettings.Detour != "direct" {
			ntpConfig["detour"] = ntpSettings.Detour
		}
		config["ntp"] = ntpConfig
		logging.Info("config_generator.ntp", "NTP 已启用: %s:%d, 间隔: %s", ntpSettings.Server, ntpSettings.ServerPort, ntpSettings.Interval)
	}

	// 5. 生成临时配置文件
	if err := s.writeConfigFile(tmpPath, config); err != nil {
		return nil, fmt.Errorf("写入临时配置文件失败: %w", err)
	}

	// 6. 验证配置
	valid, errMsg := s.validateConfig(tmpPath)

	return &model.ConfigGenerateResponse{
		Config:   redactConfigAccessTokens(config).(map[string]interface{}),
		Valid:    valid,
		Error:    redactAccessToken(errMsg),
		FilePath: tmpPath,
	}, nil
}

func redactConfigAccessTokens(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		redacted := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			redacted[key] = redactConfigAccessTokens(item)
		}
		return redacted
	case []interface{}:
		redacted := make([]interface{}, len(typed))
		for index, item := range typed {
			redacted[index] = redactConfigAccessTokens(item)
		}
		return redacted
	case []map[string]interface{}:
		redacted := make([]map[string]interface{}, len(typed))
		for index, item := range typed {
			redacted[index] = redactConfigAccessTokens(item).(map[string]interface{})
		}
		return redacted
	case string:
		return redactAccessToken(typed)
	default:
		return value
	}
}

// generateOutbounds 生成所有 outbounds 和 endpoints。WireGuard 在 sing-box 1.13 起是 endpoint，
// 不再是 outbound，但 endpoint tag 与 outbound 共享引用命名空间。
func (s *ConfigGeneratorService) generateOutbounds() ([]interface{}, []interface{}, error) {
	outbounds := []interface{}{}
	endpoints := []interface{}{}
	connectivitySettings, err := s.store.GetConnectivitySettings()
	if err != nil {
		return nil, nil, fmt.Errorf("读取连通性测速设置失败: %w", err)
	}

	// 1. 添加基础 direct 出站。sing-box 1.13 已移除 dns/block 特殊 outbound，DNS/拦截必须通过 route action 处理。
	directOutbound := map[string]interface{}{
		"type": "direct",
		"tag":  "direct",
	}
	outbounds = append(outbounds, directOutbound)

	// 2. 获取所有启用的代理集合
	collections, err := s.store.ListProxyCollectionsWithNodes()
	if err != nil {
		return nil, nil, err
	}

	// 3. 获取节点组匹配的节点 UID。sing-box group 不支持 Ackwrap 的筛选字段，必须在生成配置前完成匹配。
	nodeGroups, err := s.store.ListNodeGroups()
	if err != nil {
		return nil, nil, err
	}
	groupNodeUIDs := make(map[int64][]string)
	validGroupTags := make(map[string]bool)
	for _, group := range nodeGroups {
		if !group.Enabled {
			continue
		}
		if !isSupportedGroupType(group.Type) {
			return nil, nil, fmt.Errorf("节点组 %s 使用 sing-box 不支持的类型 %s", group.Name, group.Type)
		}

		var matchedNodes []model.Node
		if group.NodeUIDs != "" && group.NodeUIDs != "[]" && group.NodeUIDs != "null" {
			matchedNodes, err = s.store.PreviewNodeGroupManualMatches(group.NodeUIDs)
		} else {
			matchedNodes, err = s.store.PreviewNodeGroupMatches(group.FilterProtocols, group.FilterSubscriptions, group.FilterInclude, group.FilterExclude)
		}
		if err != nil {
			logging.Info("config_generator.outbound", "节点组 %s 匹配节点失败: %v", group.Name, err)
			continue
		}

		uids := make([]string, 0, len(matchedNodes))
		for _, node := range matchedNodes {
			uids = append(uids, node.UID)
		}
		if len(uids) == 0 {
			logging.Info("config_generator.outbound", "节点组 %s 没有匹配到可用节点，跳过生成", group.Name)
			continue
		}

		groupNodeUIDs[group.ID] = uids
	}

	// 4. 获取所有集合和节点组中使用的节点 UID
	usedNodeUIDs := make(map[string]bool)
	for _, col := range collections {
		if !col.Enabled {
			continue
		}
		if !isSupportedGroupType(col.Type) {
			return nil, nil, fmt.Errorf("策略组 %s 使用 sing-box 不支持的类型 %s，请改为 selector 或 urltest", col.Name, col.Type)
		}
		for _, uid := range col.NodeUIDs {
			usedNodeUIDs[uid] = true
		}
	}
	for _, uids := range groupNodeUIDs {
		for _, uid := range uids {
			usedNodeUIDs[uid] = true
		}
	}
	nodes, err := s.store.ListEnabledNodes()
	if err != nil {
		return nil, nil, err
	}
	// 核心出口检测 API 需要按 tag 直接取得任一启用节点。
	for _, node := range nodes {
		usedNodeUIDs[node.UID] = true
	}
	// 5. 为所有启用节点生成 outbound，供策略组和核心出口检测 API 复用。
	nodeTags := buildNodeOutboundTags(nodes)
	generatedNodeTags := make(map[string]string, len(nodeTags))

	for _, node := range nodes {
		// 只生成集合中使用的节点
		if !usedNodeUIDs[node.UID] {
			continue
		}

		if node.Type == "wireguard" {
			endpoint, err := s.generateWireGuardEndpoint(&node, nodeTags[node.UID], nil)
			if err != nil {
				logging.Info("config_generator.node", "跳过节点 %s: %v", node.Name, err)
				continue
			}
			endpoints = append(endpoints, endpoint)
			generatedNodeTags[node.UID] = nodeTags[node.UID]
			continue
		}
		nodeOutbound, err := s.generateNodeOutbound(&node, nodeTags[node.UID], nil)
		if err != nil {
			logging.Info("config_generator.outbound", "节点 %s 生成失败: %v", node.Name, err)
			continue
		}
		outbounds = append(outbounds, nodeOutbound)
		generatedNodeTags[node.UID] = nodeTags[node.UID]
	}
	// 6. 生成节点组 outbound
	for _, group := range nodeGroups {
		uids := groupNodeUIDs[group.ID]
		if len(uids) == 0 {
			continue
		}
		groupOutbounds := nodeUIDsToOutboundTags(uids, generatedNodeTags)
		if len(groupOutbounds) == 0 {
			logging.Info("config_generator.outbound", "节点组 %s 没有可用 outbound tag，跳过生成", group.Name)
			continue
		}

		outbound := map[string]interface{}{
			"tag":       group.Name,
			"type":      group.Type,
			"outbounds": groupOutbounds,
		}

		// urltest 特有字段
		if group.Type == "urltest" {
			outbound["url"] = connectivitySettings.TestURL
			outbound["interval"] = fmt.Sprintf("%ds", connectivitySettings.IntervalSeconds)
			outbound["tolerance"] = group.Tolerance
		}

		outbounds = append(outbounds, outbound)
		validGroupTags[group.Name] = true
	}

	// 7. 为每个集合生成 outbound（放在节点和节点组后面，这样集合可以引用它们）
	collectionTags := make([]string, 0)
	hasProxyCollection := false
	for _, col := range collections {
		if !col.Enabled {
			continue
		}

		effectiveCollection := *col
		effectiveCollection.TestURL = connectivitySettings.TestURL
		effectiveCollection.TestInterval = connectivitySettings.IntervalSeconds
		outbound, err := s.generateCollectionOutbound(&effectiveCollection, validGroupTags, generatedNodeTags, groupNodeUIDs)
		if err != nil {
			if col.Name != "proxy" {
				return nil, nil, fmt.Errorf("策略组 %s 生成失败: %w", col.Name, err)
			}
			logging.Info("config_generator.outbound", "proxy 策略组没有可用节点，降级为 direct: %v", err)
			outbound = map[string]interface{}{
				"tag":       "proxy",
				"type":      "selector",
				"outbounds": []string{"direct"},
			}
		}

		if col.Name == "proxy" {
			hasProxyCollection = true
		} else {
			collectionTags = append(collectionTags, col.Name)
		}
		outbounds = append(outbounds, outbound)
	}

	// 8. 规则管理中的“策略”动作固定引用 proxy。若用户没有显式创建 proxy 策略组，自动生成一个代理入口承接已启用策略组。
	if !hasProxyCollection {
		if len(collectionTags) == 0 {
			collectionTags = []string{"direct"}
		}
		outbounds = append(outbounds, map[string]interface{}{
			"tag":       "proxy",
			"type":      "selector",
			"outbounds": collectionTags,
		})
	}

	return outbounds, endpoints, nil
}

// generateCollectionOutbound 为代理集合生成 outbound
func (s *ConfigGeneratorService) generateCollectionOutbound(col *model.ProxyCollectionWithNodes, validGroupTags map[string]bool, nodeTags map[string]string, groupNodeUIDs map[int64][]string) (map[string]interface{}, error) {
	outbound := map[string]interface{}{
		"type": col.Type,
		"tag":  col.Name,
	}

	// 判断是引用节点组还是手动选节点
	if isCollectionGroupSource(col.SourceType) && len(col.ReferencedGroups) > 0 {
		// 引用节点组模式。node_uids 兼容存放 direct 这类真实内置出站 tag。
		referencedTags := collectionBuiltinOutboundTags(col)
		seen := make(map[string]bool, len(referencedTags))
		for _, tag := range referencedTags {
			seen[tag] = true
		}
		appendUnique := func(tags ...string) {
			for _, tag := range tags {
				if tag == "" || seen[tag] {
					continue
				}
				seen[tag] = true
				referencedTags = append(referencedTags, tag)
			}
		}
		for _, group := range col.ReferencedGroups {
			if !validGroupTags[group.Name] {
				logging.Info("config_generator.outbound", "策略组 %s 跳过空节点组引用: %s", col.Name, group.Name)
				continue
			}
			appendUnique(group.Name)
			if col.SourceType == proxyCollectionSourceNodeGroupsAndNodes {
				appendUnique(nodeUIDsToOutboundTags(groupNodeUIDs[group.ID], nodeTags)...)
			}
		}
		if len(referencedTags) == 0 {
			return nil, fmt.Errorf("策略组没有可用节点组引用")
		}
		outbound["outbounds"] = referencedTags
	} else {
		// 手动选节点模式（兼容旧数据）
		if len(col.NodeUIDs) == 0 {
			return nil, fmt.Errorf("策略组没有可用节点")
		}
		outboundTags := collectionBuiltinOutboundTags(col)
		outboundTags = append(outboundTags, nodeUIDsToOutboundTags(col.NodeUIDs, nodeTags)...)
		if len(outboundTags) == 0 {
			return nil, fmt.Errorf("策略组没有可用节点 outbound")
		}
		outbound["outbounds"] = outboundTags
	}

	// urltest 需要测试配置
	if col.Type == "urltest" {
		outbound["url"] = col.TestURL
		outbound["interval"] = fmt.Sprintf("%ds", col.TestInterval)

		outbound["tolerance"] = col.Tolerance
	}

	return outbound, nil
}

func builtinOutboundTags(values []string) []string {
	tags := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		switch value {
		case "direct":
			if seen[value] {
				continue
			}
			seen[value] = true
			tags = append(tags, value)
		}
	}
	return tags
}

func collectionBuiltinOutboundTags(col *model.ProxyCollectionWithNodes) []string {
	defaults := []string{}
	switch col.Name {
	case "全球直连":
		defaults = []string{"direct"}
	}

	seen := make(map[string]bool)
	tags := make([]string, 0, len(defaults)+len(col.NodeUIDs))
	for _, tag := range defaults {
		seen[tag] = true
		tags = append(tags, tag)
	}
	for _, tag := range builtinOutboundTags(col.NodeUIDs) {
		if seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

// generateNodeOutbound 为节点生成 outbound
func (s *ConfigGeneratorService) generateNodeOutbound(node *model.Node, tag string, domainResolver map[string]interface{}) (map[string]interface{}, error) {
	// 过滤 sing-box 不支持的协议
	if isUnsupportedNodeType(node.Type) {
		return nil, fmt.Errorf("unsupported protocol: %s", node.Type)
	}

	// 解析节点的 raw_json
	var nodeData map[string]interface{}
	if err := json.Unmarshal([]byte(node.RawJSON), &nodeData); err != nil {
		return nil, fmt.Errorf("解析节点 JSON 失败: %w", err)
	}

	// 使用可读且稳定的 tag，便于在生成配置和 Clash API 中识别节点。
	nodeData["tag"] = tag

	// 移除可能存在的非 sing-box 字段
	delete(nodeData, "name")
	mapMihomoUDPFlagToSingboxNetwork(nodeData)
	if err := mapTLSFingerprintFields(nodeData); err != nil {
		return nil, err
	}
	ensureRequiredOutboundTLS(nodeData, node.Type)
	if err := normalizeLegacyOutboundFields(nodeData, node.Type); err != nil {
		return nil, err
	}
	applyDomainResolverBinding(nodeData, domainResolver)

	return nodeData, nil
}

func (s *ConfigGeneratorService) generateWireGuardEndpoint(node *model.Node, tag string, domainResolver map[string]interface{}) (map[string]interface{}, error) {
	var nodeData map[string]interface{}
	if err := json.Unmarshal([]byte(node.RawJSON), &nodeData); err != nil {
		return nil, fmt.Errorf("解析 WireGuard 节点 JSON 失败: %w", err)
	}
	address := stringListValue(firstExistingValue(nodeData, "address", "local_address", "local-address"))
	if len(address) == 0 {
		return nil, fmt.Errorf("缺少 WireGuard address")
	}
	privateKey := firstStringValue(nodeData, "private_key", "private-key")
	publicKey := firstStringValue(nodeData, "peer_public_key", "public_key", "public-key")
	if privateKey == "" || publicKey == "" {
		return nil, fmt.Errorf("缺少 WireGuard private_key 或 public_key")
	}
	server := firstStringValue(nodeData, "server")
	serverPort := intValue(firstExistingValue(nodeData, "server_port", "port"))
	if server == "" || serverPort == 0 {
		return nil, fmt.Errorf("缺少 WireGuard peer address 或 port")
	}
	peer := map[string]interface{}{
		"address":     server,
		"port":        serverPort,
		"public_key":  publicKey,
		"allowed_ips": []string{"0.0.0.0/0", "::/0"},
	}
	if allowedIPs := stringListValue(firstExistingValue(nodeData, "allowed_ips", "allowed-ips")); len(allowedIPs) > 0 {
		peer["allowed_ips"] = allowedIPs
	}
	if psk := firstStringValue(nodeData, "pre_shared_key", "pre-shared-key", "preshared-key"); psk != "" {
		peer["pre_shared_key"] = psk
	}
	if reserved, ok := firstExistingValue(nodeData, "reserved").([]interface{}); ok && len(reserved) > 0 {
		peer["reserved"] = reserved
	} else if reservedInts := intListValue(firstExistingValue(nodeData, "reserved")); len(reservedInts) > 0 {
		peer["reserved"] = reservedInts
	}
	if keepalive := intValue(firstExistingValue(nodeData, "persistent_keepalive_interval", "persistent-keepalive", "persistent_keepalive")); keepalive > 0 {
		peer["persistent_keepalive_interval"] = keepalive
	}
	endpoint := map[string]interface{}{
		"type":        "wireguard",
		"tag":         tag,
		"address":     address,
		"private_key": privateKey,
		"peers":       []interface{}{peer},
	}
	if mtu := intValue(firstExistingValue(nodeData, "mtu")); mtu > 0 {
		endpoint["mtu"] = mtu
	}
	if workers := intValue(firstExistingValue(nodeData, "workers")); workers > 0 {
		endpoint["workers"] = workers
	}
	applyDomainResolverBinding(endpoint, domainResolver)
	return endpoint, nil
}

func firstExistingValue(data map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := data[key]; ok {
			return value
		}
	}
	return nil
}

func firstStringValue(data map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := configStringValue(data[key]); value != "" {
			return value
		}
	}
	return ""
}

func configStringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%g", v)
	case int:
		return fmt.Sprintf("%d", v)
	default:
		return ""
	}
}

func stringListValue(value interface{}) []string {
	switch v := value.(type) {
	case []string:
		return v
	case []interface{}:
		items := make([]string, 0, len(v))
		for _, item := range v {
			if str := configStringValue(item); str != "" {
				items = append(items, str)
			}
		}
		return items
	case string:
		parts := strings.Split(v, ",")
		items := make([]string, 0, len(parts))
		for _, part := range parts {
			if part = strings.TrimSpace(part); part != "" {
				items = append(items, part)
			}
		}
		return items
	default:
		return nil
	}
}

func intListValue(value interface{}) []int {
	switch v := value.(type) {
	case []int:
		return v
	case []interface{}:
		items := make([]int, 0, len(v))
		for _, item := range v {
			if i := intValue(item); i >= 0 {
				items = append(items, i)
			}
		}
		return items
	default:
		return nil
	}
}

func intValue(value interface{}) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		parsed, _ := strconv.Atoi(strings.TrimSpace(v))
		return parsed
	default:
		return 0
	}
}

func mapTLSFingerprintFields(nodeData map[string]interface{}) error {
	tlsMap, ok := nodeData["tls"].(map[string]interface{})
	if !ok {
		return nil
	}
	if insecure, exists := nodeData["skip-cert-verify"]; exists {
		tlsMap["insecure"] = configBoolValue(insecure)
		delete(nodeData, "skip-cert-verify")
	}
	if alpn, exists := nodeData["alpn"]; exists {
		tlsMap["alpn"] = alpn
		delete(nodeData, "alpn")
	}
	if legacyPins, exists := tlsMap["certificate_public_key_sha256"]; exists {
		if normalizedPins, ok := normalizeSHA256HexValues(legacyPins); ok {
			if err := mergeCertificateSHA256Pins(tlsMap, normalizedPins); err != nil {
				return err
			}
			delete(tlsMap, "certificate_public_key_sha256")
		}
	}
	utlsMap, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		return nil
	}
	fingerprint, _ := utlsMap["fingerprint"].(string)
	if fingerprint == "" || isSingboxUTLSFingerprint(fingerprint) {
		return nil
	}
	normalizedFingerprint, ok := normalizeSHA256HexString(fingerprint)
	if !ok {
		return fmt.Errorf("无法识别旧节点的 TLS fingerprint")
	}
	if err := mergeCertificateSHA256Pins(tlsMap, []string{normalizedFingerprint}); err != nil {
		return err
	}
	delete(utlsMap, "fingerprint")
	if len(utlsMap) == 1 && configBoolValue(utlsMap["enabled"]) {
		delete(tlsMap, "utls")
	}
	return nil
}

func ensureRequiredOutboundTLS(nodeData map[string]interface{}, outboundType string) {
	outboundType = strings.ToLower(strings.TrimSpace(outboundType))
	switch outboundType {
	case "anytls", "hysteria", "hysteria2", "naive", "shadowtls", "trojan", "tuic":
	default:
		return
	}
	tlsOptions, ok := nodeData["tls"].(map[string]interface{})
	if !ok {
		tlsOptions = map[string]interface{}{}
		nodeData["tls"] = tlsOptions
	}
	tlsOptions["enabled"] = true
}

func normalizeLegacyOutboundFields(nodeData map[string]interface{}, outboundType string) error {
	outboundType = strings.ToLower(strings.TrimSpace(outboundType))
	if cipher, exists := nodeData["cipher"]; exists {
		switch outboundType {
		case "vmess":
			if _, hasSecurity := nodeData["security"]; !hasSecurity {
				nodeData["security"] = cipher
			}
		case "shadowsocks":
			if _, hasMethod := nodeData["method"]; !hasMethod {
				nodeData["method"] = cipher
			}
		case "ssr":
			if _, hasMethod := nodeData["method"]; !hasMethod {
				nodeData["method"] = cipher
			}
		}
		delete(nodeData, "cipher")
	}
	if outboundType == "ssr" {
		nodeData["type"] = "shadowsocksr"
		moveConfigField(nodeData, "obfs-param", "obfs_param")
		moveConfigField(nodeData, "protocol-param", "protocol_param")
		delete(nodeData, "group")
		return nil
	}
	if outboundType != "vmess" {
		return nil
	}
	value, exists := nodeData["alter_id"]
	if !exists {
		return nil
	}
	delete(nodeData, "alter_id")
	switch typed := value.(type) {
	case nil:
		return nil
	case float64:
		if typed == 0 {
			return nil
		}
	case int:
		if typed == 0 {
			return nil
		}
	case int64:
		if typed == 0 {
			return nil
		}
	case string:
		if strings.TrimSpace(typed) == "" || strings.TrimSpace(typed) == "0" {
			return nil
		}
	}
	return fmt.Errorf("旧版 VMess alter_id 节点不受当前 sing-box 版本支持")
}

func moveConfigField(data map[string]interface{}, source, target string) {
	if value, exists := data[source]; exists {
		if _, targetExists := data[target]; !targetExists {
			data[target] = value
		}
		delete(data, source)
	}
}

func isSingboxUTLSFingerprint(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized", "chrome_psk", "chrome_psk_shuffle", "chrome_padding_psk_shuffle", "chrome_pq", "chrome_pq_psk":
		return true
	default:
		return false
	}
}

func normalizeSHA256HexString(value string) (string, bool) {
	value = strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(value)))
	if len(value) != 64 {
		return "", false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return "", false
		}
	}
	return value, true
}

func normalizeSHA256HexValues(value interface{}) ([]string, bool) {
	var values []string
	switch typed := value.(type) {
	case string:
		values = []string{typed}
	case []string:
		values = typed
	case []interface{}:
		values = make([]string, 0, len(typed))
		for _, item := range typed {
			stringValue, ok := item.(string)
			if !ok {
				return nil, false
			}
			values = append(values, stringValue)
		}
	default:
		return nil, false
	}
	if len(values) == 0 {
		return nil, false
	}
	normalized := make([]string, 0, len(values))
	for _, item := range values {
		normalizedItem, ok := normalizeSHA256HexString(item)
		if !ok {
			return nil, false
		}
		normalized = append(normalized, normalizedItem)
	}
	return normalized, true
}

func mergeCertificateSHA256Pins(tlsMap map[string]interface{}, migratedPins []string) error {
	combined := make([]string, 0, len(migratedPins))
	if existingValue, exists := tlsMap["certificate_sha256"]; exists {
		existingPins, ok := normalizeSHA256HexValues(existingValue)
		if !ok {
			return fmt.Errorf("无法识别旧节点的 certificate_sha256")
		}
		combined = append(combined, existingPins...)
	}
	seen := make(map[string]struct{}, len(combined)+len(migratedPins))
	for _, pin := range combined {
		seen[pin] = struct{}{}
	}
	for _, pin := range migratedPins {
		if _, exists := seen[pin]; exists {
			continue
		}
		seen[pin] = struct{}{}
		combined = append(combined, pin)
	}
	tlsMap["certificate_sha256"] = combined
	return nil
}

func mapMihomoUDPFlagToSingboxNetwork(nodeData map[string]interface{}) {
	// Mihomo's top-level udp flag is an adapter capability switch: false means the
	// proxy must not handle UDP. sing-box has no equivalent udp outbound field;
	// the closest schema mapping is network="tcp" for UDP-disabled nodes. true is
	// sing-box's default TCP+UDP behavior, so the flag is intentionally omitted.
	if udp, ok := nodeData["udp"]; ok {
		if !configBoolValue(udp) {
			nodeData["network"] = "tcp"
		}
		delete(nodeData, "udp")
	}
}

func dnsStrategyBindings(dnsRules []model.DNSRule, serverTags map[string]bool) map[string]model.DNSRule {
	bindings := make(map[string]model.DNSRule)
	for _, rule := range dnsRules {
		if !rule.Enabled || rule.Server == "" {
			continue
		}
		if !serverTags[rule.Server] {
			logging.Info("config_generator.dns", "跳过引用无效 DNS server 的策略绑定: %s", rule.Server)
			continue
		}
		conditions := decodeDNSRuleConditions(rule.ConditionsJSON)
		if !isDNSStrategyBindingConditions(conditions) {
			continue
		}
		for _, outbound := range dnsRuleOutboundConditions(conditions) {
			if outbound == "" || outbound == "block" {
				continue
			}
			if _, exists := bindings[outbound]; exists {
				logging.Info("config_generator.dns", "策略 %s 绑定了多个 DNS server，保留第一个", outbound)
				continue
			}
			bindings[outbound] = rule
		}
	}
	return bindings
}

func enabledDNSServerTags(servers []model.DNSServer, fakeIPEnabled bool) map[string]bool {
	tags := make(map[string]bool)
	for _, server := range servers {
		if server.Enabled && server.Tag != "" && (fakeIPEnabled || server.ServerType != "fakeip") {
			tags[server.Tag] = true
		}
	}
	if fakeIPEnabled {
		tags["fakeip"] = true
	}
	return tags
}

func (s *ConfigGeneratorService) effectiveDNSGlobalSettings() (*model.DNSGlobalSettings, error) {
	settings, err := s.store.GetDNSGlobalSettings()
	if err != nil {
		return nil, err
	}
	applyTUNManagedFakeIP(settings, s.store.GetInboundMode())
	return settings, nil
}

func applyDomainResolverBinding(outbound map[string]interface{}, resolver map[string]interface{}) {
	if len(resolver) == 0 {
		return
	}
	outbound["domain_resolver"] = resolver
}

func configBoolValue(value interface{}) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "1" || strings.EqualFold(v, "true")
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func buildNodeOutboundTags(nodes []model.Node) map[string]string {
	result := make(map[string]string, len(nodes))
	for _, node := range nodes {
		base := sanitizeOutboundTag(node.Name)
		if base == "" {
			base = sanitizeOutboundTag(node.Type)
		}
		if base == "" {
			base = "node"
		}
		result[node.UID] = fmt.Sprintf("%s-%s", base, node.UID)
	}
	return result
}

func sanitizeOutboundTag(value string) string {
	value = strings.TrimSpace(value)
	value = outboundTagUnsafePattern.ReplaceAllString(value, "-")
	value = strings.Trim(value, "-_. ")
	if len([]rune(value)) > 48 {
		runes := []rune(value)
		value = string(runes[:48])
	}
	return value
}

func nodeUIDsToOutboundTags(uids []string, nodeTags map[string]string) []string {
	tags := make([]string, 0, len(uids))
	seen := make(map[string]bool)
	for _, uid := range uids {
		tag := nodeTags[uid]
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}

// generateRoute 生成路由配置
// 根据代理模式（全局/规则/直连）决定路由策略
func (s *ConfigGeneratorService) generateRoute(defaultOutbound string) (map[string]interface{}, error) {
	route := map[string]interface{}{
		"rules":               []map[string]interface{}{},
		"rule_set":            []map[string]interface{}{},
		"default_http_client": defaultRuleSetHTTPClientTag,
	}
	inboundMode := s.store.GetInboundMode()
	if inboundMode == "tun" || inboundMode == "tun_mixed" {
		route["auto_detect_interface"] = true
	}

	// 根据代理模式决定是否加载规则
	var routeRules []map[string]interface{}
	var ruleSets []map[string]interface{}
	ruleSetTags := make(map[string]bool)

	// 内核级绕过必须先于 sniff；TCP 预匹配遇到 sniff 后不会继续匹配后续规则。
	bypassRules, err := s.defaultBypassRules()
	if err != nil {
		return nil, err
	}
	routeRules = append(routeRules, bypassRules...)
	route["find_process"] = true

	// sing-box 1.13 已移除 inbound sniff 字段，嗅探和 DNS 劫持必须使用 rule action。
	routeRules = append(routeRules, map[string]interface{}{
		"action": "sniff",
	})

	// DNS 查询交给 DNS rule action 处理，放在所有路由规则之前。
	if dnsSettings, _ := s.store.GetDNSSettings(); dnsSettings != nil && dnsSettings.Enabled {
		routeRules = append(routeRules, map[string]interface{}{
			"protocol": "dns",
			"action":   "hijack-dns",
		})
		if resolver := s.defaultDomainResolver(); len(resolver) > 0 {
			route["default_domain_resolver"] = resolver
		}
	}

	// 获取代理模式
	proxyMode := s.store.GetProxyMode()
	apiToken := strings.TrimSpace(os.Getenv("ACKWRAP_API_TOKEN"))
	apiBaseURL := internalAPIBaseURL()

	if proxyMode == "rule" {
		// 规则模式：加载所有启用的规则
		rules, err := s.store.ListRouteRules()
		if err != nil {
			return nil, err
		}
		subscriptions, err := s.store.ListRouteRuleSubscriptions()
		if err != nil {
			return nil, err
		}
		for _, sub := range subscriptions {
			if sub.Enabled {
				ruleSetTags[sub.Tag] = true
			}
		}

		ruleOutboundOverrides := s.routeRuleOutboundOverrides()

		for _, rule := range rules {
			if !rule.Enabled {
				continue
			}
			if outbound, ok := ruleOutboundOverrides[rule.ID]; ok && rule.Outbound != "block" {
				rule.Outbound = outbound
			}

			ruleMaps, err := s.generateRouteRules(&rule)
			if err != nil {
				logging.Info("config_generator.route", "规则 %s 生成失败: %v", rule.Name, err)
				continue
			}
			for _, ruleMap := range ruleMaps {
				routeRules = append(routeRules, ruleMap)
			}
			if rule.RuleType == "geoip" || rule.RuleType == "geosite" {
				ruleSets = appendGeneratedGeoRuleSets(ruleSets, ruleSetTags, rule.RuleType, rule.Values, apiBaseURL, apiToken)
			}
			if rule.RuleType == "mixed" {
				ruleSets = addMixedGeneratedRuleSets(ruleSets, ruleSetTags, rule.Values, apiBaseURL, apiToken)
			}
		}

		for _, sub := range subscriptions {
			if !sub.Enabled {
				continue
			}

			format := sub.Format
			if sub.Format == "clash" {
				format = "source"
			}

			ruleSet := map[string]interface{}{
				"type":   "remote",
				"tag":    sub.Tag,
				"format": format,
				"url":    routeRuleSubscriptionContentURL(apiBaseURL, sub.ID, apiToken),
			}
			ruleSets = append(ruleSets, ruleSet)
		}
	}
	// 全局模式和直连模式不加载规则
	// DNS 的 GeoSite 匹配也依赖 route.rule_set；即使不是规则模式也要注入。
	if dnsSettings, _ := s.effectiveDNSGlobalSettings(); dnsSettings != nil && dnsSettings.Enabled {
		dnsRules, err := s.store.ListDNSRules()
		if err != nil {
			return nil, err
		}
		for _, rule := range dnsRules {
			if !rule.Enabled {
				continue
			}
			conditions := decodeDNSRuleConditions(rule.ConditionsJSON)
			geositeValues := dnsRuleStringConditions(conditions, "geosite")
			ruleSets = appendGeneratedGeoRuleSets(ruleSets, ruleSetTags, "geosite", geositeValues, apiBaseURL, apiToken)
		}
	}

	route["rules"] = routeRules
	route["rule_set"] = ruleSets

	// 设置默认出站（final）
	var finalOutbound string
	switch proxyMode {
	case "global":
		// 全局模式：所有流量走代理
		finalOutbound = "proxy"
	case "direct":
		// 直连模式：所有流量直连
		finalOutbound = "direct"
	case "rule":
		// 规则模式：未匹配规则的流量走 defaultOutbound 或 direct
		if defaultOutbound != "" {
			if defaultOutbound == "block" || defaultOutbound == "reject" {
				return nil, fmt.Errorf("默认出站不能是 %s：sing-box route.final 必须引用真实 outbound tag", defaultOutbound)
			}
			finalOutbound = defaultOutbound
		} else {
			finalOutbound = "direct"
		}
	default:
		finalOutbound = "direct"
	}

	route["final"] = finalOutbound
	logging.Info("config_generator.route", "代理模式: %s, final outbound: %s, 规则数: %d", proxyMode, finalOutbound, len(routeRules))

	return route, nil
}

func (s *ConfigGeneratorService) defaultBypassRules() ([]map[string]interface{}, error) {
	currentExecutable, _ := os.Executable()
	coreBinaryPath := ""
	if s.paths != nil {
		coreBinaryPath = s.paths.BinaryPath
	}
	processNames := defaultBypassProcessNames(currentExecutable, coreBinaryPath)

	rules := []map[string]interface{}{
		{
			"process_name": processNames,
			"inbound":      []string{"tun-in"},
			"action":       "bypass",
			"outbound":     "direct",
		},
	}

	nodes, err := s.store.ListEnabledNodes()
	if err != nil {
		return nil, fmt.Errorf("加载节点服务器直连白名单失败: %w", err)
	}
	domains, ipCIDRs := nodeServerBypassTargets(nodes)
	if len(domains) > 0 {
		rules = append(rules, map[string]interface{}{
			"domain":   domains,
			"action":   "bypass",
			"outbound": "direct",
		})
	}
	if len(ipCIDRs) > 0 {
		rules = append(rules, map[string]interface{}{
			"ip_cidr":  ipCIDRs,
			"action":   "bypass",
			"outbound": "direct",
		})
	}
	return rules, nil
}

func defaultBypassProcessNames(currentExecutable, coreBinaryPath string) []string {
	processNames := make([]string, 0, 4)
	seenProcesses := make(map[string]bool)
	appendProcess := func(name string) {
		name = strings.TrimSpace(name)
		if name != "" && !seenProcesses[name] {
			seenProcesses[name] = true
			processNames = append(processNames, name)
		}
	}
	appendProcess("ackwrap")
	appendProcess("ackwrap.exe")
	if strings.TrimSpace(currentExecutable) != "" {
		appendProcess(filepath.Base(currentExecutable))
	}
	if strings.TrimSpace(coreBinaryPath) != "" {
		appendProcess(filepath.Base(coreBinaryPath))
	}
	return processNames
}

func nodeServerBypassTargets(nodes []model.Node) ([]string, []string) {
	domains := make([]string, 0)
	ipCIDRs := make([]string, 0)
	seenDomains := make(map[string]bool)
	seenIPs := make(map[string]bool)
	for _, node := range nodes {
		server := strings.TrimSpace(strings.Trim(node.Server, "[]"))
		if server == "" {
			continue
		}
		if address := net.ParseIP(server); address != nil {
			cidr := server + "/128"
			if address.To4() != nil {
				cidr = address.String() + "/32"
			} else {
				cidr = address.String() + "/128"
			}
			if !seenIPs[cidr] {
				seenIPs[cidr] = true
				ipCIDRs = append(ipCIDRs, cidr)
			}
			continue
		}
		domain := strings.ToLower(strings.TrimSuffix(server, "."))
		if domain != "" && !seenDomains[domain] {
			seenDomains[domain] = true
			domains = append(domains, domain)
		}
	}
	return domains, ipCIDRs
}

// generateDNSFromDatabase 从数据库读取 DNS servers 和 rules 生成完整 DNS 配置
func (s *ConfigGeneratorService) generateDNSFromDatabase() map[string]interface{} {
	// 1. 获取全局设置
	globalSettings, _ := s.effectiveDNSGlobalSettings()
	if globalSettings == nil {
		globalSettings = &model.DNSGlobalSettings{
			Enabled:          true,
			Final:            "dns_proxy",
			Strategy:         "prefer_ipv4",
			DisableCache:     false,
			DisableExpire:    false,
			IndependentCache: false,
			ReverseMapping:   false,
			CacheCapacity:    4096,
			ClientSubnet:     "",
			FakeIPEnabled:    false,
			FakeIPInet4Range: "198.19.0.0/16",
			FakeIPInet6Range: "fdfe:dcba:9876::/48",
		}
	}

	// 2. 读取所有启用的 DNS servers
	dnsServers, _ := s.store.ListDNSServers()
	servers := []map[string]interface{}{}
	serverTags := enabledDNSServerTags(dnsServers, globalSettings.FakeIPEnabled)
	bootstrapTag := selectDNSBootstrapTag(dnsServers)
	generatedBootstrapTag := ""
	if bootstrapTag == "" {
		generatedBootstrapTag = uniqueDNSServerTag("ackwrap-bootstrap-local", serverTags)
		bootstrapTag = generatedBootstrapTag
		serverTags[bootstrapTag] = true
	}
	hasFakeIPServer := false
	for _, srv := range dnsServers {
		if !srv.Enabled || (!globalSettings.FakeIPEnabled && srv.ServerType == "fakeip") {
			continue
		}
		if srv.Tag == "fakeip" {
			hasFakeIPServer = true
		}
		server := map[string]interface{}{
			"tag":  srv.Tag,
			"type": srv.ServerType,
		}
		serverIsDomain := applyDNSServerAddress(server, srv.ServerType, srv.Address)
		if serverIsDomain {
			resolver := srv.AddressResolver
			if !serverTags[resolver] || resolver == srv.Tag {
				if resolver != "" {
					logging.Info("config_generator.dns", "DNS server %s 跳过无效 address_resolver: %s", srv.Tag, resolver)
				}
				resolver = bootstrapTag
			}
			if resolver != "" && resolver != srv.Tag {
				server["domain_resolver"] = resolver
			}
		}
		if srv.AddressStrategy != "" && serverIsDomain {
			server["domain_strategy"] = srv.AddressStrategy
		}
		if srv.Strategy != "" {
			server["strategy"] = srv.Strategy
		}
		if srv.Detour != "" && srv.Detour != "direct" && srv.Detour != "block" && srv.Detour != "reject" {
			server["detour"] = srv.Detour
		}
		if srv.ClientSubnet != "" {
			server["client_subnet"] = srv.ClientSubnet
		}
		// 合并 options_json 中的额外选项
		if srv.OptionsJSON != "" && srv.OptionsJSON != "{}" {
			var options map[string]interface{}
			if json.Unmarshal([]byte(srv.OptionsJSON), &options) == nil {
				for k, v := range options {
					server[k] = v
				}
			}
		}
		delete(server, "address_resolver")
		delete(server, "address_strategy")
		servers = append(servers, server)
	}
	if generatedBootstrapTag != "" {
		servers = append(servers, map[string]interface{}{
			"tag":  generatedBootstrapTag,
			"type": "local",
		})
	}
	if globalSettings.FakeIPEnabled && !hasFakeIPServer {
		fakeIP := map[string]interface{}{
			"tag":  "fakeip",
			"type": "fakeip",
		}
		if globalSettings.FakeIPInet4Range != "" {
			fakeIP["inet4_range"] = globalSettings.FakeIPInet4Range
		}
		if globalSettings.FakeIPInet6Range != "" {
			fakeIP["inet6_range"] = globalSettings.FakeIPInet6Range
		}
		servers = append(servers, fakeIP)
	}
	if len(servers) == 0 {
		logging.Info("config_generator.dns", "没有启用的 DNS server，跳过 DNS 配置")
		return nil
	}

	// 3. 读取所有启用的 DNS rules
	dnsRules, _ := s.store.ListDNSRules()
	rules := []map[string]interface{}{}
	for _, rule := range dnsRules {
		if !rule.Enabled {
			continue
		}
		if !serverTags[rule.Server] {
			logging.Info("config_generator.dns", "跳过引用无效 DNS server 的规则: %s", rule.Server)
			continue
		}
		conditions := normalizeDNSRuleConditions(decodeDNSRuleConditions(rule.ConditionsJSON))
		if _, hasOutbound := conditions["outbound"]; hasOutbound {
			// 策略绑定会在下方按关联路由规则展开为 DNS 匹配规则。
			delete(conditions, "outbound")
		}
		if len(conditions) == 0 {
			continue
		}
		rules = append(rules, dnsRuleMap(conditions, &rule))
	}
	if s.store.GetProxyMode() == "rule" {
		rules = append(rules, s.generateDNSStrategyRules(dnsRules, serverTags)...)
	}
	if globalSettings.FakeIPEnabled {
		rules = append(rules, map[string]interface{}{
			"query_type": []string{"A", "AAAA"},
			"server":     "fakeip",
		})
	}

	// 4. 组装完整 DNS 配置
	finalServer := globalSettings.Final
	if !serverTags[finalServer] {
		finalServer = servers[0]["tag"].(string)
		logging.Info("config_generator.dns", "DNS final 引用无效，回退到: %s", finalServer)
	}
	dns := map[string]interface{}{
		"servers":           servers,
		"rules":             rules,
		"final":             finalServer,
		"strategy":          globalSettings.Strategy,
		"disable_cache":     globalSettings.DisableCache,
		"disable_expire":    globalSettings.DisableExpire,
		"independent_cache": globalSettings.IndependentCache,
		"reverse_mapping":   globalSettings.ReverseMapping,
	}
	if globalSettings.CacheCapacity > 0 {
		dns["cache_capacity"] = globalSettings.CacheCapacity
	}
	if globalSettings.ClientSubnet != "" {
		dns["client_subnet"] = globalSettings.ClientSubnet
	}

	logging.Info("config_generator.dns", "生成 DNS 配置: %d servers, %d rules", len(servers), len(rules))
	return dns
}

func dnsRuleMap(conditions map[string]interface{}, settings *model.DNSRule) map[string]interface{} {
	ruleMap := map[string]interface{}{"server": settings.Server}
	for key, value := range conditions {
		ruleMap[key] = value
	}
	if settings.DisableCache {
		ruleMap["disable_cache"] = true
	}
	if settings.RewriteTTL > 0 {
		ruleMap["rewrite_ttl"] = settings.RewriteTTL
	}
	if settings.ClientSubnet != "" {
		ruleMap["client_subnet"] = settings.ClientSubnet
	}
	return ruleMap
}

func (s *ConfigGeneratorService) generateDNSStrategyRules(dnsRules []model.DNSRule, serverTags map[string]bool) []map[string]interface{} {
	bindings := dnsStrategyBindings(dnsRules, serverTags)
	if len(bindings) == 0 {
		return nil
	}
	routeRules, err := s.store.ListRouteRules()
	if err != nil {
		logging.Info("config_generator.dns", "读取策略关联路由规则失败: %v", err)
		return nil
	}
	overrides := s.routeRuleOutboundOverrides()
	generated := make([]map[string]interface{}, 0)
	for _, routeRule := range routeRules {
		if !routeRule.Enabled || routeRule.Outbound == "block" {
			continue
		}
		outbound := routeRule.Outbound
		if override, exists := overrides[routeRule.ID]; exists {
			outbound = override
		}
		binding, exists := bindings[outbound]
		if !exists {
			continue
		}
		for _, conditions := range dnsConditionsFromRouteRule(&routeRule) {
			generated = append(generated, dnsRuleMap(conditions, &binding))
		}
	}
	logging.Info("config_generator.dns", "按策略关联路由规则生成 %d 条 DNS 规则", len(generated))
	return generated
}

func dnsConditionsFromRouteRule(rule *model.RouteRule) []map[string]interface{} {
	withInvert := func(conditions map[string]interface{}) map[string]interface{} {
		if rule.Invert {
			conditions["invert"] = true
		}
		return conditions
	}
	switch rule.RuleType {
	case "domain", "domain_suffix", "domain_keyword":
		return []map[string]interface{}{withInvert(map[string]interface{}{rule.RuleType: rule.Values})}
	case "geosite":
		return []map[string]interface{}{withInvert(map[string]interface{}{"rule_set": generatedGeoRuleSetTags("geosite", rule.Values)})}
	case "rule_set":
		return []map[string]interface{}{withInvert(map[string]interface{}{"rule_set": rule.Values})}
	case "mixed":
		items, err := parseMixedRouteRuleValues(rule.Values)
		if err != nil {
			logging.Info("config_generator.dns", "混合路由规则 %s 无法生成策略 DNS 条件: %v", rule.Name, err)
			return nil
		}
		rules := make([]map[string]interface{}, 0)
		indexes := make(map[string]int)
		appendValue := func(key, value string) {
			if index, exists := indexes[key]; exists {
				values, _ := rules[index][key].([]string)
				rules[index][key] = append(values, value)
				return
			}
			indexes[key] = len(rules)
			rules = append(rules, withInvert(map[string]interface{}{key: []string{value}}))
		}
		for _, item := range items {
			switch item.RuleType {
			case "domain", "domain_suffix", "domain_keyword":
				appendValue(item.RuleType, item.Value)
			case "geosite":
				appendValue("rule_set", generatedGeoRuleSetTag("geosite", item.Value))
			case "rule_set":
				appendValue("rule_set", item.Value)
			}
		}
		return rules
	default:
		return nil
	}
}

func applyDNSServerAddress(server map[string]interface{}, serverType, address string) bool {
	address = strings.TrimSpace(address)
	if address == "" || !isRemoteDNSServerType(serverType) {
		return false
	}
	host := address
	if parsed, err := url.Parse(address); err == nil && parsed.Scheme != "" && parsed.Hostname() != "" {
		host = parsed.Hostname()
		if portValue := parsed.Port(); portValue != "" {
			if port, err := strconv.ParseUint(portValue, 10, 16); err == nil {
				server["server_port"] = uint16(port)
			}
		}
		if (serverType == "https" || serverType == "h3") && parsed.EscapedPath() != "" && parsed.EscapedPath() != "/" {
			server["path"] = parsed.EscapedPath()
		}
	} else if splitHost, portValue, err := net.SplitHostPort(address); err == nil {
		host = splitHost
		if port, err := strconv.ParseUint(portValue, 10, 16); err == nil {
			server["server_port"] = uint16(port)
		}
	}
	host = strings.Trim(host, "[]")
	server["server"] = host
	return net.ParseIP(host) == nil
}

func isRemoteDNSServerType(serverType string) bool {
	switch serverType {
	case "udp", "tcp", "tls", "https", "quic", "h3":
		return true
	default:
		return false
	}
}

func selectDNSBootstrapTag(servers []model.DNSServer) string {
	for _, server := range servers {
		if !server.Enabled || server.Tag == "" || (server.Detour != "" && server.Detour != "direct") {
			continue
		}
		if server.ServerType == "local" {
			return server.Tag
		}
		config := make(map[string]interface{})
		if isRemoteDNSServerType(server.ServerType) && !applyDNSServerAddress(config, server.ServerType, server.Address) {
			if _, ok := config["server"]; ok {
				return server.Tag
			}
		}
	}
	return ""
}

func needsGeneratedDNSBootstrap(servers []model.DNSServer, tags map[string]bool) bool {
	for _, server := range servers {
		if !server.Enabled {
			continue
		}
		config := make(map[string]interface{})
		if !applyDNSServerAddress(config, server.ServerType, server.Address) {
			continue
		}
		if !tags[server.AddressResolver] || server.AddressResolver == server.Tag {
			return true
		}
	}
	return false
}

func hasProxyDetouredDNSServer(servers []model.DNSServer) bool {
	for _, server := range servers {
		if server.Enabled && server.Tag != "" && server.Detour != "" && server.Detour != "direct" {
			return true
		}
	}
	return false
}

func uniqueDNSServerTag(base string, tags map[string]bool) string {
	if !tags[base] {
		return base
	}
	for index := 2; ; index++ {
		candidate := fmt.Sprintf("%s-%d", base, index)
		if !tags[candidate] {
			return candidate
		}
	}
}

func decodeDNSRuleConditions(raw string) map[string]interface{} {
	conditions := make(map[string]interface{})
	if raw == "" || raw == "{}" {
		return conditions
	}
	if err := json.Unmarshal([]byte(raw), &conditions); err != nil {
		return map[string]interface{}{}
	}
	return conditions
}

func dnsRuleOutboundConditions(conditions map[string]interface{}) []string {
	return dnsRuleStringConditions(conditions, "outbound")
}

func isDNSStrategyBindingConditions(conditions map[string]interface{}) bool {
	return len(conditions) == 1 && len(dnsRuleOutboundConditions(conditions)) > 0
}

func dnsRuleStringConditions(conditions map[string]interface{}, key string) []string {
	value, ok := conditions[key]
	if !ok {
		return nil
	}
	switch v := value.(type) {
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	case []interface{}:
		outbounds := make([]string, 0, len(v))
		for _, item := range v {
			if outbound, ok := item.(string); ok && outbound != "" {
				outbounds = append(outbounds, outbound)
			}
		}
		return outbounds
	case []string:
		return v
	default:
		return nil
	}
}

func normalizeDNSRuleConditions(conditions map[string]interface{}) map[string]interface{} {
	geositeValues := dnsRuleStringConditions(conditions, "geosite")
	if _, exists := conditions["geosite"]; !exists {
		return conditions
	}
	delete(conditions, "geosite")
	if len(geositeValues) == 0 {
		return conditions
	}

	ruleSets := dnsRuleStringConditions(conditions, "rule_set")
	seen := make(map[string]bool, len(ruleSets)+len(geositeValues))
	combined := make([]string, 0, len(ruleSets)+len(geositeValues))
	for _, tag := range append(ruleSets, generatedGeoRuleSetTags("geosite", geositeValues)...) {
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		combined = append(combined, tag)
	}
	if len(combined) > 0 {
		conditions["rule_set"] = combined
	}
	return conditions
}

func (s *ConfigGeneratorService) defaultDomainResolver() map[string]interface{} {
	settings, err := s.effectiveDNSGlobalSettings()
	if err != nil || settings == nil || !settings.Enabled || settings.Final == "" {
		return nil
	}
	servers, err := s.store.ListDNSServers()
	if err != nil {
		return nil
	}
	server := selectDefaultDomainResolver(settings, servers)
	if server == "" {
		return nil
	}
	resolver := map[string]interface{}{"server": server}
	if settings.ClientSubnet != "" {
		resolver["client_subnet"] = settings.ClientSubnet
	}
	return resolver
}

func selectDefaultDomainResolver(settings *model.DNSGlobalSettings, servers []model.DNSServer) string {
	if settings == nil || !settings.Enabled {
		return ""
	}
	if bootstrapTag := selectDNSBootstrapTag(servers); bootstrapTag != "" {
		return bootstrapTag
	}
	tags := enabledDNSServerTags(servers, settings.FakeIPEnabled)
	return uniqueDNSServerTag("ackwrap-bootstrap-local", tags)
}

func (s *ConfigGeneratorService) enabledRouteRuleSubscriptionTags() map[string]bool {
	tags := map[string]bool{}
	subscriptions, err := s.store.ListRouteRuleSubscriptions()
	if err != nil {
		return tags
	}
	for _, sub := range subscriptions {
		if sub.Enabled {
			tags[sub.Tag] = true
		}
	}
	return tags
}

func (s *ConfigGeneratorService) routeRuleOutboundOverrides() map[int64]string {
	overrides := make(map[int64]string)
	collections, err := s.store.ListProxyCollectionsWithNodes()
	if err != nil {
		logging.Info("config_generator.route", "读取策略组规则绑定失败: %v", err)
		return overrides
	}
	for _, collection := range collections {
		if !collection.Enabled {
			continue
		}
		for _, ruleID := range collection.RouteRuleIDs {
			if ruleID <= 0 {
				continue
			}
			if _, exists := overrides[ruleID]; exists {
				logging.Info("config_generator.route", "规则 %d 绑定多个策略组，保留第一个策略组", ruleID)
				continue
			}
			overrides[ruleID] = collection.Name
		}
	}
	return overrides
}

// generateRouteRule 生成单条路由规则
func (s *ConfigGeneratorService) generateRouteRules(rule *model.RouteRule) ([]map[string]interface{}, error) {
	if rule.RuleType != "mixed" {
		return []map[string]interface{}{singboxRouteRule(rule.RuleType, rule.Values, rule.Outbound, rule.Invert)}, nil
	}
	return mixedSingboxRouteRules(rule.Values, rule.Outbound, rule.Invert)
}

// validateConfig 验证配置文件
func (s *ConfigGeneratorService) validateConfig(configPath string) (bool, string) {
	binaryPath := s.paths.BinaryPath
	if binaryPath == "" {
		return false, "sing-box 未安装"
	}

	logging.Info("config_generator.validate", "验证配置: %s", configPath)

	// 执行 sing-box check
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command(binaryPath, "check", "-c", configPath)
	} else {
		cmd = exec.Command(binaryPath, "check", "-c", configPath)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Sprintf("配置验证失败: %s", strings.TrimSpace(cleanLogLine(string(output))))
	}

	return true, ""
}

// Apply 将最近生成并校验通过的配置保存为命名文件并设为当前配置。
func (s *ConfigGeneratorService) Apply(fileName string, restartCore bool) error {
	fileName, err := normalizeConfigFileName(fileName)
	if err != nil {
		return err
	}
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return s.applyLocked(fileName, restartCore)
}

func (s *ConfigGeneratorService) applyLocked(fileName string, restartCore bool) error {
	configFileMu.Lock()
	err := s.applyConfigFileLocked(fileName)
	configFileMu.Unlock()
	if err != nil {
		return err
	}

	if restartCore && s.singbox != nil {
		if _, err := s.singbox.ReloadConfig(); err != nil {
			return fmt.Errorf("配置已应用，但重载核心失败: %w", err)
		}
	}

	return nil
}

func (s *ConfigGeneratorService) applyConfigFileLocked(fileName string) error {

	logging.Info("config_generator.apply", "应用配置，文件名: %s", fileName)

	tmpPath := filepath.Join(s.paths.DataDir, "config.tmp.json")
	if err := os.MkdirAll(s.paths.ConfigDir, 0755); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	targetPath := ""
	if fileName != "" {
		targetPath = filepath.Join(s.paths.ConfigDir, fileName)
	} else {
		var exists bool
		var err error
		targetPath, exists, err = s.paths.ActiveConfigPath()
		if err != nil {
			return fmt.Errorf("获取配置路径失败: %w", err)
		}
		if !exists {
			targetPath = s.paths.ConfigPath
		}
	}

	// 1. 检查临时配置是否存在
	if _, err := os.Stat(tmpPath); os.IsNotExist(err) {
		return fmt.Errorf("临时配置文件不存在，请先生成配置")
	}

	// 2. 在目标目录创建暂存文件，并再次使用实际 sing-box 校验。
	stagedFile, err := os.CreateTemp(filepath.Dir(targetPath), ".ackwrap-config-*.tmp")
	if err != nil {
		return fmt.Errorf("创建配置暂存文件失败: %w", err)
	}
	stagedPath := stagedFile.Name()
	if err := stagedFile.Close(); err != nil {
		os.Remove(stagedPath)
		return fmt.Errorf("关闭配置暂存文件失败: %w", err)
	}
	defer os.Remove(stagedPath)
	if err := copyFile(tmpPath, stagedPath); err != nil {
		return fmt.Errorf("暂存配置失败: %w", err)
	}
	if valid, validationError := s.validateConfig(stagedPath); !valid {
		return fmt.Errorf("应用配置前验证失败: %s", validationError)
	}

	// 3. 先复制旧配置作为备份，再原子替换正式配置。
	// 替换失败时原配置仍位于正式路径，不需要二次回滚。
	if _, err := os.Stat(targetPath); err == nil {
		backup, created, err := ensureDailyConfigBackup(s.paths, s.store, targetPath, time.Now())
		if err != nil {
			return fmt.Errorf("备份当前配置失败: %w", err)
		}
		if created {
			logging.Info("config_generator.apply", "已创建今日配置备份: %s", backup.FileName)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查当前配置失败: %w", err)
	}

	if err := atomicReplaceFile(stagedPath, targetPath); err != nil {
		return fmt.Errorf("应用配置失败，旧配置保持不变: %w", err)
	}
	if err := writeActiveConfigMarker(s.paths, targetPath); err != nil {
		return fmt.Errorf("配置已保存，但设置为当前配置失败: %w", err)
	}

	logging.Info("config_generator.apply", "配置已应用到: %s", targetPath)

	// 4. 删除生成阶段的临时配置
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		logging.Info("config_generator.apply", "删除临时配置失败: %v", err)
	}

	return nil
}

func normalizeConfigFileName(fileName string) (string, error) {
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		return "", fmt.Errorf("%w: 文件名不能为空", ErrInvalidConfigFileName)
	}
	if filepath.Ext(fileName) == "" {
		fileName += ".json"
	}
	if len([]rune(fileName)) > 128 || filepath.Base(fileName) != fileName || strings.IndexFunc(fileName, unicode.IsControl) >= 0 ||
		strings.ContainsAny(fileName, `<>:"/\|?*`) || !strings.EqualFold(filepath.Ext(fileName), ".json") {
		return "", fmt.Errorf("%w: 仅支持配置目录内的 .json 文件", ErrInvalidConfigFileName)
	}
	extension := filepath.Ext(fileName)
	stem := strings.TrimSuffix(fileName, extension)
	if stem == "" || strings.HasSuffix(stem, ".") || strings.HasSuffix(stem, " ") {
		return "", fmt.Errorf("%w: 文件名格式不正确", ErrInvalidConfigFileName)
	}
	reservedStem := strings.ToUpper(strings.SplitN(stem, ".", 2)[0])
	if reservedStem == "CON" || reservedStem == "PRN" || reservedStem == "AUX" || reservedStem == "NUL" ||
		(len(reservedStem) == 4 && (strings.HasPrefix(reservedStem, "COM") || strings.HasPrefix(reservedStem, "LPT")) && reservedStem[3] >= '1' && reservedStem[3] <= '9') {
		return "", fmt.Errorf("%w: 文件名为系统保留名称", ErrInvalidConfigFileName)
	}
	fileName = stem + ".json"
	if paths.IsConfigBackupName(fileName) {
		return "", fmt.Errorf("%w: 文件名不能使用备份格式", ErrInvalidConfigFileName)
	}
	return fileName, nil
}

func writeActiveConfigMarker(p *paths.Paths, targetPath string) error {
	markerFile, err := os.CreateTemp(p.ConfigDir, ".active-config-*.tmp")
	if err != nil {
		return err
	}
	markerPath := markerFile.Name()
	defer os.Remove(markerPath)
	if _, err := markerFile.WriteString(filepath.Base(targetPath)); err != nil {
		markerFile.Close()
		return err
	}
	if err := markerFile.Close(); err != nil {
		return err
	}
	return atomicReplaceFile(markerPath, p.ActiveConfigMarkerPath())
}

// generateInbounds 生成入站配置
func (s *ConfigGeneratorService) generateInbounds(listen string, port int, tunIPv4Address, tunIPv6Address string) ([]interface{}, error) {
	if listen == "" {
		listen = "0.0.0.0"
	}
	if port == 0 {
		port = model.DefaultMixedInboundPort
	}

	// 获取运行模式设置
	mode := s.store.GetInboundMode()
	autoRedirect := runtime.GOOS == "linux"

	var inbounds []interface{}

	switch mode {
	case "tun":
		// 纯 TUN 模式
		inbounds = []interface{}{
			generatedTUNInbound(autoRedirect, tunIPv4Address, tunIPv6Address),
		}
	case "mixed":
		// 纯 Mixed 模式
		inbounds = []interface{}{
			map[string]interface{}{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      listen,
				"listen_port": port,
			},
		}
	case "tun_mixed":
		fallthrough
	default:
		// TUN + Mixed 双模式（默认）
		inbounds = []interface{}{
			generatedTUNInbound(autoRedirect, tunIPv4Address, tunIPv6Address),
			map[string]interface{}{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      listen,
				"listen_port": port,
			},
		}
	}
	if autoRedirect && (mode == "tun" || mode == "tun_mixed") {
		logging.Info("config_generator.inbound", "Linux TUN 已启用 auto_redirect，由 sing-box 管理 nftables/OpenWrt fw4 兼容规则")
	}

	return inbounds, nil
}

func generatedTUNInbound(autoRedirect bool, tunIPv4Address, tunIPv6Address string) map[string]interface{} {
	inbound := map[string]interface{}{
		"type":           "tun",
		"tag":            "tun-in",
		"interface_name": "tun0",
		"address":        []string{tunIPv4Address, tunIPv6Address},
		"auto_route":     true,
		"strict_route":   true,
		"stack":          "system",
	}
	if autoRedirect {
		inbound["auto_redirect"] = true
		inbound["iproute2_table_index"] = defaultIPRoute2TableIndex
		inbound["iproute2_rule_index"] = defaultIPRoute2RuleIndex
		inbound["auto_redirect_iproute2_fallback_rule_index"] = defaultFallbackRuleIndex
		inbound["auto_redirect_output_mark"] = fmt.Sprintf("0x%x", defaultAutoRedirectMark)
	}
	return inbound
}

func normalizeTUNAddresses(ipv4Address, ipv6Address string) (string, string, error) {
	normalizedIPv4, err := normalizeTUNAddress(ipv4Address, defaultTUNIPv4Address, false)
	if err != nil {
		return "", "", fmt.Errorf("TUN IPv4 地址无效: %w", err)
	}
	normalizedIPv6, err := normalizeTUNAddress(ipv6Address, defaultTUNIPv6Address, true)
	if err != nil {
		return "", "", fmt.Errorf("TUN IPv6 地址无效: %w", err)
	}
	return normalizedIPv4, normalizedIPv6, nil
}

func normalizeTUNAddress(value, fallback string, ipv6 bool) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = fallback
	}
	ip, network, err := net.ParseCIDR(value)
	if err != nil {
		return "", fmt.Errorf("必须使用 CIDR 格式")
	}
	ones, bits := network.Mask.Size()
	if ipv6 {
		if bits != net.IPv6len*8 || ip.To4() != nil {
			return "", fmt.Errorf("必须是 IPv6 地址")
		}
	} else {
		if bits != net.IPv4len*8 {
			return "", fmt.Errorf("必须是 IPv4 地址")
		}
		ip = ip.To4()
		if ip == nil {
			return "", fmt.Errorf("必须是 IPv4 地址")
		}
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsMulticast() {
		return "", fmt.Errorf("不能使用未指定、回环或组播地址")
	}
	if ones < bits && ip.Equal(network.IP) {
		return "", fmt.Errorf("必须填写网段内的主机地址，不能使用网段地址")
	}
	if !ipv6 && ones < 31 {
		broadcast := append(net.IP(nil), network.IP...)
		for i := range broadcast {
			broadcast[i] |= ^network.Mask[i]
		}
		if ip.Equal(broadcast) {
			return "", fmt.Errorf("必须填写网段内的主机地址，不能使用广播地址")
		}
	}
	return fmt.Sprintf("%s/%d", ip.String(), ones), nil
}

// Preview 预览生成的配置
func (s *ConfigGeneratorService) Preview(defaultOutbound string) (map[string]interface{}, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()

	req, err := s.previewRequest(defaultOutbound)
	if err != nil {
		return nil, err
	}
	previewFile, err := os.CreateTemp(s.paths.DataDir, "config-preview-*.json")
	if err != nil {
		return nil, fmt.Errorf("创建配置预览文件失败: %w", err)
	}
	previewPath := previewFile.Name()
	if err := previewFile.Close(); err != nil {
		os.Remove(previewPath)
		return nil, fmt.Errorf("关闭配置预览文件失败: %w", err)
	}
	defer os.Remove(previewPath)

	resp, err := s.generateLockedTo(req, previewPath)
	if err != nil {
		return nil, err
	}
	return resp.Config, nil
}

func (s *ConfigGeneratorService) previewRequest(defaultOutbound string) (*model.ConfigGenerateRequest, error) {
	stored, err := s.store.GetConfigGenerateRequest()
	if err != nil {
		return nil, fmt.Errorf("读取配置生成参数失败: %w", err)
	}
	if stored == nil {
		stored = &model.ConfigGenerateRequest{
			DefaultOutbound: "direct",
			InboundListen:   "0.0.0.0",
			InboundPort:     model.DefaultMixedInboundPort,
			TUNIPv4Address:  defaultTUNIPv4Address,
			TUNIPv6Address:  defaultTUNIPv6Address,
			LogLevel:        s.store.GetLogLevel(),
		}
	}
	req := *stored
	if strings.TrimSpace(req.TUNIPv4Address) == "" {
		req.TUNIPv4Address = defaultTUNIPv4Address
	}
	if strings.TrimSpace(req.TUNIPv6Address) == "" {
		req.TUNIPv6Address = defaultTUNIPv6Address
	}
	if defaultOutbound != "" {
		req.DefaultOutbound = defaultOutbound
	}
	return &req, nil
}

// writeConfigFile 写入配置文件
func (s *ConfigGeneratorService) writeConfigFile(path string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}
