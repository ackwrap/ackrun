package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"strings"
)

// private is published as an independent SRS and is absent from SagerNet's
// country database. Derive JSON and lookup prefixes from that same SRS.
const geoPrivateRuleSetMaxSize = 1024 * 1024

func (svc *RouteRuleService) sagerNetPrivateRuleSetSource(ctx context.Context) ([]byte, error) {
	if svc.paths == nil || strings.TrimSpace(svc.paths.BinaryPath) == "" {
		return nil, fmt.Errorf("sing-box 未安装，无法解码 SagerNet geoip-private")
	}
	binary, _, err := svc.GeneratedGeoRuleSetContentContext(ctx, "geoip-private")
	if err != nil {
		return nil, fmt.Errorf("读取 SagerNet geoip-private 失败: %w", err)
	}
	return decompileGeoPrivateRuleSet(ctx, svc.paths.BinaryPath, svc.paths.RulesDir, binary)
}

func (svc *RouteRuleService) sagerNetPrivatePrefixes(ctx context.Context) ([]netip.Prefix, error) {
	source, err := svc.sagerNetPrivateRuleSetSource(ctx)
	if err != nil {
		return nil, err
	}
	return geoPrivateRuleSetPrefixes(source)
}

func decompileGeoPrivateRuleSet(ctx context.Context, core, directory string, binary []byte) ([]byte, error) {
	if len(binary) > geoPrivateRuleSetMaxSize {
		return nil, fmt.Errorf("SagerNet geoip-private SRS 超过大小限制")
	}
	decompileCtx, cancel := context.WithTimeout(ctx, generatedGeoRuleSetValidationTimeout)
	defer cancel()
	if err := decompileCtx.Err(); err != nil {
		return nil, fmt.Errorf("解码 SagerNet geoip-private 失败: %w", err)
	}
	file, err := os.CreateTemp(directory, ".geoip-private-*.json")
	if err != nil {
		return nil, fmt.Errorf("创建 geoip-private 临时文件失败: %w", err)
	}
	path := file.Name()
	defer os.Remove(path)
	if err := file.Close(); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(decompileCtx, core, "rule-set", "decompile", "--output", path, "stdin")
	cmd.Stdin = bytes.NewReader(binary)
	if output, err := cmd.CombinedOutput(); err != nil {
		if decompileCtx.Err() != nil {
			return nil, fmt.Errorf("解码 SagerNet geoip-private 失败: %w", decompileCtx.Err())
		}
		message := strings.TrimSpace(cleanLogLine(string(output)))
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("sing-box 解码 SagerNet geoip-private 失败: %s", message)
	}
	file, err = os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("读取 geoip-private 解码结果失败: %w", err)
	}
	defer file.Close()
	source, err := io.ReadAll(io.LimitReader(file, geoPrivateRuleSetMaxSize+1))
	if err != nil {
		return nil, err
	}
	if _, err := geoPrivateRuleSetPrefixes(source); err != nil {
		return nil, err
	}
	return source, nil
}

// Keep lookup semantics exact: accepting other fields, inverse or logical
// rules and then reading only ip_cidr would silently change their meaning.
func geoPrivateRuleSetPrefixes(source []byte) ([]netip.Prefix, error) {
	if len(source) > geoPrivateRuleSetMaxSize {
		return nil, fmt.Errorf("SagerNet geoip-private JSON 超过大小限制")
	}
	var document struct {
		Version int `json:"version"`
		Rules   []struct {
			Type   string   `json:"type,omitempty"`
			IPCIDR []string `json:"ip_cidr"`
		} `json:"rules"`
	}
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("SagerNet geoip-private 不是纯 IP CIDR 规则集: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("SagerNet geoip-private JSON 包含多余内容")
	}
	if document.Version < 1 || document.Version > generatedGeoRuleSetVersionCurrent || len(document.Rules) == 0 {
		return nil, fmt.Errorf("SagerNet geoip-private 版本无效或规则为空")
	}
	var prefixes []netip.Prefix
	for _, rule := range document.Rules {
		if (rule.Type != "" && rule.Type != "default") || len(rule.IPCIDR) == 0 {
			return nil, fmt.Errorf("SagerNet geoip-private 包含非 IP CIDR 规则或空规则")
		}
		for _, value := range rule.IPCIDR {
			prefix, err := netip.ParsePrefix(value)
			if err != nil {
				return nil, fmt.Errorf("SagerNet geoip-private 包含无效 IP CIDR: %w", err)
			}
			prefixes = append(prefixes, prefix.Masked())
		}
	}
	return prefixes, nil
}
