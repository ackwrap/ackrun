package store

import (
	"database/sql"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) ListNodeFilters() ([]model.NodeFilter, error) {
	rows, err := s.db.Query(`SELECT id, name, target, pattern, enabled, created_at, updated_at FROM node_filters ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.NodeFilter, 0)
	for rows.Next() {
		var item model.NodeFilter
		var enabled int
		if err := rows.Scan(&item.ID, &item.Name, &item.Target, &item.Pattern, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListEnabledNodeFilters() ([]model.NodeFilter, error) {
	rows, err := s.db.Query(`SELECT id, name, target, pattern, enabled, created_at, updated_at FROM node_filters WHERE enabled = 1 ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.NodeFilter, 0)
	for rows.Next() {
		var item model.NodeFilter
		var enabled int
		if err := rows.Scan(&item.ID, &item.Name, &item.Target, &item.Pattern, &enabled, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Enabled = enabled != 0
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) CreateNodeFilter(req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`INSERT INTO node_filters (name, target, pattern, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, req.Name, req.Target, req.Pattern, boolToInt(req.Enabled), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetNodeFilter(id)
}

func (s *Store) GetNodeFilter(id int64) (*model.NodeFilter, error) {
	var item model.NodeFilter
	var enabled int
	err := s.db.QueryRow(`SELECT id, name, target, pattern, enabled, created_at, updated_at FROM node_filters WHERE id = ?`, id).Scan(&item.ID, &item.Name, &item.Target, &item.Pattern, &enabled, &item.CreatedAt, &item.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	return &item, nil
}

func (s *Store) UpdateNodeFilter(id int64, req *model.NodeFilterRequest) (*model.NodeFilter, error) {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE node_filters SET name = ?, target = ?, pattern = ?, enabled = ?, updated_at = ? WHERE id = ?`, req.Name, req.Target, req.Pattern, boolToInt(req.Enabled), now, id)
	if err != nil {
		return nil, err
	}
	return s.GetNodeFilter(id)
}

func (s *Store) DeleteNodeFilter(id int64) error {
	_, err := s.db.Exec(`DELETE FROM node_filters WHERE id = ?`, id)
	return err
}
