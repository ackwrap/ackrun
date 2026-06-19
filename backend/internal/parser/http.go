package parser

import (
	"net/url"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseHTTPProxy(raw string) (*model.ParsedNode, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	typ := strings.ToLower(u.Scheme)
	server := u.Hostname()
	port := parsePort(u.Port())
	query := urlValuesToMap(u.Query())
	name := decodeURLValue(u.Fragment)
	if name == "" {
		name = server
	}
	node := map[string]any{"name": name, "type": typ, "server": server, "port": port}
	if u.User != nil {
		node["username"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	if strings.EqualFold(u.Scheme, "https") || truthy(query["tls"]) {
		node["tls"] = true
		applyTLSOptions(node, query, "servername")
	}
	return parsedNodeFromMap(raw, node)
}
