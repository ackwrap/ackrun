package parser

import "testing"

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
	if ruleSet.Version != 3 || len(ruleSet.Rules) != 1 {
		t.Fatalf("unexpected rule set: %+v", ruleSet)
	}
	rule := ruleSet.Rules[0]
	domains, ok := rule["domain_suffix"].([]string)
	if !ok || len(domains) != 2 || domains[0] != "cerebras.ai" || domains[1] != "recraft.ai" {
		t.Fatalf("unexpected domain suffix values: %+v", rule["domain_suffix"])
	}
	processNames, ok := rule["process_name"].([]string)
	if !ok || len(processNames) != 1 || processNames[0] != "LM Studio" {
		t.Fatalf("unexpected process names: %+v", rule["process_name"])
	}
	ipCIDRs, ok := rule["ip_cidr"].([]string)
	if !ok || len(ipCIDRs) != 1 || ipCIDRs[0] != "1.1.1.0/24" {
		t.Fatalf("unexpected ip cidr values: %+v", rule["ip_cidr"])
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
	domains := ruleSet.Rules[0]["domain_suffix"].([]string)
	if len(domains) != 2 || domains[0] != "example.com" || domains[1] != "api.example.net" {
		t.Fatalf("unexpected domain behavior values: %+v", domains)
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
	cidrs := ruleSet.Rules[0]["ip_cidr"].([]string)
	if len(cidrs) != 2 || cidrs[0] != "8.128.0.0/10" || cidrs[1] != "8.209.32.0/22" {
		t.Fatalf("unexpected cidr payload values: %+v", cidrs)
	}
}
