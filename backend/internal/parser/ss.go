package parser

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

var knownSSCiphers = []string{
	"aes-128-gcm", "aes-192-gcm", "aes-256-gcm",
	"aes-128-cfb", "aes-192-cfb", "aes-256-cfb",
	"aes-128-ctr", "aes-192-ctr", "aes-256-ctr",
	"chacha20-ietf-poly1305", "xchacha20-ietf-poly1305",
	"chacha20-ietf", "chacha20", "xchacha20",
	"2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm",
	"2022-blake3-chacha20-poly1305", "rc4-md5", "none",
}

func parseShadowsocks(raw string) (*model.ParsedNode, error) {
	content := strings.TrimPrefix(raw, "ss://")
	name := "SS Node"
	mainPart := content
	query := map[string]string{}
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		mainPart = content[:idx]
		name = decodeURLValue(content[idx+1:])
	}
	if idx := strings.Index(mainPart, "?"); idx != -1 {
		query = parseQueryParams(mainPart[idx+1:])
		mainPart = mainPart[:idx]
	}
	mainPart = strings.TrimSuffix(mainPart, "/")

	var method, password, server string
	var port int
	if strings.Contains(mainPart, "@") {
		atIdx := strings.LastIndex(mainPart, "@")
		authPart := decodeURLValue(mainPart[:atIdx])
		server, port = splitHostPortLoose(mainPart[atIdx+1:])
		if cipher := matchSSCipher(authPart); cipher != "" {
			method = cipher
			password = authPart[len(cipher)+1:]
		} else {
			decoded, err := base64DecodeURLSafe(authPart)
			if err != nil {
				return nil, fmt.Errorf("decode ss auth: %w", err)
			}
			method, password = splitAuth(decoded)
		}
	} else {
		decoded, err := base64DecodeURLSafe(mainPart)
		if err != nil {
			return nil, fmt.Errorf("decode ss: %w", err)
		}
		atIdx := strings.LastIndex(decoded, "@")
		if atIdx == -1 {
			return nil, fmt.Errorf("invalid ss format")
		}
		method, password = splitAuth(decoded[:atIdx])
		server, port = splitHostPortLoose(decoded[atIdx+1:])
	}
	if method == "" || server == "" {
		return nil, fmt.Errorf("invalid ss url")
	}
	node := map[string]any{"name": name, "type": "ss", "server": server, "port": port, "cipher": method, "password": password, "udp": true}
	if plugin := query["plugin"]; plugin != "" {
		applySSPlugin(node, plugin)
	}
	if plugin := query["plugin"]; plugin != "" {
		if pluginName := extractPluginName(plugin); pluginName == "v2ray-plugin" {
			applyV2rayPlugin(node, plugin)
		}
	}
	if truthy(query["tls"]) || strings.EqualFold(query["tls"], "true") {
		node["tls"] = true
		applyTLSOptions(node, query, "servername")
	}
	if network := query["type"]; network != "" && network != "tcp" {
		applyTransportOptions(node, query)
	}
	return parsedNodeFromMap(raw, node)
}

func matchSSCipher(auth string) string {
	for _, cipher := range knownSSCiphers {
		if strings.HasPrefix(auth, cipher+":") {
			return cipher
		}
	}
	return ""
}

func splitAuth(auth string) (string, string) {
	idx := strings.Index(auth, ":")
	if idx == -1 {
		return "", ""
	}
	return auth[:idx], auth[idx+1:]
}

func extractPluginName(pluginRaw string) string {
	pluginText := decodeURLValue(pluginRaw)
	parts := strings.Split(pluginText, ";")
	if len(parts) == 0 {
		return ""
	}
	return strings.TrimSpace(parts[0])
}

func applySSPlugin(node map[string]any, pluginRaw string) {
	pluginText := decodeURLValue(pluginRaw)
	parts := strings.Split(pluginText, ";")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return
	}
	plugin := strings.TrimSpace(parts[0])
	if plugin == "obfs-local" || plugin == "simple-obfs" {
		plugin = "obfs"
	}
	node["plugin"] = plugin
	opts := map[string]any{}
	for _, part := range parts[1:] {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, value := kv[0], kv[1]
		switch plugin {
		case "obfs":
			if key == "obfs" {
				opts["mode"] = value
			} else if key == "obfs-host" || key == "host" {
				opts["host"] = value
			}
		default:
			opts[key] = value
		}
	}
	if len(opts) > 0 {
		node["plugin-opts"] = opts
	}
}

func applyV2rayPlugin(node map[string]any, pluginRaw string) {
	pluginText := decodeURLValue(pluginRaw)
	parts := strings.Split(pluginText, ";")
	opts := map[string]any{}
	for _, part := range parts[1:] {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			continue
		}
		key, value := kv[0], kv[1]
		switch key {
		case "mode":
			opts["mode"] = value
		case "host":
			opts["host"] = value
		case "path":
			opts["path"] = value
		case "mux":
			opts["mux"] = truthy(value)
		case "skip-cert-verify", "tls":
			opts[key] = truthy(value)
		}
	}
	if len(opts) > 0 {
		node["plugin-opts"] = opts
	}
}
