package parser

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parsedNodeFromMap(raw string, cfg map[string]any) (*model.ParsedNode, error) {
	name := firstNonEmpty(getString(cfg, "name"), getString(cfg, "server"))
	typ := strings.ToLower(getString(cfg, "type"))
	server := getString(cfg, "server")
	port := getInt(cfg, "port")
	if typ == "" || server == "" {
		return nil, fmt.Errorf("invalid proxy config")
	}

	// 统一字段转换为 sing-box 格式
	normalized := normalizeToSingbox(cfg, typ)

	rawJSON, _ := json.Marshal(normalized)
	return &model.ParsedNode{Name: name, Type: typ, Server: server, ServerPort: port, Raw: raw, RawJSON: string(rawJSON)}, nil
}

// normalizeToSingbox 将通用字段转换为 sing-box 格式
func normalizeToSingbox(cfg map[string]any, typ string) map[string]any {
	result := make(map[string]any)

	// 复制所有字段
	for k, v := range cfg {
		result[k] = v
	}

	// 移除不需要的字段
	delete(result, "name")
	typ = normalizeProtocolType(typ)
	result["type"] = typ

	// 字段转换
	if port, ok := result["port"]; ok {
		result["server_port"] = port
		delete(result, "port")
	}

	// Cipher/Method 转换
	if cipher, ok := result["cipher"]; ok {
		switch typ {
		case "vmess":
			result["security"] = cipher
		case "shadowsocks", "ssr":
			result["method"] = cipher
		}
		delete(result, "cipher")
	}
	if typ == "ssr" {
		moveKey(result, "obfs-param", "obfs_param")
		moveKey(result, "protocol-param", "protocol_param")
		delete(result, "group")
	}

	// AlterId 转换
	if alterId, ok := result["alterId"]; ok {
		if getInt(result, "alterId") > 0 {
			result["alter_id"] = alterId
		}
		delete(result, "alterId")
	}

	// Plugin opts 转换
	if pluginOpts, ok := result["plugin-opts"]; ok {
		result["plugin_opts"] = pluginOpts
		delete(result, "plugin-opts")
	}

	if typ == "wireguard" {
		moveKey(result, "private-key", "private_key")
		moveKey(result, "public-key", "public_key")
		moveKey(result, "preshared-key", "pre_shared_key")
		moveKey(result, "pre-shared-key", "pre_shared_key")
		moveKey(result, "local-address", "address")
	}

	// TLS 布尔值转嵌套对象
	if tlsVal, ok := result["tls"]; ok {
		if b, isBool := tlsVal.(bool); isBool {
			delete(result, "tls")
			if b {
				result["tls"] = map[string]any{"enabled": true}
			}
		}
	}

	// servername / sni 移入 TLS 对象
	tlsObj, tlsIsMap := result["tls"].(map[string]any)
	sniKeys := []string{"servername", "sni"}
	for _, key := range sniKeys {
		if val, ok := result[key]; ok {
			if str, isStr := val.(string); isStr && str != "" {
				if tlsIsMap {
					tlsObj["server_name"] = str
				} else {
					result["tls"] = map[string]any{"enabled": true, "server_name": str}
					tlsObj = result["tls"].(map[string]any)
					tlsIsMap = true
				}
			}
			delete(result, key)
		}
	}

	clientFingerprint := getString(result, "client-fingerprint")
	certFingerprint := getString(result, "fingerprint")
	if clientFingerprint == "" && isKnownUTLSFingerprint(certFingerprint) {
		clientFingerprint = certFingerprint
		certFingerprint = ""
	}
	if clientFingerprint != "" || isSHA256Hex(certFingerprint) {
		if !tlsIsMap {
			result["tls"] = map[string]any{"enabled": true}
			tlsObj = result["tls"].(map[string]any)
			tlsIsMap = true
		}
		if clientFingerprint != "" {
			tlsObj["utls"] = map[string]any{"enabled": true, "fingerprint": clientFingerprint}
		}
		if isSHA256Hex(certFingerprint) {
			tlsObj["certificate_public_key_sha256"] = []string{certFingerprint}
		}
	}
	delete(result, "client-fingerprint")
	delete(result, "fingerprint")

	return result
}

func moveKey(data map[string]any, from string, to string) {
	if value, ok := data[from]; ok {
		data[to] = value
		delete(data, from)
	}
}

func isKnownUTLSFingerprint(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "chrome", "firefox", "edge", "safari", "360", "qq", "ios", "android", "random", "randomized", "chrome_psk", "chrome_psk_shuffle", "chrome_padding_psk_shuffle", "chrome_pq", "chrome_pq_psk":
		return true
	default:
		return false
	}
}

func isSHA256Hex(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func base64DecodeURLSafe(value string) (string, error) {
	value = strings.ReplaceAll(value, "-", "+")
	value = strings.ReplaceAll(value, "_", "/")
	decoded, err := base64.StdEncoding.DecodeString(withPadding(value))
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

func parseQueryParams(query string) map[string]string {
	params := make(map[string]string)
	if query == "" {
		return params
	}
	for _, pair := range strings.Split(query, "&") {
		kv := strings.SplitN(pair, "=", 2)
		key := decodeURLValue(kv[0])
		value := ""
		if len(kv) == 2 {
			value = decodeURLValue(kv[1])
		}
		params[key] = value
	}
	return params
}

func getString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case int:
		return strconv.Itoa(val)
	case int64:
		return strconv.FormatInt(val, 10)
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", val)
	}
}

func getInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case int32:
		return int(val)
	case int64:
		return int(val)
	case uint64:
		return int(val)
	case float64:
		return int(val)
	case string:
		return parsePort(val)
	default:
		return 0
	}
}

func parsePort(value string) int {
	port, _ := strconv.Atoi(value)
	return port
}

func splitHostPortLoose(host string) (string, int) {
	server, portText, err := net.SplitHostPort(host)
	if err == nil {
		return server, parsePort(portText)
	}
	parts := strings.Split(host, ":")
	if len(parts) == 2 {
		return parts[0], parsePort(parts[1])
	}
	return host, 0
}

func parseServerPortWithDefault(serverPart string, defaultPort int) (string, int) {
	server, port := splitHostPortLoose(serverPart)
	if port == 0 {
		port = defaultPort
	}
	return server, port
}

func decodeURLValue(value string) string {
	value = strings.TrimSpace(value)
	if decoded, err := url.QueryUnescape(value); err == nil {
		return decoded
	}
	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
