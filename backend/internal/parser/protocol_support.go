package parser

import (
	"fmt"
	"strings"
)

var supportedNodeProtocols = map[string]struct{}{
	"anytls":      {},
	"http":        {},
	"hysteria":    {},
	"hysteria2":   {},
	"naive":       {},
	"shadowsocks": {},
	"snell":       {},
	"socks":       {},
	"ssh":         {},
	"ssr":         {},
	"trojan":      {},
	"tuic":        {},
	"vless":       {},
	"vmess":       {},
	"wireguard":   {},
}

func IsSupportedNodeProtocol(nodeType string) bool {
	_, supported := supportedNodeProtocols[normalizeProtocolType(nodeType)]
	return supported
}

func UnsupportedNodeProtocolReason(nodeType string) string {
	nodeType = normalizeProtocolType(nodeType)
	if IsSupportedNodeProtocol(nodeType) {
		return ""
	}
	switch nodeType {
	case "shadowtls":
		return "Standalone ShadowTLS is a transport tunnel; Ackwrap requires a chained node model to use it as a proxy"
	case "tor":
		return "Tor is a managed process outbound, not a remote subscription node"
	case "bridge", "direct", "block", "dns", "selector", "urltest", "fallback", "loadbalance":
		return fmt.Sprintf("sing-box %s is an internal outbound, not a subscription node", nodeType)
	case "openconnect", "openvpn", "openvpn-client", "openvpn-server", "tailscale":
		return fmt.Sprintf("sing-box %s is an endpoint and requires a dedicated endpoint model", nodeType)
	case "mieru":
		return "Mieru is not included in the bundled sing-box core"
	default:
		return fmt.Sprintf("protocol %s is not supported by Ackwrap", strings.TrimSpace(nodeType))
	}
}
