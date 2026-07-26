package parser

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"

	"github.com/ackwrap/ackrun/internal/model"
)

type clashRuleDocument struct {
	Behavior string `yaml:"behavior"`
	Payload  []any  `yaml:"payload"`
	Rules    []any  `yaml:"rules"`
}

const maxClashLogicalRuleDepth = 32

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
	switch behavior {
	case "", "classical", "domain", "ipcidr", "ip-cidr":
	default:
		return nil, fmt.Errorf("unsupported clash rule behavior %q", behavior)
	}

	rules := make([]map[string]any, 0, len(entries))
	for index, entry := range entries {
		line := strings.TrimSpace(ruleEntryString(entry))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		var rule map[string]any
		var err error
		switch behavior {
		case "domain":
			field, value := clashDomainBehaviorRule(line)
			if value == "" || strings.Contains(value, ",") {
				err = fmt.Errorf("invalid domain behavior value %q", line)
			} else {
				rule = map[string]any{field: []string{value}}
			}
		case "ipcidr", "ip-cidr":
			var value string
			value, err = parseClashDestinationCIDRValue("IP-CIDR", line, 0)
			if err == nil {
				rule = map[string]any{"ip_cidr": []string{value}}
			}
		default:
			rule, err = parseClashClassicalRule(line, 0)
		}
		if err != nil {
			return nil, fmt.Errorf("clash rule entry %d: %w", index+1, err)
		}
		if rule != nil {
			rules = append(rules, rule)
		}
	}
	if len(rules) == 0 {
		return nil, fmt.Errorf("clash rule yaml has no supported rules")
	}

	return &model.SingboxRuleSetSource{Version: 3, Rules: rules}, nil
}

func parseClashClassicalRule(line string, depth int) (map[string]any, error) {
	if depth > maxClashLogicalRuleDepth {
		return nil, fmt.Errorf("logical rule nesting exceeds %d levels", maxClashLogicalRuleDepth)
	}
	separator := strings.IndexByte(line, ',')
	if separator < 0 {
		return parsePlainClashRule(line)
	}
	ruleType := strings.ToUpper(strings.TrimSpace(line[:separator]))
	rawValue := strings.TrimSpace(line[separator+1:])
	if ruleType == "" || rawValue == "" {
		return nil, fmt.Errorf("invalid clash rule %q", line)
	}
	if ruleType == "AND" || ruleType == "OR" || ruleType == "NOT" {
		return parseClashLogicalRule(ruleType, rawValue, depth)
	}
	return parseClashSimpleRule(ruleType, rawValue)
}

func parseClashSimpleRule(ruleType string, rawValue string) (map[string]any, error) {
	switch ruleType {
	case "DOMAIN":
		value, err := singleClashRuleValue(ruleType, rawValue)
		return stringClashRule("domain", value), err
	case "DOMAIN-SUFFIX":
		value, err := singleClashRuleValue(ruleType, rawValue)
		return stringClashRule("domain_suffix", strings.TrimPrefix(value, ".")), err
	case "DOMAIN-KEYWORD":
		value, err := singleClashRuleValue(ruleType, rawValue)
		return stringClashRule("domain_keyword", value), err
	case "DOMAIN-REGEX":
		value := trimClashRuleValue(rawValue)
		if value == "" {
			return nil, fmt.Errorf("%s requires a value", ruleType)
		}
		if _, err := regexp.Compile(value); err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", ruleType, err)
		}
		return stringClashRule("domain_regex", value), nil
	case "IP-CIDR", "IP-CIDR6":
		version := 4
		if ruleType == "IP-CIDR6" {
			version = 6
		}
		value, err := parseClashDestinationCIDRValue(ruleType, rawValue, version)
		if err != nil {
			return nil, err
		}
		return stringClashRule("ip_cidr", value), nil
	case "SRC-IP-CIDR", "SOURCE-IP-CIDR":
		value, err := singleClashRuleValue(ruleType, rawValue)
		if err != nil {
			return nil, err
		}
		if err := validateClashCIDR(value, 0); err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", ruleType, err)
		}
		return stringClashRule("source_ip_cidr", value), nil
	case "PROCESS-NAME":
		value, err := singleClashRuleValue(ruleType, rawValue)
		return stringClashRule("process_name", value), err
	case "PROCESS-PATH":
		value, err := singleClashRuleValue(ruleType, rawValue)
		return stringClashRule("process_path", value), err
	case "PROCESS-PATH-REGEX":
		value := trimClashRuleValue(rawValue)
		if value == "" {
			return nil, fmt.Errorf("%s requires a value", ruleType)
		}
		if _, err := regexp.Compile(value); err != nil {
			return nil, fmt.Errorf("invalid %s value: %w", ruleType, err)
		}
		return stringClashRule("process_path_regex", value), nil
	case "DST-PORT", "PORT":
		return parseClashPortRule(rawValue, false)
	case "SRC-PORT", "SOURCE-PORT":
		return parseClashPortRule(rawValue, true)
	case "NETWORK":
		return parseClashNetworkRule(rawValue)
	case "GEOIP", "GEOSITE", "RULE-SET", "SUB-RULE":
		return nil, fmt.Errorf("unsupported clash rule type %q: no sing-box source rule equivalent", ruleType)
	default:
		return nil, fmt.Errorf("unsupported clash rule type %q", ruleType)
	}
}

func parsePlainClashRule(line string) (map[string]any, error) {
	value := trimClashRuleValue(line)
	if value == "" {
		return nil, fmt.Errorf("empty clash rule")
	}
	switch strings.ToUpper(value) {
	case "MATCH", "FINAL", "DIRECT", "REJECT", "PROXY":
		return nil, fmt.Errorf("unsupported standalone clash rule %q", value)
	}
	if _, _, err := net.ParseCIDR(value); err == nil {
		return stringClashRule("ip_cidr", value), nil
	}
	if strings.Contains(value, ".") {
		field, domain := clashDomainBehaviorRule(value)
		return stringClashRule(field, domain), nil
	}
	return nil, fmt.Errorf("unsupported plain clash rule %q", value)
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

func parseClashLogicalRule(ruleType string, rawValue string, depth int) (map[string]any, error) {
	operands, err := splitClashLogicalOperands(rawValue)
	if err != nil {
		return nil, fmt.Errorf("invalid %s rule: %w", ruleType, err)
	}
	if ruleType == "NOT" && len(operands) != 1 {
		return nil, fmt.Errorf("NOT requires exactly one operand")
	}
	if ruleType != "NOT" && len(operands) < 2 {
		return nil, fmt.Errorf("%s requires at least two operands", ruleType)
	}
	rules := make([]map[string]any, 0, len(operands))
	for index, operand := range operands {
		rule, err := parseClashClassicalRule(operand, depth+1)
		if err != nil {
			return nil, fmt.Errorf("%s operand %d: %w", ruleType, index+1, err)
		}
		rules = append(rules, rule)
	}
	mode := strings.ToLower(ruleType)
	logical := map[string]any{"type": "logical", "mode": mode, "rules": rules}
	if ruleType == "NOT" {
		logical["mode"] = "and"
		logical["invert"] = true
	}
	return logical, nil
}

func splitClashLogicalOperands(rawValue string) ([]string, error) {
	content, err := unwrapClashRuleGroup(rawValue)
	if err != nil {
		return nil, err
	}
	operands := make([]string, 0, 2)
	for position := 0; ; {
		for position < len(content) && (content[position] == ' ' || content[position] == '\t') {
			position++
		}
		if position == len(content) {
			break
		}
		if content[position] != '(' {
			return nil, fmt.Errorf("operand %d must be enclosed in parentheses", len(operands)+1)
		}
		end, err := matchingClashRuleParenthesis(content, position)
		if err != nil {
			return nil, err
		}
		operand := strings.TrimSpace(content[position+1 : end])
		if operand == "" {
			return nil, fmt.Errorf("operand %d is empty", len(operands)+1)
		}
		operands = append(operands, operand)
		position = end + 1
		for position < len(content) && (content[position] == ' ' || content[position] == '\t') {
			position++
		}
		if position == len(content) {
			break
		}
		if content[position] != ',' {
			return nil, fmt.Errorf("expected comma after operand %d", len(operands))
		}
		position++
		for position < len(content) && (content[position] == ' ' || content[position] == '\t') {
			position++
		}
		if position == len(content) {
			return nil, fmt.Errorf("operand %d is empty", len(operands)+1)
		}
	}
	if len(operands) == 0 {
		return nil, fmt.Errorf("logical rule has no operands")
	}
	return operands, nil
}

func unwrapClashRuleGroup(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '(' {
		return "", fmt.Errorf("operands must be enclosed in parentheses")
	}
	end, err := matchingClashRuleParenthesis(value, 0)
	if err != nil {
		return "", err
	}
	if end != len(value)-1 {
		return "", fmt.Errorf("unexpected content after logical operands")
	}
	return value[1:end], nil
}

func matchingClashRuleParenthesis(value string, start int) (int, error) {
	depth := 0
	bracketDepth := 0
	var quote byte
	escaped := false
	for index := start; index < len(value); index++ {
		character := value[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			}
			continue
		}
		if character == '\'' || character == '"' {
			quote = character
			continue
		}
		if character == '[' {
			bracketDepth++
			continue
		}
		if character == ']' && bracketDepth > 0 {
			bracketDepth--
			continue
		}
		if bracketDepth > 0 {
			continue
		}
		switch character {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return index, nil
			}
			if depth < 0 {
				return 0, fmt.Errorf("unexpected closing parenthesis")
			}
		}
	}
	return 0, fmt.Errorf("unclosed parenthesis")
}

func parseClashDestinationCIDRValue(ruleType string, rawValue string, version int) (string, error) {
	parts := strings.Split(rawValue, ",")
	if len(parts) > 2 {
		return "", fmt.Errorf("%s has unsupported extra parameters", ruleType)
	}
	value := trimClashRuleValue(parts[0])
	if len(parts) == 2 && !strings.EqualFold(trimClashRuleValue(parts[1]), "no-resolve") {
		return "", fmt.Errorf("%s has unsupported option %q", ruleType, trimClashRuleValue(parts[1]))
	}
	if err := validateClashCIDR(value, version); err != nil {
		return "", fmt.Errorf("invalid %s value: %w", ruleType, err)
	}
	// sing-box headless IP rules only inspect destination addresses already present
	// in metadata, so Mihomo's no-resolve modifier needs no output field.
	return value, nil
}

func validateClashCIDR(value string, version int) error {
	ip, _, err := net.ParseCIDR(value)
	if err != nil {
		return err
	}
	if version == 4 && ip.To4() == nil {
		return fmt.Errorf("expected an IPv4 CIDR")
	}
	if version == 6 && ip.To4() != nil {
		return fmt.Errorf("expected an IPv6 CIDR")
	}
	return nil
}

func parseClashPortRule(rawValue string, source bool) (map[string]any, error) {
	values := make([]string, 0, strings.Count(rawValue, "/")+strings.Count(rawValue, ",")+1)
	for _, slashPart := range strings.Split(rawValue, "/") {
		values = append(values, strings.Split(slashPart, ",")...)
	}
	ports := make([]int, 0, len(values))
	ranges := make([]string, 0, len(values))
	for _, rawPort := range values {
		value := trimClashRuleValue(rawPort)
		if value == "" {
			return nil, fmt.Errorf("port rule contains an empty value")
		}
		if strings.Contains(value, "-") {
			bounds := strings.Split(value, "-")
			if len(bounds) != 2 {
				return nil, fmt.Errorf("invalid port range %q", value)
			}
			start, err := parseClashPort(bounds[0])
			if err != nil {
				return nil, fmt.Errorf("invalid port range %q: %w", value, err)
			}
			end, err := parseClashPort(bounds[1])
			if err != nil {
				return nil, fmt.Errorf("invalid port range %q: %w", value, err)
			}
			if start > end {
				return nil, fmt.Errorf("invalid descending port range %q", value)
			}
			ranges = append(ranges, fmt.Sprintf("%d:%d", start, end))
			continue
		}
		port, err := parseClashPort(value)
		if err != nil {
			return nil, err
		}
		ports = append(ports, port)
	}
	if len(ports) == 0 && len(ranges) == 0 {
		return nil, fmt.Errorf("port rule requires at least one port")
	}
	prefix := ""
	if source {
		prefix = "source_"
	}
	rule := map[string]any{}
	if len(ports) > 0 {
		rule[prefix+"port"] = ports
	}
	if len(ranges) > 0 {
		rule[prefix+"port_range"] = ranges
	}
	return rule, nil
}

func parseClashPort(value string) (int, error) {
	port, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("invalid port %q", value)
	}
	return port, nil
}

func parseClashNetworkRule(rawValue string) (map[string]any, error) {
	parts := strings.Split(rawValue, "/")
	networks := make([]string, 0, len(parts))
	for _, part := range parts {
		network := strings.ToLower(trimClashRuleValue(part))
		if network != "tcp" && network != "udp" {
			return nil, fmt.Errorf("unsupported NETWORK value %q", network)
		}
		networks = append(networks, network)
	}
	return map[string]any{"network": networks}, nil
}

func singleClashRuleValue(ruleType string, rawValue string) (string, error) {
	if strings.Contains(rawValue, ",") {
		return "", fmt.Errorf("%s has unsupported extra parameters", ruleType)
	}
	value := trimClashRuleValue(rawValue)
	if value == "" {
		return "", fmt.Errorf("%s requires a value", ruleType)
	}
	return value, nil
}

func trimClashRuleValue(value string) string {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && ((value[0] == '\'' && value[len(value)-1] == '\'') || (value[0] == '"' && value[len(value)-1] == '"')) {
		return strings.TrimSpace(value[1 : len(value)-1])
	}
	return value
}

func stringClashRule(field string, value string) map[string]any {
	return map[string]any{field: []string{value}}
}
