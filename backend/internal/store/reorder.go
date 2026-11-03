package store

import (
	"database/sql"
	"fmt"
)

type reorderID interface {
	~int | ~int64
}

func validateCompleteReorderIDs[T reorderID](tx *sql.Tx, table string, ids []T) error {
	var label string
	switch table {
	case "proxy_collections":
		label = "策略组"
	case "dns_servers":
		label = "DNS Server"
	case "dns_rules":
		label = "DNS 规则"
	case "node_groups":
		label = "节点组"
	case "route_rules":
		label = "路由规则"
	default:
		return fmt.Errorf("不支持排序表: %s", table)
	}
	if len(ids) == 0 {
		return fmt.Errorf("%s ID 不能为空", label)
	}
	requested := make(map[int64]struct{}, len(ids))
	for _, rawID := range ids {
		id := int64(rawID)
		if id <= 0 {
			return fmt.Errorf("%s ID 无效: %d", label, id)
		}
		if _, found := requested[id]; found {
			return fmt.Errorf("%s ID 重复: %d", label, id)
		}
		requested[id] = struct{}{}
	}

	rows, err := tx.Query(`SELECT id FROM ` + table)
	if err != nil {
		return err
	}
	existing := make(map[int64]struct{}, len(ids))
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		existing[id] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(existing) != len(requested) {
		return fmt.Errorf("%s 排序必须包含全部 ID", label)
	}
	for id := range requested {
		if _, found := existing[id]; !found {
			return fmt.Errorf("%s 不存在: %d", label, id)
		}
	}
	return nil
}
