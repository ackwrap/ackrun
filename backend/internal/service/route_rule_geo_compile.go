package service

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
)

// The caller holds the source/type database lock, also used by snapshot cleanup.
func (svc *RouteRuleService) compiledGeoRuleSetContent(ctx context.Context, tag, kind, code, databasePath, cacheDir string) ([]byte, string, error) {
	cachePath := filepath.Join(cacheDir, tag+".srs")
	if data, err := os.ReadFile(cachePath); err == nil {
		if err := validateGeneratedGeoRuleSet(data); err == nil {
			if err := svc.validateGeneratedGeoRuleSetFile(ctx, cachePath); err == nil {
				return data, "application/octet-stream", nil
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, "", fmt.Errorf("read compiled Geo rule set: %w", err)
	}
	data, err := geoDatabaseRuleSetSource(kind, code, databasePath)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, "", err
	}
	file, err := os.CreateTemp(cacheDir, "."+tag+"-*.srs")
	if err != nil {
		return nil, "", err
	}
	tmpPath := file.Name()
	defer os.Remove(tmpPath)
	if err := file.Close(); err != nil {
		return nil, "", err
	}
	if err := svc.compileGeoRuleSet(ctx, data, tmpPath); err != nil {
		return nil, "", err
	}
	binary, err := os.ReadFile(tmpPath)
	if err != nil {
		return nil, "", err
	}
	if err := validateGeneratedGeoRuleSet(binary); err != nil {
		return nil, "", fmt.Errorf("compiled Geo rule set is invalid: %w", err)
	}
	if err := atomicReplaceFile(tmpPath, cachePath); err != nil {
		return nil, "", err
	}
	logging.Info("route_rule_geo.compile", "已编译 Geo 分类为 SRS: %s，%d -> %d 字节", tag, len(data), len(binary))
	return binary, "application/octet-stream", nil
}

func (svc *RouteRuleService) compileGeoRuleSet(ctx context.Context, source []byte, outputPath string) error {
	compileCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if svc.ruleSetCompiler != nil {
		return svc.ruleSetCompiler(compileCtx, source, outputPath)
	}
	if svc.paths == nil || strings.TrimSpace(svc.paths.BinaryPath) == "" {
		return fmt.Errorf("sing-box 未安装，无法编译 Geo 规则集")
	}
	// JSON exists only in memory and the compiler's stdin; only SRS is cached.
	cmd := exec.CommandContext(compileCtx, svc.paths.BinaryPath, "rule-set", "compile", "--output", outputPath, "stdin")
	cmd.Stdin = bytes.NewReader(source)
	output, err := cmd.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(cleanLogLine(string(output)))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("sing-box 编译 Geo 规则集失败: %s", message)
	}
	return nil
}
