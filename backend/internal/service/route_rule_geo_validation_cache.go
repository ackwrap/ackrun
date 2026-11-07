package service

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

const geoValidationCacheMaxEntries = 256

type geoValidationKey struct {
	digest   [sha256.Size]byte
	corePath string
}

type geoRuleSetValidationCache struct {
	mu      sync.Mutex
	entries map[geoValidationKey]os.FileInfo
}

func (svc *RouteRuleService) geoValidationCore() (string, os.FileInfo, error) {
	if svc.paths == nil || svc.paths.BinaryPath == "" {
		if svc.ruleSetValidator != nil {
			return "", nil, nil
		}
		return "", nil, fmt.Errorf("sing-box 未安装，无法验证规则集")
	}
	path, err := exec.LookPath(svc.paths.BinaryPath)
	if err != nil {
		return "", nil, err
	}
	info, err := os.Stat(path)
	return path, info, err
}

func (svc *RouteRuleService) validateCachedGeoRuleSet(ctx context.Context, data []byte, path string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	core, info, err := svc.geoValidationCore()
	if err != nil {
		return err
	}
	key := geoValidationKey{digest: sha256.Sum256(data), corePath: core}
	cache := &svc.geoValidations
	cache.mu.Lock()
	previous, ok := cache.entries[key]
	cache.mu.Unlock()
	if ok && ((core == "" && previous == nil) || sameGeoFile(previous, info)) {
		return nil
	}
	if err := validateGeneratedGeoRuleSet(data); err != nil {
		return err
	}
	if err := svc.validateGeneratedGeoRuleSetFile(ctx, path); err != nil {
		return err
	}
	// A core replacement during validation must not certify bytes for the new core.
	afterCore, afterInfo, err := svc.geoValidationCore()
	if err != nil {
		return err
	}
	if core != afterCore || (core != "" && !sameGeoFile(info, afterInfo)) {
		return fmt.Errorf("sing-box changed during rule-set validation")
	}
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if cache.entries == nil {
		cache.entries = make(map[geoValidationKey]os.FileInfo)
	}
	if len(cache.entries) >= geoValidationCacheMaxEntries {
		for key := range cache.entries {
			delete(cache.entries, key)
			break
		}
	}
	cache.entries[key] = info
	return nil
}
