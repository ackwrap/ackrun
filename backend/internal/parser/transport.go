package parser

import (
	"net/url"
	"strings"
)

func applyTLSOptions(node map[string]any, query map[string]string, sniKey string) {
	if sni := firstNonEmpty(query["sni"], query["peer"], query["host"], query["servername"]); sni != "" {
		node[sniKey] = decodeURLValue(sni)
	}
	if alpn := query["alpn"]; alpn != "" {
		node["alpn"] = splitCSV(alpn)
	}
	if fp := query["fp"]; fp != "" {
		node["client-fingerprint"] = fp
	}
	if fingerprint := query["fingerprint"]; fingerprint != "" {
		node["fingerprint"] = fingerprint
	}
	if truthy(query["allowInsecure"]) || truthy(query["skip-cert-verify"]) || truthy(query["insecure"]) {
		node["skip-cert-verify"] = true
	}
}

func applyTransportOptions(node map[string]any, query map[string]string) {
	network := firstNonEmpty(query["type"], query["network"], getStringFromMap(node, "network"), "tcp")
	node["network"] = network
	switch network {
	case "ws":
		path := firstNonEmpty(query["path"], "/")
		wsOpts := map[string]any{"path": decodeURLValue(path)}
		if host := query["host"]; host != "" {
			wsOpts["headers"] = map[string]string{"Host": decodeURLValue(host)}
		} else {
			wsOpts["headers"] = map[string]string{}
		}
		node["ws-opts"] = wsOpts
	case "grpc":
		serviceName := firstNonEmpty(query["serviceName"], query["service-name"], query["path"])
		node["grpc-opts"] = map[string]any{"grpc-service-name": decodeURLValue(serviceName)}
	case "h2", "http":
		path := firstNonEmpty(query["path"], "/")
		h2Opts := map[string]any{"path": decodeURLValue(path)}
		if host := query["host"]; host != "" {
			h2Opts["host"] = splitCSV(decodeURLValue(host))
		} else {
			h2Opts["host"] = []string{}
		}
		node["h2-opts"] = h2Opts
	case "httpupgrade", "http-upgrade":
		node["network"] = "httpupgrade"
		node["http-upgrade-opts"] = map[string]any{"path": decodeURLValue(firstNonEmpty(query["path"], "/")), "host": decodeURLValue(query["host"])}
	case "xhttp":
		node["xhttp-opts"] = map[string]any{"path": decodeURLValue(firstNonEmpty(query["path"], "/")), "host": decodeURLValue(query["host"])}
	}
}

func applyRealityOptions(node map[string]any, query map[string]string) {
	reality := map[string]any{}
	if pbk := query["pbk"]; pbk != "" {
		reality["public-key"] = pbk
	}
	if sid, ok := query["sid"]; ok {
		reality["short-id"] = sid
	} else {
		reality["short-id"] = ""
	}
	if spx := query["spx"]; spx != "" {
		reality["spider-x"] = decodeURLValue(spx)
	}
	node["reality-opts"] = reality
	if fp := query["fp"]; fp != "" {
		node["client-fingerprint"] = fp
	}
	node["skip-cert-verify"] = true
}

func urlValuesToMap(v url.Values) map[string]string {
	m := make(map[string]string, len(v))
	for key, vals := range v {
		if len(vals) > 0 {
			m[key] = vals[0]
		}
	}
	return m
}

func truthy(value string) bool {
	return value == "1" || strings.EqualFold(value, "true")
}

func splitCSV(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func getStringFromMap(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
