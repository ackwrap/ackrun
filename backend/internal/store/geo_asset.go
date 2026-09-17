package store

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

var defaultGeoAssets = []model.GeoAsset{
	{Name: "GeoIP", Type: "geoip", Source: model.GeoSourceSagerNet, URL: model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geoip"), SyncMode: "daily", SyncTime: "03:30:00"},
	{Name: "GeoSite", Type: "geosite", Source: model.GeoSourceSagerNet, URL: model.GeoSourceAssetURL(model.GeoSourceSagerNet, "geosite"), SyncMode: "daily", SyncTime: "03:40:00"},
}

func (s *Store) EnsureDefaultGeoAssets() error {
	settings, err := s.GetGeneralSettings()
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, item := range defaultGeoAssets {
		_, err := s.db.Exec(`INSERT INTO geo_assets (name, type, source, url, use_proxy, sync_mode, sync_time, sync_weekday, created_at, updated_at) VALUES (?, ?, ?, ?, 0, ?, ?, 0, ?, ?) ON CONFLICT(type) DO NOTHING`, item.Name, item.Type, settings.GeoSource, model.GeoSourceAssetURL(settings.GeoSource, item.Type), item.SyncMode, item.SyncTime, now, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ListGeoAssets() ([]model.GeoAsset, error) {
	if err := s.EnsureDefaultGeoAssets(); err != nil {
		return nil, err
	}
	rows, err := s.db.Query(`SELECT id, name, type, source, url, use_proxy, sync_mode, sync_time, sync_weekday, sync_status, sync_error, last_sync_at, local_path, cached_updated_at, created_at, updated_at FROM geo_assets ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.GeoAsset, 0)
	for rows.Next() {
		item, err := scanGeoAsset(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetGeoAsset(id int64) (*model.GeoAsset, error) {
	if err := s.EnsureDefaultGeoAssets(); err != nil {
		return nil, err
	}
	row := s.db.QueryRow(`SELECT id, name, type, source, url, use_proxy, sync_mode, sync_time, sync_weekday, sync_status, sync_error, last_sync_at, local_path, cached_updated_at, created_at, updated_at FROM geo_assets WHERE id = ?`, id)
	item, err := scanGeoAsset(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) UpdateGeoAsset(id int64, req *model.GeoAssetRequest) (*model.GeoAsset, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE geo_assets SET url = CASE WHEN ? = '' THEN url ELSE ? END, use_proxy = ?, sync_mode = ?, sync_time = ?, sync_weekday = ?, updated_at = ? WHERE id = ?`, req.URL, req.URL, boolToInt(req.UseProxy), req.SyncMode, req.SyncTime, req.SyncWeekday, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetGeoAsset(id)
}

func (s *Store) SetGeoAssetSyncState(id int64, status string, syncError string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE geo_assets SET sync_status = ?, sync_error = ?, updated_at = ? WHERE id = ?`, status, syncError, now, id)
	return err
}

func (s *Store) UpdateGeoAssetSyncResult(id int64, localPath string) (*model.GeoAsset, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE geo_assets SET sync_status = 'updated', sync_error = '', last_sync_at = ?, local_path = ?, cached_updated_at = ?, updated_at = ? WHERE id = ?`, now, localPath, now, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetGeoAsset(id)
}

// SetGeoAssetSyncStateIfCurrent ignores work started for a superseded source or URL.
func (s *Store) SetGeoAssetSyncStateIfCurrent(item *model.GeoAsset, status, syncError string) (bool, error) {
	if item == nil {
		return false, fmt.Errorf("Geo 资源不能为空")
	}
	source, err := model.NormalizeGeoSource(item.Source)
	if err != nil {
		return false, err
	}
	result, err := s.db.Exec(`UPDATE geo_assets SET sync_status = ?, sync_error = ?, updated_at = ? WHERE id = ? AND source = ? AND url = ? AND use_proxy = ?`, status, syncError, time.Now().UnixMilli(), item.ID, source, item.URL, boolToInt(item.UseProxy))
	return geoAssetChanged(result, err)
}

func (s *Store) UpdateGeoAssetSyncResultIfCurrent(item *model.GeoAsset, localPath string) (bool, error) {
	if item == nil {
		return false, fmt.Errorf("Geo 资源不能为空")
	}
	source, err := model.NormalizeGeoSource(item.Source)
	if err != nil {
		return false, err
	}
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE geo_assets SET sync_status = 'updated', sync_error = '', last_sync_at = ?, local_path = ?, cached_updated_at = ?, updated_at = ? WHERE id = ? AND source = ? AND url = ? AND use_proxy = ?`, now, localPath, now, now, item.ID, source, item.URL, boolToInt(item.UseProxy))
	return geoAssetChanged(result, err)
}

func geoAssetChanged(result sql.Result, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count != 0, err
}

type geoAssetScanner interface {
	Scan(dest ...any) error
}

func scanGeoAsset(scanner geoAssetScanner) (*model.GeoAsset, error) {
	var item model.GeoAsset
	var useProxy int
	if err := scanner.Scan(&item.ID, &item.Name, &item.Type, &item.Source, &item.URL, &useProxy, &item.SyncMode, &item.SyncTime, &item.SyncWeekday, &item.SyncStatus, &item.SyncError, &item.LastSyncAt, &item.LocalPath, &item.CachedUpdatedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	item.UseProxy = useProxy != 0
	if item.LocalPath != "" {
		info, err := os.Stat(item.LocalPath)
		item.Available = err == nil && info.Mode().IsRegular()
	}
	return &item, nil
}
