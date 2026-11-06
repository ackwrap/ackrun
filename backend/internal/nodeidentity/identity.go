// Package nodeidentity owns the connection-field whitelist shared by parsers and storage.
package nodeidentity

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"

	"github.com/ackwrap/ackrun/internal/model"
)

func StableUID(node model.ParsedNode) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil || cfg == nil {
		cfg = map[string]any{"type": node.Type, "server": node.Server, "server_port": node.ServerPort}
	}
	encoded, _ := json.Marshal(nodeIdentityFields(cfg, node))
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:])[:16]
}

func nodeIdentityFields(cfg map[string]any, node model.ParsedNode) map[string]any {
	nodeType := firstNodeString(node.Type, nodeString(cfg, "type"))
	identity := map[string]any{
		"type": nodeType, "server": firstNodeString(node.Server, nodeString(cfg, "server")),
		"port": firstNodeInt(node.ServerPort, nodeInt(cfg, "server_port"), nodeInt(cfg, "port")),
	}
	copyNodeIdentityString(identity, cfg, "uuid", "id")
	copyNodeIdentityString(identity, cfg, "password")
	copyNodeIdentityString(identity, cfg, "cipher", "method", "security")
	copyNodeIdentityString(identity, cfg, "flow")
	copyNodeIdentityString(identity, cfg, "encryption")
	copyNodeIdentityString(identity, cfg, "username")
	copyNodeIdentityString(identity, cfg, "protocol")
	copyNodeIdentityString(identity, cfg, "obfs")
	copyNodeIdentityString(identity, cfg, "obfs-param")
	copyNodeIdentityString(identity, cfg, "psk")
	copyNodeIdentityString(identity, cfg, "private-key")
	copyNodeIdentityString(identity, cfg, "public-key")
	copyNodeIdentityString(identity, cfg, "preshared-key")
	copyNodeIdentityString(identity, cfg, "local-address", "address")
	copyNodeIdentityInt(identity, cfg, "mtu")
	if nodeType == "wireguard" && cfg["peers"] != nil {
		copyNodeIdentityValue(identity, cfg, "private_key", "private_key", "private-key")
		copyNodeIdentityValue(identity, cfg, "address", "address", "local_address", "local-address")
		copyNodeIdentityValue(identity, cfg, "peers", "peers")
		copyNodeIdentityString(identity, cfg, "public_key", "public_key", "public-key", "peer_public_key")
		copyNodeIdentityString(identity, cfg, "pre_shared_key", "pre_shared_key", "pre-shared-key", "preshared-key")
	}
	if nodeType == "ssh" {
		sshUser := firstNodeString(nodeString(cfg, "username"), nodeString(cfg, "user"))
		if sshUser == "" {
			sshUser = "root"
		}
		identity["username"] = sshUser
		copyNodeIdentityValue(identity, cfg, "private_key", "private_key", "private-key")
		copyNodeIdentityString(identity, cfg, "private_key_path", "private-key-path")
		copyNodeIdentityString(identity, cfg, "private_key_passphrase", "private-key-passphrase")
		copyNodeIdentityValue(identity, cfg, "host_key", "host-key")
		copyNodeIdentityValue(identity, cfg, "host_key_algorithms", "host-key-algorithms")
		copyNodeIdentityString(identity, cfg, "client_version", "client-version")
		copyNodeIdentityValue(identity, cfg, "ssh_cipher", "cipher")
		copyNodeIdentityValue(identity, cfg, "mac", "mac")
		copyNodeIdentityValue(identity, cfg, "kex_algorithm", "kex_algorithm")
	}
	if tls := nodeTLSIdentity(cfg); len(tls) > 0 {
		identity["tls"] = tls
	}
	if transport := nodeTransportIdentity(cfg); len(transport) > 0 {
		identity["transport"] = transport
	}
	if reality := nodeRealityIdentity(cfg); len(reality) > 0 {
		identity["reality"] = reality
	}
	if chain, err := model.DecodeNodeChain(cfg); err == nil && chain != nil {
		identity["chain_detour"] = chain.Identity
	}
	return identity
}

func nodeTLSIdentity(cfg map[string]any) map[string]any {
	tls := map[string]any{}
	if enabled, ok := cfg["tls"].(bool); ok {
		tls["enabled"] = enabled
	}
	if tlsCfg, ok := cfg["tls"].(map[string]any); ok {
		if enabled, ok := tlsCfg["enabled"].(bool); ok {
			tls["enabled"] = enabled
		}
		if serverName := firstNodeString(nodeString(tlsCfg, "server_name"), nodeString(tlsCfg, "servername"), nodeString(tlsCfg, "sni")); serverName != "" {
			tls["server_name"] = serverName
		}
		if utlsCfg, ok := tlsCfg["utls"].(map[string]any); ok {
			if fingerprint := nodeString(utlsCfg, "fingerprint"); fingerprint != "" {
				tls["fingerprint"] = fingerprint
			}
		}
	}
	if serverName := firstNodeString(nodeString(cfg, "server_name"), nodeString(cfg, "servername"), nodeString(cfg, "sni")); serverName != "" {
		tls["server_name"] = serverName
	}
	if fingerprint := firstNodeString(nodeString(cfg, "client-fingerprint"), nodeString(cfg, "fingerprint"), nodeString(cfg, "fp")); fingerprint != "" {
		tls["fingerprint"] = fingerprint
	}
	if alpn, ok := cfg["alpn"]; ok {
		tls["alpn"] = alpn
	}
	return tls
}

func nodeTransportIdentity(cfg map[string]any) map[string]any {
	transport := map[string]any{}
	if transportCfg, ok := cfg["transport"].(map[string]any); ok {
		copyNodeIdentityString(transport, transportCfg, "type")
		copyNodeIdentityString(transport, transportCfg, "path")
		if headers, ok := transportCfg["headers"].(map[string]any); ok {
			if host := firstNodeString(nodeString(headers, "Host"), nodeString(headers, "host")); host != "" {
				transport["host"] = host
			}
		}
	}
	if network := nodeString(cfg, "network"); network != "" {
		transport["type"] = network
	}
	if wsOpts, ok := cfg["ws-opts"].(map[string]any); ok {
		transport["type"] = "ws"
		copyNodeIdentityString(transport, wsOpts, "path")
		if headers, ok := wsOpts["headers"].(map[string]any); ok {
			if host := firstNodeString(nodeString(headers, "Host"), nodeString(headers, "host")); host != "" {
				transport["host"] = host
			}
		}
	}
	if grpcOpts, ok := cfg["grpc-opts"].(map[string]any); ok {
		transport["type"] = "grpc"
		copyNodeIdentityString(transport, grpcOpts, "grpc-service-name", "serviceName", "service-name")
	}
	if h2Opts, ok := cfg["h2-opts"].(map[string]any); ok {
		transport["type"] = "h2"
		copyNodeIdentityString(transport, h2Opts, "path")
		if host, ok := h2Opts["host"]; ok {
			transport["host"] = host
		}
	}
	return transport
}

func nodeRealityIdentity(cfg map[string]any) map[string]any {
	reality := map[string]any{}
	if realityOpts, ok := cfg["reality-opts"].(map[string]any); ok {
		copyNodeIdentityString(reality, realityOpts, "public-key", "public_key", "pbk")
		copyNodeIdentityString(reality, realityOpts, "short-id", "short_id", "sid")
		copyNodeIdentityString(reality, realityOpts, "spider-x", "spider_x", "spx")
	}
	copyNodeIdentityString(reality, cfg, "public-key", "public_key", "pbk")
	copyNodeIdentityString(reality, cfg, "short-id", "short_id", "sid")
	copyNodeIdentityString(reality, cfg, "spider-x", "spider_x", "spx")
	return reality
}

func copyNodeIdentityString(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := nodeString(src, key); value != "" {
			dst[keys[0]] = value
			return
		}
	}
}

func copyNodeIdentityInt(dst, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := nodeInt(src, key); value != 0 {
			dst[keys[0]] = value
			return
		}
	}
}

func copyNodeIdentityValue(dst, src map[string]any, target string, keys ...string) {
	for _, key := range keys {
		if value, ok := src[key]; ok && value != nil {
			dst[target] = value
			return
		}
	}
}

func nodeString(m map[string]any, key string) string {
	switch value := m[key].(type) {
	case string:
		return value
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		return strconv.Itoa(value)
	default:
		return ""
	}
}

func nodeInt(m map[string]any, key string) int {
	switch value := m[key].(type) {
	case int:
		return value
	case float64:
		return int(value)
	case string:
		parsed, _ := strconv.Atoi(value)
		return parsed
	default:
		return 0
	}
}

func firstNodeString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNodeInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}
