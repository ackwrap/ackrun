package parser

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseVmess(raw string) (*model.ParsedNode, error) {
	encoded := strings.TrimPrefix(raw, "vmess://")
	decoded, err := base64.StdEncoding.DecodeString(withPadding(encoded))
	if err != nil {
		decoded, err = base64.URLEncoding.DecodeString(withPadding(encoded))
	}
	if err != nil {
		return nil, err
	}

	var cfg map[string]any
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return nil, err
	}
	name := firstNonEmpty(getString(cfg, "ps"), getString(cfg, "name"), getString(cfg, "add"))
	server := getString(cfg, "add")
	port := getInt(cfg, "port")
	if server == "" {
		return nil, fmt.Errorf("vmess server missing")
	}
	node := map[string]any{
		"name":    name,
		"type":    "vmess",
		"server":  server,
		"port":    port,
		"uuid":    getString(cfg, "id"),
		"alterId": getInt(cfg, "aid"),
		"cipher":  firstNonEmpty(getString(cfg, "scy"), getString(cfg, "cipher"), "auto"),
		"udp":     true,
	}
	security := getString(cfg, "tls")
	if security == "tls" {
		node["tls"] = true
	}
	query := map[string]string{
		"type":        firstNonEmpty(getString(cfg, "net"), "tcp"),
		"path":        getString(cfg, "path"),
		"host":        getString(cfg, "host"),
		"sni":         getString(cfg, "sni"),
		"alpn":        getString(cfg, "alpn"),
		"fp":          getString(cfg, "fp"),
		"serviceName": getString(cfg, "path"),
	}
	applyTLSOptions(node, query, "servername")
	applyTransportOptions(node, query)
	return parsedNodeFromMap(raw, node)
}
