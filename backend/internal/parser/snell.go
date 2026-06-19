package parser

import (
	"net/url"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseSnell(raw string) (*model.ParsedNode, error) {
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
	node := map[string]any{"name": name, "type": "snell", "server": server, "port": port, "psk": u.User.Username()}
	for _, key := range []string{"version"} {
		if value := query[key]; value != "" {
			node[key] = decodeURLValue(value)
		}
	}
	if obfs := query["obfs"]; obfs != "" {
		node["obfs"] = obfs
		obfsOpts := map[string]any{}
		if host := query["obfs-host"]; host != "" {
			obfsOpts["host"] = decodeURLValue(host)
		}
		if obfsParam := query["obfs-param"]; obfsParam != "" {
			obfsOpts["obfs-param"] = decodeURLValue(obfsParam)
		}
		if len(obfsOpts) > 0 {
			node["obfs-opts"] = obfsOpts
		}
	}
	return parsedNodeFromMap(raw, node)
}
