package store

import (
	"database/sql"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

func (s *Store) ListSubscriptions() ([]model.Subscription, error) {
	rows, err := s.db.Query(`
		SELECT id, name, url, user_agent, sync_interval_minutes, sync_mode, sync_time, sync_weekday, sync_status, sync_progress, sync_timeout_seconds, node_count, traffic_used_bytes, traffic_total_bytes, expire_at, last_sync_at, created_at, updated_at
		FROM subscriptions ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Subscription, 0)
	for rows.Next() {
		var item model.Subscription
		if err := rows.Scan(&item.ID, &item.Name, &item.URL, &item.UserAgent, &item.SyncIntervalMins, &item.SyncMode, &item.SyncTime, &item.SyncWeekday, &item.SyncStatus, &item.SyncProgress, &item.SyncTimeoutSecs, &item.NodeCount, &item.TrafficUsedBytes, &item.TrafficTotalBytes, &item.ExpireAt, &item.LastSyncAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateSubscription(req *model.SubscriptionRequest) (*model.Subscription, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`
		INSERT INTO subscriptions (name, url, user_agent, sync_interval_minutes, sync_mode, sync_time, sync_weekday, sync_timeout_seconds, expire_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.Name, req.URL, req.UserAgent, req.SyncIntervalMins, req.SyncMode, req.SyncTime, req.SyncWeekday, req.SyncTimeoutSecs, req.ExpireAt, now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) EnsureManualSubscription() (*model.Subscription, error) {
	var id int64
	err := s.db.QueryRow(`SELECT id FROM subscriptions WHERE url = 'manual://local' ORDER BY id ASC LIMIT 1`).Scan(&id)
	if err == nil {
		return s.GetSubscription(id)
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`
		INSERT INTO subscriptions (name, url, user_agent, sync_interval_minutes, sync_mode, sync_time, sync_weekday, sync_timeout_seconds, sync_status, sync_progress, created_at, updated_at)
		VALUES (?, ?, ?, 0, 'off', '', 0, 60, 'updated', 100, ?, ?)
	`, "手动导入", "manual://local", "manual", now, now)
	if err != nil {
		return nil, err
	}
	id, err = res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) GetSubscription(id int64) (*model.Subscription, error) {
	var item model.Subscription
	err := s.db.QueryRow(`
		SELECT id, name, url, user_agent, sync_interval_minutes, sync_mode, sync_time, sync_weekday, sync_status, sync_progress, sync_timeout_seconds, node_count, traffic_used_bytes, traffic_total_bytes, expire_at, last_sync_at, created_at, updated_at
		FROM subscriptions WHERE id = ?
	`, id).Scan(&item.ID, &item.Name, &item.URL, &item.UserAgent, &item.SyncIntervalMins, &item.SyncMode, &item.SyncTime, &item.SyncWeekday, &item.SyncStatus, &item.SyncProgress, &item.SyncTimeoutSecs, &item.NodeCount, &item.TrafficUsedBytes, &item.TrafficTotalBytes, &item.ExpireAt, &item.LastSyncAt, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) UpdateSubscription(id int64, req *model.SubscriptionRequest) (*model.Subscription, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`
		UPDATE subscriptions SET name = ?, url = ?, user_agent = ?, sync_interval_minutes = ?, sync_mode = ?, sync_time = ?, sync_weekday = ?, sync_timeout_seconds = ?, expire_at = ?, updated_at = ? WHERE id = ?
	`, req.Name, req.URL, req.UserAgent, req.SyncIntervalMins, req.SyncMode, req.SyncTime, req.SyncWeekday, req.SyncTimeoutSecs, req.ExpireAt, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) DeleteSubscription(id int64) error {
	existing, err := s.GetSubscription(id)
	if err != nil || existing == nil {
		return err
	}
	return s.deleteSubscriptionNodesAndCleanup(id, true)
}

func (s *Store) ClearSubscriptionNodes(id int64) error {
	existing, err := s.GetSubscription(id)
	if err != nil || existing == nil {
		return err
	}
	return s.deleteSubscriptionNodesAndCleanup(id, false)
}

func (s *Store) deleteSubscriptionNodesAndCleanup(id int64, deleteSubscription bool) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT uid FROM nodes WHERE subscription_id = ? AND uid <> ''`, id)
	if err != nil {
		return err
	}
	uids := make([]string, 0)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return err
		}
		uids = append(uids, uid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM nodes WHERE subscription_id = ?`, id); err != nil {
		return err
	}
	if deleteSubscription {
		if _, err := tx.Exec(`DELETE FROM subscriptions WHERE id = ?`, id); err != nil {
			return err
		}
	} else {
		if _, err := tx.Exec(`UPDATE subscriptions SET node_count = 0, updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), id); err != nil {
			return err
		}
	}
	if _, err := s.cleanInvalidNodeUIDsTx(tx, uids); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) MarkSubscriptionSynced(id int64) (*model.Subscription, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE subscriptions SET sync_status = 'updated', sync_progress = 100, last_sync_at = ?, updated_at = ? WHERE id = ?`, now, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}

func (s *Store) MarkAllSubscriptionsSynced() error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE subscriptions SET sync_status = 'updated', sync_progress = 100, last_sync_at = ?, updated_at = ?`, now, now)
	return err
}

func (s *Store) SetSubscriptionSyncState(id int64, status string, progress float64) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE subscriptions SET sync_status = ?, sync_progress = ?, updated_at = ? WHERE id = ?`, status, progress, now, id)
	return err
}

// ResetInterruptedSubscriptionSyncs clears jobs that could not survive a process restart.
func (s *Store) ResetInterruptedSubscriptionSyncs() error {
	_, err := s.db.Exec(`UPDATE subscriptions SET sync_status = 'failed', sync_progress = 0 WHERE sync_status = 'syncing'`)
	return err
}

func (s *Store) UpdateSubscriptionSyncResult(id int64, nodeCount int, usedBytes int64, totalBytes int64, expireAt int64) (*model.Subscription, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`
		UPDATE subscriptions
		SET node_count = ?, traffic_used_bytes = ?, traffic_total_bytes = ?, expire_at = ?, sync_status = 'updated', sync_progress = 100, last_sync_at = ?, updated_at = ?
		WHERE id = ?
	`, nodeCount, usedBytes, totalBytes, expireAt, now, now, id)
	if err != nil {
		return nil, err
	}
	return s.GetSubscription(id)
}
