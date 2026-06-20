package store

import (
	"encoding/json"
	"time"
)

type proxyCollectionRefUpdate struct {
	id   int
	data string
}

func (s *Store) removeNodeUIDFromProxyCollections(uid string) error {
	return s.updateProxyCollectionStringRefs("node_uids", map[string]bool{uid: true})
}

func (s *Store) removeNodeUIDFromNodeGroups(uid string) error {
	return s.updateNodeGroupStringRefs("node_uids", map[string]bool{uid: true})
}

func (s *Store) removeNodeGroupRefsFromProxyCollections(ids []int64) error {
	remove := make(map[int64]bool, len(ids))
	for _, id := range ids {
		remove[id] = true
	}
	return s.updateProxyCollectionIntRefs("referenced_group_ids", remove)
}

func (s *Store) removeRouteRuleRefsFromProxyCollections(id int64) error {
	return s.updateProxyCollectionIntRefs("route_rule_ids", map[int64]bool{id: true})
}

func (s *Store) updateProxyCollectionIntRefs(column string, remove map[int64]bool) error {
	rows, err := s.db.Query(`SELECT id, ` + column + ` FROM proxy_collections WHERE ` + column + ` <> '' AND ` + column + ` <> '[]'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	updates := make([]proxyCollectionRefUpdate, 0)
	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var values []int64
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return err
		}
		kept := values[:0]
		changed := false
		for _, value := range values {
			if remove[value] {
				changed = true
				continue
			}
			kept = append(kept, value)
		}
		if !changed {
			continue
		}
		data, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		updates = append(updates, proxyCollectionRefUpdate{id: id, data: string(data)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.applyProxyCollectionRefUpdates(column, updates)
}

func (s *Store) updateProxyCollectionStringRefs(column string, remove map[string]bool) error {
	return s.updateStringJSONRefs("proxy_collections", column, remove)
}

func (s *Store) updateNodeGroupStringRefs(column string, remove map[string]bool) error {
	return s.updateStringJSONRefs("node_groups", column, remove)
}

func (s *Store) updateStringJSONRefs(table, column string, remove map[string]bool) error {
	rows, err := s.db.Query(`SELECT id, ` + column + ` FROM ` + table + ` WHERE ` + column + ` <> '' AND ` + column + ` <> '[]'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	updates := make([]proxyCollectionRefUpdate, 0)
	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return err
		}
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			return err
		}
		kept := values[:0]
		changed := false
		for _, value := range values {
			if remove[value] {
				changed = true
				continue
			}
			kept = append(kept, value)
		}
		if !changed {
			continue
		}
		data, err := json.Marshal(kept)
		if err != nil {
			return err
		}
		updates = append(updates, proxyCollectionRefUpdate{id: id, data: string(data)})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return s.applyJSONRefUpdates(table, column, updates)
}

func (s *Store) applyProxyCollectionRefUpdates(column string, updates []proxyCollectionRefUpdate) error {
	return s.applyJSONRefUpdates("proxy_collections", column, updates)
}

func (s *Store) applyJSONRefUpdates(table, column string, updates []proxyCollectionRefUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	for _, item := range updates {
		if _, err := tx.Exec(`UPDATE `+table+` SET `+column+` = ?, updated_at = ? WHERE id = ?`, item.data, now, item.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}
