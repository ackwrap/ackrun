package parser

import (
	"encoding/json"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

type singboxSubscription struct {
	Outbounds []map[string]any `json:"outbounds"`
}

func parseSingboxJSON(body []byte) []model.ParsedNode {
	var sub singboxSubscription
	if err := json.Unmarshal(body, &sub); err != nil || len(sub.Outbounds) == 0 {
		return nil
	}

	nodes := make([]model.ParsedNode, 0, len(sub.Outbounds))
	for _, outbound := range sub.Outbounds {
		typ := strings.ToLower(getString(outbound, "type"))
		server := getString(outbound, "server")
		port := getInt(outbound, "server_port")
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
		rawJSON, _ := json.Marshal(normalized)
		nodes = append(nodes, model.ParsedNode{
			Name:       name,
			Type:       typ,
			Server:     server,
			ServerPort: port,
			Raw:        string(rawJSON),
			RawJSON:    string(rawJSON),
		})
	}
	return nodes
}

func isSingboxLogicalOutbound(typ string) bool {
	switch typ {
	case "direct", "block", "dns", "selector", "urltest", "fallback", "loadbalance":
		return true
	default:
		return false
	}
}
