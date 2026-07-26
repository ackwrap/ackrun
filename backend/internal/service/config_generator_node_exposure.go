package service

import (
	"fmt"
	"strings"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *ConfigGeneratorService) generateNodeExposureConfig(
	req *model.ConfigGenerateRequest,
	outbounds []interface{},
	endpoints []interface{},
) ([]interface{}, []map[string]interface{}, error) {
	items, err := s.store.ListNodeExposures()
	if err != nil {
		return nil, nil, fmt.Errorf("读取节点暴露配置失败: %w", err)
	}
	if len(items) == 0 {
		return nil, nil, nil
	}

	nodes, err := s.store.ListEnabledNodes()
	if err != nil {
		return nil, nil, fmt.Errorf("读取节点暴露目标失败: %w", err)
	}
	nodeTags := buildNodeOutboundTags(nodes)
	validator := NewNodeExposureService(s.store)
	inbounds := make([]interface{}, 0, len(items))
	rules := make([]map[string]interface{}, 0, len(items))
	for _, item := range items {
		if !item.Enabled {
			continue
		}
		if !item.NodeExists || !item.NodeEnabled {
			return nil, nil, fmt.Errorf("节点暴露 %q 的目标节点不存在或已停用，请重新选择节点或停用该暴露", item.Name)
		}
		if err := validator.validateRuntimeConflicts(&item.NodeExposure, item.ID); err != nil {
			return nil, nil, fmt.Errorf("节点暴露 %q 校验失败: %w", item.Name, err)
		}
		if req != nil && req.InboundPort > 0 && item.ListenPort == req.InboundPort {
			return nil, nil, fmt.Errorf("节点暴露 %q 的监听端口 %d 与 Mixed 入站冲突", item.Name, item.ListenPort)
		}

		outboundTag := nodeTags[item.NodeUID]
		if outboundTag == "" || !generatedConfigHasTag(outbounds, endpoints, outboundTag) {
			return nil, nil, fmt.Errorf("节点暴露 %q 的目标节点无法生成可用出站，请检查节点协议和参数", item.Name)
		}
		inboundTag := fmt.Sprintf("node-exposure-in-%d", item.ID)
		inbound := map[string]interface{}{
			"type":        item.InboundType,
			"tag":         inboundTag,
			"listen":      item.Listen,
			"listen_port": item.ListenPort,
		}
		if item.Username != "" && item.Password != "" {
			inbound["users"] = []map[string]string{{
				"username": item.Username,
				"password": item.Password,
			}}
		}
		inbounds = append(inbounds, inbound)
		rules = append(rules, map[string]interface{}{
			"inbound":  []string{inboundTag},
			"action":   "route",
			"outbound": outboundTag,
		})
	}
	return inbounds, rules, nil
}

func generatedConfigHasTag(outbounds, endpoints []interface{}, tag string) bool {
	for _, collection := range [][]interface{}{outbounds, endpoints} {
		for _, raw := range collection {
			item, ok := raw.(map[string]interface{})
			if ok && item["tag"] == tag {
				return true
			}
		}
	}
	return false
}

func (s *ConfigGeneratorService) redactNodeExposurePasswords(message string) string {
	items, err := s.store.ListNodeExposures()
	if err != nil {
		return message
	}
	for _, item := range items {
		if item.Password != "" {
			message = strings.ReplaceAll(message, item.Password, "[REDACTED]")
		}
	}
	return message
}
