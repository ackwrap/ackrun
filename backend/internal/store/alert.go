package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const alertChannelColumns = `id, name, channel_type, enabled, config_json, destination,
	secret_context, secret_ciphertext, secret_nonce, last_status, last_error,
	last_delivered_at, created_at, updated_at`

func (s *Store) ListAlertChannels() ([]model.AlertChannel, error) {
	rows, err := s.db.Query(`SELECT ` + alertChannelColumns + ` FROM alert_channels ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AlertChannel, 0)
	for rows.Next() {
		item, err := scanAlertChannel(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetAlertChannel(id int64) (*model.AlertChannel, error) {
	return scanAlertChannel(s.db.QueryRow(`SELECT `+alertChannelColumns+` FROM alert_channels WHERE id = ?`, id))
}

func (s *Store) CreateAlertChannel(item *model.AlertChannel) error {
	now := time.Now().UTC().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	if item.LastStatus == "" {
		item.LastStatus = model.AlertDeliveryNever
	}
	config, err := json.Marshal(item.Config)
	if err != nil {
		return fmt.Errorf("encode alert channel config: %w", err)
	}
	result, err := s.db.Exec(`
		INSERT INTO alert_channels (
			name, channel_type, enabled, config_json, destination, secret_context,
			secret_ciphertext, secret_nonce, last_status, last_error,
			last_delivered_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.Name, item.Type, boolToInt(item.Enabled), string(config), item.Destination,
		item.SecretContext, item.SecretCiphertext, item.SecretNonce, item.LastStatus,
		item.LastError, item.LastDeliveredAt, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) UpdateAlertChannel(item *model.AlertChannel) error {
	item.UpdatedAt = time.Now().UTC().UnixMilli()
	config, err := json.Marshal(item.Config)
	if err != nil {
		return fmt.Errorf("encode alert channel config: %w", err)
	}
	result, err := s.db.Exec(`
		UPDATE alert_channels SET
			name = ?, channel_type = ?, enabled = ?, config_json = ?, destination = ?,
			secret_context = ?, secret_ciphertext = ?, secret_nonce = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, item.Type, boolToInt(item.Enabled), string(config), item.Destination,
		item.SecretContext, item.SecretCiphertext, item.SecretNonce, item.UpdatedAt, item.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAlertChannel(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`DELETE FROM alert_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if _, err := tx.Exec(`
		UPDATE alert_rules SET enabled = 0, updated_at = ?
		WHERE NOT EXISTS (
			SELECT 1 FROM alert_rule_channels WHERE alert_rule_channels.rule_id = alert_rules.id
		)
	`, time.Now().UTC().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateAlertChannelDeliveryState(id int64, status, lastError string, deliveredAt int64) error {
	_, err := s.db.Exec(`
		UPDATE alert_channels
		SET last_status = ?, last_error = ?, last_delivered_at = ?, updated_at = ?
		WHERE id = ?
	`, status, lastError, deliveredAt, deliveredAt, id)
	return err
}

type alertScanner interface {
	Scan(dest ...any) error
}

func scanAlertChannel(scanner alertScanner) (*model.AlertChannel, error) {
	var item model.AlertChannel
	var enabled int
	var configJSON string
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Type, &enabled, &configJSON, &item.Destination,
		&item.SecretContext, &item.SecretCiphertext, &item.SecretNonce, &item.LastStatus,
		&item.LastError, &item.LastDeliveredAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(configJSON), &item.Config); err != nil {
		return nil, fmt.Errorf("decode alert channel config: %w", err)
	}
	if item.Config.SMTPRecipients == nil {
		item.Config.SMTPRecipients = []string{}
	}
	item.Enabled = enabled != 0
	item.HasSecret = len(item.SecretCiphertext) > 0 && len(item.SecretNonce) > 0
	return &item, nil
}

func (s *Store) ListAlertRules() ([]model.AlertRule, error) {
	rows, err := s.db.Query(`
		SELECT id, name, enabled, event_types_json, cooldown_minutes, created_at, updated_at
		FROM alert_rules ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AlertRule, 0)
	for rows.Next() {
		item, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		ids, err := s.listAlertRuleChannelIDs(items[index].ID)
		if err != nil {
			return nil, err
		}
		items[index].ChannelIDs = ids
	}
	return items, nil
}

func (s *Store) GetAlertRule(id int64) (*model.AlertRule, error) {
	item, err := scanAlertRule(s.db.QueryRow(`
		SELECT id, name, enabled, event_types_json, cooldown_minutes, created_at, updated_at
		FROM alert_rules WHERE id = ?
	`, id))
	if err != nil {
		return nil, err
	}
	item.ChannelIDs, err = s.listAlertRuleChannelIDs(id)
	return item, err
}

func (s *Store) CreateAlertRule(item *model.AlertRule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	events, err := json.Marshal(item.EventTypes)
	if err != nil {
		return fmt.Errorf("encode alert rule events: %w", err)
	}
	result, err := tx.Exec(`
		INSERT INTO alert_rules (name, enabled, event_types_json, cooldown_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, item.Name, boolToInt(item.Enabled), string(events), item.CooldownMinutes, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	if err := replaceAlertRuleChannels(tx, item.ID, item.ChannelIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) UpdateAlertRule(item *model.AlertRule) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	item.UpdatedAt = time.Now().UTC().UnixMilli()
	events, err := json.Marshal(item.EventTypes)
	if err != nil {
		return fmt.Errorf("encode alert rule events: %w", err)
	}
	result, err := tx.Exec(`
		UPDATE alert_rules SET name = ?, enabled = ?, event_types_json = ?, cooldown_minutes = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, boolToInt(item.Enabled), string(events), item.CooldownMinutes, item.UpdatedAt, item.ID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	if err := replaceAlertRuleChannels(tx, item.ID, item.ChannelIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteAlertRule(id int64) error {
	result, err := s.db.Exec(`DELETE FROM alert_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func scanAlertRule(scanner alertScanner) (*model.AlertRule, error) {
	var item model.AlertRule
	var enabled int
	var eventsJSON string
	if err := scanner.Scan(&item.ID, &item.Name, &enabled, &eventsJSON, &item.CooldownMinutes, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(eventsJSON), &item.EventTypes); err != nil {
		return nil, fmt.Errorf("decode alert rule events: %w", err)
	}
	if item.EventTypes == nil {
		item.EventTypes = []string{}
	}
	item.Enabled = enabled != 0
	return &item, nil
}

func (s *Store) listAlertRuleChannelIDs(ruleID int64) ([]int64, error) {
	rows, err := s.db.Query(`SELECT channel_id FROM alert_rule_channels WHERE rule_id = ? ORDER BY channel_id`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type alertRuleChannelExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func replaceAlertRuleChannels(execer alertRuleChannelExecer, ruleID int64, channelIDs []int64) error {
	if _, err := execer.Exec(`DELETE FROM alert_rule_channels WHERE rule_id = ?`, ruleID); err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if _, err := execer.Exec(`INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES (?, ?)`, ruleID, channelID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) ClaimAlertRuleCooldown(ruleID int64, dedupKey string, now int64, cooldown time.Duration) (bool, error) {
	cutoff := now - cooldown.Milliseconds()
	result, err := s.db.Exec(`
		INSERT INTO alert_rule_cooldowns (rule_id, dedup_key, last_sent_at)
		VALUES (?, ?, ?)
		ON CONFLICT(rule_id, dedup_key) DO UPDATE SET last_sent_at = excluded.last_sent_at
		WHERE alert_rule_cooldowns.last_sent_at <= ?
	`, ruleID, dedupKey, now, cutoff)
	if err != nil {
		return false, err
	}
	count, err := result.RowsAffected()
	return count > 0, err
}

func (s *Store) CreateAlertDelivery(item *model.AlertDelivery) error {
	if item.DeliveredAt <= 0 {
		item.DeliveredAt = time.Now().UTC().UnixMilli()
	}
	result, err := s.db.Exec(`
		INSERT INTO alert_deliveries (
			rule_id, channel_id, channel_name, channel_type, event_type, event_title,
			success, status_code, error, is_test, delivered_at
		) VALUES ((SELECT id FROM alert_rules WHERE id = ?), (SELECT id FROM alert_channels WHERE id = ?), ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.RuleID, item.ChannelID, item.ChannelName, item.ChannelType, item.EventType,
		item.EventTitle, boolToInt(item.Success), item.StatusCode, item.Error, boolToInt(item.IsTest), item.DeliveredAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) ListAlertDeliveries(filter model.AlertDeliveryFilter) (*model.AlertDeliveryPage, error) {
	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 25
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}
	where, args := buildAlertDeliveryWhere(filter)
	var total int64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM alert_deliveries`+where, args...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append(append([]any(nil), args...), limit, offset)
	rows, err := s.db.Query(`
		SELECT id, rule_id, channel_id, channel_name, channel_type, event_type, event_title,
			success, status_code, error, is_test, delivered_at
		FROM alert_deliveries`+where+`
		ORDER BY delivered_at DESC, id DESC LIMIT ? OFFSET ?
	`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.AlertDelivery, 0)
	for rows.Next() {
		var item model.AlertDelivery
		var ruleID, channelID sql.NullInt64
		var success, isTest int
		if err := rows.Scan(
			&item.ID, &ruleID, &channelID, &item.ChannelName, &item.ChannelType,
			&item.EventType, &item.EventTitle, &success, &item.StatusCode, &item.Error,
			&isTest, &item.DeliveredAt,
		); err != nil {
			return nil, err
		}
		if ruleID.Valid {
			item.RuleID = &ruleID.Int64
		}
		if channelID.Valid {
			item.ChannelID = &channelID.Int64
		}
		item.Success, item.IsTest = success != 0, isTest != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &model.AlertDeliveryPage{Items: items, Total: total, PageSize: limit}, nil
}

func (s *Store) ClearAlertDeliveries() error {
	_, err := s.db.Exec(`DELETE FROM alert_deliveries`)
	return err
}

func (s *Store) PruneAlertDeliveries(keepNewest int) (int64, error) {
	if keepNewest < 0 {
		return 0, fmt.Errorf("alert delivery retention must not be negative")
	}
	var result sql.Result
	var err error
	if keepNewest == 0 {
		result, err = s.db.Exec(`DELETE FROM alert_deliveries`)
	} else {
		result, err = s.db.Exec(`
			DELETE FROM alert_deliveries
			WHERE id NOT IN (
				SELECT id FROM alert_deliveries ORDER BY delivered_at DESC, id DESC LIMIT ?
			)
		`, keepNewest)
	}
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func buildAlertDeliveryWhere(filter model.AlertDeliveryFilter) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 3)
	if filter.ChannelID > 0 {
		clauses = append(clauses, "channel_id = ?")
		args = append(args, filter.ChannelID)
	}
	if filter.EventType != "" {
		clauses = append(clauses, "event_type = ?")
		args = append(args, filter.EventType)
	}
	if filter.Status == model.AlertDeliverySuccess {
		clauses = append(clauses, "success = 1")
	} else if filter.Status == model.AlertDeliveryFailed {
		clauses = append(clauses, "success = 0")
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}
