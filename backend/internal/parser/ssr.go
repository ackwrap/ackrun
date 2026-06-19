package parser

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseShadowsocksR(raw string) (*model.ParsedNode, error) {
	decoded, err := base64DecodeURLSafe(strings.TrimPrefix(raw, "ssr://"))
	if err != nil {
		return nil, fmt.Errorf("decode ssr: %w", err)
	}
	parts := strings.SplitN(decoded, "/?", 2)
	segments := strings.Split(parts[0], ":")
	if len(segments) < 6 {
		return nil, fmt.Errorf("invalid ssr format")
	}
	password, _ := base64DecodeURLSafe(segments[len(segments)-1])
	obfs := segments[len(segments)-2]
	method := segments[len(segments)-3]
	protocol := segments[len(segments)-4]
	port := parsePort(segments[len(segments)-5])
	server := strings.Join(segments[:len(segments)-5], ":")
	name := "SSR Node"
	node := map[string]any{"name": name, "type": "ssr", "server": server, "port": port, "cipher": method, "password": password, "protocol": protocol, "obfs": obfs, "udp": true}
	if len(parts) == 2 {
		params := parseQueryParams(parts[1])
		if remarks := params["remarks"]; remarks != "" {
			if decodedName, err := base64DecodeURLSafe(remarks); err == nil {
				name = decodedName
				node["name"] = name
			}
		}
		if obfsParam := params["obfsparam"]; obfsParam != "" {
			if decoded, err := base64DecodeURLSafe(obfsParam); err == nil {
				node["obfs-param"] = decoded
			}
		}
		if protParam := params["protoparam"]; protParam != "" {
			if decoded, err := base64DecodeURLSafe(protParam); err == nil {
				node["protocol-param"] = decoded
			}
		}
		if group := params["group"]; group != "" {
			if decoded, err := base64DecodeURLSafe(group); err == nil {
				node["group"] = decoded
			}
		}
	}
	return parsedNodeFromMap(raw, node)
}
