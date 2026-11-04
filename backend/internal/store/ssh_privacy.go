package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) GetSSHPrivacySettings() (*model.SSHPrivacySettings, error) {
	settings := &model.SSHPrivacySettings{}
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'ssh.privacy'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return settings, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func (s *Store) SetSSHPrivacySettings(settings model.SSHPrivacySettings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('ssh.privacy', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(raw), time.Now().UnixMilli())
	return err
}
