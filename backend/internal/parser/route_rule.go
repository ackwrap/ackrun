package parser

import (
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/ackwrap/ackwrap/internal/model"
)

type clashRuleDocument struct {
	Behavior string `yaml:"behavior"`
	Payload  []any  `yaml:"payload"`
	Rules    []any  `yaml:"rules"`
}

func ParseClashRuleSetYAML(body []byte) (*model.SingboxRuleSetSource, error) {
	var doc clashRuleDocument
	if err := yaml.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("parse clash rule yaml: %w", err)
	}

	entries := doc.Payload
	if len(entries) == 0 {
		entries = doc.Rules
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("clash rule yaml has no payload or rules")
	}

	behavior := strings.ToLower(strings.TrimSpace(doc.Behavior))
	stringValues := map[string][]string{}
	intValues := map[string][]int{}
	for _, entry := range entries {
		line := strings.TrimSpace(ruleEntryString(entry))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch behavior {
		case "domain":
			field, value := clashDomainBehaviorRule(line)
			appendStringRuleValue(stringValues, field, value)
		case "ipcidr", "ip-cidr":
			appendStringRuleValue(stringValues, "ip_cidr", line)
		default:
			appendClashClassicalRule(line, stringValues, intValues)
		}
	}

	rule := map[string]any{}
	for _, key := range sortedStringKeys(stringValues) {
		rule[key] = stringValues[key]
	}
	for _, key := range sortedIntKeys(intValues) {
		rule[key] = intValues[key]
	}
	if len(rule) == 0 {
		return nil, fmt.Errorf("clash rule yaml has no supported rules")
	}

	return &model.SingboxRuleSetSource{Version: 3, Rules: []map[string]any{rule}}, nil
}

func appendClashClassicalRule(line string, stringValues map[string][]string, intValues map[string][]int) {
	parts := splitClashRuleLine(line)
	if len(parts) < 2 {
		appendPlainClashRule(line, stringValues)
		return
	}
	ruleType := strings.ToUpper(strings.TrimSpace(parts[0]))
	value := strings.TrimSpace(parts[1])
	if value == "" {
		return
	}
	switch ruleType {
	case "DOMAIN":
		appendStringRuleValue(stringValues, "domain", value)
	case "DOMAIN-SUFFIX":
		appendStringRuleValue(stringValues, "domain_suffix", strings.TrimPrefix(value, "."))
	case "DOMAIN-KEYWORD":
		appendStringRuleValue(stringValues, "domain_keyword", value)
	case "DOMAIN-REGEX":
		appendStringRuleValue(stringValues, "domain_regex", value)
	case "IP-CIDR", "IP-CIDR6":
		appendStringRuleValue(stringValues, "ip_cidr", value)
	case "SRC-IP-CIDR", "SOURCE-IP-CIDR":
		appendStringRuleValue(stringValues, "source_ip_cidr", value)
	case "GEOIP":
		appendStringRuleValue(stringValues, "geoip", strings.ToLower(value))
	case "GEOSITE":
		appendStringRuleValue(stringValues, "geosite", strings.ToLower(value))
	case "RULE-SET":
		appendStringRuleValue(stringValues, "rule_set", value)
	case "PROCESS-NAME":
		appendStringRuleValue(stringValues, "process_name", value)
	case "PROCESS-PATH":
		appendStringRuleValue(stringValues, "process_path", value)
	case "DST-PORT", "PORT":
		appendIntRuleValue(intValues, "port", value)
	case "SRC-PORT", "SOURCE-PORT":
		appendIntRuleValue(intValues, "source_port", value)
	}
}

func appendPlainClashRule(line string, stringValues map[string][]string) {
	value := strings.TrimSpace(line)
	value = strings.Trim(value, "'")
	value = strings.Trim(value, "\"")
	if value == "" {
		return
	}
	switch strings.ToUpper(value) {
	case "MATCH", "FINAL", "DIRECT", "REJECT", "PROXY":
		return
	}
	if _, _, err := net.ParseCIDR(value); err == nil {
		appendStringRuleValue(stringValues, "ip_cidr", value)
		return
	}
	if strings.Contains(value, ".") {
		field, domain := clashDomainBehaviorRule(value)
		appendStringRuleValue(stringValues, field, domain)
	}
}

func clashDomainBehaviorRule(value string) (string, string) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "'")
	value = strings.Trim(value, "\"")
	if strings.HasPrefix(value, "+.") {
		return "domain_suffix", strings.TrimPrefix(value, "+.")
	}
	if strings.HasPrefix(value, ".") {
		return "domain_suffix", strings.TrimPrefix(value, ".")
	}
	return "domain_suffix", value
}

func ruleEntryString(entry any) string {
	switch value := entry.(type) {
	case string:
		return value
	case fmt.Stringer:
		return value.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}

func splitClashRuleLine(line string) []string {
	parts := strings.Split(line, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		part = strings.Trim(part, "'")
		part = strings.Trim(part, "\"")
		out = append(out, part)
	}
	return out
}

func appendStringRuleValue(values map[string][]string, key string, value string) {
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	for _, existing := range values[key] {
		if existing == value {
			return
		}
	}
	values[key] = append(values[key], value)
}

func appendIntRuleValue(values map[string][]int, key string, value string) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port <= 0 || port > 65535 {
		return
	}
	for _, existing := range values[key] {
		if existing == port {
			return
		}
	}
	values[key] = append(values[key], port)
}

func sortedStringKeys(values map[string][]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sortedIntKeys(values map[string][]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
