package parser

import (
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseURIList(content string) []model.ParsedNode {
	text := strings.TrimSpace(content)
	if decoded, ok := decodeBase64(text); ok {
		text = decoded
	}

	nodes := make([]model.ParsedNode, 0)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || !strings.Contains(line, "://") {
			continue
		}
		node, err := ParseProxyURI(line)
		if err != nil {
			continue
		}
		nodes = append(nodes, *node)
	}
	return nodes
}
