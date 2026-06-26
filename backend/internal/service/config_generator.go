package service

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/paths"
	"github.com/ackwrap/ackwrap/internal/store"
)

var outboundTagUnsafePattern = regexp.MustCompile(`[^A-Za-z0-9_.\-\p{Han}]+`)

// ConfigGeneratorService 配置生成服务
type ConfigGeneratorService struct {
	store   *store.Store
	paths   *paths.Paths
	singbox *SingboxService
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
	if logLevel == "" {
		logLevel = "info"
	}

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
		"inbounds":  inbounds,
		"outbounds": outbounds,
		"route":     route,
	}
	if len(endpoints) > 0 {
		config["endpoints"] = endpoints
	}

	dnsGlobalSettings, _ := s.store.GetDNSGlobalSettings()
	if dnsGlobalSettings != nil && dnsGlobalSettings.Enabled {
		config["dns"] = s.generateDNSFromDatabase()
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
			"store_rdrc":   expSettings.CacheFileStoreRDRC,
		}
		if expSettings.CacheFileRDRCTimeout != "" {
			cacheFile["rdrc_timeout"] = expSettings.CacheFileRDRCTimeout
		}
		experimental["cache_file"] = cacheFile
		logging.Info("config_generator.experimental", "启用缓存文件: store_fakeip=%t, store_rdrc=%t", expSettings.CacheFileStoreFakeIP, expSettings.CacheFileStoreRDRC)
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
		config["ntp"] = map[string]interface{}{
			"enabled":         true,
			"server":          ntpSettings.Server,
			"server_port":     ntpSettings.ServerPort,
			"interval":        ntpSettings.Interval,
			"detour":          ntpSettings.Detour,
			"write_to_system": false,
		}
		logging.Info("config_generator.ntp", "NTP 已启用: %s:%d, 间隔: %s", ntpSettings.Server, ntpSettings.ServerPort, ntpSettings.Interval)
	}

	// 5. 生成临时配置文件
	tmpPath := filepath.Join(s.paths.DataDir, "config.tmp.json")
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

	// 1. 添加基础 direct 和 block 出站。sing-box 1.13 已移除 dns outbound，DNS 通过 route action=hijack-dns 处理。
	directOutbound := map[string]interface{}{
		"type": "direct",
		"tag":  "direct",
	}
	applyDomainResolverBinding(directOutbound, domainResolverBindings["direct"])
	outbounds = append(outbounds, directOutbound)
	outbounds = append(outbounds, map[string]interface{}{
		"type": "block",
		"tag":  "block",
	})
	outbounds = append(outbounds, map[string]interface{}{
		"type": "block",
		"tag":  "reject",
	})

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
		validGroupTags[group.Name] = true
	}

	// 4. 获取所有集合和节点组中使用的节点 UID
	usedNodeUIDs := make(map[string]bool)
	for _, col := range collections {
		if !col.Enabled {
			continue
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
	nodeDomainResolvers := collectionNodeDomainResolverBindings(collections, groupNodeUIDs, usedNodeUIDs, domainResolverBindings)

	// 5. 为集合和节点组使用的节点生成 outbound
	nodeReq := model.NodeListRequest{Enabled: boolPtr(true)}
	nodeResp, err := s.store.ListNodes(nodeReq)
	if err != nil {
		return nil, nil, err
	}
	nodeTags := buildNodeOutboundTags(nodeResp.Items)

	for _, node := range nodeResp.Items {
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
			continue
		}
		nodeOutbound, err := s.generateNodeOutbound(&node, nodeTags[node.UID], nodeDomainResolvers[node.UID])
		if err != nil {
			logging.Info("config_generator.outbound", "节点 %s 生成失败: %v", node.Name, err)
			continue
		}
		outbounds = append(outbounds, nodeOutbound)
	}

	// 6. 生成节点组 outbound
	for _, group := range nodeGroups {
		uids := groupNodeUIDs[group.ID]
		if len(uids) == 0 {
			continue
		}
		groupOutbounds := nodeUIDsToOutboundTags(uids, nodeTags)
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
			outbound["url"] = group.TestURL
			outbound["interval"] = fmt.Sprintf("%ds", group.TestInterval)
			outbound["tolerance"] = group.Tolerance
		}

		outbounds = append(outbounds, outbound)
	}

	// 7. 为每个集合生成 outbound（放在节点和节点组后面，这样集合可以引用它们）
	collectionTags := make([]string, 0)
	hasProxyCollection := false
	for _, col := range collections {
		if !col.Enabled {
			continue
		}

		outbound, err := s.generateCollectionOutbound(col, validGroupTags, nodeTags)
		if err != nil {
			logging.Info("config_generator.outbound", "集合 %s 生成失败: %v", col.Name, err)
			continue
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
func (s *ConfigGeneratorService) generateCollectionOutbound(col *model.ProxyCollectionWithNodes, validGroupTags map[string]bool, nodeTags map[string]string) (map[string]interface{}, error) {
	outbound := map[string]interface{}{
		"type": col.Type,
		"tag":  col.Name,
	}

	// 判断是引用节点组还是手动选节点
	if col.SourceType == "node_groups" && len(col.ReferencedGroups) > 0 {
		// 引用节点组模式。node_uids 兼容存放 direct/reject 这类内置出站 tag。
		referencedTags := collectionBuiltinOutboundTags(col)
		for _, group := range col.ReferencedGroups {
			if !validGroupTags[group.Name] {
				logging.Info("config_generator.outbound", "策略组 %s 跳过空节点组引用: %s", col.Name, group.Name)
				continue
			}
			referencedTags = append(referencedTags, group.Name)
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

	// urltest 和 fallback 需要测试配置
	if col.Type == "urltest" || col.Type == "fallback" {
		outbound["url"] = col.TestURL
		outbound["interval"] = fmt.Sprintf("%ds", col.TestInterval)

		if col.Type == "urltest" {
			outbound["tolerance"] = col.Tolerance
		}
	}

	return outbound, nil
}

func builtinOutboundTags(values []string) []string {
	tags := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		switch value {
		case "direct", "reject", "block":
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
	case "应用净化":
		defaults = []string{"reject", "block", "direct"}
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
	unsupportedTypes := map[string]bool{
		"ssr":    true, // ShadowsocksR
		"snell":  true, // Snell
		"mieru":  true, // Mieru
		"anytls": true, // AnyTLS (需要自行编译，预编译版本不支持)
	}

	if unsupportedTypes[node.Type] {
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
	mapTLSFingerprintFields(nodeData)
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

func mapTLSFingerprintFields(nodeData map[string]interface{}) {
	tlsMap, ok := nodeData["tls"].(map[string]interface{})
	if !ok {
		return
	}
	utlsMap, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		return
	}
	fingerprint, _ := utlsMap["fingerprint"].(string)
	if fingerprint == "" || isSingboxUTLSFingerprint(fingerprint) {
		return
	}
	if isSHA256HexString(fingerprint) {
		tlsMap["certificate_public_key_sha256"] = []string{fingerprint}
	}
	delete(utlsMap, "fingerprint")
	if len(utlsMap) == 1 && configBoolValue(utlsMap["enabled"]) {
		delete(tlsMap, "utls")
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

func isSHA256HexString(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
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
	dnsRules, err := s.store.ListDNSRules()
	if err != nil {
		logging.Info("config_generator.dns", "读取 DNS 出口绑定失败: %v", err)
		return bindings
	}
	for _, rule := range dnsRules {
		if !rule.Enabled || rule.Server == "" {
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
			resolver := map[string]interface{}{"server": rule.Server}
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
	if col.SourceType != "node_groups" {
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
	used := make(map[string]bool)
	for _, node := range nodes {
		base := sanitizeOutboundTag(node.Name)
		if base == "" {
			base = sanitizeOutboundTag(node.Type)
		}
		if base == "" {
			base = "node"
		}
		shortUID := node.UID
		if len(shortUID) > 8 {
			shortUID = shortUID[:8]
		}
		tag := fmt.Sprintf("%s-%s", base, shortUID)
		if used[tag] {
			tag = fmt.Sprintf("%s-%s", tag, node.UID)
		}
		used[tag] = true
		result[node.UID] = tag
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
		"rules":    []map[string]interface{}{},
		"rule_set": []map[string]interface{}{},
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
			if outbound, ok := ruleOutboundOverrides[rule.ID]; ok {
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
				ruleSets = appendGeneratedGeoRuleSets(ruleSets, ruleSetTags, rule.RuleType, rule.Values)
			}
			if rule.RuleType == "mixed" {
				ruleSets = addMixedGeneratedRuleSets(ruleSets, ruleSetTags, rule.Values)
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
				"type":            "remote",
				"tag":             sub.Tag,
				"format":          format,
				"url":             fmt.Sprintf("http://127.0.0.1:8080/api/v1/rules/subscriptions/%d/content", sub.ID),
				"download_detour": "direct",
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
		if srv.Address != "" {
			server["server"] = srv.Address
		}
		if srv.AddressResolver != "" {
			server["address_resolver"] = srv.AddressResolver
		}
		if srv.AddressStrategy != "" {
			server["address_strategy"] = srv.AddressStrategy
		}
		if srv.Strategy != "" {
			server["strategy"] = srv.Strategy
		}
		if srv.Detour != "" && srv.Detour != "direct" {
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
		servers = append(servers, server)
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

	// 3. 读取所有启用的 DNS rules
	dnsRules, _ := s.store.ListDNSRules()
	rules := []map[string]interface{}{}
	for _, rule := range dnsRules {
		if !rule.Enabled {
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
	dns := map[string]interface{}{
		"servers":           servers,
		"rules":             rules,
		"final":             globalSettings.Final,
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
	resolver := map[string]interface{}{"server": settings.Final}
	if settings.ClientSubnet != "" {
		resolver["client_subnet"] = settings.ClientSubnet
	}
	return resolver
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
		return false, fmt.Sprintf("配置验证失败: %s", string(output))
	}

	return true, ""
}

// Apply 应用配置
func (s *ConfigGeneratorService) Apply(restartCore bool) error {
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

	// 2. 备份当前配置
	if _, err := os.Stat(targetPath); err == nil {
		backupPath := filepath.Join(s.paths.ConfigDir, fmt.Sprintf("config.backup.%d.json", time.Now().Unix()))
		if err := copyFile(targetPath, backupPath); err != nil {
			logging.Info("config_generator.apply", "备份配置失败: %v", err)
		} else {
			logging.Info("config_generator.apply", "配置已备份到: %s", backupPath)
		}
	}

	// 3. 复制临时配置到正式配置
	if err := copyFile(tmpPath, targetPath); err != nil {
		return fmt.Errorf("应用配置失败: %w", err)
	}

	logging.Info("config_generator.apply", "配置已应用到: %s", targetPath)

	// 4. 删除临时配置
	os.Remove(tmpPath)

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
		listen = "0.0.0.0"
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
	req := &model.ConfigGenerateRequest{
		DefaultOutbound: defaultOutbound,
	}
	resp, err := s.Generate(req)
	if err != nil {
		return nil, err
	}
	return resp.Config, nil
}

// writeConfigFile 写入配置文件
func (s *ConfigGeneratorService) writeConfigFile(path string, config map[string]interface{}) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func boolPtr(b bool) *bool {
	return &b
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}
