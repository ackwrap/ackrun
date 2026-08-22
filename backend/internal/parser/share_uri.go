package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

var shareableProxySchemes = map[string]bool{
	"anytls": true, "http": true, "https": true, "hy2": true,
	"hysteria": true, "hysteria2": true, "mieru": true, "naive": true,
	"naive+http": true, "naive+https": true, "snell": true, "socks": true,
	"socks4": true, "socks4a": true, "socks5": true, "ss": true,
	"ssr": true, "trojan": true, "tuic": true, "vless": true,
	"vmess": true, "wg": true, "wireguard": true, "ssh": true,
}

func EncodeProxyURI(node model.Node) (string, error) {
	if raw := reusableProxyURI(node.Raw); raw != "" {
		return raw, nil
	}

	var options map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &options); err != nil {
		return "", fmt.Errorf("节点配置不是有效 JSON")
	}
	switch strings.ToLower(strings.TrimSpace(node.Type)) {
	case "vmess":
		return encodeVMessURI(node, options)
	case "vless":
		return encodeVLESSURI(node, options)
	case "shadowsocks", "ss":
		return encodeShadowsocksURI(node, options)
	case "ssr":
		return encodeShadowsocksRURI(node, options)
	case "trojan":
		return encodeTrojanURI(node, options)
	case "socks", "socks4", "socks4a", "socks5":
		return encodeSocksURI(node, options)
	case "http", "https":
		return encodeHTTPURI(node, options)
	case "hysteria", "hysteria2":
		return encodeHysteriaURI(node, options)
	case "tuic":
		return encodeTUICURI(node, options)
	case "anytls":
		return encodeAnyTLSURI(node, options)
	case "wireguard", "wg":
		return encodeWireGuardURI(node, options)
	case "naive":
		return encodeNaiveURI(node, options)
	case "mieru":
		return encodeMieruURI(node, options)
	case "snell":
		return encodeSnellURI(node, options)
	case "ssh":
		return encodeSSHURI(node, options)
	default:
		return "", fmt.Errorf("%s 节点缺少可复用的原始分享链接", node.Type)
	}
}

func reusableProxyURI(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\r\n") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || !shareableProxySchemes[strings.ToLower(parsed.Scheme)] {
		return ""
	}
	if _, err := ParseProxyURI(raw); err != nil {
		return ""
	}
	return raw
}

func encodeVMessURI(node model.Node, options map[string]any) (string, error) {
	server, port, uuid, err := shareServerCredentials(node, options)
	if err != nil {
		return "", err
	}
	network, path, host, serviceName := shareTransport(options)
	if network == "grpc" {
		path = serviceName
	}
	tlsOptions := nestedMap(options, "tls")
	config := map[string]any{
		"v":    "2",
		"ps":   node.Name,
		"add":  server,
		"port": strconv.Itoa(port),
		"id":   uuid,
		"aid":  getInt(options, "alter_id"),
		"scy":  firstNonEmpty(getString(options, "security"), "auto"),
		"net":  network,
		"type": "none",
		"host": host,
		"path": path,
	}
	if boolOrString(tlsOptions["enabled"]) {
		config["tls"] = "tls"
	}
	copyTLSShareFields(config, tlsOptions)
	encoded, err := json.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("编码 VMess 分享配置: %w", err)
	}
	return "vmess://" + base64.StdEncoding.EncodeToString(encoded), nil
}

func encodeVLESSURI(node model.Node, options map[string]any) (string, error) {
	server, port, uuid, err := shareServerCredentials(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	query.Set("encryption", firstNonEmpty(getString(options, "encryption"), "none"))
	if flow := getString(options, "flow"); flow != "" {
		query.Set("flow", flow)
	}
	addTransportShareQuery(query, options)

	tlsOptions := nestedMap(options, "tls")
	if boolOrString(tlsOptions["enabled"]) {
		security := "tls"
		if reality := nestedMap(tlsOptions, "reality"); boolOrString(reality["enabled"]) {
			security = "reality"
			if publicKey := getString(reality, "public_key"); publicKey != "" {
				query.Set("pbk", publicKey)
			}
			if shortID := getString(reality, "short_id"); shortID != "" {
				query.Set("sid", shortID)
			}
		}
		query.Set("security", security)
		addTLSShareQuery(query, tlsOptions)
	}

	shareURL := url.URL{
		Scheme:   "vless",
		User:     url.User(uuid),
		Host:     net.JoinHostPort(server, strconv.Itoa(port)),
		RawQuery: query.Encode(),
		Fragment: node.Name,
	}
	return shareURL.String(), nil
}

func encodeShadowsocksURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	method := getString(options, "method")
	password := getString(options, "password")
	if method == "" {
		return "", fmt.Errorf("Shadowsocks 节点缺少 method")
	}
	auth := base64.RawURLEncoding.EncodeToString([]byte(method + ":" + password))
	query := url.Values{}
	if plugin := getString(options, "plugin"); plugin != "" {
		if pluginOptions := sharePluginOptions(options["plugin_opts"]); pluginOptions != "" {
			plugin += ";" + pluginOptions
		}
		query.Set("plugin", plugin)
	}
	addOptionalTLSAndTransportQuery(query, options)
	return buildOpaqueShareURI("ss", auth+"@"+net.JoinHostPort(server, strconv.Itoa(port)), query, node.Name), nil
}

func encodeShadowsocksRURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	protocol := getString(options, "protocol")
	method := getString(options, "method")
	obfs := getString(options, "obfs")
	password := getString(options, "password")
	if protocol == "" || method == "" || obfs == "" || password == "" {
		return "", fmt.Errorf("SSR 节点缺少 protocol、method、obfs 或 password")
	}
	base := strings.Join([]string{
		server,
		strconv.Itoa(port),
		protocol,
		method,
		obfs,
		base64.RawURLEncoding.EncodeToString([]byte(password)),
	}, ":")
	query := url.Values{}
	setBase64Query(query, "remarks", node.Name)
	setBase64Query(query, "obfsparam", getString(options, "obfs_param"))
	setBase64Query(query, "protoparam", getString(options, "protocol_param"))
	content := base
	if encodedQuery := query.Encode(); encodedQuery != "" {
		content += "/?" + encodedQuery
	}
	return "ssr://" + base64.RawURLEncoding.EncodeToString([]byte(content)), nil
}

func encodeTrojanURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	password := getString(options, "password")
	if password == "" {
		return "", fmt.Errorf("Trojan 节点缺少 password")
	}
	query := url.Values{}
	addTLSShareQuery(query, nestedMap(options, "tls"))
	addTransportShareQuery(query, options)
	return buildUserShareURI("trojan", password, "", server, port, query, node.Name), nil
}

func encodeSocksURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	scheme := "socks5"
	switch getString(options, "version") {
	case "4":
		scheme = "socks4"
	case "4a":
		scheme = "socks4a"
	}
	query := url.Values{}
	if enabled, exists := options["udp"]; exists {
		query.Set("udp", strconv.FormatBool(boolOrString(enabled)))
	}
	if uot, exists := options["udp_over_tcp"]; exists {
		query.Set("uot", strconv.FormatBool(boolOrString(uot)))
	}
	return buildUserShareURI(scheme, getString(options, "username"), getString(options, "password"), server, port, query, node.Name), nil
}

func encodeHTTPURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	tlsOptions := nestedMap(options, "tls")
	scheme := "http"
	if boolOrString(tlsOptions["enabled"]) || strings.EqualFold(node.Type, "https") {
		scheme = "https"
		addTLSShareQuery(query, tlsOptions)
	}
	return buildUserShareURI(scheme, getString(options, "username"), getString(options, "password"), server, port, query, node.Name), nil
}

func encodeHysteriaURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	typ := strings.ToLower(node.Type)
	scheme := "hysteria"
	auth := firstNonEmpty(getString(options, "auth_str"), getString(options, "auth"))
	if typ == "hysteria2" {
		scheme = "hysteria2"
		auth = getString(options, "password")
	}
	query := url.Values{}
	addTLSShareQuery(query, nestedMap(options, "tls"))
	if typ == "hysteria2" {
		addHysteria2ShareQuery(query, options)
	} else {
		addHysteria1ShareQuery(query, options)
	}
	return buildUserShareURI(scheme, auth, "", server, port, query, node.Name), nil
}

func encodeTUICURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	uuid := getString(options, "uuid")
	password := getString(options, "password")
	if uuid == "" || password == "" {
		return "", fmt.Errorf("TUIC 节点缺少 uuid 或 password")
	}
	query := url.Values{}
	addTLSShareQuery(query, nestedMap(options, "tls"))
	setStringQuery(query, "congestion_control", options, "congestion_control")
	setStringQuery(query, "udp_relay_mode", options, "udp_relay_mode")
	setBoolQuery(query, "reduce_rtt", options, "zero_rtt_handshake")
	setStringQuery(query, "heartbeat", options, "heartbeat")
	return buildUserShareURI("tuic", uuid, password, server, port, query, node.Name), nil
}

func encodeAnyTLSURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	password := getString(options, "password")
	if password == "" {
		return "", fmt.Errorf("AnyTLS 节点缺少 password")
	}
	query := url.Values{}
	addTLSShareQuery(query, nestedMap(options, "tls"))
	setStringQuery(query, "idle_session_check_interval", options, "idle_session_check_interval")
	setStringQuery(query, "idle_session_timeout", options, "idle_session_timeout")
	setIntQuery(query, "min_idle_session", options, "min_idle_session")
	return buildUserShareURI("anytls", password, "", server, port, query, node.Name), nil
}

func encodeWireGuardURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	setStringQuery(query, "private-key", options, "private_key")
	setStringQuery(query, "public-key", options, "public_key")
	setStringQuery(query, "preshared-key", options, "pre_shared_key")
	setListQuery(query, "address", options["address"])
	setListQuery(query, "reserved", options["reserved"])
	setIntQuery(query, "mtu", options, "mtu")
	setJSONQuery(query, "peers", options["peers"])
	return buildUserShareURI("wireguard", "", "", server, port, query, node.Name), nil
}

func encodeNaiveURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	tlsOptions := nestedMap(options, "tls")
	scheme := "naive+http"
	if boolOrString(tlsOptions["enabled"]) {
		scheme = "naive+https"
		addTLSShareQuery(query, tlsOptions)
	}
	return buildUserShareURI(scheme, getString(options, "username"), getString(options, "password"), server, port, query, node.Name), nil
}

func encodeMieruURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	setStringQuery(query, "protocol", options, "protocol")
	setStringQuery(query, "transport", options, "transport")
	setStringQuery(query, "multiplexing", options, "multiplexing")
	if tlsOptions := nestedMap(options, "tls"); boolOrString(tlsOptions["enabled"]) {
		query.Set("tls", "1")
		addTLSShareQuery(query, tlsOptions)
	}
	return buildUserShareURI("mieru", getString(options, "username"), getString(options, "password"), server, port, query, node.Name), nil
}

func encodeSnellURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	psk := getString(options, "psk")
	if psk == "" {
		return "", fmt.Errorf("Snell 节点缺少 psk")
	}
	query := url.Values{}
	setIntQuery(query, "version", options, "version")
	setStringQuery(query, "userkey", options, "userkey")
	setBoolQuery(query, "reuse", options, "reuse")
	setStringQuery(query, "network", options, "network")
	setStringQuery(query, "obfs_mode", options, "obfs_mode")
	setStringQuery(query, "obfs_host", options, "obfs_host")
	setStringQuery(query, "mode", options, "mode")
	return buildUserShareURI("snell", psk, "", server, port, query, node.Name), nil
}

func encodeSSHURI(node model.Node, options map[string]any) (string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", err
	}
	query := url.Values{}
	setStringQuery(query, "private_key_path", options, "private_key_path")
	setStringQuery(query, "private_key_passphrase", options, "private_key_passphrase")
	setStringQuery(query, "client_version", options, "client_version")
	addRepeatedQueryValues(query, "private_key", options["private_key"])
	addRepeatedQueryValues(query, "host_key", options["host_key"])
	addRepeatedQueryValues(query, "host_key_algorithms", options["host_key_algorithms"])
	addRepeatedQueryValues(query, "cipher", options["cipher"])
	addRepeatedQueryValues(query, "mac", options["mac"])
	addRepeatedQueryValues(query, "kex_algorithm", options["kex_algorithm"])
	user := firstNonEmpty(getString(options, "user"), getString(options, "username"))
	return buildUserShareURI("ssh", user, getString(options, "password"), server, port, query, node.Name), nil
}

func shareServerCredentials(node model.Node, options map[string]any) (string, int, string, error) {
	server, port, err := shareServerAddress(node, options)
	if err != nil {
		return "", 0, "", err
	}
	uuid := getString(options, "uuid")
	if uuid == "" {
		return "", 0, "", fmt.Errorf("%s 节点缺少生成分享链接所需的地址、端口或 UUID", node.Type)
	}
	return server, port, uuid, nil
}

func shareServerAddress(node model.Node, options map[string]any) (string, int, error) {
	server := firstNonEmpty(getString(options, "server"), node.Server)
	port := getInt(options, "server_port")
	if port <= 0 {
		port = node.ServerPort
	}
	if server == "" || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("%s 节点缺少有效地址或端口", node.Type)
	}
	return server, port, nil
}

func buildUserShareURI(scheme, username, password, server string, port int, query url.Values, name string) string {
	shareURL := url.URL{
		Scheme:   scheme,
		Host:     net.JoinHostPort(server, strconv.Itoa(port)),
		RawQuery: query.Encode(),
		Fragment: name,
	}
	if username != "" || password != "" {
		if password != "" {
			shareURL.User = url.UserPassword(username, password)
		} else {
			shareURL.User = url.User(username)
		}
	}
	return shareURL.String()
}

func buildOpaqueShareURI(scheme, authority string, query url.Values, name string) string {
	result := scheme + "://" + authority
	if encodedQuery := query.Encode(); encodedQuery != "" {
		result += "?" + encodedQuery
	}
	if name != "" {
		result += "#" + url.PathEscape(name)
	}
	return result
}

func addOptionalTLSAndTransportQuery(query url.Values, options map[string]any) {
	if tlsOptions := nestedMap(options, "tls"); boolOrString(tlsOptions["enabled"]) {
		query.Set("tls", "1")
		addTLSShareQuery(query, tlsOptions)
	}
	if _, exists := options["transport"]; exists {
		addTransportShareQuery(query, options)
	}
}

func sharePluginOptions(value any) string {
	if text, ok := value.(string); ok {
		return text
	}
	options, ok := value.(map[string]any)
	if !ok || len(options) == 0 {
		return ""
	}
	keys := make([]string, 0, len(options))
	for key := range options {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := scalarString(options[key])
		if value == "" || value == "true" {
			parts = append(parts, key)
		} else {
			parts = append(parts, key+"="+value)
		}
	}
	return strings.Join(parts, ";")
}

func addHysteria1ShareQuery(query url.Values, options map[string]any) {
	setFirstStringQuery(query, "up", options, "up_mbps", "up")
	setFirstStringQuery(query, "down", options, "down_mbps", "down")
	if obfs, ok := options["obfs"].(string); ok && obfs != "" {
		query.Set("obfs", obfs)
	}
	setFirstStringQuery(query, "obfs-param", options, "obfs_param", "obfs-param")
	setIntQuery(query, "recv_window", options, "recv_window")
	setIntQuery(query, "recv_window_conn", options, "recv_window_conn")
	setBoolQuery(query, "disable_mtu_discovery", options, "disable_mtu_discovery")
	setListQuery(query, "mport", options["server_ports"])
}

func addHysteria2ShareQuery(query url.Values, options map[string]any) {
	setListQuery(query, "mport", options["server_ports"])
	setStringQuery(query, "hop_interval", options, "hop_interval")
	setStringQuery(query, "hop_interval_max", options, "hop_interval_max")
	setStringQuery(query, "bbr_profile", options, "bbr_profile")
	setIntQuery(query, "up", options, "up_mbps")
	setIntQuery(query, "down", options, "down_mbps")
	setBoolQuery(query, "brutal_debug", options, "brutal_debug")
	if obfs := nestedMap(options, "obfs"); len(obfs) > 0 {
		setStringQuery(query, "obfs", obfs, "type")
		setStringQuery(query, "obfs-password", obfs, "password")
		setIntQuery(query, "min_packet_size", obfs, "min_packet_size")
		setIntQuery(query, "max_packet_size", obfs, "max_packet_size")
	}
	if pin := firstString(nestedMap(options, "tls")["certificate_sha256"]); pin != "" {
		query.Set("pinSHA256", pin)
	}
}

func setBase64Query(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, base64.RawURLEncoding.EncodeToString([]byte(value)))
	}
}

func setStringQuery(query url.Values, queryKey string, options map[string]any, optionKey string) {
	if value := getString(options, optionKey); value != "" {
		query.Set(queryKey, value)
	}
}

func setFirstStringQuery(query url.Values, queryKey string, options map[string]any, optionKeys ...string) {
	for _, optionKey := range optionKeys {
		if value := scalarString(options[optionKey]); value != "" {
			query.Set(queryKey, value)
			return
		}
	}
}

func setIntQuery(query url.Values, queryKey string, options map[string]any, optionKey string) {
	if value := getInt(options, optionKey); value > 0 {
		query.Set(queryKey, strconv.Itoa(value))
	}
}

func setBoolQuery(query url.Values, queryKey string, options map[string]any, optionKey string) {
	if value, exists := options[optionKey]; exists && boolOrString(value) {
		query.Set(queryKey, "1")
	}
}

func setListQuery(query url.Values, queryKey string, value any) {
	values := scalarValues(value)
	if len(values) > 0 {
		query.Set(queryKey, strings.Join(values, ","))
	}
}

func addRepeatedQueryValues(query url.Values, queryKey string, value any) {
	for _, item := range scalarValues(value) {
		query.Add(queryKey, item)
	}
}

func setJSONQuery(query url.Values, queryKey string, value any) {
	if text := scalarString(value); text != "" {
		query.Set(queryKey, text)
		return
	}
	if value == nil {
		return
	}
	encoded, err := json.Marshal(value)
	if err == nil && string(encoded) != "null" {
		query.Set(queryKey, string(encoded))
	}
}

func scalarValues(value any) []string {
	switch values := value.(type) {
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text := scalarString(value); text != "" {
				result = append(result, text)
			}
		}
		return result
	case []string:
		return values
	default:
		if text := scalarString(value); text != "" {
			return []string{text}
		}
	}
	return nil
}

func scalarString(value any) string {
	switch value := value.(type) {
	case string:
		return value
	case json.Number:
		return value.String()
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(value), 'f', -1, 32)
	case int:
		return strconv.Itoa(value)
	case int64:
		return strconv.FormatInt(value, 10)
	case bool:
		return strconv.FormatBool(value)
	default:
		return ""
	}
}

func addTransportShareQuery(query url.Values, options map[string]any) {
	network, path, host, serviceName := shareTransport(options)
	query.Set("type", network)
	if path != "" {
		query.Set("path", path)
	}
	if host != "" {
		query.Set("host", host)
	}
	if serviceName != "" {
		query.Set("serviceName", serviceName)
	}
}

func shareTransport(options map[string]any) (network, path, host, serviceName string) {
	transport := nestedMap(options, "transport")
	network = strings.ToLower(getString(transport, "type"))
	switch network {
	case "", "tcp":
		network = "tcp"
	case "websocket":
		network = "ws"
	case "http_upgrade", "http-upgrade":
		network = "httpupgrade"
	}
	path = getString(transport, "path")
	serviceName = getString(transport, "service_name")
	host = firstString(transport["host"])
	if headers := nestedMap(transport, "headers"); host == "" {
		host = firstNonEmpty(getString(headers, "Host"), getString(headers, "host"))
	}
	return
}

func copyTLSShareFields(target map[string]any, tlsOptions map[string]any) {
	if serverName := getString(tlsOptions, "server_name"); serverName != "" {
		target["sni"] = serverName
	}
	if alpn := strings.Join(stringValues(tlsOptions["alpn"]), ","); alpn != "" {
		target["alpn"] = alpn
	}
	if utls := nestedMap(tlsOptions, "utls"); boolOrString(utls["enabled"]) {
		target["fp"] = getString(utls, "fingerprint")
	}
	if boolOrString(tlsOptions["insecure"]) {
		target["allowInsecure"] = 1
	}
}

func addTLSShareQuery(query url.Values, tlsOptions map[string]any) {
	if serverName := getString(tlsOptions, "server_name"); serverName != "" {
		query.Set("sni", serverName)
	}
	if alpn := strings.Join(stringValues(tlsOptions["alpn"]), ","); alpn != "" {
		query.Set("alpn", alpn)
	}
	if boolOrString(tlsOptions["insecure"]) {
		query.Set("allowInsecure", "1")
	}
	if utls := nestedMap(tlsOptions, "utls"); boolOrString(utls["enabled"]) {
		if fingerprint := getString(utls, "fingerprint"); fingerprint != "" {
			query.Set("fp", fingerprint)
		}
	}
}

func nestedMap(source map[string]any, key string) map[string]any {
	value, _ := source[key].(map[string]any)
	return value
}

func firstString(value any) string {
	values := stringValues(value)
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func stringValues(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	case string:
		if values != "" {
			return []string{values}
		}
	}
	return nil
}
