package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

var ErrSensitiveAdvancedHealthText = errors.New("advanced health text contains sensitive connection fields")

func (s *Store) UpsertAdvancedHealthState(item *model.AdvancedHealthState) error {
	return upsertAdvancedHealthState(s.db, item)
}

func upsertAdvancedHealthState(execer advancedHealthExecer, item *model.AdvancedHealthState) error {
	if containsSensitiveConnectionField(item.TargetRef) || containsSensitiveConnectionField(item.DisplayName) || containsSensitiveConnectionField(item.LastError) {
		return ErrSensitiveAdvancedHealthText
	}
	item.UpdatedAt = time.Now().UTC().UnixMilli()
	_, err := execer.Exec(`
		INSERT INTO advanced_health_states (
			target_key, target_type, target_ref, display_name, status, latency_ms,
			consecutive_failures, consecutive_successes, circuit_open_until,
			last_error, last_checked_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(target_key) DO UPDATE SET
			target_type = excluded.target_type,
			target_ref = excluded.target_ref,
			display_name = excluded.display_name,
			status = excluded.status,
			latency_ms = excluded.latency_ms,
			consecutive_failures = excluded.consecutive_failures,
			consecutive_successes = excluded.consecutive_successes,
			circuit_open_until = excluded.circuit_open_until,
			last_error = excluded.last_error,
			last_checked_at = excluded.last_checked_at,
			updated_at = excluded.updated_at
	`, item.TargetKey, item.TargetType, item.TargetRef, item.DisplayName, item.Status, item.LatencyMS,
		item.ConsecutiveFailures, item.ConsecutiveSuccesses, item.CircuitOpenUntil,
		item.LastError, item.LastCheckedAt, item.UpdatedAt)
	return err
}

func (s *Store) ListAdvancedHealthStates() ([]model.AdvancedHealthState, error) {
	rows, err := s.db.Query(`
		SELECT target_key, target_type, target_ref, display_name, status, latency_ms,
			consecutive_failures, consecutive_successes, circuit_open_until,
			last_error, last_checked_at, updated_at
		FROM advanced_health_states
		ORDER BY display_name COLLATE NOCASE ASC, target_key ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AdvancedHealthState, 0)
	for rows.Next() {
		var item model.AdvancedHealthState
		if err := rows.Scan(
			&item.TargetKey, &item.TargetType, &item.TargetRef, &item.DisplayName, &item.Status,
			&item.LatencyMS, &item.ConsecutiveFailures, &item.ConsecutiveSuccesses,
			&item.CircuitOpenUntil, &item.LastError, &item.LastCheckedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) AppendAdvancedHealthEvent(item *model.AdvancedHealthEvent) error {
	return appendAdvancedHealthEvent(s.db, item)
}

func appendAdvancedHealthEvent(execer advancedHealthExecer, item *model.AdvancedHealthEvent) error {
	if containsSensitiveConnectionField(item.DisplayName) || containsSensitiveConnectionField(item.Message) {
		return ErrSensitiveAdvancedHealthText
	}
	if item.CreatedAt <= 0 {
		item.CreatedAt = time.Now().UTC().UnixMilli()
	}
	result, err := execer.Exec(`
		INSERT INTO advanced_health_events
			(target_key, target_type, display_name, event_type, message, latency_ms, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, item.TargetKey, item.TargetType, item.DisplayName, item.EventType, item.Message, item.LatencyMS, item.CreatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

type advancedHealthExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func (s *Store) ApplyAdvancedHealthResults(states []model.AdvancedHealthState, events []model.AdvancedHealthEvent) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for index := range states {
		if err := upsertAdvancedHealthState(tx, &states[index]); err != nil {
			return err
		}
	}
	for index := range events {
		if err := appendAdvancedHealthEvent(tx, &events[index]); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListAdvancedHealthEvents(limit, offset int) ([]model.AdvancedHealthEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.Query(`
		SELECT id, target_key, target_type, display_name, event_type, message, latency_ms, created_at
		FROM advanced_health_events
		ORDER BY created_at DESC, id DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AdvancedHealthEvent, 0)
	for rows.Next() {
		var item model.AdvancedHealthEvent
		if err := rows.Scan(&item.ID, &item.TargetKey, &item.TargetType, &item.DisplayName,
			&item.EventType, &item.Message, &item.LatencyMS, &item.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PruneAdvancedHealthEvents(keepNewest int) (int64, error) {
	if keepNewest < 0 {
		return 0, fmt.Errorf("health event retention must not be negative")
	}
	var result sql.Result
	var err error
	if keepNewest == 0 {
		result, err = s.db.Exec(`DELETE FROM advanced_health_events`)
	} else {
		result, err = s.db.Exec(`
			DELETE FROM advanced_health_events
			WHERE id NOT IN (
				SELECT id FROM advanced_health_events ORDER BY created_at DESC, id DESC LIMIT ?
			)
		`, keepNewest)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func containsSensitiveConnectionField(value string) bool {
	lower := strings.ToLower(value)
	for _, marker := range []string{
		`"server"`, "server=", "server:", `"uuid"`, "uuid=", "uuid:",
		`"password"`, "password=", "password:", `"private_key"`, "private_key=",
		`"cipher"`, "cipher=", `"raw"`, `"raw_json"`,
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
