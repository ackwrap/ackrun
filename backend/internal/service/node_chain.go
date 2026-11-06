package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func nodeHasChain(rawJSON string) bool {
	var options map[string]any
	if json.Unmarshal([]byte(rawJSON), &options) != nil {
		return false
	}
	_, exists := options[model.NodeChainMetadataKey]
	return exists
}

// Filtering an upstream must never turn its dependents into direct connections.
func filterUnavailableNodeChains(nodes []model.ParsedNode) ([]model.ParsedNode, []excludedSubscriptionNode) {
	available := map[string]int{}
	for _, node := range nodes {
		available[store.StableNodeUID(node)]++
	}
	reasons := make([]string, len(nodes))
	changed := true
	for changed {
		changed = false
		for index, node := range nodes {
			if reasons[index] != "" {
				continue
			}
			var options map[string]any
			_ = json.Unmarshal([]byte(node.RawJSON), &options)
			chain, err := model.DecodeNodeChain(options)
			if err == nil && chain == nil {
				continue
			}
			valid := err == nil
			if chain != nil {
				refs := []string{chain.Detour}
				groups := map[string]bool{}
				for _, group := range chain.Groups {
					groups[group.ID] = true
					refs = append(refs, group.Outbounds...)
				}
				for _, ref := range refs {
					switch {
					case ref == model.NodeChainDirect:
					case strings.HasPrefix(ref, model.NodeChainNodePrefix):
						valid = valid && available[strings.TrimPrefix(ref, model.NodeChainNodePrefix)] > 0
					case strings.HasPrefix(ref, model.NodeChainGroupPrefix):
						valid = valid && groups[ref]
					default:
						valid = false
					}
				}
			}
			if !valid {
				reasons[index] = "代理链的前置节点已被过滤或依赖无效，不能降级为直连"
				available[store.StableNodeUID(node)]--
				changed = true
			}
		}
	}
	kept := make([]model.ParsedNode, 0, len(nodes))
	var excluded []excludedSubscriptionNode
	for index, node := range nodes {
		if reasons[index] != "" {
			excluded = append(excluded, excludedSubscriptionNode{Name: node.Name, Type: node.Type, Reason: reasons[index]})
		} else {
			kept = append(kept, node)
		}
	}
	return kept, excluded
}

func applySubscriptionNodeChains(nodes []model.Node, generatedTags map[string]string, outbounds, endpoints []interface{}) ([]interface{}, error) {
	available := map[int64]map[string]bool{}
	for _, node := range nodes {
		if available[node.SubscriptionID] == nil {
			available[node.SubscriptionID] = map[string]bool{}
		}
		available[node.SubscriptionID][node.UID] = true
	}
	byTag := map[string]map[string]interface{}{}
	for _, items := range [][]interface{}{outbounds, endpoints} {
		for _, item := range items {
			if outbound, ok := item.(map[string]interface{}); ok {
				tag, _ := outbound["tag"].(string)
				byTag[tag] = outbound
			}
		}
	}
	groups := map[string]model.NodeChainGroup{}
	chainCount := 0
	for _, node := range nodes {
		var options map[string]any
		if json.Unmarshal([]byte(node.RawJSON), &options) != nil {
			continue // The ordinary outbound generator reports malformed JSON.
		}
		chain, err := model.DecodeNodeChain(options)
		if err != nil {
			return nil, fmt.Errorf("节点 %s 的代理链元数据无效", node.Name)
		}
		if chain == nil {
			continue
		}
		chainCount++
		localGroups := map[string]model.NodeChainGroup{}
		for _, group := range chain.Groups {
			localGroups[group.ID] = group
		}
		groupTag := func(ref string) string {
			name := sanitizeOutboundTag(localGroups[ref].Name)
			return fmt.Sprintf("ackwrap-chain-%d-%s-%s", node.SubscriptionID, name, strings.TrimPrefix(ref, model.NodeChainGroupPrefix))
		}
		resolve := func(ref string) (string, error) {
			switch {
			case ref == model.NodeChainDirect:
				return "direct", nil
			case strings.HasPrefix(ref, model.NodeChainNodePrefix):
				uid := strings.TrimPrefix(ref, model.NodeChainNodePrefix)
				if tag := generatedTags[uid]; tag != "" && available[node.SubscriptionID][uid] {
					return tag, nil
				}
			case strings.HasPrefix(ref, model.NodeChainGroupPrefix):
				if _, exists := localGroups[ref]; exists {
					return groupTag(ref), nil
				}
			}
			return "", fmt.Errorf("节点 %s 的代理链依赖缺失、已停用或生成失败", node.Name)
		}
		outbound := byTag[generatedTags[node.UID]]
		if outbound == nil {
			return nil, fmt.Errorf("链式节点 %s 未能生成，不能丢弃代理链", node.Name)
		}
		outbound["detour"], err = resolve(chain.Detour)
		if err != nil {
			return nil, err
		}
		for _, group := range chain.Groups {
			if group.Type != "selector" && group.Type != "urltest" || len(group.Outbounds) == 0 {
				return nil, fmt.Errorf("节点 %s 的代理链分组无效", node.Name)
			}
			tag := groupTag(group.ID)
			if _, exists := groups[tag]; exists {
				continue
			}
			members := make([]string, 0, len(group.Outbounds))
			for _, ref := range group.Outbounds {
				member, err := resolve(ref)
				if err != nil {
					return nil, err
				}
				members = append(members, member)
			}
			generated := map[string]interface{}{"type": group.Type, "tag": tag, "outbounds": members}
			if group.InterruptExistConnections {
				generated["interrupt_exist_connections"] = true
			}
			if group.Type == "selector" && group.Default != "" {
				generated["default"], err = resolve(group.Default)
				if err != nil {
					return nil, err
				}
			}
			if group.Type == "urltest" {
				if group.URL != "" {
					generated["url"] = group.URL
				}
				if group.Interval != "" {
					generated["interval"] = group.Interval
				}
				if group.IdleTimeout != "" {
					generated["idle_timeout"] = group.IdleTimeout
				}
				if group.Tolerance != 0 {
					generated["tolerance"] = group.Tolerance
				}
			}
			groups[tag] = group
			outbounds = append(outbounds, generated)
		}
	}
	if chainCount > 0 {
		logging.Info("config_generator.chain", "generated %d chained nodes with %d dependency groups", chainCount, len(groups))
	}
	return outbounds, nil
}
