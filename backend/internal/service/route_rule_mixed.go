package service

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

var geoIPCodePattern = regexp.MustCompile(`^[a-z0-9_-]+$`)

var geoIPCodeSet = map[string]bool{
	"private": true,
	"ad":      true, "ae": true, "af": true, "ag": true, "ai": true, "al": true, "am": true, "ao": true, "aq": true, "ar": true, "as": true, "at": true, "au": true, "aw": true, "ax": true, "az": true,
	"ba": true, "bb": true, "bd": true, "be": true, "bf": true, "bg": true, "bh": true, "bi": true, "bj": true, "bl": true, "bm": true, "bn": true, "bo": true, "bq": true, "br": true, "bs": true, "bt": true, "bv": true, "bw": true, "by": true, "bz": true,
	"ca": true, "cc": true, "cd": true, "cf": true, "cg": true, "ch": true, "ci": true, "ck": true, "cl": true, "cm": true, "cn": true, "co": true, "cr": true, "cu": true, "cv": true, "cw": true, "cx": true, "cy": true, "cz": true,
	"de": true, "dj": true, "dk": true, "dm": true, "do": true, "dz": true,
	"ec": true, "ee": true, "eg": true, "eh": true, "er": true, "es": true, "et": true, "eu": true,
	"fi": true, "fj": true, "fk": true, "fm": true, "fo": true, "fr": true,
	"ga": true, "gb": true, "gd": true, "ge": true, "gf": true, "gg": true, "gh": true, "gi": true, "gl": true, "gm": true, "gn": true, "gp": true, "gq": true, "gr": true, "gs": true, "gt": true, "gu": true, "gw": true, "gy": true,
	"hk": true, "hm": true, "hn": true, "hr": true, "ht": true, "hu": true,
	"id": true, "ie": true, "il": true, "im": true, "in": true, "io": true, "iq": true, "ir": true, "is": true, "it": true,
	"je": true, "jm": true, "jo": true, "jp": true,
	"ke": true, "kg": true, "kh": true, "ki": true, "km": true, "kn": true, "kp": true, "kr": true, "kw": true, "ky": true, "kz": true,
	"la": true, "lb": true, "lc": true, "li": true, "lk": true, "lr": true, "ls": true, "lt": true, "lu": true, "lv": true, "ly": true,
	"ma": true, "mc": true, "md": true, "me": true, "mf": true, "mg": true, "mh": true, "mk": true, "ml": true, "mm": true, "mn": true, "mo": true, "mp": true, "mq": true, "mr": true, "ms": true, "mt": true, "mu": true, "mv": true, "mw": true, "mx": true, "my": true, "mz": true,
	"na": true, "nc": true, "ne": true, "nf": true, "ng": true, "ni": true, "nl": true, "no": true, "np": true, "nr": true, "nu": true, "nz": true,
	"om": true,
	"pa": true, "pe": true, "pf": true, "pg": true, "ph": true, "pk": true, "pl": true, "pm": true, "pn": true, "pr": true, "ps": true, "pt": true, "pw": true, "py": true,
	"qa": true, "re": true, "ro": true, "rs": true, "ru": true, "rw": true,
	"sa": true, "sb": true, "sc": true, "sd": true, "se": true, "sg": true, "sh": true, "si": true, "sj": true, "sk": true, "sl": true, "sm": true, "sn": true, "so": true, "sr": true, "ss": true, "st": true, "sv": true, "sx": true, "sy": true, "sz": true,
	"tc": true, "td": true, "tf": true, "tg": true, "th": true, "tj": true, "tk": true, "tl": true, "tm": true, "tn": true, "to": true, "tr": true, "tt": true, "tv": true, "tw": true, "tz": true,
	"ua": true, "ug": true, "um": true, "us": true, "uy": true, "uz": true,
	"va": true, "vc": true, "ve": true, "vg": true, "vi": true, "vn": true, "vu": true,
	"wf": true, "ws": true, "ye": true, "yt": true, "za": true, "zm": true, "zw": true,
}

type mixedRouteRuleValue struct {
	RuleType string
	Value    string
}

func parseMixedRouteRuleValues(values []string) ([]mixedRouteRuleValue, error) {
	items := make([]mixedRouteRuleValue, 0, len(values))
	for _, raw := range values {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		sep := strings.IndexAny(line, ":=")
		if sep <= 0 || sep >= len(line)-1 {
			return nil, fmt.Errorf("mixed rule value must use type:value format: %s", line)
		}
		ruleType := strings.TrimSpace(line[:sep])
		value := strings.TrimSpace(line[sep+1:])
		if !isRouteRuleType(ruleType) || ruleType == "mixed" {
			return nil, fmt.Errorf("unsupported mixed rule type: %s", ruleType)
		}
		if value == "" {
			return nil, fmt.Errorf("mixed rule value cannot be empty: %s", line)
		}
		items = append(items, mixedRouteRuleValue{RuleType: ruleType, Value: value})
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("mixed rule values are required")
	}
	return items, nil
}

func isRouteRuleType(ruleType string) bool {
	switch ruleType {
	case "domain", "domain_suffix", "domain_keyword", "ip_cidr", "geoip", "geosite", "rule_set", "mixed":
		return true
	default:
		return false
	}
}

func singboxRouteRule(ruleType string, values []string, outbound string, invert bool) map[string]interface{} {
	ruleMap := routeRuleAction(outbound)
	switch ruleType {
	case "geoip", "geosite":
		ruleMap["rule_set"] = generatedGeoRuleSetTags(ruleType, values)
	case "rule_set":
		ruleMap["rule_set"] = values
	default:
		ruleMap[routeRuleSingboxKey(ruleType)] = values
	}
	if invert {
		ruleMap["invert"] = true
	}
	return ruleMap
}

func routeRuleAction(outbound string) map[string]interface{} {
	if outbound == "block" {
		return map[string]interface{}{"action": "reject"}
	}
	return map[string]interface{}{"action": "route", "outbound": outbound}
}

func mixedSingboxRouteRules(values []string, outbound string, invert bool) ([]map[string]interface{}, error) {
	items, err := parseMixedRouteRuleValues(values)
	if err != nil {
		return nil, err
	}
	rules := make([]map[string]interface{}, 0)
	groupIndex := make(map[string]int)

	appendGrouped := func(key string, value string) {
		if index, ok := groupIndex[key]; ok {
			existing, _ := rules[index][key].([]string)
			rules[index][key] = append(existing, value)
			return
		}
		rule := routeRuleAction(outbound)
		rule[key] = []string{value}
		if invert {
			rule["invert"] = true
		}
		groupIndex[key] = len(rules)
		rules = append(rules, rule)
	}

	for _, item := range items {
		switch item.RuleType {
		case "geoip", "geosite":
			for _, tag := range generatedGeoRuleSetTags(item.RuleType, []string{item.Value}) {
				appendGrouped("rule_set", tag)
			}
		case "rule_set":
			appendGrouped("rule_set", item.Value)
		default:
			appendGrouped(routeRuleSingboxKey(item.RuleType), item.Value)
		}
	}
	return rules, nil
}

func addMixedGeneratedRuleSets(ruleSets []map[string]interface{}, seen map[string]bool, values []string, baseURL string) []map[string]interface{} {
	items, err := parseMixedRouteRuleValues(values)
	if err != nil {
		return ruleSets
	}
	for _, item := range items {
		if item.RuleType == "geoip" || item.RuleType == "geosite" {
			ruleSets = appendGeneratedGeoRuleSets(ruleSets, seen, item.RuleType, []string{item.Value}, baseURL)
		}
	}
	return ruleSets
}

func validateIPCidrValues(values []string) error {
	for _, raw := range values {
		value := strings.TrimSpace(raw)
		if value == "" {
			continue
		}
		if _, err := netip.ParsePrefix(value); err == nil {
			continue
		}
		if _, err := netip.ParseAddr(value); err == nil {
			continue
		}
		return fmt.Errorf("invalid ip_cidr value: %s", value)
	}
	return nil
}

func validateGeoIPValues(values []string) error {
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "" {
			continue
		}
		if strings.ContainsAny(value, "/.:@") {
			return fmt.Errorf("geoip value must be a region code, not IP/domain: %s", raw)
		}
		if !geoIPCodePattern.MatchString(value) {
			return fmt.Errorf("invalid geoip code: %s", raw)
		}
		if !geoIPCodeSet[value] {
			return fmt.Errorf("geoip code 不存在或不支持: %s", raw)
		}
	}
	return nil
}

func validateMixedRouteRuleValues(values []string) error {
	items, err := parseMixedRouteRuleValues(values)
	if err != nil {
		return err
	}
	for _, item := range items {
		switch item.RuleType {
		case "ip_cidr":
			if err := validateIPCidrValues([]string{item.Value}); err != nil {
				return err
			}
		case "geoip":
			if err := validateGeoIPValues([]string{item.Value}); err != nil {
				return err
			}
		}
	}
	return nil
}
