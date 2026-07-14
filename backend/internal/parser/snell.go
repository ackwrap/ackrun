package parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

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
	version, err := strconv.Atoi(query["version"])
	if err != nil {
		return nil, fmt.Errorf("snell: invalid version")
	}
	// sing-box uses the v4 wire protocol for v5 clients because v5 QUIC mode is intentionally unsupported.
	if version == 5 {
		version = 4
	}
	if version != 4 && version != 6 {
		return nil, fmt.Errorf("snell: unsupported version %d", version)
	}
	node["version"] = version
	if userKey := firstQueryValue(query, "userkey", "user-key"); userKey != "" {
		node["userkey"] = decodeURLValue(userKey)
	}
	if reuse := query["reuse"]; reuse == "1" || strings.EqualFold(reuse, "true") {
		node["reuse"] = true
	}
	if network := query["network"]; network != "" {
		node["network"] = network
	}
	if version == 4 {
		if obfsMode := firstQueryValue(query, "obfs_mode", "obfs-mode", "obfs"); obfsMode != "" {
			node["obfs_mode"] = obfsMode
		}
		if obfsHost := firstQueryValue(query, "obfs_host", "obfs-host"); obfsHost != "" {
			node["obfs_host"] = decodeURLValue(obfsHost)
		}
	} else if mode := query["mode"]; mode != "" {
		node["mode"] = mode
	}
	if version == 6 {
		if psk, _ := node["psk"].(string); len(psk) < 12 {
			return nil, fmt.Errorf("snell: version 6 psk must be at least 12 bytes")
		}
	}
	return parsedNodeFromMap(raw, node)
}
