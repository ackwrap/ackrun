package parser

import "github.com/ackwrap/ackwrap/internal/model"

func ParseSubscriptionNodes(body []byte) ([]model.ParsedNode, error) {
	if nodes := parseSingboxJSON(body); len(nodes) > 0 {
		return nodes, nil
	}
	if nodes := parseClashYAML(body); len(nodes) > 0 {
		return nodes, nil
	}
	return parseURIList(string(body)), nil
}
