package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const advancedAccessLogColumns = `id, core_event_id, event_time, network, inbound, source_hash,
	destination_summary, domain_summary, outbound_label, platform, platform_route_id,
	session_lease_id, decision, error_summary, created_at`

func (s *Store) InsertAdvancedAccessLogs(items []model.AdvancedAccessLog) (int, error) {
	if len(items) == 0 {
		return 0, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`
		INSERT INTO advanced_access_logs (
			core_event_id, event_time, network, inbound, source_hash, destination_summary,
			domain_summary, outbound_label, platform, platform_route_id, session_lease_id,
			decision, error_summary, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(core_event_id) DO NOTHING
	`)
	if err != nil {
		return 0, err
	}
	defer statement.Close()
	now := time.Now().UTC().UnixMilli()
	inserted := 0
	for index := range items {
		item := &items[index]
		if item.CoreEventID == "" {
			return 0, fmt.Errorf("core event ID must not be empty")
		}
		if item.CreatedAt <= 0 {
			item.CreatedAt = now
		}
		result, err := statement.Exec(
			item.CoreEventID, item.EventTime, item.Network, item.Inbound, item.SourceHash,
			item.DestinationSummary, item.DomainSummary, item.OutboundLabel, item.Platform,
			item.PlatformRouteID, item.SessionLeaseID, item.Decision, item.ErrorSummary, item.CreatedAt,
		)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		inserted += int(count)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return inserted, nil
}

func (s *Store) ListAdvancedAccessLogs(filter model.AdvancedAccessLogFilter) (*model.AdvancedAccessLogPage, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	where, args := buildAdvancedAccessLogWhere(filter)
	total, err := s.countAdvancedAccessLogs(where, args)
	if err != nil {
		return nil, err
	}
	queryArgs := append(append([]any(nil), args...), limit, offset)
	rows, err := s.db.Query(`
		SELECT `+advancedAccessLogColumns+`
		FROM advanced_access_logs`+where+`
		ORDER BY event_time DESC, id DESC
		LIMIT ? OFFSET ?
	`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AdvancedAccessLog, 0)
	for rows.Next() {
		item, err := scanAdvancedAccessLog(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &model.AdvancedAccessLogPage{Items: items, Total: total, Limit: limit, Offset: offset}, nil
}

func (s *Store) CountAdvancedAccessLogs(filter model.AdvancedAccessLogFilter) (int64, error) {
	where, args := buildAdvancedAccessLogWhere(filter)
	return s.countAdvancedAccessLogs(where, args)
}

func (s *Store) ClearAdvancedAccessLogs() error {
	_, err := s.db.Exec(`DELETE FROM advanced_access_logs`)
	return err
}

func (s *Store) CleanupAdvancedAccessLogs(retentionDays, maxEntries int) (int64, error) {
	if retentionDays < 0 || maxEntries < 0 {
		return 0, fmt.Errorf("access log retention and maximum entries must not be negative")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var deleted int64
	if retentionDays > 0 {
		cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays).UnixMilli()
		result, err := tx.Exec(`DELETE FROM advanced_access_logs WHERE event_time < ?`, cutoff)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += count
	}
	if maxEntries > 0 {
		result, err := tx.Exec(`
			DELETE FROM advanced_access_logs
			WHERE id NOT IN (
				SELECT id FROM advanced_access_logs ORDER BY event_time DESC, id DESC LIMIT ?
			)
		`, maxEntries)
		if err != nil {
			return 0, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		deleted += count
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return deleted, nil
}

func (s *Store) countAdvancedAccessLogs(where string, args []any) (int64, error) {
	var total int64
	err := s.db.QueryRow(`SELECT COUNT(*) FROM advanced_access_logs`+where, args...).Scan(&total)
	return total, err
}

func buildAdvancedAccessLogWhere(filter model.AdvancedAccessLogFilter) (string, []any) {
	clauses := make([]string, 0, 5)
	args := make([]any, 0, 10)
	if filter.Platform != "" {
		clauses = append(clauses, "platform = ?")
		args = append(args, filter.Platform)
	}
	if filter.Decision != "" {
		clauses = append(clauses, "decision = ?")
		args = append(args, filter.Decision)
	}
	if filter.Keyword != "" {
		keyword := "%" + filter.Keyword + "%"
		clauses = append(clauses, `(inbound LIKE ? OR source_hash LIKE ? OR destination_summary LIKE ? OR
			domain_summary LIKE ? OR outbound_label LIKE ? OR error_summary LIKE ?)`)
		for range 6 {
			args = append(args, keyword)
		}
	}
	if filter.FromTime > 0 {
		clauses = append(clauses, "event_time >= ?")
		args = append(args, filter.FromTime)
	}
	if filter.ToTime > 0 {
		clauses = append(clauses, "event_time <= ?")
		args = append(args, filter.ToTime)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func scanAdvancedAccessLog(scanner advancedRowScanner) (*model.AdvancedAccessLog, error) {
	var item model.AdvancedAccessLog
	var platformRouteID, sessionLeaseID sql.NullInt64
	if err := scanner.Scan(
		&item.ID, &item.CoreEventID, &item.EventTime, &item.Network, &item.Inbound,
		&item.SourceHash, &item.DestinationSummary, &item.DomainSummary, &item.OutboundLabel,
		&item.Platform, &platformRouteID, &sessionLeaseID, &item.Decision, &item.ErrorSummary,
		&item.CreatedAt,
	); err != nil {
		return nil, err
	}
	item.PlatformRouteID = nullInt64Pointer(platformRouteID)
	item.SessionLeaseID = nullInt64Pointer(sessionLeaseID)
	return &item, nil
}
