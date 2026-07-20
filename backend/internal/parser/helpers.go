package parser

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
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
	normalized, err := normalizeToSingbox(cfg, typ)
	if err != nil {
		return nil, err
	}

	rawJSON, _ := json.Marshal(normalized)
	return &model.ParsedNode{Name: name, Type: typ, Server: server, ServerPort: port, Raw: raw, RawJSON: string(rawJSON)}, nil
}

// normalizeToSingbox 将通用字段转换为 sing-box 格式
func normalizeToSingbox(cfg map[string]any, typ string) (map[string]any, error) {
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
	if typ == "hysteria" {
		moveKey(result, "obfs-param", "obfs")
		moveKey(result, "obfs_param", "obfs")
		moveKey(result, "receive-window", "recv_window")
		moveKey(result, "receive_window", "recv_window")
		moveKey(result, "receive-window-conn", "recv_window_conn")
		moveKey(result, "receive_window_conn", "recv_window_conn")
		moveKey(result, "disable-mtu-discovery", "disable_mtu_discovery")
		for _, key := range []string{"recv_window", "recv_window_conn"} {
			if value := getInt(result, key); value > 0 {
				result[key] = value
			}
		}
	}
	if typ == "tuic" {
		moveKey(result, "udp-relay-mode", "udp_relay_mode")
		moveKey(result, "reduce-rtt", "zero_rtt_handshake")
		moveKey(result, "reduce_rtt", "zero_rtt_handshake")
		moveKey(result, "zero-rtt-handshake", "zero_rtt_handshake")
		if token := getString(result, "token"); token != "" {
			delete(result, "token")
			if getString(result, "uuid") == "" || getString(result, "password") == "" {
				return nil, fmt.Errorf("legacy TUIC token authentication is not supported by the current sing-box version")
			}
		}
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
	sniKeys := []string{"servername", "server_name", "sni"}
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
	if alpn, ok := result["alpn"]; ok {
		if !tlsIsMap {
			result["tls"] = map[string]any{"enabled": true}
			tlsObj = result["tls"].(map[string]any)
			tlsIsMap = true
		}
		tlsObj["alpn"] = alpn
		delete(result, "alpn")
	}
	for _, key := range []string{"skip-cert-verify", "skip_cert_verify", "insecure"} {
		if insecure, ok := result[key]; ok {
			if !tlsIsMap {
				result["tls"] = map[string]any{"enabled": true}
				tlsObj = result["tls"].(map[string]any)
				tlsIsMap = true
			}
			tlsObj["insecure"] = boolOrString(insecure)
			delete(result, key)
		}
	}

	clientFingerprint := firstNonEmpty(getString(result, "client-fingerprint"), getString(result, "client_fingerprint"))
	certFingerprint := firstNonEmpty(getString(result, "certificate-sha256"), getString(result, "certificate_sha256"), getString(result, "fingerprint"))
	if clientFingerprint == "" && isKnownUTLSFingerprint(certFingerprint) {
		clientFingerprint = certFingerprint
		certFingerprint = ""
	}
	normalizedCertificateFingerprint, hasCertificateFingerprint := normalizeSHA256Hex(certFingerprint)
	if typ == "shadowsocks" && certFingerprint != "" && !hasCertificateFingerprint && clientFingerprint == "" {
		if !tlsIsMap {
			result["tls"] = map[string]any{"enabled": true}
			tlsObj = result["tls"].(map[string]any)
			tlsIsMap = true
		}
	}
	if clientFingerprint != "" || hasCertificateFingerprint {
		if !tlsIsMap {
			result["tls"] = map[string]any{"enabled": true}
			tlsObj = result["tls"].(map[string]any)
			tlsIsMap = true
		}
		if clientFingerprint != "" {
			tlsObj["utls"] = map[string]any{"enabled": true, "fingerprint": clientFingerprint}
		}
		if hasCertificateFingerprint {
			tlsObj["certificate_sha256"] = []string{normalizedCertificateFingerprint}
		}
	}
	delete(result, "client-fingerprint")
	delete(result, "client_fingerprint")
	delete(result, "fingerprint")
	delete(result, "certificate-sha256")
	delete(result, "certificate_sha256")
	normalizeRealityOptions(result)
	if typ == "shadowsocks" {
		if err := normalizeShadowsocksPlugin(result); err != nil {
			return nil, err
		}
	}
	if reason := normalizeV2RayTransport(result, result, typ); reason != "" {
		return nil, fmt.Errorf("%s", reason)
	}
	if typ == "socks" {
		if tlsOptions, ok := result["tls"].(map[string]any); ok && boolOrString(tlsOptions["enabled"]) {
			return nil, fmt.Errorf("TLS-wrapped SOCKS is not supported by the current sing-box version")
		}
	}

	return result, nil
}

func normalizeShadowsocksPlugin(config map[string]any) error {
	if tlsValue, exists := config["tls"]; exists {
		switch value := tlsValue.(type) {
		case nil:
			delete(config, "tls")
		case bool:
			if !value {
				delete(config, "tls")
			} else {
				return fmt.Errorf("TLS-wrapped Shadowsocks is not supported by the current sing-box version")
			}
		case string:
			switch strings.ToLower(strings.TrimSpace(value)) {
			case "", "0", "false":
				delete(config, "tls")
			case "1", "true":
				return fmt.Errorf("TLS-wrapped Shadowsocks is not supported by the current sing-box version")
			default:
				return fmt.Errorf("invalid Shadowsocks TLS value")
			}
		default:
			return fmt.Errorf("TLS-wrapped Shadowsocks is not supported by the current sing-box version")
		}
	}

	plugin := strings.TrimSpace(getString(config, "plugin"))
	network := strings.ToLower(strings.TrimSpace(getString(config, "network")))
	if network != "" && network != "tcp" && network != "udp" {
		return fmt.Errorf("shadowsocks transport %q must be configured through an explicit SIP003 plugin", network)
	}
	normalizedPlugin, normalizedOptions, err := NormalizeShadowsocksSIP003Plugin(plugin, config["plugin_opts"])
	if err != nil {
		return err
	}
	if normalizedPlugin == "" {
		delete(config, "plugin")
		delete(config, "plugin_opts")
		return nil
	}
	config["plugin"] = normalizedPlugin
	if normalizedOptions == "" {
		delete(config, "plugin_opts")
	} else {
		config["plugin_opts"] = normalizedOptions
	}
	return nil
}

type sip003PluginOption struct {
	value    string
	hasValue bool
}

// NormalizeShadowsocksSIP003Plugin validates the SIP003 plugins implemented by
// the bundled sing-box and emits a stable option string without dropping fields.
func NormalizeShadowsocksSIP003Plugin(plugin string, rawOptions any) (string, string, error) {
	plugin = strings.ToLower(strings.TrimSpace(plugin))
	switch plugin {
	case "obfs", "simple-obfs":
		plugin = "obfs-local"
	case "", "obfs-local", "v2ray-plugin":
	default:
		return "", "", fmt.Errorf("unsupported Shadowsocks SIP003 plugin %q", plugin)
	}

	options, err := normalizeSIP003OptionInput(rawOptions)
	if err != nil {
		return "", "", fmt.Errorf("invalid Shadowsocks plugin_opts: %w", err)
	}
	if plugin == "" {
		if len(options) > 0 {
			return "", "", fmt.Errorf("Shadowsocks plugin_opts requires an explicit SIP003 plugin")
		}
		return "", "", nil
	}

	canonical := make(map[string]sip003PluginOption, len(options))
	for key, option := range options {
		normalizedKey := key
		if plugin == "obfs-local" {
			switch key {
			case "mode":
				normalizedKey = "obfs"
			case "host":
				normalizedKey = "obfs-host"
			case "obfs", "obfs-host":
			default:
				return "", "", fmt.Errorf("unsupported obfs-local option %q", key)
			}
			if _, exists := canonical[normalizedKey]; exists {
				return "", "", fmt.Errorf("duplicate obfs-local option %q", normalizedKey)
			}
			if normalizedKey == "obfs" {
				mode := strings.ToLower(option.value)
				if mode != "http" && mode != "tls" {
					return "", "", fmt.Errorf("unsupported obfs-local mode %q", option.value)
				}
				option = sip003PluginOption{value: mode, hasValue: true}
			} else {
				option = sip003PluginOption{value: option.value, hasValue: true}
			}
			canonical[normalizedKey] = option
			continue
		}

		switch key {
		case "tls":
			canonical[key] = sip003PluginOption{}
		case "mode":
			mode := strings.ToLower(option.value)
			if mode != "websocket" && mode != "quic" {
				return "", "", fmt.Errorf("unsupported v2ray-plugin mode %q", option.value)
			}
			canonical[key] = sip003PluginOption{value: mode, hasValue: true}
		case "mux":
			mux, parseErr := strconv.Atoi(strings.TrimSpace(option.value))
			if parseErr != nil || mux < 0 {
				return "", "", fmt.Errorf("invalid v2ray-plugin mux value %q", option.value)
			}
			canonical[key] = sip003PluginOption{value: strconv.Itoa(mux), hasValue: true}
		case "host", "path", "cert", "certRaw":
			canonical[key] = sip003PluginOption{value: option.value, hasValue: true}
		default:
			return "", "", fmt.Errorf("unsupported v2ray-plugin option %q", key)
		}
	}
	return plugin, encodeSIP003PluginOptions(canonical), nil
}

func normalizeSIP003OptionInput(raw any) (map[string]sip003PluginOption, error) {
	switch options := raw.(type) {
	case nil:
		return map[string]sip003PluginOption{}, nil
	case string:
		return parseSIP003PluginOptions(options)
	case map[string]any:
		result := make(map[string]sip003PluginOption, len(options))
		for rawKey, rawValue := range options {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				return nil, fmt.Errorf("empty option key")
			}
			switch value := rawValue.(type) {
			case string:
				result[key] = sip003PluginOption{value: value, hasValue: true}
			case bool:
				switch key {
				case "tls":
					if value {
						result[key] = sip003PluginOption{}
					}
				case "mux":
					if value {
						result[key] = sip003PluginOption{value: "1", hasValue: true}
					} else {
						result[key] = sip003PluginOption{value: "0", hasValue: true}
					}
				default:
					return nil, fmt.Errorf("boolean value is not valid for option %q", key)
				}
			case int:
				result[key] = sip003PluginOption{value: strconv.Itoa(value), hasValue: true}
			case int64:
				result[key] = sip003PluginOption{value: strconv.FormatInt(value, 10), hasValue: true}
			case float64:
				if value != float64(int64(value)) {
					return nil, fmt.Errorf("non-integer value is not valid for option %q", key)
				}
				result[key] = sip003PluginOption{value: strconv.FormatInt(int64(value), 10), hasValue: true}
			default:
				return nil, fmt.Errorf("option %q has unsupported value type %T", key, rawValue)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unsupported option container %T", raw)
	}
}

func parseSIP003PluginOptions(value string) (map[string]sip003PluginOption, error) {
	result := map[string]sip003PluginOption{}
	for offset := 0; offset < len(value); {
		key, hasValue, optionValue, next, err := parseSIP003PluginOption(value, offset)
		if err != nil {
			return nil, err
		}
		if _, exists := result[key]; exists {
			return nil, fmt.Errorf("duplicate option %q", key)
		}
		result[key] = sip003PluginOption{value: optionValue, hasValue: hasValue}
		offset = next
	}
	return result, nil
}

func parseSIP003PluginOption(value string, offset int) (string, bool, string, int, error) {
	readPart := func(start int, stopAtEquals bool) (string, byte, int, error) {
		var part strings.Builder
		for i := start; i < len(value); i++ {
			switch value[i] {
			case '\\':
				i++
				if i >= len(value) {
					return "", 0, 0, fmt.Errorf("nothing follows final escape")
				}
				part.WriteByte(value[i])
			case ';':
				return part.String(), ';', i + 1, nil
			case '=':
				if stopAtEquals {
					return part.String(), '=', i + 1, nil
				}
				part.WriteByte(value[i])
			default:
				part.WriteByte(value[i])
			}
		}
		return part.String(), 0, len(value), nil
	}
	key, delimiter, next, err := readPart(offset, true)
	if err != nil {
		return "", false, "", 0, err
	}
	if key == "" {
		return "", false, "", 0, fmt.Errorf("empty option key")
	}
	if delimiter != '=' {
		return key, false, "1", next, nil
	}
	optionValue, _, end, err := readPart(next, false)
	return key, true, optionValue, end, err
}

func encodeSIP003PluginOptions(options map[string]sip003PluginOption) string {
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		option := options[key]
		part := escapePluginOption(key)
		if option.hasValue {
			part += "=" + escapePluginOption(option.value)
		}
		parts = append(parts, part)
	}
	return strings.Join(parts, ";")
}

func escapePluginOption(value string) string {
	return strings.NewReplacer("\\", "\\\\", ";", "\\;", "=", "\\=").Replace(value)
}

func normalizeRealityOptions(result map[string]any) {
	realityOpts, ok := result["reality-opts"].(map[string]any)
	if !ok {
		delete(result, "reality-opts")
		return
	}
	delete(result, "reality-opts")
	tls, ok := result["tls"].(map[string]any)
	if !ok {
		tls = map[string]any{}
		result["tls"] = tls
	}
	tls["enabled"] = true
	reality := map[string]any{"enabled": true}
	if publicKey := firstNonEmpty(getString(realityOpts, "public-key"), getString(realityOpts, "public_key"), getString(realityOpts, "pbk")); publicKey != "" {
		reality["public_key"] = publicKey
	}
	if shortID := firstNonEmpty(getString(realityOpts, "short-id"), getString(realityOpts, "short_id"), getString(realityOpts, "sid")); shortID != "" {
		reality["short_id"] = shortID
	}
	tls["reality"] = reality
}

func normalizeV2RayTransport(source, result map[string]any, typ string) string {
	usesV2RayTransport := typ == "vmess" || typ == "vless" || typ == "trojan"
	network := strings.ToLower(getString(source, "network"))
	legacyOptionKeys := []string{"ws-opts", "grpc-opts", "h2-opts", "http-upgrade-opts", "xhttp-opts"}
	defer func() {
		for _, key := range legacyOptionKeys {
			delete(result, key)
		}
	}()
	if !usesV2RayTransport || network == "" || network == "tcp" || network == "udp" {
		return ""
	}
	transport := map[string]any{"type": network}
	switch network {
	case "ws":
		if options, ok := source["ws-opts"].(map[string]any); ok {
			transport["path"] = firstNonEmpty(getString(options, "path"), "/")
			if headers := stringMap(options["headers"]); len(headers) > 0 {
				transport["headers"] = headers
			}
			if maxEarlyData := getInt(options, "max-early-data"); maxEarlyData > 0 {
				transport["max_early_data"] = maxEarlyData
			}
			if headerName := getString(options, "early-data-header-name"); headerName != "" {
				transport["early_data_header_name"] = headerName
			}
		}
	case "grpc":
		if options, ok := source["grpc-opts"].(map[string]any); ok {
			if serviceName := getString(options, "grpc-service-name"); serviceName != "" {
				transport["service_name"] = serviceName
			}
		}
	case "http", "h2":
		transport["type"] = "http"
		if options, ok := source["h2-opts"].(map[string]any); ok {
			transport["path"] = firstNonEmpty(getString(options, "path"), "/")
			if host, exists := options["host"]; exists {
				transport["host"] = host
			}
		}
	case "httpupgrade":
		if options, ok := source["http-upgrade-opts"].(map[string]any); ok {
			if host := getString(options, "host"); host != "" {
				transport["host"] = host
			}
			if path := getString(options, "path"); path != "" {
				transport["path"] = path
			}
			if headers := stringMap(options["headers"]); len(headers) > 0 {
				transport["headers"] = headers
			}
		}
	default:
		delete(result, "network")
		return fmt.Sprintf("%s transport is not supported by sing-box", network)
	}
	result["transport"] = transport
	delete(result, "network")
	return ""
}

func stringMap(value any) map[string]any {
	result := map[string]any{}
	switch headers := value.(type) {
	case map[string]any:
		for key, item := range headers {
			result[key] = toStringValue(item)
		}
	case map[string]string:
		for key, item := range headers {
			result[key] = item
		}
	}
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

func normalizeSHA256Hex(value string) (string, bool) {
	value = strings.ToLower(strings.NewReplacer(":", "", "-", "").Replace(strings.TrimSpace(value)))
	if len(value) != 64 {
		return "", false
	}
	_, err := hex.DecodeString(value)
	return value, err == nil
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
