package service

import (
	"fmt"
	"strings"
)

func generatedGeoRuleSetTag(ruleType, value string) string {
	ruleType = strings.ToLower(strings.TrimSpace(ruleType))
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, ruleType+"-") {
		return value
	}
	return ruleType + "-" + value
}

func generatedGeoRuleSetURL(tag string) string {
	if strings.HasPrefix(tag, "geosite-") {
		return fmt.Sprintf("https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/%s.srs", tag)
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/%s.srs", tag)
}

func generatedGeoRuleSetContentURL(baseURL, tag string) string {
	path := fmt.Sprintf("/api/v1/rules/geo/rule-sets/%s/content", tag)
	if strings.TrimSpace(baseURL) == "" {
		return path
	}
	return strings.TrimRight(baseURL, "/") + path
}

func appendGeneratedGeoRuleSets(ruleSets []map[string]interface{}, seen map[string]bool, ruleType string, values []string, baseURL string) []map[string]interface{} {
	for _, value := range values {
		tag := generatedGeoRuleSetTag(ruleType, value)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		ruleSets = append(ruleSets, map[string]interface{}{
			"tag":             tag,
			"type":            "remote",
			"format":          "binary",
			"url":             generatedGeoRuleSetContentURL(baseURL, tag),
			"download_detour": "direct",
		})
	}
	return ruleSets
}

func generatedGeoRuleSetTags(ruleType string, values []string) []string {
	tags := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		tag := generatedGeoRuleSetTag(ruleType, value)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}
