package parser

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseTrojan(raw string) (*model.ParsedNode, error) {
	content := strings.TrimPrefix(raw, "trojan://")
	name := "Trojan Node"
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
		return nil, fmt.Errorf("invalid trojan url")
	}
	server, port := parseServerPortWithDefault(mainPart[atIdx+1:], 443)
	node := map[string]any{"name": name, "type": "trojan", "server": server, "port": port, "password": mainPart[:atIdx], "udp": true, "tls": true}
	applyTLSOptions(node, query, "sni")
	applyTransportOptions(node, query)
	return parsedNodeFromMap(raw, node)
}
