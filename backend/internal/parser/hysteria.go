package parser

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/ackwrap/ackwrap/internal/model"
)

func parseHysteria(raw string) (*model.ParsedNode, error) {
	return parseHysteriaGeneric(raw, "hysteria")
}

func parseHysteria2(raw string) (*model.ParsedNode, error) {
	return parseHysteriaGeneric(raw, "hysteria2")
}

func parseHysteriaGeneric(raw string, typ string) (*model.ParsedNode, error) {
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
	node := map[string]any{"name": name, "type": typ, "server": server, "port": port, "udp": true}
	if auth := firstNonEmpty(u.User.Username(), query["auth"], query["password"]); auth != "" {
		if typ == "hysteria2" {
			node["password"] = auth
		} else {
			node["auth_str"] = auth
		}
	}
	node["tls"] = true
	applyTLSOptions(node, query, "sni")
	if typ == "hysteria" {
		if up := query["up"]; up != "" {
			if val, err := strconv.Atoi(up); err == nil {
				node["up"] = val
			} else {
				node["up"] = up
			}
		}
		if down := query["down"]; down != "" {
			if val, err := strconv.Atoi(down); err == nil {
				node["down"] = val
			} else {
				node["down"] = down
			}
		}
		if obfs, ok := query["obfs"]; ok && obfs != "" {
			node["obfs"] = obfs
		}
		if obfsParam, ok := query["obfs-param"]; ok && obfsParam != "" {
			node["obfs-param"] = decodeURLValue(obfsParam)
		}
		if alpn := query["alpn"]; alpn != "" {
			node["alpn"] = splitCSV(alpn)
		}
		if recvWindow := query["recv_window"]; recvWindow != "" {
			node["receive-window"] = recvWindow
		}
		if recvWindowConn := query["recv_window_conn"]; recvWindowConn != "" {
			node["receive-window-conn"] = recvWindowConn
		}
		if mtudiscovery := query["disable_mtu_discovery"]; mtudiscovery == "1" || strings.EqualFold(mtudiscovery, "true") {
			node["disable-mtu-discovery"] = true
		}
	}
	if typ == "hysteria2" {
		if obfs, ok := query["obfs"]; ok && obfs != "" {
			node["obfs"] = obfs
		}
		if obfsParam, ok := query["obfs-password"]; ok && obfsParam != "" {
			node["obfs-password"] = decodeURLValue(obfsParam)
		}
	}
	return parsedNodeFromMap(raw, node)
}
