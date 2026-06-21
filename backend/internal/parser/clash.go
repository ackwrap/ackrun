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
		normalized := normalizeClashProxy(proxy, normalizedType)
		rawJSON, _ := json.Marshal(normalized)
		nodes = append(nodes, model.ParsedNode{
			Name:       name,
			Type:       normalizedType,
			Server:     server,
			ServerPort: port,
			Raw:        string(rawJSON),
			RawJSON:    string(rawJSON),
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

func normalizeClashProxy(proxy map[string]any, typ string) map[string]any {
	result := map[string]any{
		"type":        typ,
		"server":      getString(proxy, "server"),
		"server_port": getInt(proxy, "port"),
	}

	copyClashField(result, proxy, "uuid")
	if alterId, ok := proxy["alterId"]; ok {
		result["alter_id"] = alterId
	}

	// Cipher 字段映射
	if cipher, ok := proxy["cipher"]; ok {
		if typ == "vmess" {
			result["security"] = cipher
		} else if typ == "shadowsocks" {
			result["method"] = cipher
		} else {
			result["cipher"] = cipher
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

	copyClashField(result, proxy, "password")
	copyClashField(result, proxy, "psk")
	copyClashField(result, proxy, "flow")
	copyClashField(result, proxy, "plugin")
	copyClashField(result, proxy, "protocol")
	copyClashField(result, proxy, "obfs")
	if v, ok := proxy["obfs-param"]; ok {
		result["obfs_param"] = v
	}
	copyClashField(result, proxy, "auth_str")
	if v, ok := proxy["private-key"]; ok {
		result["private_key"] = v
	}
	if v, ok := proxy["public-key"]; ok {
		result["public_key"] = v
	}
	if v, ok := proxy["preshared-key"]; ok {
		result["pre_shared_key"] = v
	}
	copyClashField(result, proxy, "reserved")
	copyClashField(result, proxy, "mtu")
	copyClashField(result, proxy, "username")

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

		// packet_encoding: packet-encoding 优先级最高
		if pe, ok := proxy["packet-encoding"]; ok && getString(proxy, "packet-encoding") != "" {
			result["packet_encoding"] = pe
		} else if pa, ok := proxy["packet-addr"]; ok && boolOrString(pa) {
			result["packet_encoding"] = "packetaddr"
		} else if xudp, ok := proxy["xudp"]; ok && boolOrString(xudp) {
			result["packet_encoding"] = "xudp"
		}
	}

	// Hysteria2 obfs 对象格式
	if typ == "hysteria2" {
		obfsType := getString(proxy, "obfs")
		obfsPassword := getString(proxy, "obfs-password")
		if obfsType != "" {
			result["obfs"] = map[string]any{
				"type":     obfsType,
				"password": obfsPassword,
			}
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
	}

	// WireGuard address 格式转换
	if typ == "wireguard" {
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
	}

	// TLS 配置
	// 判断是否需要 TLS：
	// 1. 显式 tls: true
	// 2. h2/http 传输隐式需要 TLS
	// 3. Reality 是替代 TLS 的方案（也需要 TLS 结构体）
	// 4. Trojan 协议自带 TLS，只有在提供 TLS 选项时才生成配置
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
	// Hysteria2 / TUIC 必须使用 TLS
	if typ == "hysteria2" || typ == "tuic" {
		needsTLS = true
		hasExplicitTLS = true
	}
	// Trojan: 只有在提供了 TLS 相关选项时才生成 tls 块
	if typ == "trojan" {
		sni := getString(proxy, "sni")
		servername := getString(proxy, "servername")
		fp := getString(proxy, "fingerprint")
		cfp := getString(proxy, "client-fingerprint")
		hasSkip := proxy["skip-cert-verify"] != nil
		hasAlpn := proxy["alpn"] != nil
		if sni != "" || servername != "" || fp != "" || cfp != "" || hasSkip || hasAlpn {
			needsTLS = true
		}
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
		certFingerprint := getString(proxy, "fingerprint")
		if clientFingerprint == "" && isKnownUTLSFingerprint(certFingerprint) {
			clientFingerprint = certFingerprint
			certFingerprint = ""
		}
		if clientFingerprint != "" {
			tls["utls"] = map[string]any{"enabled": true, "fingerprint": clientFingerprint}
		}
		if isSHA256Hex(certFingerprint) {
			tls["certificate_public_key_sha256"] = []string{certFingerprint}
		}
		if alpn, ok := proxy["alpn"]; ok {
			tls["alpn"] = alpn
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

	if network != "" && network != "tcp" {
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
		case "xhttp":
			transport["type"] = "httpupgrade"
			if xhttpOpts, ok := proxy["xhttp-opts"].(map[string]any); ok {
				if host := getString(xhttpOpts, "host"); host != "" {
					transport["host"] = host
				}
				if path := getString(xhttpOpts, "path"); path != "" {
					transport["path"] = path
				}
				if headers, ok := xhttpOpts["headers"].(map[string]any); ok {
					h := make(map[string]any)
					for k, v := range headers {
						h[k] = toStringValue(v)
					}
					transport["headers"] = h
				}
			}
		}
		result["transport"] = transport
		delete(result, "network")
	}

	// Plugin 配置
	if pluginOpts, ok := proxy["plugin-opts"]; ok {
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
	if udp, ok := proxy["udp"]; ok && !boolOrString(udp) {
		result["network"] = "tcp"
	}

	return result
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
