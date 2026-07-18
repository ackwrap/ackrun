package service

import (
	"encoding/json"
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

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

var outboundTagUnsafePattern = regexp.MustCompile(`[^A-Za-z0-9_.\-\p{Han}]+`)

const (
	defaultRuleSetHTTPClientTag = "ackwrap-rule-set-direct"
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
	if err := s.applyLocked(true); err != nil {
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
			DefaultOutbound: "proxy",
			InboundListen:   "127.0.0.1",
			InboundPort:     7890,
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

	logging.Info("config_generator.generate", "开始生成配置，默认出站: %s", req.DefaultOutbound)

	// 1. 生成 inbounds
	inbounds := s.generateInbounds(req.InboundListen, req.InboundPort)

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

	dnsGlobalSettings, _ := s.store.GetDNSGlobalSettings()
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
		Config:   config,
		Valid:    valid,
		Error:    errMsg,
		FilePath: tmpPath,
	}, nil
}

// generateOutbounds 生成所有 outbounds 和 endpoints。WireGuard 在 sing-box 1.13 起是 endpoint，
// 不再是 outbound，但 endpoint tag 与 outbound 共享引用命名空间。
func (s *ConfigGeneratorService) generateOutbounds() ([]interface{}, []interface{}, error) {
	outbounds := []interface{}{}
	endpoints := []interface{}{}
	domainResolverBindings := s.dnsOutboundResolverBindings()
	connectivitySettings, err := s.store.GetConnectivitySettings()
	if err != nil {
		return nil, nil, fmt.Errorf("读取连通性测速设置失败: %w", err)
	}

	// 1. 添加基础 direct 出站。sing-box 1.13 已移除 dns/block 特殊 outbound，DNS/拦截必须通过 route action 处理。
	directOutbound := map[string]interface{}{
		"type": "direct",
		"tag":  "direct",
	}
	applyDomainResolverBinding(directOutbound, domainResolverBindings["direct"])
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
	nodeDomainResolvers := collectionNodeDomainResolverBindings(collections, groupNodeUIDs, usedNodeUIDs, domainResolverBindings)
	if resolver := domainResolverBindings["proxy"]; len(resolver) > 0 {
		for _, node := range nodes {
			if len(nodeDomainResolvers[node.UID]) == 0 {
				nodeDomainResolvers[node.UID] = resolver
			}
		}
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
			endpoint, err := s.generateWireGuardEndpoint(&node, nodeTags[node.UID], nodeDomainResolvers[node.UID])
			if err != nil {
				logging.Info("config_generator.node", "跳过节点 %s: %v", node.Name, err)
				continue
			}
			endpoints = append(endpoints, endpoint)
			generatedNodeTags[node.UID] = nodeTags[node.UID]
			continue
		}
		nodeOutbound, err := s.generateNodeOutbound(&node, nodeTags[node.UID], nodeDomainResolvers[node.UID])
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
			return nil, nil, fmt.Errorf("策略组 %s 生成失败: %w", col.Name, err)
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

func (s *ConfigGeneratorService) dnsOutboundResolverBindings() map[string]map[string]interface{} {
	bindings := make(map[string]map[string]interface{})
	globalSettings, err := s.store.GetDNSGlobalSettings()
	if err != nil || globalSettings == nil || !globalSettings.Enabled {
		return bindings
	}
	dnsServers, err := s.store.ListDNSServers()
	if err != nil {
		logging.Info("config_generator.dns", "读取 DNS server 失败: %v", err)
		return bindings
	}
	serverTags := enabledDNSServerTags(dnsServers, globalSettings.FakeIPEnabled)
	dnsRules, err := s.store.ListDNSRules()
	if err != nil {
		logging.Info("config_generator.dns", "读取 DNS 出口绑定失败: %v", err)
		return bindings
	}
	for _, rule := range dnsRules {
		if !rule.Enabled || rule.Server == "" {
			continue
		}
		if !serverTags[rule.Server] {
			logging.Info("config_generator.dns", "跳过引用无效 DNS server 的 outbound 绑定: %s", rule.Server)
			continue
		}
		conditions := decodeDNSRuleConditions(rule.ConditionsJSON)
		for _, outbound := range dnsRuleOutboundConditions(conditions) {
			if outbound == "" || outbound == "block" {
				continue
			}
			if _, exists := bindings[outbound]; exists {
				logging.Info("config_generator.dns", "outbound %s 绑定了多个 DNS server，保留第一个", outbound)
				continue
			}
			resolverTag := safeNodeResolverTag(rule.Server, dnsServers, serverTags)
			if resolverTag == "" {
				logging.Info("config_generator.dns", "跳过无法安全引导的 outbound DNS 绑定: %s", rule.Server)
				continue
			}
			if resolverTag != rule.Server {
				logging.Info("config_generator.dns", "outbound DNS server %s 依赖代理 detour，节点解析回退到直连 bootstrap: %s", rule.Server, resolverTag)
			}
			resolver := map[string]interface{}{"server": resolverTag}
			if rule.DisableCache {
				resolver["disable_cache"] = true
			}
			if rule.RewriteTTL > 0 {
				resolver["rewrite_ttl"] = rule.RewriteTTL
			}
			if rule.ClientSubnet != "" {
				resolver["client_subnet"] = rule.ClientSubnet
			}
			bindings[outbound] = resolver
		}
	}
	return bindings
}

func enabledDNSServerTags(servers []model.DNSServer, fakeIPEnabled bool) map[string]bool {
	tags := make(map[string]bool)
	for _, server := range servers {
		if server.Enabled && server.Tag != "" {
			tags[server.Tag] = true
		}
	}
	if fakeIPEnabled {
		tags["fakeip"] = true
	}
	return tags
}

func collectionNodeDomainResolverBindings(collections []*model.ProxyCollectionWithNodes, groupNodeUIDs map[int64][]string, usedNodeUIDs map[string]bool, bindings map[string]map[string]interface{}) map[string]map[string]interface{} {
	nodeResolvers := make(map[string]map[string]interface{})
	hasProxyCollection := false
	for _, col := range collections {
		if !col.Enabled {
			continue
		}
		if col.Name == "proxy" {
			hasProxyCollection = true
		}
		resolver := bindings[col.Name]
		if len(resolver) == 0 {
			continue
		}
		for _, uid := range collectionNodeUIDs(col, groupNodeUIDs) {
			if _, exists := nodeResolvers[uid]; !exists {
				nodeResolvers[uid] = resolver
			}
		}
	}
	if resolver := bindings["proxy"]; len(resolver) > 0 && !hasProxyCollection {
		for uid := range usedNodeUIDs {
			if _, exists := nodeResolvers[uid]; !exists {
				nodeResolvers[uid] = resolver
			}
		}
	}
	return nodeResolvers
}

func collectionNodeUIDs(col *model.ProxyCollectionWithNodes, groupNodeUIDs map[int64][]string) []string {
	if !isCollectionGroupSource(col.SourceType) {
		return col.NodeUIDs
	}
	uids := make([]string, 0)
	seen := make(map[string]bool)
	for _, group := range col.ReferencedGroups {
		for _, uid := range groupNodeUIDs[group.ID] {
			if seen[uid] {
				continue
			}
			seen[uid] = true
			uids = append(uids, uid)
		}
	}
	return uids
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

	// 根据代理模式决定是否加载规则
	var routeRules []map[string]interface{}
	var ruleSets []map[string]interface{}
	ruleSetTags := make(map[string]bool)

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

	// AckWrap、sing-box 和节点服务器必须优先直连，避免 TUN/全局模式形成代理回环。
	bypassRules, err := s.defaultBypassRules()
	if err != nil {
		return nil, err
	}
	routeRules = append(routeRules, bypassRules...)
	route["find_process"] = true

	// 获取代理模式
	proxyMode := s.store.GetProxyMode()

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
				ruleSets = appendGeneratedGeoRuleSets(ruleSets, ruleSetTags, rule.RuleType, rule.Values, "http://127.0.0.1:8080")
			}
			if rule.RuleType == "mixed" {
				ruleSets = addMixedGeneratedRuleSets(ruleSets, ruleSetTags, rule.Values, "http://127.0.0.1:8080")
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
				"url":    fmt.Sprintf("http://127.0.0.1:8080/api/v1/rules/subscriptions/%d/content", sub.ID),
			}
			ruleSets = append(ruleSets, ruleSet)
		}
	}
	// 全局模式和直连模式不加载规则

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
			"action":       "route",
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
			"action":   "route",
			"outbound": "direct",
		})
	}
	if len(ipCIDRs) > 0 {
		rules = append(rules, map[string]interface{}{
			"ip_cidr":  ipCIDRs,
			"action":   "route",
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
	globalSettings, _ := s.store.GetDNSGlobalSettings()
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
	if bootstrapTag == "" && (needsGeneratedDNSBootstrap(dnsServers, serverTags) || hasProxyDetouredDNSServer(dnsServers)) {
		generatedBootstrapTag = uniqueDNSServerTag("ackwrap-bootstrap-local", serverTags)
		bootstrapTag = generatedBootstrapTag
		serverTags[bootstrapTag] = true
	}
	hasFakeIPServer := false
	for _, srv := range dnsServers {
		if !srv.Enabled {
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
		conditions := decodeDNSRuleConditions(rule.ConditionsJSON)
		if _, hasOutbound := conditions["outbound"]; hasOutbound {
			// DNS rule outbound matching was deprecated in sing-box 1.12 and is generated
			// as outbound domain_resolver instead.
			delete(conditions, "outbound")
		}
		if len(conditions) == 0 {
			continue
		}
		ruleMap := map[string]interface{}{
			"server": rule.Server,
		}
		for k, v := range conditions {
			ruleMap[k] = v
		}
		if rule.DisableCache {
			ruleMap["disable_cache"] = true
		}
		if rule.RewriteTTL > 0 {
			ruleMap["rewrite_ttl"] = rule.RewriteTTL
		}
		if rule.ClientSubnet != "" {
			ruleMap["client_subnet"] = rule.ClientSubnet
		}
		rules = append(rules, ruleMap)
	}
	if globalSettings.FakeIPEnabled {
		rules = append([]map[string]interface{}{{
			"query_type": []string{"A", "AAAA"},
			"server":     "fakeip",
		}}, rules...)
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

func safeNodeResolverTag(requested string, servers []model.DNSServer, tags map[string]bool) string {
	for _, server := range servers {
		if server.Enabled && server.Tag == requested {
			if server.Detour == "" || server.Detour == "direct" {
				return requested
			}
			break
		}
	}
	if bootstrapTag := selectDNSBootstrapTag(servers); bootstrapTag != "" {
		return bootstrapTag
	}
	if requested != "" && tags[requested] {
		return uniqueDNSServerTag("ackwrap-bootstrap-local", tags)
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
	value, ok := conditions["outbound"]
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

func (s *ConfigGeneratorService) defaultDomainResolver() map[string]interface{} {
	settings, err := s.store.GetDNSGlobalSettings()
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
	tags := enabledDNSServerTags(servers, settings.FakeIPEnabled)
	if tags[settings.Final] {
		return settings.Final
	}
	for _, server := range servers {
		if server.Enabled && server.Tag != "" {
			return server.Tag
		}
	}
	if settings.FakeIPEnabled {
		return "fakeip"
	}
	return ""
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

// Apply 应用配置
func (s *ConfigGeneratorService) Apply(restartCore bool) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	return s.applyLocked(restartCore)
}

func (s *ConfigGeneratorService) applyLocked(restartCore bool) error {
	configFileMu.Lock()
	defer configFileMu.Unlock()

	logging.Info("config_generator.apply", "应用配置，重启核心: %v", restartCore)

	tmpPath := filepath.Join(s.paths.DataDir, "config.tmp.json")
	targetPath, exists, err := s.paths.ActiveConfigPath()
	if err != nil {
		return fmt.Errorf("获取配置路径失败: %w", err)
	}
	if !exists {
		targetPath = s.paths.ConfigPath
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
	backupPath := ""
	if _, err := os.Stat(targetPath); err == nil {
		backupPath = filepath.Join(s.paths.ConfigDir, fmt.Sprintf("config.backup.%d.json", time.Now().UnixNano()))
		if err := copyFile(targetPath, backupPath); err != nil {
			return fmt.Errorf("备份当前配置失败: %w", err)
		}
		logging.Info("config_generator.apply", "配置已备份到: %s", backupPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("检查当前配置失败: %w", err)
	}

	if err := atomicReplaceFile(stagedPath, targetPath); err != nil {
		return fmt.Errorf("应用配置失败，旧配置保持不变: %w", err)
	}

	logging.Info("config_generator.apply", "配置已应用到: %s", targetPath)

	// 4. 删除生成阶段的临时配置
	if err := os.Remove(tmpPath); err != nil && !os.IsNotExist(err) {
		logging.Info("config_generator.apply", "删除临时配置失败: %v", err)
	}

	if restartCore && s.singbox != nil {
		if _, err := s.singbox.ReloadConfig(); err != nil {
			return fmt.Errorf("配置已应用，但重载核心失败: %w", err)
		}
	}

	return nil
}

// generateInbounds 生成入站配置
func (s *ConfigGeneratorService) generateInbounds(listen string, port int) []interface{} {
	if listen == "" {
		listen = "127.0.0.1"
	}
	if port == 0 {
		port = 7890
	}

	// 获取运行模式设置
	mode := s.store.GetInboundMode()

	var inbounds []interface{}

	switch mode {
	case "tun":
		// 纯 TUN 模式
		inbounds = []interface{}{
			map[string]interface{}{
				"type":           "tun",
				"tag":            "tun-in",
				"interface_name": "tun0",
				"address":        []string{"172.19.0.1/30"},
				"auto_route":     true,
				"strict_route":   true,
				"stack":          "system",
			},
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
			map[string]interface{}{
				"type":           "tun",
				"tag":            "tun-in",
				"interface_name": "tun0",
				"address":        []string{"172.19.0.1/30"},
				"auto_route":     true,
				"strict_route":   true,
				"stack":          "system",
			},
			map[string]interface{}{
				"type":        "mixed",
				"tag":         "mixed-in",
				"listen":      listen,
				"listen_port": port,
			},
		}
	}

	return inbounds
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
			DefaultOutbound: "proxy",
			InboundListen:   "127.0.0.1",
			InboundPort:     7890,
			LogLevel:        s.store.GetLogLevel(),
		}
	}
	req := *stored
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
	return os.WriteFile(path, data, 0644)
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
