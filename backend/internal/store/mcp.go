package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type MCPConfig struct {
	Enabled   bool   `json:"enabled"`
	TokenHash string `json:"token_hash"`
}

func (s *Store) GetMCPConfig() (MCPConfig, error) {
	var cfg MCPConfig
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'ackrun.mcp'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal([]byte(raw), &cfg)
	return cfg, err
}

func (s *Store) SetMCPConfig(cfg MCPConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('ackrun.mcp', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(raw), time.Now().UnixMilli())
	return err
}
