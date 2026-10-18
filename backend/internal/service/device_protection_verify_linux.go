//go:build linux

package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"sort"
)

// Keep the compiler's echo of our own transaction, rather than trusting a later
// ruleset dump as the expected policy. A concurrent external edit must not become
// the new baseline. The JSON sidecar is not loaded by fw4's *.nft include.
func (g *nftDeviceProtection) verificationPath() string { return g.path + ".json" }

type protectionKernelPolicy struct {
	Table  map[string]any              `json:"table"`
	Chains map[string]map[string]any   `json:"chains"`
	Rules  map[string][]map[string]any `json:"rules"`
}

type protectionKernelRecord struct {
	Version int                    `json:"version"`
	Policy  protectionKernelPolicy `json:"policy"`
}

func parseProtectionKernelPolicy(data []byte, echo bool) (protectionKernelPolicy, error) {
	var document struct {
		NFTables []map[string]json.RawMessage `json:"nftables"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return protectionKernelPolicy{}, fmt.Errorf("解析设备防泄漏内核规则内容失败")
	}
	policy := protectionKernelPolicy{Chains: map[string]map[string]any{}, Rules: map[string][]map[string]any{}}
	for _, entry := range document.NFTables {
		if echo {
			if raw, ok := entry["delete"]; ok {
				var deletion struct {
					Table *struct{ Family, Name string } `json:"table"`
				}
				if json.Unmarshal(raw, &deletion) != nil || deletion.Table == nil || deletion.Table.Family != "inet" || deletion.Table.Name != deviceProtectionTable {
					return policy, fmt.Errorf("设备防泄漏安装回执包含未知删除操作")
				}
				policy = protectionKernelPolicy{Chains: map[string]map[string]any{}, Rules: map[string][]map[string]any{}}
				continue
			}
			if raw, ok := entry["add"]; ok {
				var added map[string]json.RawMessage
				if err := json.Unmarshal(raw, &added); err != nil {
					return policy, fmt.Errorf("设备防泄漏安装回执无效")
				}
				entry = added
			}
		}
		if _, ok := entry["metainfo"]; ok && len(entry) == 1 {
			continue
		}
		if len(entry) != 1 {
			return policy, fmt.Errorf("设备防泄漏内核对象无效")
		}
		for kind, raw := range entry {
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil || object == nil || object["family"] != "inet" {
				return policy, fmt.Errorf("设备防泄漏内核对象无效")
			}
			delete(object, "handle")
			switch kind {
			case "table":
				// On first install, the idempotent add/delete preamble echoes
				// an empty table. nft --echo does not emit its deletion.
				if echo && object["name"] == deviceProtectionTable && len(object) == 2 && policy.Table == nil && len(policy.Chains) == 0 {
					continue
				}
				if object["name"] != deviceProtectionTable || policy.Table != nil {
					return policy, fmt.Errorf("设备防泄漏内核表无效")
				}
				policy.Table = object
			case "chain", "rule":
				if object["table"] != deviceProtectionTable {
					return policy, fmt.Errorf("设备防泄漏内核对象不属于受管表")
				}
				if kind == "chain" {
					name, _ := object["name"].(string)
					if name == "" || policy.Chains[name] != nil {
						return policy, fmt.Errorf("设备防泄漏内核链无效")
					}
					policy.Chains[name] = object
				} else {
					chain, _ := object["chain"].(string)
					expressions, ok := object["expr"].([]any)
					if chain == "" || !ok || len(expressions) == 0 {
						return policy, fmt.Errorf("设备防泄漏内核规则缺少条件或动作")
					}
					// Only counter values and object handles are runtime state.
					// Preserve expression and per-chain rule order, hooks and actions.
					for _, expression := range expressions {
						if statement, ok := expression.(map[string]any); ok {
							if counter, ok := statement["counter"].(map[string]any); ok {
								delete(counter, "packets")
								delete(counter, "bytes")
							}
						}
					}
					normalizeProtectionSets(object)
					policy.Rules[chain] = append(policy.Rules[chain], object)
				}
			default:
				return policy, fmt.Errorf("设备防泄漏内核表包含意外对象")
			}
		}
	}
	if policy.Table == nil || (len(policy.Chains) != 3 && len(policy.Chains) != 5) || len(policy.Rules) != len(policy.Chains) {
		return policy, fmt.Errorf("设备防泄漏内核链缺失或不完整")
	}
	if len(policy.Chains) == 5 && (policy.Chains["mac_ingress"] == nil || len(policy.Rules["mac_ingress"]) == 0 || policy.Chains["mac_route"] == nil || len(policy.Rules["mac_route"]) == 0) {
		return policy, fmt.Errorf("设备 MAC 入口链缺失或不完整")
	}
	for _, name := range []string{"forward", "input", "prerouting"} {
		if policy.Chains[name] == nil || len(policy.Rules[name]) == 0 {
			return policy, fmt.Errorf("设备防泄漏内核链缺失或不完整")
		}
	}
	return policy, nil
}

// Anonymous set element order is not semantically meaningful. nft may enumerate
// its hash/interval sets differently after a reload; rule order remains exact.
func normalizeProtectionSets(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			normalizeProtectionSets(child)
			if elements, ok := child.([]any); key == "set" && ok {
				sort.Slice(elements, func(i, j int) bool {
					a, _ := json.Marshal(elements[i])
					b, _ := json.Marshal(elements[j])
					return bytes.Compare(a, b) < 0
				})
			}
		}
	case []any:
		for _, child := range value {
			normalizeProtectionSets(child)
		}
	}
}

func (g *nftDeviceProtection) recordKernelPolicy(echo []byte, owner string) error {
	policy, err := parseProtectionKernelPolicy(echo, true)
	if err != nil {
		return err
	}
	if policy.Table["comment"] != owner {
		return fmt.Errorf("设备防泄漏安装回执与目标策略不一致")
	}
	data, err := json.Marshal(protectionKernelRecord{Version: 1, Policy: policy})
	if err != nil {
		return err
	}
	return writeProtectionFile(g.verificationPath(), data)
}

func (g *nftDeviceProtection) verifyKernelPolicy(owner string) error {
	data, err := os.ReadFile(g.verificationPath())
	if err != nil {
		return fmt.Errorf("设备防泄漏缺少内核规则校验记录，请停止核心后重新启用保护")
	}
	var record protectionKernelRecord
	if json.Unmarshal(data, &record) != nil || record.Version != 1 || record.Policy.Table["comment"] != owner {
		return fmt.Errorf("设备防泄漏内核规则校验记录无效，请停止核心后重新启用保护")
	}
	data, err = g.run("", "-j", "list", "table", "inet", deviceProtectionTable)
	if err != nil {
		return fmt.Errorf("读取设备防泄漏内核规则内容失败: %w", err)
	}
	actual, err := parseProtectionKernelPolicy(data, false)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, record.Policy) {
		return fmt.Errorf("设备防泄漏内核规则内容或链配置已改变，请停止核心后重新启用保护")
	}
	return nil
}
