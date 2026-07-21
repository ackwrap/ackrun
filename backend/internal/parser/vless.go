package parser

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseVless(raw string) (*model.ParsedNode, error) {
	content := strings.TrimPrefix(raw, "vless://")
	name := "VLESS Node"
	mainPart := content
	if idx := strings.LastIndex(content, "#"); idx != -1 {
		mainPart = content[:idx]
		name = decodeURLValue(content[idx+1:])
	}
	query := map[string]string{}
	if idx := strings.Index(mainPart, "?"); idx != -1 {
		query = parseQueryParams(mainPart[idx+1:])
		mainPart = mainPart[:idx]
	}
	mainPart = strings.TrimSuffix(mainPart, "/")
	atIdx := strings.LastIndex(mainPart, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("invalid vless url")
	}
	server, port := parseServerPortWithDefault(mainPart[atIdx+1:], 443)
	security := firstNonEmpty(query["security"], "none")
	node := map[string]any{"name": name, "type": "vless", "server": server, "port": port, "uuid": mainPart[:atIdx], "udp": true, "tls": security == "tls" || security == "reality", "flow": query["flow"], "encryption": firstNonEmpty(query["encryption"], "none")}
	applyTLSOptions(node, query, "servername")
	applyTransportOptions(node, query)
	if security == "reality" {
		applyRealityOptions(node, query)
	}
	return parsedNodeFromMap(raw, node)
}
