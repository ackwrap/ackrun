package store

import (
	"database/sql"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) ListRouteRuleSubscriptions() ([]model.RouteRuleSubscription, error) {
	rows, err := s.db.Query(`SELECT id, name, enabled, tag, url, format, use_proxy, sync_mode, sync_time, sync_weekday, sync_status, sync_progress, sync_error, last_sync_at, cached_path, cached_updated_at, created_at, updated_at FROM route_rule_subscriptions ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.RouteRuleSubscription, 0)
	for rows.Next() {
		item, err := scanRouteRuleSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) CreateRouteRuleSubscription(req *model.RouteRuleSubscriptionRequest) (*model.RouteRuleSubscription, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`INSERT INTO route_rule_subscriptions (name, enabled, tag, url, format, use_proxy, sync_mode, sync_time, sync_weekday, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.Name, boolToInt(req.Enabled), req.Tag, req.URL, req.Format, boolToInt(req.UseProxy), req.SyncMode, req.SyncTime, req.SyncWeekday, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetRouteRuleSubscription(id)
}

func (s *Store) GetRouteRuleSubscription(id int64) (*model.RouteRuleSubscription, error) {
	row := s.db.QueryRow(`SELECT id, name, enabled, tag, url, format, use_proxy, sync_mode, sync_time, sync_weekday, sync_status, sync_progress, sync_error, last_sync_at, cached_path, cached_updated_at, created_at, updated_at FROM route_rule_subscriptions WHERE id = ?`, id)
	item, err := scanRouteRuleSubscription(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (s *Store) UpdateRouteRuleSubscription(id int64, req *model.RouteRuleSubscriptionRequest) (*model.RouteRuleSubscription, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE route_rule_subscriptions SET name = ?, enabled = ?, tag = ?, url = ?, format = ?, use_proxy = ?, sync_mode = ?, sync_time = ?, sync_weekday = ?, sync_status = 'idle', sync_progress = 0, sync_error = '', updated_at = CASE WHEN updated_at >= ? THEN updated_at + 1 ELSE ? END WHERE id = ?`, req.Name, boolToInt(req.Enabled), req.Tag, req.URL, req.Format, boolToInt(req.UseProxy), req.SyncMode, req.SyncTime, req.SyncWeekday, now, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetRouteRuleSubscription(id)
}

func (s *Store) DeleteRouteRuleSubscription(id int64) error {
	_, err := s.db.Exec(`DELETE FROM route_rule_subscriptions WHERE id = ?`, id)
	return err
}

func (s *Store) SetRouteRuleSubscriptionSyncState(id int64, status string, progress float64, syncError string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = ?, sync_progress = ?, sync_error = ?, updated_at = ? WHERE id = ?`, status, progress, syncError, now, id)
	return err
}

// ClaimRouteRuleSubscriptionSync atomically moves a subscription into syncing state.
// A missing subscription returns sql.ErrNoRows; an existing syncing subscription returns claimed=false.
func (s *Store) ClaimRouteRuleSubscriptionSync(id int64, progress float64) (*model.RouteRuleSubscription, bool, error) {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = 'syncing', sync_progress = ?, sync_error = '', updated_at = CASE WHEN updated_at >= ? THEN updated_at + 1 ELSE ? END WHERE id = ? AND sync_status <> 'syncing'`, progress, now, now, id)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	item, err := s.GetRouteRuleSubscription(id)
	if err != nil {
		return nil, false, err
	}
	if item == nil {
		return nil, false, sql.ErrNoRows
	}
	return item, rows == 1, nil
}

// ResetInterruptedRouteRuleSubscriptionSyncs clears jobs that could not survive a process restart.
func (s *Store) ResetInterruptedRouteRuleSubscriptionSyncs() error {
	_, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = 'failed', sync_progress = 0, sync_error = '同步被服务重启中断' WHERE sync_status = 'syncing'`)
	return err
}

func (s *Store) UpdateRouteRuleSubscriptionSyncResult(id int64, cachedPath string) (*model.RouteRuleSubscription, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = 'updated', sync_progress = 100, sync_error = '', last_sync_at = ?, cached_path = ?, cached_updated_at = ?, updated_at = ? WHERE id = ?`, now, cachedPath, now, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetRouteRuleSubscription(id)
}

func (s *Store) UpdateRouteRuleSubscriptionSyncResultIfCurrent(id, expectedUpdatedAt int64, cachedPath string) (*model.RouteRuleSubscription, bool, error) {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = 'updated', sync_progress = 100, sync_error = '', last_sync_at = ?, cached_path = ?, cached_updated_at = ?, updated_at = CASE WHEN updated_at >= ? THEN updated_at + 1 ELSE ? END WHERE id = ? AND updated_at = ?`, now, cachedPath, now, now, now, id, expectedUpdatedAt)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	item, err := s.GetRouteRuleSubscription(id)
	if err != nil {
		return nil, false, err
	}
	return item, rows == 1, nil
}

func (s *Store) SetRouteRuleSubscriptionSyncStateIfCurrent(id, expectedUpdatedAt int64, status string, progress float64, syncError string) (bool, error) {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE route_rule_subscriptions SET sync_status = ?, sync_progress = ?, sync_error = ?, updated_at = CASE WHEN updated_at >= ? THEN updated_at + 1 ELSE ? END WHERE id = ? AND updated_at = ?`, status, progress, syncError, now, now, id, expectedUpdatedAt)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	return rows == 1, err
}

type routeRuleSubscriptionScanner interface {
	Scan(dest ...any) error
}

func scanRouteRuleSubscription(scanner routeRuleSubscriptionScanner) (*model.RouteRuleSubscription, error) {
	var item model.RouteRuleSubscription
	var enabled, useProxy int
	if err := scanner.Scan(&item.ID, &item.Name, &enabled, &item.Tag, &item.URL, &item.Format, &useProxy, &item.SyncMode, &item.SyncTime, &item.SyncWeekday, &item.SyncStatus, &item.SyncProgress, &item.SyncError, &item.LastSyncAt, &item.CachedPath, &item.CachedUpdatedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.UseProxy = useProxy != 0
	return &item, nil
}
