package parser

import (
	"net/url"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseSocks(raw string) (*model.ParsedNode, error) {
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
	typ := "socks"
	version := "5"
	if strings.EqualFold(u.Scheme, "socks4") {
		typ = "socks"
		version = "4"
	} else if strings.EqualFold(u.Scheme, "socks4a") {
		typ = "socks"
		version = "4a"
	}
	node := map[string]any{"name": name, "type": typ, "server": server, "port": port, "version": version}
	if u.User != nil {
		node["username"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	node["udp"] = true
	if truthy(query["udp"]) {
		node["udp"] = true
	} else if query["udp"] == "false" || query["udp"] == "0" {
		node["udp"] = false
	}
	if uot := query["uot"]; uot != "" {
		node["uot"] = truthy(uot)
	}
	if truthy(query["tls"]) || strings.EqualFold(u.Scheme, "socks5+tls") || strings.EqualFold(u.Scheme, "sockstls") {
		node["tls"] = true
		applyTLSOptions(node, query, "servername")
	}
	return parsedNodeFromMap(raw, node)
}
