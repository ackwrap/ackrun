package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

type SSHMCPConfig struct {
	Enabled      bool   `json:"enabled"`
	LocalEnabled bool   `json:"local_enabled"`
	LocalName    string `json:"local_name"`
	TokenHash    string `json:"token_hash"`
}

func (s *Store) GetSSHMCPConfig() (SSHMCPConfig, error) {
	cfg := SSHMCPConfig{LocalName: "本机"}
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'ssh.mcp'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal([]byte(raw), &cfg)
	if cfg.LocalName == "" {
		cfg.LocalName = "本机"
	}
	return cfg, err
}

func (s *Store) SetSSHMCPConfig(cfg SSHMCPConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`INSERT INTO app_settings (key, value, updated_at) VALUES ('ssh.mcp', ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(raw), time.Now().UnixMilli())
	return err
}
