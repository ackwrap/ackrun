package parser

import (
	"net/url"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseWireGuard(raw string) (*model.ParsedNode, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	server := u.Hostname()
	port := parsePort(u.Port())
	query := urlValuesToMap(u.Query())
	name := decodeURLValue(u.Fragment)
	if name == "" {
		name = server
	}
	node := map[string]any{"name": name, "type": "wireguard", "server": server, "port": port}
	for _, key := range []string{"private-key", "public-key", "preshared-key"} {
		if value := query[key]; value != "" {
			node[key] = decodeURLValue(value)
		}
	}
	if value := query["pre-shared-key"]; value != "" {
		node["pre-shared-key"] = decodeURLValue(value)
	}
	if reserved := query["reserved"]; reserved != "" {
		parts := strings.Split(decodeURLValue(reserved), ",")
		reservedInts := make([]int, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				var v int
				for _, ch := range p {
					if ch >= '0' && ch <= '9' {
						v = v*10 + int(ch-'0')
					}
				}
				reservedInts = append(reservedInts, v)
			}
		}
		if len(reservedInts) > 0 {
			node["reserved"] = reservedInts
		}
	}
	if address := query["address"]; address != "" {
		addrs := strings.Split(decodeURLValue(address), ",")
		for i, a := range addrs {
			addrs[i] = strings.TrimSpace(a)
		}
		node["address"] = addrs
	}
	if mtu := query["mtu"]; mtu != "" {
		node["mtu"] = parsePort(mtu)
	}
	if peers := query["peers"]; peers != "" {
		node["peers"] = decodeURLValue(peers)
	}
	return parsedNodeFromMap(raw, node)
}
