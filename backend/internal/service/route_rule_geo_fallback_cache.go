package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

// Config generation and DNS validation also construct rule services. Serialize
// publication by directory, across those instances as well as the main service.
var geoFallbackCacheLocks sync.Map

func (svc *RouteRuleService) sagerNetFallbackDatabase(kind string, useProxy bool) (string, error) {
	if svc.paths == nil || svc.paths.GeoDir == "" {
		return "", fmt.Errorf("Geo 数据目录未配置")
	}
	current := filepath.Join(svc.paths.GeoDir, model.GeoSourceSagerNet, "fallback", kind+".db")
	key, err := filepath.Abs(current)
	if err != nil {
		return "", err
	}
	value, _ := geoFallbackCacheLocks.LoadOrStore(strings.ToLower(key), &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	defer lock.Unlock()
	_, cacheErr := geoAssetCodes(kind, current)
	if cacheErr != nil {
		// Bootstrap from an existing SagerNet snapshot, including the database
		// downloaded before switching sources. The accepted copy is outside
		// snapshot pruning, so failed future refreshes cannot evict it.
		if snapshot, err := svc.latestUsableGeoSnapshot(model.GeoSourceSagerNet, kind); err == nil {
			cacheErr = publishGeoFallbackDatabase(snapshot.path, current)
			if cacheErr == nil {
				_ = os.Chtimes(current, snapshot.updated, snapshot.updated)
			}
		}
	}
	if cacheErr == nil {
		if info, err := os.Stat(current); err == nil && time.Since(info.ModTime()) < generatedGeoRuleSetUpdateInterval {
			return current, nil
		}
	}
	asset := &model.GeoAsset{
		Type: kind, Source: model.GeoSourceSagerNet, UseProxy: useProxy,
		URL: model.GeoSourceAssetURL(model.GeoSourceSagerNet, kind),
	}
	path, err := svc.fetchAndCacheGeoAsset(asset, nil)
	if err == nil && cacheErr == nil {
		err = svc.validateGeoFallbackRefresh(kind, current, path)
	}
	if err == nil {
		err = publishGeoFallbackDatabase(path, current)
	}
	if err == nil {
		logging.Info("geo.fallback", "已更新 SagerNet %s 回退数据", kind)
		return current, nil
	}
	if cacheErr == nil {
		logging.Error("geo.fallback", "SagerNet %s 更新失败，保留有效回退缓存: %v", kind, err)
		return current, nil
	}
	return "", err
}

func publishGeoFallbackDatabase(source, destination string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".fallback-*.db")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return atomicReplaceFile(file.Name(), destination)
}

func (svc *RouteRuleService) validateGeoFallbackRefresh(kind, previous, next string) error {
	asset, err := svc.selectedGeoAsset(kind)
	if err != nil || asset.Source != model.GeoSourceLoyalsoldier || !asset.Available {
		return err
	}
	catalog, err := svc.geoCategoryCatalog([]model.GeoAsset{asset})
	if err != nil {
		return err
	}
	oldCodes, err := geoAssetCodes(kind, previous)
	if err != nil {
		return err
	}
	newCodes, err := geoAssetCodes(kind, next)
	if err != nil {
		return err
	}
	removed := make(map[string]bool, len(oldCodes))
	for _, code := range oldCodes {
		removed[generatedGeoRuleSetTag(kind, code)] = true
	}
	for _, code := range newCodes {
		delete(removed, generatedGeoRuleSetTag(kind, code))
	}
	wanted, err := svc.requiredGeneratedGeoCategories()
	if err != nil {
		return err
	}
	var missing []string
	for tag := range wanted {
		if removed[tag] && catalog.entries[tag].source == "" {
			missing = append(missing, tag)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return fmt.Errorf("SagerNet 更新缺少正在使用的回退分类: %s", strings.Join(missing, ", "))
	}
	return nil
}
