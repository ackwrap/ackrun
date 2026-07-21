package parser

import (
	"net/url"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseTuic(raw string) (*model.ParsedNode, error) {
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
	node := map[string]any{"name": name, "type": "tuic", "server": server, "port": port, "udp": true}
	if u.User != nil {
		node["uuid"] = u.User.Username()
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	if token := query["token"]; token != "" {
		node["token"] = token
	}
	node["tls"] = true
	applyTLSOptions(node, query, "sni")
	if cc := query["congestion_control"]; cc != "" {
		node["congestion_control"] = cc
	} else if cc = query["congestion-control"]; cc != "" {
		node["congestion_control"] = cc
	}
	if udpRelayMode := query["udp_relay_mode"]; udpRelayMode != "" {
		node["udp-relay-mode"] = udpRelayMode
	} else if udpRelayMode = query["udp-relay-mode"]; udpRelayMode != "" {
		node["udp-relay-mode"] = udpRelayMode
	}
	if zeroRtt := query["reduce_rtt"]; zeroRtt == "1" || zeroRtt == "true" {
		node["reduce-rtt"] = true
	} else if zeroRtt = query["reduce-rtt"]; zeroRtt == "1" || zeroRtt == "true" {
		node["reduce-rtt"] = true
	}
	if heartbeat := query["heartbeat"]; heartbeat != "" {
		node["heartbeat"] = heartbeat
	}
	return parsedNodeFromMap(raw, node)
}
