package parser

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseClashRuleSetYAMLClassicalPayload(t *testing.T) {
	body := []byte(`payload:
  - DOMAIN-SUFFIX,cerebras.ai
  - DOMAIN-SUFFIX,recraft.ai
  - PROCESS-NAME,LM Studio
  - IP-CIDR,1.1.1.0/24,no-resolve
  - DOMAIN-SUFFIX,cerebras.ai
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse clash rule yaml: %v", err)
	}
	if ruleSet.Version != 3 || len(ruleSet.Rules) != 5 {
		t.Fatalf("unexpected rule set: %+v", ruleSet)
	}
	want := []map[string]any{
		{"domain_suffix": []string{"cerebras.ai"}},
		{"domain_suffix": []string{"recraft.ai"}},
		{"process_name": []string{"LM Studio"}},
		{"ip_cidr": []string{"1.1.1.0/24"}},
		{"domain_suffix": []string{"cerebras.ai"}},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("classical rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLDomainBehavior(t *testing.T) {
	body := []byte(`behavior: domain
payload:
  - '+.example.com'
  - api.example.net
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse clash rule yaml: %v", err)
	}
	want := []map[string]any{
		{"domain_suffix": []string{"example.com"}},
		{"domain_suffix": []string{"api.example.net"}},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("domain behavior rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLPlainCIDRPayload(t *testing.T) {
	body := []byte(`payload:
  - '8.128.0.0/10'
  - '8.209.32.0/22'
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse clash rule yaml: %v", err)
	}
	want := []map[string]any{
		{"ip_cidr": []string{"8.128.0.0/10"}},
		{"ip_cidr": []string{"8.209.32.0/22"}},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("plain CIDR rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLTelegramPayload(t *testing.T) {
	body := []byte(`payload:
  - DOMAIN-KEYWORD,telegram
  - DOMAIN-SUFFIX,t.me
  - DOMAIN-SUFFIX,tdesktop.com
  - DOMAIN-SUFFIX,telegra.ph
  - DOMAIN-SUFFIX,telegram.me
  - DOMAIN-SUFFIX,telegram.org
  - DOMAIN-SUFFIX,telesco.pe
  - IP-CIDR,91.108.0.0/16,no-resolve
  - IP-CIDR,95.161.64.0/20,no-resolve
  - IP-CIDR,109.239.140.0/24,no-resolve
  - IP-CIDR,149.154.160.0/20,no-resolve
  - IP-CIDR6,2001:67c:4e8::/48,no-resolve
  - IP-CIDR6,2001:b28:f23d::/48,no-resolve
  - IP-CIDR6,2001:b28:f23f::/48,no-resolve
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse Telegram rule yaml: %v", err)
	}
	if len(ruleSet.Rules) != 14 {
		t.Fatalf("Telegram rule count = %d, want 14", len(ruleSet.Rules))
	}
	if !reflect.DeepEqual(ruleSet.Rules[0], map[string]any{"domain_keyword": []string{"telegram"}}) {
		t.Fatalf("unexpected keyword rule: %#v", ruleSet.Rules[0])
	}
	if !reflect.DeepEqual(ruleSet.Rules[7], map[string]any{"ip_cidr": []string{"91.108.0.0/16"}}) {
		t.Fatalf("unexpected IPv4 rule: %#v", ruleSet.Rules[7])
	}
	if !reflect.DeepEqual(ruleSet.Rules[13], map[string]any{"ip_cidr": []string{"2001:b28:f23f::/48"}}) {
		t.Fatalf("unexpected IPv6 rule: %#v", ruleSet.Rules[13])
	}
}

func TestParseClashRuleSetYAMLLogicalRules(t *testing.T) {
	body := []byte(`behavior: classical
payload:
  - 'AND,((DOMAIN-SUFFIX,example.com),(OR,((NETWORK,UDP),(DST-PORT,443))))'
  - 'NOT,((IP-CIDR,192.0.2.0/24,no-resolve))'
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse logical rule yaml: %v", err)
	}
	want := []map[string]any{
		{
			"type": "logical",
			"mode": "and",
			"rules": []map[string]any{
				{"domain_suffix": []string{"example.com"}},
				{
					"type": "logical",
					"mode": "or",
					"rules": []map[string]any{
						{"network": []string{"udp"}},
						{"port": []int{443}},
					},
				},
			},
		},
		{
			"type":   "logical",
			"mode":   "and",
			"rules":  []map[string]any{{"ip_cidr": []string{"192.0.2.0/24"}}},
			"invert": true,
		},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("logical rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLPortRanges(t *testing.T) {
	body := []byte(`payload:
  - DST-PORT,80/443/1000-2000
  - SRC-PORT,53,123,500-600
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse port rules: %v", err)
	}
	want := []map[string]any{
		{"port": []int{80, 443}, "port_range": []string{"1000:2000"}},
		{"source_port": []int{53, 123}, "source_port_range": []string{"500:600"}},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("port rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLRegexRules(t *testing.T) {
	body := []byte(`payload:
  - 'DOMAIN-REGEX,^foo,(bar|baz)$'
  - 'PROCESS-PATH-REGEX,^/usr/(local/)?bin/'
`)
	ruleSet, err := ParseClashRuleSetYAML(body)
	if err != nil {
		t.Fatalf("parse regex rules: %v", err)
	}
	want := []map[string]any{
		{"domain_regex": []string{"^foo,(bar|baz)$"}},
		{"process_path_regex": []string{"^/usr/(local/)?bin/"}},
	}
	if !reflect.DeepEqual(ruleSet.Rules, want) {
		t.Fatalf("regex rules = %#v, want %#v", ruleSet.Rules, want)
	}
}

func TestParseClashRuleSetYAMLLogicalDepthLimit(t *testing.T) {
	nestedRule := func(depth int) string {
		rule := "DOMAIN,example.com"
		for range depth {
			rule = "NOT,((" + rule + "))"
		}
		return rule
	}
	if _, err := ParseClashRuleSetYAML([]byte("payload:\n  - \"" + nestedRule(maxClashLogicalRuleDepth) + "\"\n")); err != nil {
		t.Fatalf("maximum supported logical depth failed: %v", err)
	}
	_, err := ParseClashRuleSetYAML([]byte("payload:\n  - \"" + nestedRule(maxClashLogicalRuleDepth+1) + "\"\n"))
	if err == nil || !strings.Contains(err.Error(), "logical rule nesting exceeds") {
		t.Fatalf("logical depth error = %v", err)
	}
}

func TestParseClashRuleSetYAMLRejectsUnsupportedOrMalformedRules(t *testing.T) {
	testCases := []struct {
		name    string
		entry   string
		message string
	}{
		{name: "unsupported", entry: "GEOIP,CN", message: `unsupported clash rule type "GEOIP"`},
		{name: "modifier", entry: "IP-CIDR,192.0.2.0/24,resolve", message: `unsupported option "resolve"`},
		{name: "logical arity", entry: "AND,((DOMAIN,example.com))", message: "AND requires at least two operands"},
		{name: "logical parentheses", entry: "OR,(DOMAIN,example.com)", message: "operand 1 must be enclosed in parentheses"},
		{name: "logical trailing comma", entry: "OR,((DOMAIN,example.com),(NETWORK,TCP),)", message: "operand 3 is empty"},
		{name: "invalid cidr", entry: "IP-CIDR,not-a-cidr", message: "invalid IP-CIDR value"},
		{name: "empty port middle", entry: "DST-PORT,80//443", message: "port rule contains an empty value"},
		{name: "empty port trailing", entry: "DST-PORT,80/", message: "port rule contains an empty value"},
		{name: "empty source port", entry: "SRC-PORT,,53", message: "port rule contains an empty value"},
		{name: "zero port", entry: "DST-PORT,0", message: `invalid port "0"`},
		{name: "oversized port", entry: "DST-PORT,65536", message: `invalid port "65536"`},
		{name: "descending port range", entry: "DST-PORT,2000-1000", message: `invalid descending port range "2000-1000"`},
		{name: "invalid domain regex", entry: "DOMAIN-REGEX,[", message: "invalid DOMAIN-REGEX value"},
		{name: "invalid process regex", entry: "PROCESS-PATH-REGEX,(unclosed", message: "invalid PROCESS-PATH-REGEX value"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ParseClashRuleSetYAML([]byte("payload:\n  - '" + testCase.entry + "'\n"))
			if err == nil || !strings.Contains(err.Error(), testCase.message) {
				t.Fatalf("error = %v, want message containing %q", err, testCase.message)
			}
			if !strings.Contains(err.Error(), "entry 1") {
				t.Fatalf("error does not identify payload entry: %v", err)
			}
		})
	}
}
