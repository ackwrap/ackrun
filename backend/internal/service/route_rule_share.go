package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	routeRuleShareFormat  = "ackwrap-route-rules"
	routeRuleShareVersion = 1
	maxSharedRouteRules   = 500
)

type routeRuleSharePayload struct {
	Format        string               `json:"format"`
	Version       int                  `json:"version"`
	FinalOutbound string               `json:"final_outbound"`
	Rules         []routeRuleShareItem `json:"rules"`
}

type routeRuleShareItem struct {
	Name     string   `json:"name"`
	Enabled  bool     `json:"enabled"`
	RuleType string   `json:"rule_type"`
	Values   []string `json:"values"`
	Outbound string   `json:"outbound"`
	Invert   bool     `json:"invert"`
}

func (svc *RouteRuleService) Share() (*model.RouteRuleShareResponse, error) {
	rules, err := svc.store.ListRouteRules()
	if err != nil {
		return nil, err
	}
	payload := routeRuleSharePayload{
		Format:        routeRuleShareFormat,
		Version:       routeRuleShareVersion,
		FinalOutbound: "direct",
		Rules:         make([]routeRuleShareItem, 0, len(rules)),
	}
	for _, rule := range rules {
		if rule.SystemKey == SystemRuleGlobalDirectKey || rule.RuleType == "fallback" {
			payload.FinalOutbound = rule.Outbound
			continue
		}
		if rule.SystemKey != "" {
			continue
		}
		payload.Rules = append(payload.Rules, routeRuleShareItem{
			Name: rule.Name, Enabled: rule.Enabled, RuleType: rule.RuleType,
			Values: append([]string(nil), rule.Values...), Outbound: rule.Outbound, Invert: rule.Invert,
		})
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码规则分享配置: %w", err)
	}
	logging.Info("route_rule.share", "generated route rule share code: rules=%d", len(payload.Rules))
	return &model.RouteRuleShareResponse{
		Code:      base64.StdEncoding.EncodeToString(encoded),
		RuleCount: len(payload.Rules),
	}, nil
}

func (svc *RouteRuleService) ImportShare(req *model.RouteRuleImportRequest) (*model.RouteRuleImportResponse, error) {
	if len(req.Code) > model.MaxRouteRuleShareCodeSize {
		return nil, fmt.Errorf("规则分享码超过 %d 字节限制", model.MaxRouteRuleShareCodeSize)
	}
	code := strings.Map(func(value rune) rune {
		if unicode.IsSpace(value) {
			return -1
		}
		return value
	}, strings.TrimSpace(req.Code))
	if code == "" {
		return nil, fmt.Errorf("规则分享码不能为空")
	}
	if len(code) > model.MaxRouteRuleShareCodeSize {
		return nil, fmt.Errorf("规则分享码超过 %d 字节限制", model.MaxRouteRuleShareCodeSize)
	}
	decoded, err := base64.StdEncoding.Strict().DecodeString(code)
	if err != nil {
		return nil, fmt.Errorf("规则分享码不是有效 Base64")
	}
	var payload routeRuleSharePayload
	if err := json.Unmarshal(decoded, &payload); err != nil {
		return nil, fmt.Errorf("规则分享码内容不是有效 JSON")
	}
	if payload.Format != routeRuleShareFormat || payload.Version != routeRuleShareVersion {
		return nil, fmt.Errorf("不支持的规则分享码格式或版本")
	}
	if len(payload.Rules) > maxSharedRouteRules {
		return nil, fmt.Errorf("分享码规则数超过 %d 条限制", maxSharedRouteRules)
	}
	if payload.FinalOutbound != "direct" && payload.FinalOutbound != "proxy" {
		return nil, fmt.Errorf("最终策略只支持直连 direct 或策略 proxy")
	}

	existing, err := svc.store.ListRouteRules()
	if err != nil {
		return nil, err
	}
	existingIDs := make(map[string]int64, len(existing))
	for _, rule := range existing {
		existingIDs[rule.Name] = rule.ID
	}
	requests := make([]model.RouteRuleRequest, 0, len(payload.Rules))
	seenNames := make(map[string]bool, len(payload.Rules))
	for index, item := range payload.Rules {
		rule := model.RouteRuleRequest{
			Name: item.Name, Enabled: item.Enabled, RuleType: item.RuleType,
			Values: item.Values, Outbound: item.Outbound, Invert: item.Invert,
		}
		if err := svc.validateRouteRule(&rule); err != nil {
			return nil, fmt.Errorf("第 %d 条规则无效: %w", index+1, err)
		}
		if IsSystemRouteRuleName(rule.Name) {
			return nil, fmt.Errorf("第 %d 条规则不能使用系统规则名称", index+1)
		}
		if seenNames[rule.Name] {
			return nil, fmt.Errorf("分享码包含重复规则名称: %s", rule.Name)
		}
		seenNames[rule.Name] = true
		if err := svc.validateRouteRuleOutboundName(&rule, existingIDs[rule.Name]); err != nil {
			return nil, fmt.Errorf("第 %d 条规则无效: %w", index+1, err)
		}
		requests = append(requests, rule)
	}
	created, updated, err := svc.store.MergeRouteRules(requests, payload.FinalOutbound)
	if err != nil {
		return nil, normalizeRouteRuleNameConflict(err)
	}
	logging.Info("route_rule.import", "imported route rule share code: rules=%d created=%d updated=%d", len(requests), created, updated)
	return &model.RouteRuleImportResponse{
		Success: true, Message: "规则分享码已导入",
		Created: created, Updated: updated, RuleCount: len(requests),
	}, nil
}
