package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

const platformRouteColumns = `id, name, platform, enabled, priority, inbound_exposure_ids,
	source_cidrs, domains, domain_suffixes, domain_keywords, destination_cidrs,
	target_type, target_subscription_id, target_node_uid, target_collection_id,
	fallback_type, fallback_subscription_id, fallback_node_uid, fallback_collection_id,
	created_at, updated_at`

const sessionLeaseColumns = `id, name, enabled, client_cidr, inbound_exposure_ids, platform_route_id,
	target_type, target_subscription_id, target_node_uid, target_collection_id,
	fallback_type, fallback_subscription_id, fallback_node_uid, fallback_collection_id,
	expires_at, created_at, updated_at`

func (s *Store) CreatePlatformRoute(item *model.PlatformRoute) error {
	arrays, err := encodePlatformRouteArrays(item)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if item.Priority <= 0 {
		if err := tx.QueryRow(`SELECT COALESCE(MAX(priority), 0) + 10 FROM platform_routes`).Scan(&item.Priority); err != nil {
			return err
		}
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	now := time.Now().UTC().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	result, err := tx.Exec(`
		INSERT INTO platform_routes (
			name, platform, enabled, priority, inbound_exposure_ids, source_cidrs, domains,
			domain_suffixes, domain_keywords, destination_cidrs, target_type,
			target_subscription_id, target_node_uid, target_collection_id, fallback_type,
			fallback_subscription_id, fallback_node_uid, fallback_collection_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.Name, item.Platform, boolToInt(item.Enabled), item.Priority, arrays[0], arrays[1], arrays[2],
		arrays[3], arrays[4], arrays[5], item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID,
		item.TargetCollectionID, item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID,
		item.FallbackCollectionID, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetPlatformRoute(id int64) (*model.PlatformRoute, error) {
	return scanPlatformRoute(s.db.QueryRow(`SELECT `+platformRouteColumns+` FROM platform_routes WHERE id = ?`, id))
}

func (s *Store) ListPlatformRoutes() ([]model.PlatformRoute, error) {
	rows, err := s.db.Query(`SELECT ` + platformRouteColumns + ` FROM platform_routes ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.PlatformRoute, 0)
	for rows.Next() {
		item, err := scanPlatformRoute(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) UpdatePlatformRoute(id int64, item *model.PlatformRoute) error {
	arrays, err := encodePlatformRouteArrays(item)
	if err != nil {
		return err
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	item.UpdatedAt = time.Now().UTC().UnixMilli()
	result, err := s.db.Exec(`
		UPDATE platform_routes SET name = ?, platform = ?, enabled = ?, priority = ?,
			inbound_exposure_ids = ?, source_cidrs = ?, domains = ?, domain_suffixes = ?,
			domain_keywords = ?, destination_cidrs = ?, target_type = ?, target_subscription_id = ?,
			target_node_uid = ?, target_collection_id = ?, fallback_type = ?, fallback_subscription_id = ?,
			fallback_node_uid = ?, fallback_collection_id = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, item.Platform, boolToInt(item.Enabled), item.Priority, arrays[0], arrays[1], arrays[2],
		arrays[3], arrays[4], arrays[5], item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID,
		item.TargetCollectionID, item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID,
		item.FallbackCollectionID, item.UpdatedAt, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func (s *Store) DeletePlatformRoute(id int64) error {
	result, err := s.db.Exec(`DELETE FROM platform_routes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func (s *Store) ReorderPlatformRoutes(ids []int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := validateCompleteReorderIDs(tx, "platform_routes", ids); err != nil {
		return err
	}
	now := time.Now().UTC().UnixMilli()
	for index, id := range ids {
		if _, err := tx.Exec(`UPDATE platform_routes SET priority = ?, updated_at = ? WHERE id = ?`, (index+1)*10, now, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) CreateSessionLease(item *model.SessionLease) error {
	inboundExposureIDs, err := encodeJSONArray(item.InboundExposureIDs)
	if err != nil {
		return fmt.Errorf("encode session lease inbound exposure IDs: %w", err)
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	now := time.Now().UTC().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	result, err := s.db.Exec(`
		INSERT INTO session_leases (
			name, enabled, client_cidr, inbound_exposure_ids, platform_route_id, target_type,
			target_subscription_id, target_node_uid, target_collection_id, fallback_type,
			fallback_subscription_id, fallback_node_uid, fallback_collection_id, expires_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, item.Name, boolToInt(item.Enabled), item.ClientCIDR, inboundExposureIDs, item.PlatformRouteID,
		item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID, item.TargetCollectionID,
		item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID, item.FallbackCollectionID,
		item.ExpiresAt, item.CreatedAt, item.UpdatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) GetSessionLease(id int64) (*model.SessionLease, error) {
	return scanSessionLease(s.db.QueryRow(`SELECT `+sessionLeaseColumns+` FROM session_leases WHERE id = ?`, id))
}

func (s *Store) ListSessionLeases() ([]model.SessionLease, error) {
	rows, err := s.db.Query(`SELECT ` + sessionLeaseColumns + ` FROM session_leases ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.SessionLease, 0)
	for rows.Next() {
		item, err := scanSessionLease(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) UpdateSessionLease(id int64, item *model.SessionLease) error {
	inboundExposureIDs, err := encodeJSONArray(item.InboundExposureIDs)
	if err != nil {
		return fmt.Errorf("encode session lease inbound exposure IDs: %w", err)
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	item.UpdatedAt = time.Now().UTC().UnixMilli()
	result, err := s.db.Exec(`
		UPDATE session_leases SET name = ?, enabled = ?, client_cidr = ?, inbound_exposure_ids = ?,
			platform_route_id = ?, target_type = ?, target_subscription_id = ?, target_node_uid = ?,
			target_collection_id = ?, fallback_type = ?, fallback_subscription_id = ?,
			fallback_node_uid = ?, fallback_collection_id = ?, expires_at = ?, updated_at = ?
		WHERE id = ?
	`, item.Name, boolToInt(item.Enabled), item.ClientCIDR, inboundExposureIDs, item.PlatformRouteID,
		item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID, item.TargetCollectionID,
		item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID, item.FallbackCollectionID,
		item.ExpiresAt, item.UpdatedAt, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func (s *Store) UpdateSessionLeaseExpiresAt(id, expiresAt int64) error {
	result, err := s.db.Exec(`UPDATE session_leases SET expires_at = ?, updated_at = ? WHERE id = ?`, expiresAt, time.Now().UTC().UnixMilli(), id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func (s *Store) DeleteSessionLease(id int64) error {
	result, err := s.db.Exec(`DELETE FROM session_leases WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

type advancedRowScanner interface {
	Scan(dest ...any) error
}

func scanPlatformRoute(scanner advancedRowScanner) (*model.PlatformRoute, error) {
	var item model.PlatformRoute
	var enabled int
	var inboundExposureIDs, sourceCIDRs, domains, domainSuffixes, domainKeywords, destinationCIDRs string
	var targetSubscriptionID, targetCollectionID, fallbackSubscriptionID, fallbackCollectionID sql.NullInt64
	var targetNodeUID, fallbackNodeUID sql.NullString
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.Platform, &enabled, &item.Priority, &inboundExposureIDs,
		&sourceCIDRs, &domains, &domainSuffixes, &domainKeywords, &destinationCIDRs,
		&item.TargetType, &targetSubscriptionID, &targetNodeUID, &targetCollectionID,
		&item.FallbackType, &fallbackSubscriptionID, &fallbackNodeUID, &fallbackCollectionID,
		&item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.TargetSubscriptionID = nullInt64Pointer(targetSubscriptionID)
	item.TargetNodeUID = nullStringPointer(targetNodeUID)
	item.TargetCollectionID = nullInt64Pointer(targetCollectionID)
	item.FallbackSubscriptionID = nullInt64Pointer(fallbackSubscriptionID)
	item.FallbackNodeUID = nullStringPointer(fallbackNodeUID)
	item.FallbackCollectionID = nullInt64Pointer(fallbackCollectionID)
	arrays := []struct {
		name string
		raw  string
		dest any
	}{
		{"inbound_exposure_ids", inboundExposureIDs, &item.InboundExposureIDs},
		{"source_cidrs", sourceCIDRs, &item.SourceCIDRs},
		{"domains", domains, &item.Domains},
		{"domain_suffixes", domainSuffixes, &item.DomainSuffixes},
		{"domain_keywords", domainKeywords, &item.DomainKeywords},
		{"destination_cidrs", destinationCIDRs, &item.DestinationCIDRs},
	}
	for _, array := range arrays {
		if err := decodeJSONArray(array.raw, array.dest); err != nil {
			return nil, fmt.Errorf("decode platform route %s: %w", array.name, err)
		}
	}
	return &item, nil
}

func scanSessionLease(scanner advancedRowScanner) (*model.SessionLease, error) {
	var item model.SessionLease
	var enabled int
	var inboundExposureIDs string
	var platformRouteID, targetSubscriptionID, targetCollectionID, fallbackSubscriptionID, fallbackCollectionID sql.NullInt64
	var targetNodeUID, fallbackNodeUID sql.NullString
	if err := scanner.Scan(
		&item.ID, &item.Name, &enabled, &item.ClientCIDR, &inboundExposureIDs, &platformRouteID,
		&item.TargetType, &targetSubscriptionID, &targetNodeUID, &targetCollectionID,
		&item.FallbackType, &fallbackSubscriptionID, &fallbackNodeUID, &fallbackCollectionID,
		&item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt,
	); err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.PlatformRouteID = nullInt64Pointer(platformRouteID)
	item.TargetSubscriptionID = nullInt64Pointer(targetSubscriptionID)
	item.TargetNodeUID = nullStringPointer(targetNodeUID)
	item.TargetCollectionID = nullInt64Pointer(targetCollectionID)
	item.FallbackSubscriptionID = nullInt64Pointer(fallbackSubscriptionID)
	item.FallbackNodeUID = nullStringPointer(fallbackNodeUID)
	item.FallbackCollectionID = nullInt64Pointer(fallbackCollectionID)
	if err := decodeJSONArray(inboundExposureIDs, &item.InboundExposureIDs); err != nil {
		return nil, fmt.Errorf("decode session lease inbound_exposure_ids: %w", err)
	}
	return &item, nil
}

func encodePlatformRouteArrays(item *model.PlatformRoute) ([6]string, error) {
	values := []any{item.InboundExposureIDs, item.SourceCIDRs, item.Domains, item.DomainSuffixes, item.DomainKeywords, item.DestinationCIDRs}
	var encoded [6]string
	for index, value := range values {
		raw, err := encodeJSONArray(value)
		if err != nil {
			return encoded, fmt.Errorf("encode platform route array %d: %w", index, err)
		}
		encoded[index] = raw
	}
	return encoded, nil
}

func encodeJSONArray(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(raw) == "null" {
		return "[]", nil
	}
	return string(raw), nil
}

func decodeJSONArray(raw string, destination any) error {
	if raw == "" || raw == "null" {
		return fmt.Errorf("expected JSON array")
	}
	if err := json.Unmarshal([]byte(raw), destination); err != nil {
		return err
	}
	return nil
}

func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}

func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
