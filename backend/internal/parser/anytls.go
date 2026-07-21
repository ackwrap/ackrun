package parser

import (
	"net/url"
	"strconv"

	"github.com/ackwrap/ackrun/internal/model"
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
	if value := firstQueryValue(query, "idle_session_check_interval", "idle-session-check-interval"); value != "" {
		node["idle_session_check_interval"] = value
	}
	if value := firstQueryValue(query, "idle_session_timeout", "idle-session-timeout"); value != "" {
		node["idle_session_timeout"] = value
	}
	if value := firstQueryValue(query, "min_idle_session", "min-idle-session"); value != "" {
		if count, parseErr := strconv.Atoi(value); parseErr == nil && count >= 0 {
			node["min_idle_session"] = count
		}
	}
	return parsedNodeFromMap(raw, node)
}

func firstQueryValue(query map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := query[key]; value != "" {
			return value
		}
	}
	return ""
}
