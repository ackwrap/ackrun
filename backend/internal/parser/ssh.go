package parser

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func parseSSH(raw string) (*model.ParsedNode, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	server := u.Hostname()
	portText := u.Port()
	port := 22
	if portText != "" {
		port, err = strconv.Atoi(portText)
		if err != nil || port < 1 || port > 65535 {
			return nil, fmt.Errorf("ssh: invalid server port")
		}
	} else if strings.HasSuffix(u.Host, ":") {
		return nil, fmt.Errorf("ssh: invalid server port")
	}
	name := decodeURLValue(u.Fragment)
	if name == "" {
		name = server
	}
	node := map[string]any{"name": name, "type": "ssh", "server": server, "port": port}
	if u.User != nil {
		if user := u.User.Username(); user != "" {
			node["user"] = user
		}
		if password, ok := u.User.Password(); ok {
			node["password"] = password
		}
	}
	query := u.Query()
	if _, exists := node["user"]; !exists {
		copySSHQueryValue(node, query, "user", "user", "username")
	}
	if _, exists := node["password"]; !exists {
		copySSHQueryValue(node, query, "password", "password")
	}
	copySSHQueryValue(node, query, "private_key_path", "private_key_path", "private-key-path")
	copySSHQueryValue(node, query, "private_key_passphrase", "private_key_passphrase", "private-key-passphrase")
	copySSHQueryValue(node, query, "client_version", "client_version", "client-version")
	copySSHQueryList(node, query, "private_key", "private_key", "private-key")
	copySSHQueryList(node, query, "host_key", "host_key", "host-key")
	copySSHQueryList(node, query, "host_key_algorithms", "host_key_algorithms", "host-key-algorithms")
	copySSHQueryList(node, query, "cipher", "cipher")
	copySSHQueryList(node, query, "mac", "mac")
	copySSHQueryList(node, query, "kex_algorithm", "kex_algorithm", "kex-algorithm")
	return parsedNodeFromMap(raw, node)
}

func copySSHQueryValue(dst map[string]any, query url.Values, target string, keys ...string) {
	for _, key := range keys {
		if value := query.Get(key); value != "" {
			dst[target] = value
			return
		}
	}
}

func copySSHQueryList(dst map[string]any, query url.Values, target string, keys ...string) {
	for _, key := range keys {
		values := query[key]
		if len(values) == 1 && values[0] != "" {
			dst[target] = []string{values[0]}
			return
		}
		if len(values) > 1 {
			kept := make([]string, 0, len(values))
			for _, value := range values {
				if value != "" {
					kept = append(kept, value)
				}
			}
			if len(kept) > 0 {
				dst[target] = kept
				return
			}
		}
	}
}
