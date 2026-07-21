package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
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

func (s *Store) MigrateConfigGenerateRequestTUNAddresses(previousIPv4, defaultIPv4, previousIPv6, defaultIPv6 string) (*model.ConfigGenerateRequest, bool, error) {
	result, err := s.db.Exec(`
		UPDATE app_settings
		SET value = json_set(
			value,
			'$.tun_ipv4_address', CASE
				WHEN TRIM(COALESCE(json_extract(value, '$.tun_ipv4_address'), '')) = ''
					OR json_extract(value, '$.tun_ipv4_address') = ? THEN ?
				ELSE json_extract(value, '$.tun_ipv4_address')
			END,
			'$.tun_ipv6_address', CASE
				WHEN TRIM(COALESCE(json_extract(value, '$.tun_ipv6_address'), '')) = ''
					OR json_extract(value, '$.tun_ipv6_address') = ? THEN ?
				ELSE json_extract(value, '$.tun_ipv6_address')
			END
		), updated_at = ?
		WHERE key = ? AND (
			TRIM(COALESCE(json_extract(value, '$.tun_ipv4_address'), '')) = ''
			OR json_extract(value, '$.tun_ipv4_address') = ?
			OR TRIM(COALESCE(json_extract(value, '$.tun_ipv6_address'), '')) = ''
			OR json_extract(value, '$.tun_ipv6_address') = ?
		)
	`, previousIPv4, defaultIPv4, previousIPv6, defaultIPv6, time.Now().UnixMilli(), configGeneratorRequestKey, previousIPv4, previousIPv6)
	if err != nil {
		return nil, false, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	request, err := s.GetConfigGenerateRequest()
	if err != nil {
		return nil, false, err
	}
	return request, rowsAffected > 0, nil
}
