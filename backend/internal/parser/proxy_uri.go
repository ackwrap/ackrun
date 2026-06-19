package parser

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func ParseProxyURI(raw string) (*model.ParsedNode, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, "vmess://"):
		return parseVmess(raw)
	case strings.HasPrefix(raw, "ss://"):
		return parseShadowsocks(raw)
	case strings.HasPrefix(raw, "ssr://"):
		return parseShadowsocksR(raw)
	case strings.HasPrefix(raw, "trojan://"):
		return parseTrojan(raw)
	case strings.HasPrefix(raw, "vless://"):
		return parseVless(raw)
	case strings.HasPrefix(raw, "socks://"), strings.HasPrefix(raw, "socks5://"), strings.HasPrefix(raw, "socks4://"), strings.HasPrefix(raw, "socks4a://"):
		return parseSocks(raw)
	case strings.HasPrefix(raw, "hysteria://"):
		return parseHysteria(raw)
	case strings.HasPrefix(raw, "hy2://"), strings.HasPrefix(raw, "hysteria2://"):
		return parseHysteria2(raw)
	case strings.HasPrefix(raw, "tuic://"):
		return parseTuic(raw)
	case strings.HasPrefix(raw, "anytls://"):
		return parseAnyTLS(raw)
	case strings.HasPrefix(raw, "wireguard://"), strings.HasPrefix(raw, "wg://"):
		return parseWireGuard(raw)
	case strings.HasPrefix(raw, "http://"), strings.HasPrefix(raw, "https://"):
		return parseHTTPProxy(raw)
	case strings.HasPrefix(raw, "naive://"), strings.HasPrefix(raw, "naive+https://"), strings.HasPrefix(raw, "naive+http://"):
		return parseNaive(raw)
	case strings.HasPrefix(raw, "mieru://"):
		return parseMieru(raw)
	case strings.HasPrefix(raw, "snell://"):
		return parseSnell(raw)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	typ := normalizeProxyType(strings.ToLower(u.Scheme))
	server := u.Hostname()
	port := parsePort(u.Port())
	if server == "" && u.Host != "" {
		server, port = splitHostPortLoose(u.Host)
	}
	name := decodeURLValue(u.Fragment)
	if name == "" {
		name = server
	}
	if typ == "" || server == "" {
		return nil, fmt.Errorf("invalid proxy uri")
	}
	rawJSON, _ := json.Marshal(map[string]any{"name": name, "type": typ, "server": server, "port": port})
	return &model.ParsedNode{Name: name, Type: typ, Server: server, ServerPort: port, Raw: raw, RawJSON: string(rawJSON)}, nil
}

func normalizeProxyType(typ string) string {
	switch typ {
	case "hy2":
		return "hysteria2"
	case "socks5", "socks4", "socks4a":
		return "socks"
	default:
		return typ
	}
}
