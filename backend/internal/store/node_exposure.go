package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

var ErrNodeExposureTargetUnavailable = errors.New("node exposure target is unavailable")

const nodeExposureColumns = `e.id, e.name, e.subscription_id, e.node_uid, e.inbound_type,
	e.listen, e.listen_port, e.username, e.password, e.enabled, e.created_at, e.updated_at,
	COALESCE(n.name, ''), COALESCE(n.type, ''), COALESCE(s.name, ''),
	CASE WHEN n.id IS NULL THEN 0 ELSE 1 END, COALESCE(n.enabled, 0)`

func (s *Store) CreateNodeExposure(item *model.NodeExposure) error {
	now := time.Now().UnixMilli()
	item.CreatedAt = now
	item.UpdatedAt = now
	result, err := s.db.Exec(`
		INSERT INTO node_exposures
			(name, subscription_id, node_uid, inbound_type, listen, listen_port, username, password, enabled, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
		WHERE ? = 0 OR EXISTS (
			SELECT 1 FROM nodes WHERE subscription_id = ? AND uid = ? AND enabled = 1
		)
	`, item.Name, item.SubscriptionID, item.NodeUID, item.InboundType, item.Listen, item.ListenPort,
		item.Username, item.Password, boolToInt(item.Enabled), item.CreatedAt, item.UpdatedAt,
		boolToInt(item.Enabled), item.SubscriptionID, item.NodeUID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrNodeExposureTargetUnavailable
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) GetNodeExposure(id int64) (*model.NodeExposureWithNode, error) {
	row := s.db.QueryRow(`
		SELECT `+nodeExposureColumns+`
		FROM node_exposures e
		LEFT JOIN subscriptions s ON s.id = e.subscription_id
		LEFT JOIN nodes n ON n.subscription_id = e.subscription_id AND n.uid = e.node_uid
		WHERE e.id = ?
	`, id)
	return scanNodeExposure(row)
}

func (s *Store) ListNodeExposures() ([]model.NodeExposureWithNode, error) {
	rows, err := s.db.Query(`
		SELECT ` + nodeExposureColumns + `
		FROM node_exposures e
		LEFT JOIN subscriptions s ON s.id = e.subscription_id
		LEFT JOIN nodes n ON n.subscription_id = e.subscription_id AND n.uid = e.node_uid
		ORDER BY e.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.NodeExposureWithNode, 0)
	for rows.Next() {
		item, err := scanNodeExposure(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) UpdateNodeExposure(id int64, item *model.NodeExposure) error {
	item.UpdatedAt = time.Now().UnixMilli()
	result, err := s.db.Exec(`
		UPDATE node_exposures
		SET name = ?, subscription_id = ?, node_uid = ?, inbound_type = ?, listen = ?, listen_port = ?,
			username = ?, password = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND (? = 0 OR EXISTS (
			SELECT 1 FROM nodes WHERE subscription_id = ? AND uid = ? AND enabled = 1
		))
	`, item.Name, item.SubscriptionID, item.NodeUID, item.InboundType, item.Listen, item.ListenPort,
		item.Username, item.Password, boolToInt(item.Enabled), item.UpdatedAt, id,
		boolToInt(item.Enabled), item.SubscriptionID, item.NodeUID)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	var exists int
	if err := s.db.QueryRow(`SELECT 1 FROM node_exposures WHERE id = ?`, id).Scan(&exists); err != nil {
		return err
	}
	return ErrNodeExposureTargetUnavailable
}

func (s *Store) DeleteNodeExposure(id int64) error {
	result, err := s.db.Exec(`DELETE FROM node_exposures WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireRowsAffected(result)
}

func (s *Store) RestoreNodeExposure(item *model.NodeExposure) error {
	_, err := s.db.Exec(`
		INSERT INTO node_exposures
			(id, name, subscription_id, node_uid, inbound_type, listen, listen_port, username, password, enabled, created_at, updated_at)
		SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?,
			CASE WHEN ? = 1 AND EXISTS (
				SELECT 1 FROM nodes WHERE subscription_id = ? AND uid = ? AND enabled = 1
			) THEN 1 ELSE 0 END,
			?, ?
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			subscription_id = excluded.subscription_id,
			node_uid = excluded.node_uid,
			inbound_type = excluded.inbound_type,
			listen = excluded.listen,
			listen_port = excluded.listen_port,
			username = excluded.username,
			password = excluded.password,
			enabled = excluded.enabled,
			created_at = excluded.created_at,
			updated_at = excluded.updated_at
	`, item.ID, item.Name, item.SubscriptionID, item.NodeUID, item.InboundType, item.Listen, item.ListenPort,
		item.Username, item.Password, boolToInt(item.Enabled), item.SubscriptionID, item.NodeUID, item.CreatedAt, item.UpdatedAt)
	return err
}

func (s *Store) GetNodeByReference(subscriptionID int64, uid string) (*model.Node, error) {
	var item model.Node
	err := scanNode(s.db.QueryRow(`
		SELECT n.id, n.uid, n.subscription_id, COALESCE(s.name, '') AS subscription_name,
			n.name, n.name_overridden, n.type, n.server, n.server_port, n.raw, n.raw_json, n.enabled, n.preferred,
			n.latency_ms, n.status, n.last_test_at, n.test_latency_ms, n.test_success, n.created_at, n.updated_at
		FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id
		WHERE n.subscription_id = ? AND n.uid = ?
	`, subscriptionID, uid), &item)
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func scanNodeExposure(scanner rowScanner) (*model.NodeExposureWithNode, error) {
	var item model.NodeExposureWithNode
	var enabled, nodeExists, nodeEnabled int
	if err := scanner.Scan(
		&item.ID, &item.Name, &item.SubscriptionID, &item.NodeUID, &item.InboundType,
		&item.Listen, &item.ListenPort, &item.Username, &item.Password, &enabled, &item.CreatedAt, &item.UpdatedAt,
		&item.NodeName, &item.NodeType, &item.SubscriptionName, &nodeExists, &nodeEnabled,
	); err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	item.HasPassword = item.Password != ""
	item.NodeExists = nodeExists != 0
	item.NodeEnabled = nodeEnabled != 0
	return &item, nil
}

func requireRowsAffected(result sql.Result) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}
