package store

import (
	"database/sql"
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

func (s *Store) removeNodeUIDsFromProxyCollections(uids []string) error {
	return s.updateProxyCollectionStringRefs("node_uids", stringRemoveSet(uids))
}

func (s *Store) removeNodeUIDFromNodeGroups(uid string) error {
	return s.updateNodeGroupStringRefs("node_uids", map[string]bool{uid: true})
}

func (s *Store) removeNodeUIDsFromNodeGroups(uids []string) error {
	return s.updateNodeGroupStringRefs("node_uids", stringRemoveSet(uids))
}

func stringRemoveSet(values []string) map[string]bool {
	remove := make(map[string]bool, len(values))
	for _, value := range values {
		if value != "" {
			remove[value] = true
		}
	}
	return remove
}

func globallyMissingNodeUIDsTx(tx *sql.Tx, removedUIDs []string) (map[string]bool, error) {
	remove := stringRemoveSet(removedUIDs)
	rows, err := tx.Query(`SELECT DISTINCT uid FROM nodes WHERE uid <> ''`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			rows.Close()
			return nil, err
		}
		delete(remove, uid)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return remove, nil
}

func (s *Store) emptyNodeGroupIDsTx(tx *sql.Tx, remove map[string]bool) ([]int64, error) {
	rows, err := tx.Query(`SELECT id, node_uids FROM node_groups`)
	if err != nil {
		return nil, err
	}
	emptyIDs := make([]int64, 0)
	for rows.Next() {
		var id int64
		var nodeUIDs string
		if err := rows.Scan(&id, &nodeUIDs); err != nil {
			rows.Close()
			return nil, err
		}
		if !hasManualNodeUIDs(nodeUIDs) {
			continue
		}
		var values []string
		if err := json.Unmarshal([]byte(nodeUIDs), &values); err != nil {
			rows.Close()
			return nil, err
		}
		keptCount := 0
		for _, uid := range values {
			if !remove[uid] {
				keptCount++
			}
		}
		if keptCount == 0 {
			emptyIDs = append(emptyIDs, id)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return emptyIDs, nil
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
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := updateIntJSONRefsTx(tx, "proxy_collections", column, remove); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) updateProxyCollectionStringRefs(column string, remove map[string]bool) error {
	return s.updateStringJSONRefs("proxy_collections", column, remove)
}

func (s *Store) updateNodeGroupStringRefs(column string, remove map[string]bool) error {
	return s.updateStringJSONRefs("node_groups", column, remove)
}

func (s *Store) updateStringJSONRefs(table, column string, remove map[string]bool) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := updateStringJSONRefsTx(tx, table, column, remove); err != nil {
		return err
	}
	return tx.Commit()
}

func updateStringJSONRefsTx(tx *sql.Tx, table, column string, remove map[string]bool) (int, error) {
	rows, err := tx.Query(`SELECT id, ` + column + ` FROM ` + table + ` WHERE ` + column + ` <> '' AND ` + column + ` <> '[]'`)
	if err != nil {
		return 0, err
	}
	updates := make([]proxyCollectionRefUpdate, 0)
	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		var values []string
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			rows.Close()
			return 0, err
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
			rows.Close()
			return 0, err
		}
		updates = append(updates, proxyCollectionRefUpdate{id: id, data: string(data)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	return len(updates), applyJSONRefUpdatesTx(tx, table, column, updates)
}

func updateIntJSONRefsTx(tx *sql.Tx, table, column string, remove map[int64]bool) (int, error) {
	rows, err := tx.Query(`SELECT id, ` + column + ` FROM ` + table + ` WHERE ` + column + ` <> '' AND ` + column + ` <> '[]'`)
	if err != nil {
		return 0, err
	}
	updates := make([]proxyCollectionRefUpdate, 0)
	for rows.Next() {
		var id int
		var raw string
		if err := rows.Scan(&id, &raw); err != nil {
			rows.Close()
			return 0, err
		}
		var values []int64
		if err := json.Unmarshal([]byte(raw), &values); err != nil {
			rows.Close()
			return 0, err
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
			rows.Close()
			return 0, err
		}
		updates = append(updates, proxyCollectionRefUpdate{id: id, data: string(data)})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	return len(updates), applyJSONRefUpdatesTx(tx, table, column, updates)
}

func applyJSONRefUpdatesTx(tx *sql.Tx, table, column string, updates []proxyCollectionRefUpdate) error {
	now := time.Now().UnixMilli()
	for _, item := range updates {
		if _, err := tx.Exec(`UPDATE `+table+` SET `+column+` = ?, updated_at = ? WHERE id = ?`, item.data, now, item.id); err != nil {
			return err
		}
	}
	return nil
}
