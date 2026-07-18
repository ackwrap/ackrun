package store

import (
	"strconv"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

func (s *Store) GetCoreRestartSettings() (*model.CoreRestartSettings, error) {
	settings := &model.CoreRestartSettings{Mode: "daily", Time: "04:00:00", Weekday: 1}
	rows, err := s.db.Query(`
		SELECT key, value FROM app_settings
		WHERE key IN ('core_restart.mode', 'core_restart.time', 'core_restart.weekday')
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		switch key {
		case "core_restart.mode":
			settings.Mode = value
		case "core_restart.time":
			settings.Time = value
		case "core_restart.weekday":
			if weekday, err := strconv.Atoi(value); err == nil {
				settings.Weekday = weekday
			}
		}
	}
	return settings, rows.Err()
}

func (s *Store) SetCoreRestartSettings(settings *model.CoreRestartSettings) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	values := map[string]string{
		"core_restart.mode":    settings.Mode,
		"core_restart.time":    settings.Time,
		"core_restart.weekday": strconv.Itoa(settings.Weekday),
	}
	for key, value := range values {
		if _, err := tx.Exec(`
			INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, key, value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}
