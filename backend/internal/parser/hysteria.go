package parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
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
	if typ == "hysteria2" {
		if pin := firstQueryValue(query, "pinSHA256", "pinsha256", "pin-sha256", "pin_sha256"); pin != "" {
			normalizedPin, ok := normalizeSHA256Hex(pin)
			if !ok {
				return nil, fmt.Errorf("invalid Hysteria2 pinSHA256")
			}
			node["certificate-sha256"] = normalizedPin
		}
	}
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
		if obfsType := query["obfs"]; obfsType != "" {
			obfs := map[string]any{"type": obfsType}
			if password := query["obfs-password"]; password != "" {
				obfs["password"] = decodeURLValue(password)
			}
			if obfsType == "gecko" {
				if size, parseErr := strconv.Atoi(firstQueryValue(query, "min_packet_size", "min-packet-size")); parseErr == nil && size > 0 {
					obfs["min_packet_size"] = size
				}
				if size, parseErr := strconv.Atoi(firstQueryValue(query, "max_packet_size", "max-packet-size")); parseErr == nil && size > 0 {
					obfs["max_packet_size"] = size
				}
			}
			node["obfs"] = obfs
		}
		if ports := firstQueryValue(query, "server_ports", "server-ports", "mport"); ports != "" {
			node["server_ports"] = ports
		}
		for _, field := range []string{"hop_interval", "hop_interval_max", "bbr_profile"} {
			if value := firstQueryValue(query, field, strings.ReplaceAll(field, "_", "-")); value != "" {
				node[field] = value
			}
		}
		for queryKey, outputKey := range map[string]string{"up": "up_mbps", "down": "down_mbps"} {
			if value, parseErr := strconv.Atoi(query[queryKey]); parseErr == nil && value > 0 {
				node[outputKey] = value
			}
		}
		if value := query["brutal_debug"]; value == "1" || strings.EqualFold(value, "true") {
			node["brutal_debug"] = true
		}
	}
	return parsedNodeFromMap(raw, node)
}
