package service

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

type geoCategorySource struct {
	source string
	path   string
}

// A category has exactly one owner. Fallback adds missing category names; it
// never adds SagerNet domains/IPs to an existing Loyalsoldier category.
type geoCategoryCatalog struct {
	svc      *RouteRuleService
	assets   map[string]model.GeoAsset
	entries  map[string]geoCategorySource
	fallback map[string]bool
}

func (svc *RouteRuleService) geoCategoryCatalog(assets []model.GeoAsset) (*geoCategoryCatalog, error) {
	catalog := &geoCategoryCatalog{
		svc: svc, assets: make(map[string]model.GeoAsset),
		entries: make(map[string]geoCategorySource), fallback: make(map[string]bool),
	}
	for _, asset := range assets {
		catalog.assets[asset.Type] = asset
		if asset.LocalPath == "" {
			continue
		}
		if err := catalog.addDatabase(asset); err != nil {
			return nil, err
		}
	}
	return catalog, nil
}

func (catalog *geoCategoryCatalog) addDatabase(asset model.GeoAsset) error {
	codes, err := geoAssetCodes(asset.Type, asset.LocalPath)
	if err != nil {
		return fmt.Errorf("读取 %s %s 分类失败: %w", asset.Source, asset.Type, err)
	}
	if asset.Source == model.GeoSourceSagerNet && asset.Type == "geoip" {
		// Published as a separate native SRS, outside the country database.
		codes = append(codes, "private")
	}
	for _, code := range codes {
		tag := generatedGeoRuleSetTag(asset.Type, code)
		if _, exists := catalog.entries[tag]; !exists {
			catalog.entries[tag] = geoCategorySource{source: asset.Source, path: asset.LocalPath}
		}
	}
	return nil
}

func (catalog *geoCategoryCatalog) addFallback(kind string) error {
	asset, ok := catalog.assets[kind]
	if !ok || asset.Source != model.GeoSourceLoyalsoldier || catalog.fallback[kind] {
		return nil
	}
	path, err := catalog.svc.sagerNetFallbackDatabase(kind, asset.UseProxy)
	if err != nil {
		return fmt.Errorf("SagerNet %s 回退数据未就绪: %w", kind, err)
	}
	if err := catalog.addDatabase(model.GeoAsset{Type: kind, Source: model.GeoSourceSagerNet, LocalPath: path}); err != nil {
		return err
	}
	catalog.fallback[kind] = true
	return nil
}

func (catalog *geoCategoryCatalog) resolve(tag string) (geoCategorySource, error) {
	if selected, ok := catalog.entries[tag]; ok {
		return selected, nil
	}
	kind, _, _ := strings.Cut(tag, "-")
	if err := catalog.addFallback(kind); err != nil {
		return geoCategorySource{}, fmt.Errorf("无法解析 Geo 分类 %s: %w", tag, err)
	}
	if selected, ok := catalog.entries[tag]; ok {
		logging.Info("geo.fallback", "Loyalsoldier 缺少分类 %s，使用 SagerNet 同名分类", tag)
		return selected, nil
	}
	if catalog.assets[kind].Source == model.GeoSourceLoyalsoldier {
		return geoCategorySource{}, fmt.Errorf("Loyalsoldier 和 SagerNet 均不存在 Geo 分类: %s", tag)
	}
	return geoCategorySource{}, fmt.Errorf("SagerNet 不存在 Geo 分类: %s", tag)
}

func (catalog *geoCategoryCatalog) codes(kind string) []string {
	var codes []string
	for tag := range catalog.entries {
		if code, ok := strings.CutPrefix(tag, kind+"-"); ok {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return codes
}

func (svc *RouteRuleService) selectedGeoAsset(kind string) (model.GeoAsset, error) {
	assets, err := svc.store.ListGeoAssets()
	if err != nil {
		return model.GeoAsset{}, err
	}
	for _, asset := range assets {
		if asset.Type == kind {
			return asset, nil
		}
	}
	return model.GeoAsset{}, fmt.Errorf("%s 数据库未配置", kind)
}

func (svc *RouteRuleService) loyalsoldierGeoTags(kind string) ([]string, bool, error) {
	asset, err := svc.selectedGeoAsset(kind)
	if err != nil {
		return nil, false, err
	}
	if asset.LocalPath == "" {
		return []string{}, false, nil
	}
	if _, err := os.Stat(asset.LocalPath); os.IsNotExist(err) {
		return []string{}, false, nil
	}
	catalog, err := svc.geoCategoryCatalog([]model.GeoAsset{asset})
	if err != nil {
		return nil, false, err
	}
	if err := catalog.addFallback(kind); err != nil {
		logging.Error("geo.fallback", "回退分类暂不可用，保留 Loyalsoldier 分类列表: %v", err)
		return catalog.codes(kind), true, err
	}
	return catalog.codes(kind), true, nil
}

func (svc *RouteRuleService) validateLoyalsoldierCategories(kind string, codes []string) (bool, error) {
	asset, err := svc.selectedGeoAsset(kind)
	if err != nil {
		return true, err
	}
	if asset.Source != model.GeoSourceLoyalsoldier {
		return false, nil
	}
	subscriptions, err := svc.store.ListRouteRuleSubscriptions()
	if err != nil {
		return true, err
	}
	owned := make(map[string]bool)
	for _, subscription := range subscriptions {
		if subscription.Enabled {
			owned[subscription.Tag] = true
		}
	}
	var wanted []string
	for _, code := range codes {
		tag := generatedGeoRuleSetTag(kind, code)
		if !owned[tag] {
			wanted = append(wanted, tag)
		}
	}
	if len(wanted) == 0 {
		return true, nil
	}
	if !asset.Available {
		return true, fmt.Errorf("Loyalsoldier %s 数据库未就绪，请先同步 Geo 数据", kind)
	}
	catalog, err := svc.geoCategoryCatalog([]model.GeoAsset{asset})
	if err != nil {
		return true, err
	}
	for _, tag := range wanted {
		if _, err := catalog.resolve(tag); err != nil {
			return true, err
		}
	}
	return true, nil
}
