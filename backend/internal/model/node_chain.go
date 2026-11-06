package model

import (
	"encoding/json"
	"fmt"
)

const (
	NodeChainMetadataKey = "_ackwrap_chain"
	NodeChainNodePrefix  = "node:"
	NodeChainGroupPrefix = "group:"
	NodeChainDirect      = "direct"
)

// NodeChain stores subscription-scoped references as connection UIDs, not names.
// Its metadata is removed before a configuration is passed to sing-box.
type NodeChain struct {
	Detour   string           `json:"detour"`
	Identity string           `json:"identity"`
	Groups   []NodeChainGroup `json:"groups,omitempty"`
}

type NodeChainGroup struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name,omitempty"`
	Type                      string   `json:"type"`
	Outbounds                 []string `json:"outbounds"`
	Default                   string   `json:"default,omitempty"`
	URL                       string   `json:"url,omitempty"`
	Interval                  string   `json:"interval,omitempty"`
	Tolerance                 int      `json:"tolerance,omitempty"`
	IdleTimeout               string   `json:"idle_timeout,omitempty"`
	InterruptExistConnections bool     `json:"interrupt_exist_connections,omitempty"`
}

func DecodeNodeChain(options map[string]any) (*NodeChain, error) {
	value, exists := options[NodeChainMetadataKey]
	if !exists {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("invalid node chain metadata")
	}
	var chain NodeChain
	if err := json.Unmarshal(encoded, &chain); err != nil || chain.Detour == "" || chain.Identity == "" {
		return nil, fmt.Errorf("invalid node chain metadata")
	}
	return &chain, nil
}
