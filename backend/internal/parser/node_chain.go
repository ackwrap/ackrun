package parser

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/nodeidentity"
)

type chainSource struct {
	node   int
	group  *model.NodeChainGroup
	direct bool
	reason string
}

type chainResolution struct {
	ref   string
	err   error
	state int
}

// resolveSubscriptionChains resolves names only within the current document.
// Persisted references use connection UIDs, so renames and other subscriptions
// cannot silently retarget an imported proxy chain.
func resolveSubscriptionChains(nodes []model.ParsedNode, groups []map[string]any, directNames []string, clash bool) []model.ParsedNode {
	sources := map[string]chainSource{}
	addSource := func(name string, source chainSource) {
		if name == "" {
			return
		}
		if _, exists := sources[name]; exists {
			source = chainSource{node: -1, reason: "chain dependency name is ambiguous"}
		}
		sources[name] = source
	}
	configs := make([]map[string]any, len(nodes))
	for index, node := range nodes {
		_ = json.Unmarshal([]byte(node.RawJSON), &configs[index])
		addSource(node.Name, chainSource{node: index})
	}
	for _, name := range directNames {
		addSource(name, chainSource{node: -1, direct: true})
	}
	for _, raw := range groups {
		name := getString(raw, "tag")
		if clash {
			name = getString(raw, "name")
		}
		group, reason := normalizeChainGroup(raw, clash)
		addSource(name, chainSource{node: -1, group: group, reason: reason})
	}
	nodeStates := make([]chainResolution, len(nodes))
	groupStates := map[string]*chainResolution{}
	resolvedGroups := map[string]model.NodeChainGroup{}
	identities := map[string]string{model.NodeChainDirect: model.NodeChainDirect}
	var resolveRef func(string, int) (string, error)
	var resolveNode func(int, int) (string, error)
	resolveNode = func(index, depth int) (string, error) {
		state := &nodeStates[index]
		if state.state == 1 || depth > 64 {
			return "", fmt.Errorf("chain contains a cycle or exceeds 64 hops")
		}
		if state.state == 2 {
			return state.ref, state.err
		}
		state.state = 1
		node := &nodes[index]
		config := configs[index]
		if node.UnsupportedReason != "" || config == nil {
			state.err = fmt.Errorf("chain dependency is not a supported node")
		} else if target := getString(config, "detour"); strings.TrimSpace(target) != "" {
			ref, err := resolveRef(target, depth+1)
			if err != nil {
				state.err = err
			} else {
				chain := model.NodeChain{Detour: ref, Identity: identities[ref]}
				seen := map[string]bool{}
				var collectGroups func(string)
				collectGroups = func(ref string) {
					if seen[ref] || !strings.HasPrefix(ref, model.NodeChainGroupPrefix) {
						return
					}
					seen[ref] = true
					group := resolvedGroups[ref]
					for _, member := range group.Outbounds {
						collectGroups(member)
					}
					chain.Groups = append(chain.Groups, group)
				}
				collectGroups(ref)
				sort.Slice(chain.Groups, func(i, j int) bool { return chain.Groups[i].ID < chain.Groups[j].ID })
				config[model.NodeChainMetadataKey] = chain
				delete(config, "detour")
				encoded, _ := json.Marshal(config)
				node.RawJSON = string(encoded)
				node.Raw = node.RawJSON
			}
		}
		if state.err == nil {
			node.UID = nodeidentity.StableUID(*node)
			state.ref = model.NodeChainNodePrefix + node.UID
			identities[state.ref] = state.ref
		} else if node.UnsupportedReason == "" {
			node.UnsupportedReason = "chain: " + state.err.Error()
		}
		state.state = 2
		return state.ref, state.err
	}
	resolveRef = func(name string, depth int) (string, error) {
		source, exists := sources[name]
		if !exists {
			if clash && name == "DIRECT" {
				return model.NodeChainDirect, nil
			}
			return "", fmt.Errorf("chain dependency is missing from the subscription")
		}
		if source.reason != "" {
			return "", fmt.Errorf("%s", source.reason)
		}
		if source.direct {
			return model.NodeChainDirect, nil
		}
		if source.node >= 0 {
			return resolveNode(source.node, depth)
		}
		state := groupStates[name]
		if state == nil {
			state = &chainResolution{}
			groupStates[name] = state
		}
		if state.state == 1 || depth > 64 {
			return "", fmt.Errorf("chain contains a cycle or exceeds 64 hops")
		}
		if state.state == 2 {
			return state.ref, state.err
		}
		state.state = 1
		group := *source.group
		group.Outbounds = nil
		for _, member := range source.group.Outbounds {
			ref, err := resolveRef(member, depth+1)
			if err != nil {
				state.err = err
				break
			}
			group.Outbounds = append(group.Outbounds, ref)
		}
		if state.err == nil && group.Default != "" {
			group.Default, state.err = resolveRef(group.Default, depth+1)
			found := false
			for _, ref := range group.Outbounds {
				found = found || ref == group.Default
			}
			if state.err == nil && !found {
				state.err = fmt.Errorf("chain selector default is not one of its members")
			}
		}
		if state.err == nil {
			identity := group
			identity.Name = ""
			identity.Outbounds = make([]string, len(group.Outbounds))
			for index, ref := range group.Outbounds {
				identity.Outbounds[index] = identities[ref]
			}
			identity.Default = identities[group.Default]
			encoded, _ := json.Marshal(identity)
			sum := sha256.Sum256(encoded)
			fingerprint := model.NodeChainGroupPrefix + hex.EncodeToString(sum[:])[:16]
			// Keep independently selectable source groups separate, even when
			// their current members are identical. Names do not enter the UID.
			sum = sha256.Sum256([]byte(group.Name + "\x00" + fingerprint))
			group.ID = model.NodeChainGroupPrefix + hex.EncodeToString(sum[:])[:16]
			resolvedGroups[group.ID] = group
			identities[group.ID] = fingerprint
			state.ref = group.ID
		}
		state.state = 2
		return state.ref, state.err
	}
	for index := range nodes {
		_, _ = resolveNode(index, 0)
	}
	return nodes
}

func normalizeChainGroup(raw map[string]any, clash bool) (*model.NodeChainGroup, string) {
	group := &model.NodeChainGroup{Type: getString(raw, "type"), Name: getString(raw, "tag")}
	allowed := " type tag outbounds default url interval tolerance idle_timeout interrupt_exist_connections "
	if clash {
		group.Name = getString(raw, "name")
		group.Outbounds = chainMemberNames(raw["proxies"])
		allowed = " name type proxies url interval tolerance icon hidden "
		switch group.Type {
		case "select":
			group.Type = "selector"
		case "url-test":
			group.Type = "urltest"
		}
		if interval := getInt(raw, "interval"); interval > 0 {
			group.Interval = fmt.Sprintf("%ds", interval)
		}
	} else {
		group.Outbounds = chainMemberNames(raw["outbounds"])
		group.Default = getString(raw, "default")
		group.Interval = getString(raw, "interval")
		group.IdleTimeout = getString(raw, "idle_timeout")
		group.InterruptExistConnections = boolOrString(raw["interrupt_exist_connections"])
	}
	if group.Type != "selector" && group.Type != "urltest" {
		return nil, "chain proxy group type is not supported; use a static select/selector or url-test/urltest group"
	}
	for key := range raw {
		if !strings.Contains(allowed, " "+key+" ") {
			return nil, "chain proxy group contains unsupported options or external providers"
		}
	}
	if len(group.Outbounds) == 0 {
		return nil, "chain proxy group has no members"
	}
	group.URL = getString(raw, "url")
	group.Tolerance = getInt(raw, "tolerance")
	if group.Tolerance < 0 || group.Tolerance > 65535 {
		return nil, "chain URL-test tolerance is invalid"
	}
	for _, value := range []string{group.Interval, group.IdleTimeout} {
		if value != "" {
			if _, err := time.ParseDuration(value); err != nil {
				return nil, "chain proxy group duration is invalid"
			}
		}
	}
	if group.Type == "selector" && (group.URL != "" || group.Interval != "" || group.IdleTimeout != "" || group.Tolerance != 0) {
		return nil, "chain selector contains URL-test-only options"
	}
	return group, ""
}

func isPlainChainDirect(raw map[string]any) bool {
	for key := range raw {
		if key != "name" && key != "tag" && key != "type" {
			return false
		}
	}
	return true
}

// Proxy names are opaque references; trimming them can disconnect real nodes.
func chainMemberNames(value any) []string {
	var names []string
	switch values := value.(type) {
	case []string:
		names = values
	case []any:
		for _, value := range values {
			name, ok := value.(string)
			if !ok {
				return nil
			}
			names = append(names, name)
		}
	}
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return nil
		}
	}
	return names
}
