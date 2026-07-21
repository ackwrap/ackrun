package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) ListGeoIPProviders() ([]model.GeoIPProvider, error) {
	rows, err := s.db.Query(`SELECT id, name, provider_key, template, url, ip_parameter, mapping_json, enabled, is_default, builtin, created_at, updated_at FROM geoip_providers ORDER BY is_default DESC, builtin DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.GeoIPProvider, 0)
	for rows.Next() {
		item, err := scanGeoIPProvider(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetGeoIPProvider(id int64) (*model.GeoIPProvider, error) {
	return scanGeoIPProvider(s.db.QueryRow(`SELECT id, name, provider_key, template, url, ip_parameter, mapping_json, enabled, is_default, builtin, created_at, updated_at FROM geoip_providers WHERE id = ?`, id))
}

func (s *Store) GetGeoIPProviderByKey(key string) (*model.GeoIPProvider, error) {
	return scanGeoIPProvider(s.db.QueryRow(`SELECT id, name, provider_key, template, url, ip_parameter, mapping_json, enabled, is_default, builtin, created_at, updated_at FROM geoip_providers WHERE provider_key = ?`, key))
}

func (s *Store) GetDefaultGeoIPProvider() (*model.GeoIPProvider, error) {
	return scanGeoIPProvider(s.db.QueryRow(`SELECT id, name, provider_key, template, url, ip_parameter, mapping_json, enabled, is_default, builtin, created_at, updated_at FROM geoip_providers WHERE enabled = 1 ORDER BY is_default DESC, builtin DESC, id ASC LIMIT 1`))
}

type rowScanner interface {
	Scan(...any) error
}

func scanGeoIPProvider(row rowScanner) (*model.GeoIPProvider, error) {
	var item model.GeoIPProvider
	var mappingJSON string
	var enabled, isDefault, builtin int
	if err := row.Scan(&item.ID, &item.Name, &item.Key, &item.Template, &item.URL, &item.IPParameter, &mappingJSON, &enabled, &isDefault, &builtin, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(mappingJSON), &item.Mapping); err != nil {
		return nil, fmt.Errorf("decode GeoIP provider mapping: %w", err)
	}
	item.Enabled = enabled != 0
	item.IsDefault = isDefault != 0
	item.Builtin = builtin != 0
	return &item, nil
}

func (s *Store) CreateGeoIPProvider(req *model.GeoIPProviderRequest) (*model.GeoIPProvider, error) {
	now := time.Now().Unix()
	key := fmt.Sprintf("custom:%d", time.Now().UnixNano())
	mappingJSON, err := json.Marshal(req.Mapping)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if req.IsDefault {
		if _, err := tx.Exec(`UPDATE geoip_providers SET is_default = 0, updated_at = ? WHERE is_default = 1`, now); err != nil {
			return nil, err
		}
	}
	result, err := tx.Exec(`INSERT INTO geoip_providers (name, provider_key, template, url, ip_parameter, mapping_json, enabled, is_default, builtin, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`, req.Name, key, req.Template, req.URL, req.IPParameter, string(mappingJSON), boolToInt(req.Enabled), boolToInt(req.IsDefault), now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetGeoIPProvider(id)
}

func (s *Store) UpdateGeoIPProvider(id int64, req *model.GeoIPProviderRequest) (*model.GeoIPProvider, error) {
	now := time.Now().Unix()
	mappingJSON, err := json.Marshal(req.Mapping)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if req.IsDefault {
		if _, err := tx.Exec(`UPDATE geoip_providers SET is_default = 0, updated_at = ? WHERE is_default = 1 AND id <> ?`, now, id); err != nil {
			return nil, err
		}
	}
	result, err := tx.Exec(`UPDATE geoip_providers SET name = ?, template = ?, url = ?, ip_parameter = ?, mapping_json = ?, enabled = ?, is_default = ?, updated_at = ? WHERE id = ?`, req.Name, req.Template, req.URL, req.IPParameter, string(mappingJSON), boolToInt(req.Enabled), boolToInt(req.IsDefault), now, id)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetGeoIPProvider(id)
}

func (s *Store) DeleteGeoIPProvider(id int64) error {
	result, err := s.db.Exec(`DELETE FROM geoip_providers WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) ListConnectivityTargets() ([]model.ConnectivityTarget, error) {
	rows, err := s.db.Query(`SELECT id, name, url, enabled, builtin, created_at, updated_at FROM connectivity_targets ORDER BY builtin DESC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.ConnectivityTarget, 0)
	for rows.Next() {
		item, err := scanConnectivityTarget(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetConnectivityTarget(id int64) (*model.ConnectivityTarget, error) {
	return scanConnectivityTarget(s.db.QueryRow(`SELECT id, name, url, enabled, builtin, created_at, updated_at FROM connectivity_targets WHERE id = ?`, id))
}

func (s *Store) GetConnectivityTargetByURL(rawURL string) (*model.ConnectivityTarget, error) {
	return scanConnectivityTarget(s.db.QueryRow(`SELECT id, name, url, enabled, builtin, created_at, updated_at FROM connectivity_targets WHERE url = ?`, rawURL))
}

func scanConnectivityTarget(row rowScanner) (*model.ConnectivityTarget, error) {
	var item model.ConnectivityTarget
	var enabled, builtin int
	if err := row.Scan(&item.ID, &item.Name, &item.URL, &enabled, &builtin, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.Builtin = builtin != 0
	return &item, nil
}

func (s *Store) CreateConnectivityTarget(req *model.ConnectivityTargetRequest) (*model.ConnectivityTarget, error) {
	now := time.Now().Unix()
	result, err := s.db.Exec(`INSERT INTO connectivity_targets (name, url, enabled, builtin, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`, req.Name, req.URL, boolToInt(req.Enabled), now, now)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetConnectivityTarget(id)
}

func (s *Store) UpdateConnectivityTarget(id int64, req *model.ConnectivityTargetRequest) (*model.ConnectivityTarget, error) {
	result, err := s.db.Exec(`UPDATE connectivity_targets SET name = ?, url = ?, enabled = ?, updated_at = ? WHERE id = ?`, req.Name, req.URL, boolToInt(req.Enabled), time.Now().Unix(), id)
	if err != nil {
		return nil, err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return nil, err
		}
		return nil, sql.ErrNoRows
	}
	return s.GetConnectivityTarget(id)
}

func (s *Store) DeleteConnectivityTarget(id int64) error {
	result, err := s.db.Exec(`DELETE FROM connectivity_targets WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if affected, err := result.RowsAffected(); err != nil || affected == 0 {
		if err != nil {
			return err
		}
		return sql.ErrNoRows
	}
	return nil
}
