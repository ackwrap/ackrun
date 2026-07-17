package parser

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/goccy/go-yaml"
)

type clashSubscription struct {
	Proxies []map[string]any `yaml:"proxies"`
}

func parseClashYAML(body []byte) []model.ParsedNode {
	var sub clashSubscription
	if err := yaml.Unmarshal(body, &sub); err != nil || len(sub.Proxies) == 0 {
		return nil
	}

	nodes := make([]model.ParsedNode, 0, len(sub.Proxies))
	for _, proxy := range sub.Proxies {
		name := getString(proxy, "name")
		typ := strings.ToLower(getString(proxy, "type"))
		normalizedType := normalizeProtocolType(typ)
		server := getString(proxy, "server")
		port := getInt(proxy, "port")
		if name == "" || typ == "" || server == "" || port == 0 {
			continue
		}
		normalized, unsupportedReason := normalizeClashProxy(proxy, normalizedType)
		rawJSON, _ := json.Marshal(normalized)
		nodes = append(nodes, model.ParsedNode{
			Name:              name,
			Type:              normalizedType,
			Server:            server,
			ServerPort:        port,
			Raw:               string(rawJSON),
			RawJSON:           string(rawJSON),
			UnsupportedReason: unsupportedReason,
		})
	}
	return nodes
}

func normalizeProtocolType(typ string) string {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "ss":
		return "shadowsocks"
	case "socks5", "socks4", "socks4a":
		return "socks"
	default:
		return strings.ToLower(strings.TrimSpace(typ))
	}
}

func normalizeClashProxy(proxy map[string]any, typ string) (map[string]any, string) {
	result := map[string]any{
		"type":        typ,
		"server":      getString(proxy, "server"),
		"server_port": getInt(proxy, "port"),
	}
	unsupportedReason := ""
	if !isSupportedClashProtocol(typ) {
		unsupportedReason = fmt.Sprintf("Clash protocol %s is not supported by AckWrap", typ)
	}

	copyClashField(result, proxy, "uuid")
	if alterID := getInt(proxy, "alterId"); typ == "vmess" && alterID > 0 {
		result["alter_id"] = alterID
	}

	// Cipher 字段映射
	if cipher, ok := proxy["cipher"]; ok {
		switch typ {
		case "vmess":
			result["security"] = cipher
		case "shadowsocks", "ssr":
			result["method"] = cipher
		}
	}

	// Shadowsocks udp_over_tcp
	if typ == "shadowsocks" {
		if uot, ok := proxy["udp-over-tcp"]; ok && boolOrString(uot) {
			if ver, ok := proxy["udp-over-tcp-version"]; ok {
				// 有 version，使用完整格式
				result["udp_over_tcp"] = map[string]any{
					"enabled": true,
					"version": ver,
				}
			} else {
				// 无 version，使用简化格式
				result["udp_over_tcp"] = true
			}
		}
	}
	if typ == "ssr" {
		copyClashField(result, proxy, "protocol")
		copyClashField(result, proxy, "obfs")
		if value := getString(proxy, "protocol-param"); value != "" {
			result["protocol_param"] = value
		}
		if value := getString(proxy, "obfs-param"); value != "" {
			result["obfs_param"] = value
		}
	}

	copyClashField(result, proxy, "password")
	copyClashField(result, proxy, "flow")
	copyClashField(result, proxy, "username")
	if typ == "shadowsocks" {
		copyClashField(result, proxy, "plugin")
	}
	if typ == "vless" {
		copyClashField(result, proxy, "encryption")
	}
	if typ == "snell" {
		copyClashField(result, proxy, "psk")
	}

	if value, ok := proxy["tfo"]; ok {
		result["tcp_fast_open"] = boolOrString(value)
	}
	if value, ok := proxy["mptcp"]; ok {
		result["tcp_multi_path"] = boolOrString(value)
	}
	if bindInterface := getString(proxy, "interface-name"); bindInterface != "" {
		result["bind_interface"] = bindInterface
	}
	if routingMark := getInt(proxy, "routing-mark"); routingMark > 0 {
		result["routing_mark"] = routingMark
	}
	if strategy := clashIPVersionStrategy(getString(proxy, "ip-version")); strategy != "" {
		result["domain_strategy"] = strategy
	}

	// VMess/VLESS 专有字段
	if typ == "vmess" || typ == "vless" {
		// global_padding, authenticated_length (VMess)
		if typ == "vmess" {
			if gp, ok := proxy["global-padding"]; ok {
				result["global_padding"] = boolOrString(gp)
			}
			if al, ok := proxy["authenticated-length"]; ok {
				result["authenticated_length"] = boolOrString(al)
			}
		}
		if smux, ok := proxy["smux"].(map[string]any); ok && boolOrString(smux["enabled"]) {
			result["multiplex"] = normalizeClashMultiplex(smux)
		}

		// packet_encoding: packet-encoding 优先级最高
		if pe, ok := proxy["packet-encoding"]; ok && getString(proxy, "packet-encoding") != "" {
			result["packet_encoding"] = pe
		} else if pa, ok := proxy["packet-addr"]; ok && boolOrString(pa) {
			result["packet_encoding"] = "packetaddr"
		} else if xudp, ok := proxy["xudp"]; ok && boolOrString(xudp) {
			result["packet_encoding"] = "xudp"
		}
	}
	if typ == "trojan" {
		if ssOpts, ok := proxy["ss-opts"].(map[string]any); ok && boolOrString(ssOpts["enabled"]) {
			unsupportedReason = "Trojan ss-opts is not supported by sing-box"
		}
		if smux, ok := proxy["smux"].(map[string]any); ok && boolOrString(smux["enabled"]) {
			result["multiplex"] = normalizeClashMultiplex(smux)
		}
	}
	if typ == "shadowsocks" {
		if smux, ok := proxy["smux"].(map[string]any); ok && boolOrString(smux["enabled"]) {
			result["multiplex"] = normalizeClashMultiplex(smux)
		}
	}
	if typ == "anytls" {
		if seconds := getInt(proxy, "idle-session-check-interval"); seconds > 0 {
			result["idle_session_check_interval"] = fmt.Sprintf("%ds", seconds)
		}
		if seconds := getInt(proxy, "idle-session-timeout"); seconds > 0 {
			result["idle_session_timeout"] = fmt.Sprintf("%ds", seconds)
		}
		if count := getInt(proxy, "min-idle-session"); count > 0 {
			result["min_idle_session"] = count
		}
	}
	if typ == "http" {
		copyClashField(result, proxy, "headers")
		copyClashField(result, proxy, "path")
	}
	if typ == "socks" {
		switch strings.ToLower(getString(proxy, "type")) {
		case "socks4", "socks4a":
			result["version"] = "4"
		default:
			result["version"] = "5"
		}
		if tlsEnabled, ok := proxy["tls"]; ok && boolOrString(tlsEnabled) {
			unsupportedReason = "TLS-wrapped SOCKS is not supported by sing-box"
		}
	}

	if typ == "hysteria" {
		if protocol := firstNonEmpty(getString(proxy, "obfs-protocol"), getString(proxy, "protocol")); protocol != "" && protocol != "udp" {
			unsupportedReason = "Hysteria transport protocol is not supported by sing-box"
		}
		if ports, err := normalizeHysteriaPortRanges(firstNonNil(proxy["ports"], proxy["mport"])); err != nil {
			unsupportedReason = "Hysteria port hopping range is invalid"
		} else if len(ports) > 0 {
			result["server_ports"] = ports
		}
		if interval, maximum, err := normalizeHysteriaHopInterval(getString(proxy, "hop-interval")); err != nil || maximum != "" {
			unsupportedReason = "Hysteria hop interval is invalid or unsupported"
		} else if interval != "" {
			result["hop_interval"] = interval
		}
		if up := firstNonEmpty(getString(proxy, "up"), getString(proxy, "up-speed")); up != "" {
			if mbps := parseBandwidth(up); mbps > 0 {
				result["up_mbps"] = mbps
			}
		}
		if down := firstNonEmpty(getString(proxy, "down"), getString(proxy, "down-speed")); down != "" {
			if mbps := parseBandwidth(down); mbps > 0 {
				result["down_mbps"] = mbps
			}
		}
		if auth := getString(proxy, "auth"); auth != "" {
			result["auth"] = auth
		} else if authString := firstNonEmpty(getString(proxy, "auth-str"), getString(proxy, "auth_str")); authString != "" {
			result["auth_str"] = authString
		}
		copyClashField(result, proxy, "obfs")
		if window := getInt(proxy, "recv-window-conn"); window > 0 {
			result["recv_window_conn"] = window
		}
		if window := getInt(proxy, "recv-window"); window > 0 {
			result["recv_window"] = window
		}
		if disabled, ok := proxy["disable-mtu-discovery"]; ok {
			result["disable_mtu_discovery"] = boolOrString(disabled)
		}
	}

	// Hysteria2 port hopping, obfuscation, bandwidth and QUIC options.
	if typ == "hysteria2" {
		if ports, err := normalizeHysteriaPortRanges(firstNonNil(proxy["ports"], proxy["mport"])); err != nil {
			unsupportedReason = "Hysteria2 port hopping range is invalid"
		} else if len(ports) > 0 {
			result["server_ports"] = ports
		}

		if minInterval, maxInterval, err := normalizeHysteriaHopInterval(getString(proxy, "hop-interval")); err != nil {
			unsupportedReason = "Hysteria2 hop interval is invalid"
		} else if minInterval != "" {
			result["hop_interval"] = minInterval
			if maxInterval != "" {
				result["hop_interval_max"] = maxInterval
			}
		}

		obfsType := getString(proxy, "obfs")
		obfsPassword := getString(proxy, "obfs-password")
		if obfsType != "" {
			obfs := map[string]any{
				"type":     obfsType,
				"password": obfsPassword,
			}
			if minPacketSize := getInt(proxy, "obfs-min-packet-size"); minPacketSize > 0 {
				obfs["min_packet_size"] = minPacketSize
			}
			if maxPacketSize := getInt(proxy, "obfs-max-packet-size"); maxPacketSize > 0 {
				obfs["max_packet_size"] = maxPacketSize
			}
			result["obfs"] = obfs
		}

		// up/down 字符串转换为 up_mbps/down_mbps
		if up := getString(proxy, "up"); up != "" {
			if mbps := parseBandwidth(up); mbps > 0 {
				result["up_mbps"] = mbps
			}
		}
		if down := getString(proxy, "down"); down != "" {
			if mbps := parseBandwidth(down); mbps > 0 {
				result["down_mbps"] = mbps
			}
		}
		if profile := getString(proxy, "bbr-profile"); profile != "" {
			result["bbr_profile"] = profile
		}
		if debug, ok := proxy["brutal-debug"]; ok {
			result["brutal_debug"] = boolOrString(debug)
		}
		mapClashQUICOptions(result, proxy)
		if realm, ok := proxy["realm-opts"].(map[string]any); ok && boolOrString(realm["enable"]) {
			unsupportedReason = "Hysteria2 realm-opts cannot be safely combined with a static Clash server"
		}
	}

	// WireGuard address 格式转换
	if typ == "wireguard" {
		if v, ok := proxy["private-key"]; ok {
			result["private_key"] = v
		}
		if v, ok := proxy["public-key"]; ok {
			result["public_key"] = v
		}
		if v, ok := proxy["preshared-key"]; ok {
			result["pre_shared_key"] = v
		} else if v, ok := proxy["pre-shared-key"]; ok {
			result["pre_shared_key"] = v
		}
		if reserved := normalizeWireGuardReserved(proxy["reserved"]); len(reserved) > 0 {
			result["reserved"] = reserved
		}
		copyClashField(result, proxy, "mtu")
		if allowedIPs := stringList(proxy["allowed-ips"]); len(allowedIPs) > 0 {
			result["allowed_ips"] = allowedIPs
		}
		if keepalive := getInt(proxy, "persistent-keepalive"); keepalive > 0 {
			result["persistent_keepalive_interval"] = keepalive
		}
		var addresses []string
		// 优先使用 local-address，没有时才用 ip/ipv6
		if v, ok := proxy["local-address"]; ok && v != nil {
			switch val := v.(type) {
			case string:
				if val != "" {
					addresses = []string{val}
				}
			case []any:
				for _, item := range val {
					if s, ok := item.(string); ok && s != "" {
						addresses = append(addresses, s)
					}
				}
			case []string:
				addresses = val
			}
		}
		// 如果 local-address 没有提供，使用 ip/ipv6
		if len(addresses) == 0 {
			if ip := getString(proxy, "ip"); ip != "" {
				addresses = append(addresses, ensureCIDR(ip, false))
			}
			if ipv6 := getString(proxy, "ipv6"); ipv6 != "" {
				addresses = append(addresses, ensureCIDR(ipv6, true))
			}
		}
		if len(addresses) > 0 {
			result["address"] = addresses
		}
	}

	// TUIC 特殊字段映射
	if typ == "tuic" {
		if token := getString(proxy, "token"); token != "" && getString(proxy, "uuid") == "" {
			unsupportedReason = "TUIC v4 token authentication is not supported by sing-box"
		}
		// reduce-rtt → zero_rtt_handshake
		if rtt, ok := proxy["reduce-rtt"]; ok {
			result["zero_rtt_handshake"] = boolOrString(rtt)
		}
		// congestion-controller → congestion_control
		if cc := getString(proxy, "congestion-controller"); cc != "" {
			result["congestion_control"] = cc
		}
		// udp-relay-mode → udp_relay_mode
		if urm := getString(proxy, "udp-relay-mode"); urm != "" {
			result["udp_relay_mode"] = urm
		}
		if heartbeat := getInt(proxy, "heartbeat-interval"); heartbeat > 0 {
			result["heartbeat"] = fmt.Sprintf("%dms", heartbeat)
		}
		if udpOverStream, ok := proxy["udp-over-stream"]; ok {
			result["udp_over_stream"] = boolOrString(udpOverStream)
		}
		mapClashQUICOptions(result, proxy)
	}

	if typ == "snell" {
		version := getInt(proxy, "version")
		if version == 0 || version == 5 {
			version = 4
		}
		if version != 4 && version != 6 {
			unsupportedReason = fmt.Sprintf("Snell version %d is not supported by sing-box", version)
		} else {
			result["version"] = version
		}
		copyClashField(result, proxy, "reuse")
		if obfs, ok := proxy["obfs-opts"].(map[string]any); ok {
			mode := getString(obfs, "mode")
			if mode == "shadow-tls" {
				unsupportedReason = "Snell ShadowTLS obfuscation is not supported by sing-box"
			} else if mode != "" {
				result["obfs_mode"] = mode
				if host := getString(obfs, "host"); host != "" {
					result["obfs_host"] = host
				}
			}
		}
	}

	if typ == "naive" {
		if concurrency := getInt(proxy, "insecure-concurrency"); concurrency > 0 {
			result["insecure_concurrency"] = concurrency
		}
		if headers, ok := proxy["extra-headers"]; ok {
			result["extra_headers"] = headers
		}
	}

	// TLS 配置
	// 判断是否需要 TLS：
	// 1. 显式 tls: true
	// 2. h2/http 传输隐式需要 TLS
	// 3. Reality 是替代 TLS 的方案（也需要 TLS 结构体）
	// 4. 部分协议强制使用 TLS
	needsTLS := false
	hasExplicitTLS := false
	if tlsVal, ok := proxy["tls"]; ok && boolOrString(tlsVal) {
		needsTLS = true
		hasExplicitTLS = true
	}
	net := getString(proxy, "network")
	if net == "h2" || net == "http" {
		needsTLS = true
	}
	if _, hasReality := proxy["reality-opts"]; hasReality {
		needsTLS = true
	}
	// AnyTLS / Hysteria / Hysteria2 / Naive / Trojan / TUIC 必须使用 TLS
	switch typ {
	case "anytls", "hysteria", "hysteria2", "naive", "trojan", "tuic":
		needsTLS = true
		hasExplicitTLS = true
	}

	tls := map[string]any{}
	if needsTLS {
		if hasExplicitTLS || net == "h2" || net == "http" {
			tls["enabled"] = true
		}
		if sni := firstNonEmpty(getString(proxy, "sni"), getString(proxy, "servername")); sni != "" {
			tls["server_name"] = sni
		}
		if skip, ok := proxy["skip-cert-verify"]; ok && boolOrString(skip) {
			tls["insecure"] = true
		}
		// TUIC disable-sni
		if disableSni, ok := proxy["disable-sni"]; ok && boolOrString(disableSni) {
			tls["disable_sni"] = true
		}
		clientFingerprint := getString(proxy, "client-fingerprint")
		certificateFingerprint := getString(proxy, "fingerprint")
		if clientFingerprint == "" && isKnownUTLSFingerprint(certificateFingerprint) {
			clientFingerprint = certificateFingerprint
		} else if certificateFingerprint != "" && !isKnownUTLSFingerprint(certificateFingerprint) {
			normalizedFingerprint, ok := normalizeSHA256Hex(certificateFingerprint)
			if !ok {
				unsupportedReason = "certificate SHA-256 fingerprint is invalid"
			} else {
				tls["certificate_sha256"] = []string{normalizedFingerprint}
			}
		}
		if clientFingerprint != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": clientFingerprint}
		}
		if alpn, ok := proxy["alpn"]; ok {
			tls["alpn"] = alpn
		}
		if certificate := getString(proxy, "certificate"); certificate != "" {
			if strings.Contains(certificate, "BEGIN CERTIFICATE") {
				tls["client_certificate"] = []string{certificate}
			} else {
				tls["client_certificate_path"] = certificate
			}
		}
		if privateKey := getString(proxy, "private-key"); privateKey != "" {
			if strings.Contains(privateKey, "BEGIN") {
				tls["client_key"] = []string{privateKey}
			} else {
				tls["client_key_path"] = privateKey
			}
		}
		if echOptions, ok := proxy["ech-opts"].(map[string]any); ok && boolOrString(echOptions["enable"]) {
			ech := map[string]any{"enabled": true}
			if config, exists := echOptions["config"]; exists {
				normalizedConfig, err := normalizeClashECHConfig(config)
				if err != nil {
					unsupportedReason = "ECH config cannot be safely converted to sing-box PEM format"
				} else {
					ech["config"] = normalizedConfig
				}
			}
			if queryServerName := getString(echOptions, "query-server-name"); queryServerName != "" {
				ech["query_server_name"] = queryServerName
			}
			tls["ech"] = ech
		}

		// Reality 配置
		if realityOpts, ok := proxy["reality-opts"].(map[string]any); ok {
			reality := map[string]any{"enabled": true}
			if pk, ok := realityOpts["public-key"]; ok {
				reality["public_key"] = pk
			}
			if sid, ok := realityOpts["short-id"]; ok {
				reality["short_id"] = sid
			}
			tls["reality"] = reality
		}
	}
	if len(tls) > 0 {
		result["tls"] = tls
	}

	// Transport 配置
	network := getString(proxy, "network")
	if network == "" && typ == "vmess" {
		network = "tcp"
	}

	usesV2RayTransport := typ == "vmess" || typ == "vless" || typ == "trojan"
	if usesV2RayTransport && network != "" && network != "tcp" {
		transport := map[string]any{"type": network}
		switch network {
		case "ws":
			if wsOpts, ok := proxy["ws-opts"].(map[string]any); ok {
				transport["path"] = firstNonEmpty(getString(wsOpts, "path"), "/")
				if headers, ok := wsOpts["headers"].(map[string]any); ok {
					h := make(map[string]any)
					for k, v := range headers {
						h[k] = toStringValue(v)
					}
					transport["headers"] = h
				}
				if maxEarlyData := getInt(wsOpts, "max-early-data"); maxEarlyData > 0 {
					transport["max_early_data"] = maxEarlyData
				}
				if headerName := getString(wsOpts, "early-data-header-name"); headerName != "" {
					transport["early_data_header_name"] = headerName
				}
			}
		case "grpc":
			if grpcOpts, ok := proxy["grpc-opts"].(map[string]any); ok {
				if sn, ok := grpcOpts["grpc-service-name"]; ok {
					transport["service_name"] = sn
				}
			}
		case "http", "h2":
			transport["type"] = "http"
			if h2Opts, ok := proxy["h2-opts"].(map[string]any); ok {
				transport["path"] = firstNonEmpty(getString(h2Opts, "path"), "/")
				if host, ok := h2Opts["host"]; ok {
					transport["host"] = host
				}
			}
		case "httpupgrade":
			if upgradeOptions, ok := proxy["http-upgrade-opts"].(map[string]any); ok {
				if host := getString(upgradeOptions, "host"); host != "" {
					transport["host"] = host
				}
				if path := getString(upgradeOptions, "path"); path != "" {
					transport["path"] = path
				}
				if headers, ok := upgradeOptions["headers"].(map[string]any); ok {
					h := make(map[string]any)
					for k, v := range headers {
						h[k] = toStringValue(v)
					}
					transport["headers"] = h
				}
			}
		case "xhttp", "mkcp", "mekya":
			unsupportedReason = fmt.Sprintf("%s transport is not supported by sing-box", network)
			delete(transport, "type")
		default:
			unsupportedReason = fmt.Sprintf("%s transport is not supported by sing-box", network)
			delete(transport, "type")
		}
		if len(transport) > 0 {
			result["transport"] = transport
		}
		delete(result, "network")
	}

	// Plugin 配置
	if pluginOpts, ok := proxy["plugin-opts"]; typ == "shadowsocks" && ok {
		// sing-box plugin_opts 需要字符串格式：key=value;key2=value2
		if optsMap, isMap := pluginOpts.(map[string]any); isMap {
			// 对 key 排序以确保 UID 稳定性
			keys := make([]string, 0, len(optsMap))
			for k := range optsMap {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var parts []string
			for _, k := range keys {
				v := optsMap[k]
				// 简单值直接拼接，嵌套值转 JSON
				switch v.(type) {
				case string, bool, int, int64, float64:
					parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				default:
					if b, err := json.Marshal(v); err == nil {
						parts = append(parts, fmt.Sprintf("%s=%s", k, string(b)))
					}
				}
			}
			result["plugin_opts"] = strings.Join(parts, ";")
		} else {
			// 如果已经是字符串，直接使用
			result["plugin_opts"] = pluginOpts
		}
	}

	// Clash 的 udp 字段转换
	// udp: false 表示不支持 UDP，需要设置 network: "tcp"
	// udp: true 或不设置表示支持 UDP，sing-box 默认支持，无需配置
	if udp, ok := proxy["udp"]; ok && !boolOrString(udp) && clashSupportsNetworkField(typ) {
		result["network"] = "tcp"
	}

	return result, unsupportedReason
}

func copyClashField(dst, src map[string]any, key string) {
	if v, ok := src[key]; ok && v != nil {
		dst[key] = v
	}
}

func boolOrString(v any) bool {
	switch val := v.(type) {
	case bool:
		return val
	case string:
		return strings.EqualFold(val, "true") || val == "1"
	case int:
		return val != 0
	case float64:
		return val != 0
	default:
		return false
	}
}

func toStringValue(v any) string {
	switch val := v.(type) {
	case string:
		return val
	default:
		return ""
	}
}

func parseBandwidth(s string) int {
	// 解析 "100 Mbps" / "100Mbps" / "1 Gbps" → Mbps 值
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	// 提取数值部分
	val := 0
	multiplier := 1

	if strings.HasSuffix(s, "gbps") || strings.HasSuffix(s, "gb/s") {
		s = strings.TrimSuffix(s, "gbps")
		s = strings.TrimSuffix(s, "gb/s")
		multiplier = 1000
	} else if strings.HasSuffix(s, "mbps") || strings.HasSuffix(s, "mb/s") {
		s = strings.TrimSuffix(s, "mbps")
		s = strings.TrimSuffix(s, "mb/s")
		multiplier = 1
	} else if strings.HasSuffix(s, "kbps") || strings.HasSuffix(s, "kb/s") {
		s = strings.TrimSuffix(s, "kbps")
		s = strings.TrimSuffix(s, "kb/s")
		// kbps < 1 Mbps，直接舍弃（sing-box 按 Mbps 计）
		if n, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && n >= 1000 {
			return n / 1000 // 1000 kbps = 1 Mbps
		}
		return 0
	}
	s = strings.TrimSpace(s)

	if n, err := strconv.Atoi(s); err == nil {
		val = n * multiplier
	}
	return val
}

func ensureCIDR(ip string, isIPv6 bool) string {
	// 如果已经包含 /，直接返回
	if strings.Contains(ip, "/") {
		return ip
	}
	// IPv4 添加 /32，IPv6 添加 /128
	if isIPv6 {
		return ip + "/128"
	}
	return ip + "/32"
}
