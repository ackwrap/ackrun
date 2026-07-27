package store

import (
	"fmt"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const advancedSettingsPrefix = "advanced."

func DefaultAdvancedSettings() model.AdvancedSettings {
	return model.AdvancedSettings{
		HealthIntervalSeconds:  60,
		HealthTimeoutSeconds:   5,
		FailureThreshold:       3,
		RecoveryThreshold:      2,
		CircuitOpenSeconds:     300,
		AccessLogRetentionDays: 7,
		AccessLogMaxEntries:    10000,
		AccessLogPrivacyMode:   model.AdvancedPrivacyStrict,
	}
}

func (s *Store) GetAdvancedSettings() (*model.AdvancedSettings, error) {
	settings := DefaultAdvancedSettings()
	rows, err := s.db.Query(`SELECT key, value FROM app_settings WHERE key LIKE 'advanced.%'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key, value string
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		if err := applyAdvancedSetting(&settings, key, value); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *Store) SetAdvancedSettings(settings *model.AdvancedSettings) error {
	values := []struct {
		key   string
		value string
	}{
		{"routing_enabled", strconv.FormatBool(settings.RoutingEnabled)},
		{"leases_enabled", strconv.FormatBool(settings.LeasesEnabled)},
		{"health_enabled", strconv.FormatBool(settings.HealthEnabled)},
		{"health_interval_seconds", strconv.Itoa(settings.HealthIntervalSeconds)},
		{"health_timeout_seconds", strconv.Itoa(settings.HealthTimeoutSeconds)},
		{"failure_threshold", strconv.Itoa(settings.FailureThreshold)},
		{"recovery_threshold", strconv.Itoa(settings.RecoveryThreshold)},
		{"circuit_open_seconds", strconv.Itoa(settings.CircuitOpenSeconds)},
		{"access_logs_enabled", strconv.FormatBool(settings.AccessLogsEnabled)},
		{"access_log_retention_days", strconv.Itoa(settings.AccessLogRetentionDays)},
		{"access_log_max_entries", strconv.Itoa(settings.AccessLogMaxEntries)},
		{"access_log_privacy_mode", settings.AccessLogPrivacyMode},
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Unix()
	for _, item := range values {
		if _, err := tx.Exec(`
			INSERT INTO app_settings (key, value, updated_at)
			VALUES (?, ?, ?)
			ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
		`, advancedSettingsPrefix+item.key, item.value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func applyAdvancedSetting(settings *model.AdvancedSettings, key, value string) error {
	parseBool := func() (bool, error) {
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return false, fmt.Errorf("parse %s: %w", key, err)
		}
		return parsed, nil
	}
	parseInt := func() (int, error) {
		parsed, err := strconv.Atoi(value)
		if err != nil {
			return 0, fmt.Errorf("parse %s: %w", key, err)
		}
		return parsed, nil
	}
	var err error
	switch key {
	case advancedSettingsPrefix + "routing_enabled":
		settings.RoutingEnabled, err = parseBool()
	case advancedSettingsPrefix + "leases_enabled":
		settings.LeasesEnabled, err = parseBool()
	case advancedSettingsPrefix + "health_enabled":
		settings.HealthEnabled, err = parseBool()
	case advancedSettingsPrefix + "health_interval_seconds":
		settings.HealthIntervalSeconds, err = parseInt()
	case advancedSettingsPrefix + "health_timeout_seconds":
		settings.HealthTimeoutSeconds, err = parseInt()
	case advancedSettingsPrefix + "failure_threshold":
		settings.FailureThreshold, err = parseInt()
	case advancedSettingsPrefix + "recovery_threshold":
		settings.RecoveryThreshold, err = parseInt()
	case advancedSettingsPrefix + "circuit_open_seconds":
		settings.CircuitOpenSeconds, err = parseInt()
	case advancedSettingsPrefix + "access_logs_enabled":
		settings.AccessLogsEnabled, err = parseBool()
	case advancedSettingsPrefix + "access_log_retention_days":
		settings.AccessLogRetentionDays, err = parseInt()
	case advancedSettingsPrefix + "access_log_max_entries":
		settings.AccessLogMaxEntries, err = parseInt()
	case advancedSettingsPrefix + "access_log_privacy_mode":
		settings.AccessLogPrivacyMode = value
	}
	return err
}
