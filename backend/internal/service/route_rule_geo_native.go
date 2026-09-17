package service

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

func (svc *RouteRuleService) loyalsoldierGeoIPRuleSetContent(ctx context.Context, tag string) ([]byte, string, error) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	code, ok := strings.CutPrefix(tag, "geoip-")
	if !ok || !geoIPCodePattern.MatchString(code) {
		return nil, "", fmt.Errorf("invalid Loyalsoldier GeoIP rule set tag")
	}
	if svc.paths == nil || svc.paths.RulesDir == "" {
		return nil, "", fmt.Errorf("rules directory is not configured")
	}
	upstreamURL := "https://raw.githubusercontent.com/Loyalsoldier/geoip/release/srs/" + code + ".srs"
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo", "loyalsoldier", "native")
	return svc.cachedGeneratedGeoRuleSetContentContext(ctx, tag, upstreamURL, cacheDir)
}
