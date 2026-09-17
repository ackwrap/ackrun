package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/geoquery"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

const geoDatabaseMaxSize = geoquery.MaxDatabaseSize

func (svc *RouteRuleService) validateGeoIPRuleValues(kind string, values []string) error {
	var wanted []string
	if kind == "geoip" {
		wanted = values
	} else if kind == "mixed" {
		items, err := parseMixedRouteRuleValues(values)
		if err != nil {
			return err
		}
		for _, item := range items {
			if item.RuleType == "geoip" {
				wanted = append(wanted, item.Value)
			}
		}
	}
	if len(wanted) == 0 {
		return nil
	}
	if handled, err := svc.validateLoyalsoldierCategories("geoip", wanted); handled {
		return err
	}
	codes, ready, err := svc.loadGeoTags("geoip")
	if err != nil {
		return err
	}
	if !ready {
		return fmt.Errorf("GeoIP 数据库未就绪，请先同步后再选择分类")
	}
	known := make(map[string]bool, len(codes))
	for _, code := range codes {
		known[code] = true
	}
	for _, value := range wanted {
		if !known[strings.ToLower(strings.TrimSpace(value))] {
			return fmt.Errorf("当前来源不存在 GeoIP 分类: %s", value)
		}
	}
	return nil
}

// PrepareGeoSource leaves the selected source and its usable files unchanged on failure.
func (svc *RouteRuleService) PrepareGeoSource(source string) ([]model.GeoAsset, error) {
	source, err := model.NormalizeGeoSource(source)
	if err != nil {
		return nil, err
	}
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return nil, err
	}
	logging.Info("geo.source", "准备切换 Geo 来源: %s", source)
	for i := range assets {
		asset := &assets[i]
		asset.Source = source
		asset.URL = model.GeoSourceAssetURL(source, asset.Type)
		asset.LocalPath, err = svc.fetchAndCacheGeoAsset(asset, nil)
		if err != nil {
			logging.Error("geo.source", "目标来源下载或校验失败，保留原来源: %s/%s: %v", source, asset.Type, err)
			return nil, fmt.Errorf("%s %s 下载或校验失败，原来源未更改: %w", source, asset.Name, err)
		}
	}
	if err := svc.validateGeoSourceCategories(assets); err != nil {
		logging.Error("geo.source", "目标来源缺少所需分类，保留原来源: %s: %v", source, err)
		return nil, err
	}
	return assets, nil
}

func (svc *RouteRuleService) requiredGeneratedGeoCategories() (map[string]bool, error) {
	wanted := map[string]bool{}
	rules, err := svc.store.ListRouteRules()
	if err != nil {
		return nil, err
	}
	add := func(kind string, values []string) {
		if kind == "geoip" || kind == "geosite" {
			for _, tag := range generatedGeoRuleSetTags(kind, values) {
				wanted[tag] = true
			}
		}
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		add(rule.RuleType, rule.Values)
		if rule.RuleType == "mixed" {
			values, err := parseMixedRouteRuleValues(rule.Values)
			if err != nil {
				return nil, err
			}
			for _, value := range values {
				add(value.RuleType, []string{value.Value})
			}
		}
	}
	dnsRules, err := svc.store.ListDNSRules()
	if err != nil {
		return nil, err
	}
	for _, rule := range dnsRules {
		if rule.Enabled {
			add("geosite", dnsRuleStringConditions(decodeDNSRuleConditions(rule.ConditionsJSON), "geosite"))
		}
	}
	subscriptions, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		return nil, err
	}
	for _, subscription := range subscriptions {
		if subscription.Enabled {
			// Preview and config generation give explicitly subscribed sets
			// ownership of their tags before adding generated Geo sets.
			delete(wanted, subscription.Tag)
		}
	}
	return wanted, nil
}

func (svc *RouteRuleService) validateGeoSourceCategories(assets []model.GeoAsset) error {
	wanted, err := svc.requiredGeneratedGeoCategories()
	if err != nil {
		return err
	}
	catalog, err := svc.geoCategoryCatalog(assets)
	if err != nil {
		return err
	}
	var missing []string
	for tag := range wanted {
		if _, err := catalog.resolve(tag); err != nil {
			missing = append(missing, err.Error())
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("已启用规则的 Geo 分类不可用: %s；原来源及有效数据未更改", strings.Join(missing, "; "))
	}
	return nil
}

func geoAssetCodes(kind, path string) ([]string, error) {
	if kind == "geoip" {
		reader, err := geoquery.OpenGeoIP(path)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return reader.Codes()
	}
	reader, codes, err := geoquery.OpenGeosite(path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	// Validate all entries before a downloaded file can replace a working database.
	for _, code := range codes {
		items, err := reader.Read(code)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Type > geoquery.GeositeRuleTypeDomainRegex || item.Value == "" {
				return nil, fmt.Errorf("invalid geosite entry in %s", code)
			}
			if item.Type == geoquery.GeositeRuleTypeDomainRegex {
				if _, err := regexp.Compile(item.Value); err != nil {
					return nil, fmt.Errorf("invalid geosite expression in %s: %w", code, err)
				}
			}
		}
	}
	return codes, nil
}

func (svc *RouteRuleService) cacheGeoDatabase(asset *model.GeoAsset, body []byte) (string, error) {
	source, err := model.NormalizeGeoSource(asset.Source)
	if err != nil {
		return "", err
	}
	if asset.Type != "geoip" && asset.Type != "geosite" {
		return "", fmt.Errorf("unsupported geo database type")
	}
	unlock := svc.lockGeneratedGeoRuleSet(geoDatabaseCacheLockKey(source, asset.Type))
	defer unlock()
	ext := ".db"
	if source == model.GeoSourceLoyalsoldier {
		ext = ".dat"
	}
	digest := sha256.Sum256(body)
	dir := filepath.Join(svc.paths.GeoDir, source)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(dir, "geo-*"+ext)
	if err != nil {
		return "", err
	}
	name := file.Name()
	defer os.Remove(name)
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return "", err
	}
	if err := file.Close(); err != nil {
		return "", err
	}
	if codes, err := geoAssetCodes(asset.Type, name); err != nil {
		return "", fmt.Errorf("invalid %s database: %w", asset.Type, err)
	} else if len(codes) == 0 {
		return "", fmt.Errorf("%s database has no categories", asset.Type)
	}
	path := filepath.Join(dir, fmt.Sprintf("%s-%x%s", asset.Type, digest, ext))
	if err := atomicReplaceFile(name, path); err != nil {
		return "", err
	}
	// The caller has not yet published this path to the store. Keep it as well
	// as the currently selected asset if cleanup cannot infer which is active.
	if err := svc.pruneGeoDatabaseCache(source, asset.Type, path); err != nil {
		logging.Error("geo.cache", "清理旧 Geo 缓存失败，保留已同步数据库: %s/%s: %v", source, asset.Type, err)
	}
	return path, nil
}

func geoDatabaseCacheLockKey(source, kind string) string {
	return "database/" + source + "/" + kind
}

type geoDatabaseSnapshot struct {
	path    string
	version string
	updated time.Time
}

func (svc *RouteRuleService) geoDatabaseSnapshots(source, kind string) ([]geoDatabaseSnapshot, error) {
	dir := filepath.Join(svc.paths.GeoDir, source)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ext := ".db"
	if source == model.GeoSourceLoyalsoldier {
		ext = ".dat"
	}
	var snapshots []geoDatabaseSnapshot
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasPrefix(name, kind+"-") || !strings.HasSuffix(name, ext) {
			continue
		}
		version := strings.TrimSuffix(strings.TrimPrefix(name, kind+"-"), ext)
		if !validGeoAssetVersion(version) || version != strings.ToLower(version) {
			continue // Unknown names, directories and symlinks belong to someone else.
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		snapshots = append(snapshots, geoDatabaseSnapshot{path: filepath.Join(dir, name), version: version, updated: info.ModTime()})
	}
	sort.Slice(snapshots, func(i, j int) bool {
		if snapshots[i].updated.Equal(snapshots[j].updated) {
			return snapshots[i].version > snapshots[j].version
		}
		return snapshots[i].updated.After(snapshots[j].updated)
	})
	return snapshots, nil
}

func sameGeoCachePath(left, right string) bool {
	left, leftErr := filepath.Abs(left)
	right, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

// Check both lexical and resolved paths before deleting a cache subtree. In
// particular, a provider directory replaced by a symlink must not escape GeoDir
// or RulesDir. A configured root itself may legitimately be a symlink.
func geoCachePathWithin(root, path string) error {
	for _, resolve := range []func(string) (string, error){filepath.Abs, filepath.EvalSymlinks} {
		resolvedRoot, err := resolve(root)
		if err != nil {
			return err
		}
		resolvedPath, err := resolve(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(resolvedRoot, resolvedPath)
		if err != nil || relative == "." || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("Geo cache path is outside its configured directory")
		}
	}
	return nil
}

// The caller holds the source/type database lock; this function never acquires
// another cache lock, so conversion cannot deadlock with cache publication.
func (svc *RouteRuleService) pruneGeoDatabaseCache(source, kind, publishedPath string) error {
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return err // If the active path is unknown, delete nothing.
	}
	snapshots, err := svc.geoDatabaseSnapshots(source, kind)
	if err != nil {
		return err
	}
	removed := 0
	for index, snapshot := range snapshots {
		keep := index < 2 || sameGeoCachePath(snapshot.path, publishedPath)
		for _, asset := range assets {
			keep = keep || (asset.LocalPath != "" && sameGeoCachePath(snapshot.path, asset.LocalPath))
		}
		if keep {
			continue
		}
		if err := geoCachePathWithin(svc.paths.GeoDir, snapshot.path); err != nil {
			return err
		}
		if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		removed++
		// A digest directory is shared across types. Retain it if another
		// database still has that digest (possible for an empty category).
		ext := filepath.Ext(snapshot.path)
		shared := false
		for _, otherKind := range []string{"geoip", "geosite"} {
			_, err := os.Lstat(filepath.Join(svc.paths.GeoDir, source, otherKind+"-"+snapshot.version+ext))
			shared = shared || err == nil || !os.IsNotExist(err)
		}
		if shared || svc.paths.RulesDir == "" {
			continue
		}
		cacheDir := filepath.Join(svc.paths.RulesDir, "geo", source, snapshot.version)
		info, err := os.Lstat(cacheDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if err := geoCachePathWithin(svc.paths.RulesDir, cacheDir); err != nil {
			return err
		}
		if err := os.RemoveAll(cacheDir); err != nil {
			return err
		}
	}
	if removed > 0 {
		logging.Info("geo.cache", "已清理旧 Geo 快照: %s/%s: %d，保留最近两个及当前使用版本", source, kind, removed)
	}
	return nil
}

func (svc *RouteRuleService) latestUsableGeoSnapshot(source, kind string) (geoDatabaseSnapshot, error) {
	snapshots, err := svc.geoDatabaseSnapshots(source, kind)
	if err != nil {
		return geoDatabaseSnapshot{}, err
	}
	for _, snapshot := range snapshots {
		if codes, err := geoAssetCodes(kind, snapshot.path); err == nil && len(codes) > 0 {
			return snapshot, nil
		} else {
			logging.Error("geo.cache", "忽略无法读取的 Geo 历史快照: %s/%s/%s: %v", source, kind, snapshot.version, err)
		}
	}
	return geoDatabaseSnapshot{}, fmt.Errorf("%s %s 没有可用的保留快照，请先同步 Geo 数据", source, kind)
}

func geoAssetVersion(asset model.GeoAsset) string {
	base := strings.TrimSuffix(filepath.Base(asset.LocalPath), filepath.Ext(asset.LocalPath))
	version := strings.TrimPrefix(base, asset.Type+"-")
	if !validGeoAssetVersion(version) {
		return ""
	}
	return version
}

func validGeoAssetVersion(version string) bool {
	if len(version) != sha256.Size*2 {
		return false
	}
	_, err := hex.DecodeString(version)
	return err == nil
}

// Pin the source policy in the URL. The content endpoint may use SagerNet only
// when the requested category does not exist in Loyalsoldier.
func applyGeneratedGeoSource(db *store.Store, p *paths.Paths, ruleSets []map[string]any) error {
	settings, err := db.GetGeneralSettings()
	if err != nil || settings.GeoSource != model.GeoSourceLoyalsoldier {
		return err
	}
	assets, err := db.ListGeoAssets()
	if err != nil {
		return err
	}
	svc := NewRouteRuleService(db, p, nil)
	catalog, err := svc.geoCategoryCatalog(assets)
	if err != nil {
		return err
	}
	for _, ruleSet := range ruleSets {
		raw, _ := ruleSet["url"].(string)
		parsed, err := url.Parse(raw)
		if err != nil || !strings.Contains(parsed.Path, "/rules/geo/rule-sets/") {
			continue // Independent rule subscriptions retain their own source and format.
		}
		tag, _ := ruleSet["tag"].(string)
		if _, err := catalog.resolve(tag); err != nil {
			return err
		}
		kind, _, _ := strings.Cut(tag, "-")
		version := ""
		for _, asset := range assets {
			if asset.Type == kind && asset.Source == settings.GeoSource && asset.Available {
				version = geoAssetVersion(asset)
			}
		}
		if version == "" {
			return fmt.Errorf("%s %s 数据库未就绪，请先同步 Geo 数据", settings.GeoSource, kind)
		}
		query := parsed.Query()
		query.Set("source", settings.GeoSource)
		query.Set("version", version)
		query.Set("format", "binary")
		parsed.RawQuery = query.Encode()
		ruleSet["url"] = parsed.String()
		ruleSet["format"] = "binary"
	}
	return nil
}

func (svc *RouteRuleService) GeneratedGeoRuleSetContentForSource(ctx context.Context, tag, source, version string, requestedFormat ...string) ([]byte, string, error) {
	source, err := model.NormalizeGeoSource(source)
	if err != nil {
		return nil, "", err
	}
	format := ""
	if len(requestedFormat) > 0 {
		format = requestedFormat[0]
	}
	if format != "" && format != "source" && format != "binary" {
		return nil, "", fmt.Errorf("unsupported generated geo rule-set format: %s", format)
	}
	if source == model.GeoSourceSagerNet {
		return svc.GeneratedGeoRuleSetContentContext(ctx, tag)
	}
	tag = strings.ToLower(strings.TrimSpace(tag))
	if !isGeneratedGeoRuleSetTag(tag) {
		return nil, "", fmt.Errorf("invalid generated geo rule set tag")
	}
	if svc.paths == nil || svc.paths.GeoDir == "" || svc.paths.RulesDir == "" {
		return nil, "", fmt.Errorf("geo directories are not configured")
	}
	if version != "" && !validGeoAssetVersion(version) {
		return nil, "", fmt.Errorf("invalid geo database version")
	}
	kind, code, _ := strings.Cut(tag, "-")
	unlock := svc.lockGeneratedGeoRuleSet(geoDatabaseCacheLockKey(source, kind))
	defer unlock()
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return nil, "", err
	}
	var path string
	for _, asset := range assets {
		if asset.Source == source && asset.Type == kind && asset.Available {
			path, version = asset.LocalPath, geoAssetVersion(asset)
		}
	}
	if path == "" && version != "" {
		// Keep the previous core configuration usable until the new config is applied.
		path = filepath.Join(svc.paths.GeoDir, source, kind+"-"+version+".dat")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			snapshot, err := svc.latestUsableGeoSnapshot(source, kind)
			if err != nil {
				return nil, "", err
			}
			logging.Info("geo.cache", "旧 Geo 版本已清理，使用同来源保留快照: %s/%s: %s -> %s", source, kind, version, snapshot.version)
			path, version = snapshot.path, snapshot.version
		}
	}
	if path == "" || version == "" {
		return nil, "", fmt.Errorf("%s %s 数据库未就绪，请先同步 Geo 数据", source, kind)
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	asset := model.GeoAsset{Type: kind, Source: source, LocalPath: path}
	for _, current := range assets {
		if current.Type == kind {
			asset.UseProxy = current.UseProxy
		}
	}
	catalog, err := svc.geoCategoryCatalog([]model.GeoAsset{asset})
	if err != nil {
		return nil, "", err
	}
	selected, err := catalog.resolve(tag)
	if err != nil {
		return nil, "", err
	}
	if selected.source == model.GeoSourceSagerNet {
		logging.Info("geo.fallback", "提供 SagerNet 回退规则集: %s", tag)
		if format == "binary" {
			return svc.GeneratedGeoRuleSetContentContext(ctx, tag)
		}
		if kind == "geoip" && code == "private" {
			data, err := svc.sagerNetPrivateRuleSetSource(ctx)
			return data, "application/json; charset=utf-8", err
		}
		data, err := geoDatabaseRuleSetSource(kind, code, selected.path)
		return data, "application/json; charset=utf-8", err
	}
	if format == "binary" && kind == "geoip" {
		return svc.loyalsoldierGeoIPRuleSetContent(ctx, tag)
	}
	cacheDir := filepath.Join(svc.paths.RulesDir, "geo", source, version)
	if format == "binary" {
		return svc.compiledGeoRuleSetContent(ctx, tag, kind, code, path, cacheDir)
	}
	// Previously generated configurations requested JSON without a format
	// parameter. Keep those URLs usable until the new binary config is applied.
	cachePath := filepath.Join(cacheDir, tag+".json")
	if data, err := os.ReadFile(cachePath); err == nil && json.Valid(data) {
		return data, "application/json; charset=utf-8", nil
	}
	data, err := geoDatabaseRuleSetSource(kind, code, path)
	if err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, "", err
	}
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return nil, "", err
	}
	logging.Info("route_rule_geo.convert", "已转换 Geo 分类: %s/%s", source, tag)
	return data, "application/json; charset=utf-8", nil
}

func geoDatabaseRuleSetSource(kind, code, path string) ([]byte, error) {
	rule := map[string]any{}
	if kind == "geoip" {
		reader, err := geoquery.OpenGeoIP(path)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		prefixes, err := reader.Prefixes(code)
		if err != nil {
			return nil, err
		}
		values := make([]string, 0, len(prefixes))
		for _, prefix := range prefixes {
			values = append(values, prefix.String())
		}
		if len(values) > 0 {
			rule["ip_cidr"] = values
		}
	} else {
		reader, _, err := geoquery.OpenGeosite(path)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		items, err := reader.Read(code)
		if err != nil {
			return nil, err
		}
		fields := []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"}
		for _, item := range items {
			if int(item.Type) >= len(fields) || item.Value == "" {
				return nil, fmt.Errorf("unsupported geosite item in %s", code)
			}
			key := fields[item.Type]
			values, _ := rule[key].([]string)
			rule[key] = append(values, item.Value)
		}
	}
	rules := []map[string]any{}
	if len(rule) > 0 {
		rules = append(rules, rule)
	}
	return json.Marshal(map[string]any{"version": 3, "rules": rules})
}
