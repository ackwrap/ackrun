package parser

import (
	"net/url"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseMieru(raw string) (*model.ParsedNode, error) {
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
	node := map[string]any{"name": name, "type": "mieru", "server": server, "port": port}
	if u.User != nil {
		node["username"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	for _, key := range []string{"protocol", "transport", "multiplexing"} {
		if value := query[key]; value != "" {
			node[key] = value
		}
	}
	if truthy(query["tls"]) {
		node["tls"] = true
		applyTLSOptions(node, query, "sni")
	}
	return parsedNodeFromMap(raw, node)
}
