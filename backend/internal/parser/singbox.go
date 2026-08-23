package parser

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

type singboxSubscription struct {
	Outbounds []map[string]any `json:"outbounds"`
	Endpoints []map[string]any `json:"endpoints"`
}

func parseSingboxJSON(body []byte) []model.ParsedNode {
	var sub singboxSubscription
	if err := json.Unmarshal(body, &sub); err != nil || len(sub.Outbounds)+len(sub.Endpoints) == 0 {
		return nil
	}

	nodes := make([]model.ParsedNode, 0, len(sub.Outbounds)+len(sub.Endpoints))
	for _, outbound := range sub.Outbounds {
		typ := normalizeProtocolType(strings.ToLower(getString(outbound, "type")))
		server := getString(outbound, "server")
		port := getInt(outbound, "server_port")
		if typ == "ssh" && port == 0 {
			port = 22
		}
		if typ == "" || server == "" || port == 0 || isSingboxLogicalOutbound(typ) {
			continue
		}
		name := firstNonEmpty(getString(outbound, "tag"), getString(outbound, "name"), server)
		normalized := make(map[string]any, len(outbound)+3)
		for key, value := range outbound {
			normalized[key] = value
		}
		normalized["name"] = name
		normalized["type"] = typ
		normalized["server"] = server
		// 保持 server_port，不添加 port（sing-box 原生格式）
		if _, hasServerPort := normalized["server_port"]; !hasServerPort {
			normalized["server_port"] = port
		}
		unsupportedReason := ""
		if !IsSupportedNodeProtocol(typ) {
			unsupportedReason = UnsupportedNodeProtocolReason(typ)
		}
		if typ == "shadowsocks" {
			if err := normalizeShadowsocksPlugin(normalized); err != nil {
				unsupportedReason = err.Error()
			}
		}
		rawJSON, _ := json.Marshal(normalized)
		nodes = append(nodes, model.ParsedNode{
			Name:              name,
			Type:              typ,
			Server:            server,
			ServerPort:        port,
			Raw:               string(rawJSON),
			RawJSON:           string(rawJSON),
			UnsupportedReason: unsupportedReason,
		})
	}
	for _, endpoint := range sub.Endpoints {
		if node, ok := parseSingboxEndpoint(endpoint); ok {
			nodes = append(nodes, node)
		}
	}
	return nodes
}

func parseSingboxEndpoint(endpoint map[string]any) (model.ParsedNode, bool) {
	typ := normalizeProtocolType(strings.ToLower(getString(endpoint, "type")))
	if typ == "" {
		return model.ParsedNode{}, false
	}
	name := firstNonEmpty(getString(endpoint, "tag"), getString(endpoint, "name"), typ)
	rawJSON, _ := json.Marshal(endpoint)
	if typ != "wireguard" {
		return model.ParsedNode{
			Name:              name,
			Type:              typ,
			Raw:               string(rawJSON),
			RawJSON:           string(rawJSON),
			UnsupportedReason: UnsupportedNodeProtocolReason(typ),
		}, true
	}
	unsupportedReason := ""
	if len(stringList(endpoint["address"])) == 0 || getString(endpoint, "private_key") == "" {
		unsupportedReason = "WireGuard endpoint 缺少 address 或 private_key"
	}
	peers, ok := endpoint["peers"].([]any)
	if !ok || len(peers) == 0 {
		unsupportedReason = "WireGuard endpoint 缺少 peers"
	}
	server := ""
	port := 0
	for index, value := range peers {
		peer, ok := value.(map[string]any)
		if !ok {
			unsupportedReason = fmt.Sprintf("WireGuard peer %d 格式无效", index+1)
			continue
		}
		peerAddress := getString(peer, "address")
		peerPort := getInt(peer, "port")
		if getString(peer, "public_key") == "" {
			unsupportedReason = fmt.Sprintf("WireGuard peer %d 缺少 public_key", index+1)
		}
		if (peerAddress == "") != (peerPort == 0) || peerPort < 0 || peerPort > 65535 {
			unsupportedReason = fmt.Sprintf("WireGuard peer %d address 或 port 无效", index+1)
		}
		if server == "" && peerAddress != "" && peerPort > 0 {
			server = peerAddress
			port = peerPort
		}
	}
	if server == "" || port == 0 {
		unsupportedReason = "WireGuard endpoint 没有可连接的 peer"
	}
	normalized := make(map[string]any, len(endpoint))
	for key, value := range endpoint {
		normalized[key] = value
	}
	normalized["type"] = "wireguard"
	rawJSON, _ = json.Marshal(normalized)
	return model.ParsedNode{
		Name:              name,
		Type:              "wireguard",
		Server:            server,
		ServerPort:        port,
		Raw:               string(rawJSON),
		RawJSON:           string(rawJSON),
		UnsupportedReason: unsupportedReason,
	}, true
}

func isSingboxLogicalOutbound(typ string) bool {
	switch typ {
	case "direct", "block", "dns", "selector", "urltest", "fallback", "loadbalance":
		return true
	default:
		return false
	}
}
