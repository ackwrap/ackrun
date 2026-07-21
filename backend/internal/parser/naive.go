package parser

import (
	"net/url"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseNaive(raw string) (*model.ParsedNode, error) {
	normalized := strings.TrimPrefix(raw, "naive+")
	u, err := url.Parse(normalized)
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
	node := map[string]any{"name": name, "type": "naive", "server": server, "port": port, "tls": strings.EqualFold(u.Scheme, "https")}
	if u.User != nil {
		node["username"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	applyTLSOptions(node, query, "sni")
	return parsedNodeFromMap(raw, node)
}
