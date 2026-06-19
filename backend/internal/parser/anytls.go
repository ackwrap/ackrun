package parser

import (
	"net/url"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseAnyTLS(raw string) (*model.ParsedNode, error) {
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
	node := map[string]any{"name": name, "type": "anytls", "server": server, "port": port, "password": u.User.Username()}
	node["tls"] = true
	applyTLSOptions(node, query, "sni")
	return parsedNodeFromMap(raw, node)
}
