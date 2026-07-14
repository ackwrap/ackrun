package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

const configGeneratorRequestKey = "config_generator.request"

func (s *Store) GetConfigGenerateRequest() (*model.ConfigGenerateRequest, error) {
	var value string
	if err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = ?`, configGeneratorRequestKey).Scan(&value); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	var request model.ConfigGenerateRequest
	if err := json.Unmarshal([]byte(value), &request); err != nil {
		return nil, err
	}
	return &request, nil
}

func (s *Store) SetConfigGenerateRequest(request *model.ConfigGenerateRequest) error {
	value, err := json.Marshal(request)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO app_settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, configGeneratorRequestKey, string(value), time.Now().UnixMilli())
	return err
}
