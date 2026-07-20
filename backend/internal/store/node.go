package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

func (s *Store) ReplaceSubscriptionNodes(subscriptionID int64, nodes []model.ParsedNode) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existingRows, err := tx.Query(`SELECT uid, name, name_overridden, enabled, preferred, latency_ms, status FROM nodes WHERE subscription_id = ? AND uid <> ''`, subscriptionID)
	if err != nil {
		return err
	}
	existing := make(map[string]nodeState)
	for existingRows.Next() {
		var uid string
		var state nodeState
		if err := existingRows.Scan(&uid, &state.Name, &state.NameOverridden, &state.Enabled, &state.Preferred, &state.LatencyMS, &state.Status); err != nil {
			existingRows.Close()
			return err
		}
		existing[uid] = state
	}
	if err := existingRows.Err(); err != nil {
		existingRows.Close()
		return err
	}
	existingRows.Close()

	if _, err := tx.Exec(`DELETE FROM nodes WHERE subscription_id = ?`, subscriptionID); err != nil {
		return err
	}

	now := time.Now().UnixMilli()
	stmt, err := tx.Prepare(`
		INSERT INTO nodes (uid, subscription_id, name, name_overridden, type, server, server_port, raw, raw_json, enabled, preferred, latency_ms, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	seen := make(map[string]int, len(nodes))
	for _, node := range nodes {
		uid := node.UID
		if uid == "" {
			uid = StableNodeUID(node)
		}
		baseUID := uid
		seen[baseUID]++
		if seen[baseUID] > 1 {
			uid = baseUID + "-" + shortNodeHash(node.Name+node.Raw) + "-" + shortNodeSuffix(seen[baseUID])
		}
		state := nodeState{Enabled: 1, Preferred: 0, Status: "unknown"}
		name := node.Name
		if old, ok := existing[uid]; ok {
			state = old
			if old.NameOverridden != 0 && old.Name != "" {
				name = old.Name
			}
		}
		if _, err := stmt.Exec(uid, subscriptionID, name, state.NameOverridden, node.Type, node.Server, node.ServerPort, node.Raw, node.RawJSON, state.Enabled, state.Preferred, state.LatencyMS, state.Status, now, now); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) UpsertSubscriptionNodes(subscriptionID int64, nodes []model.ParsedNode) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existingRows, err := tx.Query(`SELECT uid, name, name_overridden, enabled, preferred, latency_ms, status FROM nodes WHERE subscription_id = ? AND uid <> ''`, subscriptionID)
	if err != nil {
		return err
	}
	existing := make(map[string]nodeState)
	for existingRows.Next() {
		var uid string
		var state nodeState
		if err := existingRows.Scan(&uid, &state.Name, &state.NameOverridden, &state.Enabled, &state.Preferred, &state.LatencyMS, &state.Status); err != nil {
			existingRows.Close()
			return err
		}
		existing[uid] = state
	}
	if err := existingRows.Err(); err != nil {
		existingRows.Close()
		return err
	}
	existingRows.Close()

	now := time.Now().UnixMilli()
	seen := make(map[string]int, len(nodes))
	for _, node := range nodes {
		uid := node.UID
		if uid == "" {
			uid = StableNodeUID(node)
		}
		baseUID := uid
		seen[baseUID]++
		if seen[baseUID] > 1 {
			uid = baseUID + "-" + shortNodeHash(node.Name+node.Raw) + "-" + shortNodeSuffix(seen[baseUID])
		}
		state := nodeState{Enabled: 1, Preferred: 0, Status: "unknown"}
		name := node.Name
		if old, ok := existing[uid]; ok {
			state = old
			if old.NameOverridden != 0 && old.Name != "" {
				name = old.Name
			}
		}
		if _, err := tx.Exec(`
			INSERT INTO nodes (uid, subscription_id, name, name_overridden, type, server, server_port, raw, raw_json, enabled, preferred, latency_ms, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(subscription_id, uid) WHERE uid <> '' DO UPDATE SET
				name = excluded.name,
				name_overridden = excluded.name_overridden,
				type = excluded.type,
				server = excluded.server,
				server_port = excluded.server_port,
				raw = excluded.raw,
				raw_json = excluded.raw_json,
				enabled = excluded.enabled,
				preferred = excluded.preferred,
				latency_ms = excluded.latency_ms,
				status = excluded.status,
				updated_at = excluded.updated_at
		`, uid, subscriptionID, name, state.NameOverridden, node.Type, node.Server, node.ServerPort, node.Raw, node.RawJSON, state.Enabled, state.Preferred, state.LatencyMS, state.Status, now, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`
		UPDATE subscriptions
		SET node_count = (SELECT COUNT(*) FROM nodes WHERE subscription_id = ?), updated_at = ?
		WHERE id = ?
	`, subscriptionID, now, subscriptionID); err != nil {
		return err
	}
	return tx.Commit()
}

type nodeState struct {
	Enabled        int
	Preferred      int
	LatencyMS      int
	Status         string
	Name           string
	NameOverridden int
}

func (s *Store) ListNodesBySubscription(subscriptionID int64) ([]model.Node, error) {
	rows, err := s.db.Query(`
		SELECT id, uid, subscription_id, '' AS subscription_name, name, name_overridden, type, server, server_port, raw, raw_json, enabled, preferred, latency_ms, status, last_test_at, test_latency_ms, test_success, created_at, updated_at
		FROM nodes WHERE subscription_id = ? ORDER BY id ASC
	`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]model.Node, 0)
	for rows.Next() {
		var item model.Node
		if err := scanNode(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) ListNodes(req model.NodeListRequest) (*model.NodeListResponse, error) {
	where, args := buildNodeWhere(req)
	countQuery := `SELECT COUNT(*) FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id` + where
	var total int
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, err
	}
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if req.Offset < 0 {
		req.Offset = 0
	}
	query := `
		SELECT n.id, n.uid, n.subscription_id, COALESCE(s.name, '') AS subscription_name,
			n.name, n.name_overridden, n.type, n.server, n.server_port, n.raw, n.raw_json, n.enabled, n.preferred,
			n.latency_ms, n.status, n.last_test_at, n.test_latency_ms, n.test_success, n.created_at, n.updated_at
		FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id` + where + `
		ORDER BY n.name COLLATE NOCASE ASC, n.subscription_id ASC, n.uid ASC LIMIT ? OFFSET ?`
	queryArgs := append(args, limit, req.Offset)
	rows, err := s.db.Query(query, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Node, 0)
	for rows.Next() {
		var item model.Node
		if err := scanNode(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return &model.NodeListResponse{Items: items, Total: total}, rows.Err()
}

func (s *Store) GetSubscriptionNodeUIDs(subscriptionID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT uid FROM nodes WHERE subscription_id = ? AND uid <> ''`, subscriptionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	uids := make([]string, 0)
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		uids = append(uids, uid)
	}
	return uids, rows.Err()
}

func (s *Store) ListNodesByUIDs(uids []string) ([]model.Node, error) {
	if len(uids) == 0 {
		return []model.Node{}, nil
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(uids)), ",")
	args := make([]any, 0, len(uids))
	for _, uid := range uids {
		args = append(args, uid)
	}
	rows, err := s.db.Query(`
		SELECT n.id, n.uid, n.subscription_id, COALESCE(s.name, '') AS subscription_name,
			n.name, n.name_overridden, n.type, n.server, n.server_port, n.raw, n.raw_json, n.enabled, n.preferred,
			n.latency_ms, n.status, n.last_test_at, n.test_latency_ms, n.test_success, n.created_at, n.updated_at
		FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id WHERE n.uid IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.Node, 0)
	for rows.Next() {
		var item model.Node
		if err := scanNode(rows, &item); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) UpdateNodeName(uid string, name string) error {
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`UPDATE nodes SET name = ?, name_overridden = 1, updated_at = ? WHERE uid = ?`, name, now, uid)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("node not found")
	}
	return nil
}

func (s *Store) UpdateNodeTCPing(uid string, latencyMS int, status string) error {
	now := time.Now().UnixMilli()
	_, err := s.db.Exec(`UPDATE nodes SET latency_ms = ?, status = ?, updated_at = ? WHERE uid = ?`, latencyMS, status, now, uid)
	return err
}

func (s *Store) UpdateNodeHealthCheck(uid string, latencyMS int, success bool, testedAt int64) error {
	_, err := s.db.Exec(`UPDATE nodes SET test_latency_ms = ?, test_success = ?, last_test_at = ?, updated_at = ? WHERE uid = ?`, latencyMS, boolToInt(success), testedAt, testedAt, uid)
	return err
}

func (s *Store) DeleteNode(uid string) error {
	s.nodeRefsMu.Lock()
	defer s.nodeRefsMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var subscriptionID int64
	if err := tx.QueryRow(`SELECT subscription_id FROM nodes WHERE uid = ?`, uid).Scan(&subscriptionID); err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("node not found: %s", uid)
		}
		return err
	}
	res, err := tx.Exec(`DELETE FROM nodes WHERE uid = ?`, uid)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return fmt.Errorf("node not found: %s", uid)
	}
	if _, err := s.cleanInvalidNodeUIDsTx(tx, []string{uid}); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		UPDATE subscriptions
		SET node_count = (SELECT COUNT(*) FROM nodes WHERE subscription_id = ?), updated_at = ?
		WHERE id = ?
	`, subscriptionID, time.Now().UnixMilli(), subscriptionID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) NodeFacets() (*model.NodeFacetsResponse, error) {
	resp := &model.NodeFacetsResponse{Types: []model.NodeFacetItem{}, Subscriptions: []model.NodeFacetItem{}}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM nodes`).Scan(&resp.Total); err != nil {
		return nil, err
	}
	typeRows, err := s.db.Query(`SELECT type, COUNT(*) FROM nodes GROUP BY type ORDER BY type ASC`)
	if err != nil {
		return nil, err
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var item model.NodeFacetItem
		if err := typeRows.Scan(&item.Value, &item.Count); err != nil {
			return nil, err
		}
		item.Label = item.Value
		resp.Types = append(resp.Types, item)
	}
	if err := typeRows.Err(); err != nil {
		return nil, err
	}
	subRows, err := s.db.Query(`
		SELECT CAST(n.subscription_id AS TEXT), COALESCE(s.name, CAST(n.subscription_id AS TEXT)), COUNT(*)
		FROM nodes n LEFT JOIN subscriptions s ON s.id = n.subscription_id
		GROUP BY n.subscription_id, s.name ORDER BY s.name ASC, n.subscription_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer subRows.Close()
	for subRows.Next() {
		var item model.NodeFacetItem
		if err := subRows.Scan(&item.Value, &item.Label, &item.Count); err != nil {
			return nil, err
		}
		resp.Subscriptions = append(resp.Subscriptions, item)
	}
	if err := subRows.Err(); err != nil {
		return nil, err
	}
	return resp, nil
}

func (s *Store) SetNodeEnabled(uid string, enabled bool) error {
	return s.setNodeBool(uid, "enabled", enabled)
}

func (s *Store) SetNodePreferred(uid string, preferred bool) error {
	return s.setNodeBool(uid, "preferred", preferred)
}

func (s *Store) setNodeBool(uid string, column string, value bool) error {
	if column != "enabled" && column != "preferred" {
		return fmt.Errorf("invalid node column")
	}
	now := time.Now().UnixMilli()
	boolValue := 0
	if value {
		boolValue = 1
	}
	res, err := s.db.Exec(`UPDATE nodes SET `+column+` = ?, updated_at = ? WHERE uid = ?`, boolValue, now, uid)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return fmt.Errorf("node not found")
	}
	return nil
}

type nodeScanner interface {
	Scan(dest ...any) error
}

func scanNode(scanner nodeScanner, item *model.Node) error {
	var enabled, preferred, testSuccess int
	var nameOverridden int
	if err := scanner.Scan(&item.ID, &item.UID, &item.SubscriptionID, &item.SubscriptionName, &item.Name, &nameOverridden, &item.Type, &item.Server, &item.ServerPort, &item.Raw, &item.RawJSON, &enabled, &preferred, &item.LatencyMS, &item.Status, &item.LastTestAt, &item.TestLatencyMS, &testSuccess, &item.CreatedAt, &item.UpdatedAt); err != nil {
		return err
	}
	item.Enabled = enabled != 0
	item.Preferred = preferred != 0
	item.NameOverridden = nameOverridden != 0
	item.TestSuccess = testSuccess != 0
	return nil
}

func buildNodeWhere(req model.NodeListRequest) (string, []any) {
	clauses := make([]string, 0)
	args := make([]any, 0)
	if req.SubscriptionID > 0 {
		clauses = append(clauses, "n.subscription_id = ?")
		args = append(args, req.SubscriptionID)
	}
	if req.Keyword != "" {
		clauses = append(clauses, "(n.name LIKE ? OR n.server LIKE ? OR n.type LIKE ? OR n.uid LIKE ?)")
		keyword := "%" + req.Keyword + "%"
		args = append(args, keyword, keyword, keyword, keyword)
	}
	if req.Type != "" {
		clauses = append(clauses, "n.type = ?")
		args = append(args, req.Type)
	}
	if req.Status != "" {
		clauses = append(clauses, "n.status = ?")
		args = append(args, req.Status)
	}
	if req.Enabled != nil {
		clauses = append(clauses, "n.enabled = ?")
		args = append(args, boolToInt(*req.Enabled))
	}
	if req.Preferred != nil {
		clauses = append(clauses, "n.preferred = ?")
		args = append(args, boolToInt(*req.Preferred))
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func StableNodeUID(node model.ParsedNode) string {
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.RawJSON), &cfg); err != nil || cfg == nil {
		cfg = map[string]any{
			"type":        node.Type,
			"server":      node.Server,
			"server_port": node.ServerPort,
		}
	}
	identity := nodeIdentityFields(cfg, node)
	encoded, _ := json.Marshal(identity)
	return shortNodeHash(string(encoded))
}

func nodeIdentityFields(cfg map[string]any, node model.ParsedNode) map[string]any {
	identity := map[string]any{
		"type":   firstNodeString(node.Type, nodeString(cfg, "type")),
		"server": firstNodeString(node.Server, nodeString(cfg, "server")),
		"port":   firstNodeInt(node.ServerPort, nodeInt(cfg, "server_port"), nodeInt(cfg, "port")),
	}

	copyNodeIdentityString(identity, cfg, "uuid", "id")
	copyNodeIdentityString(identity, cfg, "password")
	copyNodeIdentityString(identity, cfg, "cipher", "method", "security")
	copyNodeIdentityString(identity, cfg, "flow")
	copyNodeIdentityString(identity, cfg, "encryption")
	copyNodeIdentityString(identity, cfg, "username")
	copyNodeIdentityString(identity, cfg, "protocol")
	copyNodeIdentityString(identity, cfg, "obfs")
	copyNodeIdentityString(identity, cfg, "obfs-param")
	copyNodeIdentityString(identity, cfg, "psk")
	copyNodeIdentityString(identity, cfg, "private-key")
	copyNodeIdentityString(identity, cfg, "public-key")
	copyNodeIdentityString(identity, cfg, "preshared-key")
	copyNodeIdentityString(identity, cfg, "local-address", "address")
	copyNodeIdentityInt(identity, cfg, "mtu")

	if tls := nodeTLSIdentity(cfg); len(tls) > 0 {
		identity["tls"] = tls
	}
	if transport := nodeTransportIdentity(cfg); len(transport) > 0 {
		identity["transport"] = transport
	}
	if reality := nodeRealityIdentity(cfg); len(reality) > 0 {
		identity["reality"] = reality
	}

	return identity
}

func nodeTLSIdentity(cfg map[string]any) map[string]any {
	tls := map[string]any{}
	if enabled, ok := cfg["tls"].(bool); ok {
		tls["enabled"] = enabled
	}
	if tlsCfg, ok := cfg["tls"].(map[string]any); ok {
		if enabled, ok := tlsCfg["enabled"].(bool); ok {
			tls["enabled"] = enabled
		}
		if serverName := firstNodeString(nodeString(tlsCfg, "server_name"), nodeString(tlsCfg, "servername"), nodeString(tlsCfg, "sni")); serverName != "" {
			tls["server_name"] = serverName
		}
		if utlsCfg, ok := tlsCfg["utls"].(map[string]any); ok {
			if fingerprint := nodeString(utlsCfg, "fingerprint"); fingerprint != "" {
				tls["fingerprint"] = fingerprint
			}
		}
	}
	if serverName := firstNodeString(nodeString(cfg, "server_name"), nodeString(cfg, "servername"), nodeString(cfg, "sni")); serverName != "" {
		tls["server_name"] = serverName
	}
	if fingerprint := firstNodeString(nodeString(cfg, "client-fingerprint"), nodeString(cfg, "fingerprint"), nodeString(cfg, "fp")); fingerprint != "" {
		tls["fingerprint"] = fingerprint
	}
	if alpn, ok := cfg["alpn"]; ok {
		tls["alpn"] = alpn
	}
	return tls
}

func nodeTransportIdentity(cfg map[string]any) map[string]any {
	transport := map[string]any{}
	if transportCfg, ok := cfg["transport"].(map[string]any); ok {
		copyNodeIdentityString(transport, transportCfg, "type")
		copyNodeIdentityString(transport, transportCfg, "path")
		if headers, ok := transportCfg["headers"].(map[string]any); ok {
			if host := firstNodeString(nodeString(headers, "Host"), nodeString(headers, "host")); host != "" {
				transport["host"] = host
			}
		}
	}
	if network := nodeString(cfg, "network"); network != "" {
		transport["type"] = network
	}
	if wsOpts, ok := cfg["ws-opts"].(map[string]any); ok {
		transport["type"] = "ws"
		copyNodeIdentityString(transport, wsOpts, "path")
		if headers, ok := wsOpts["headers"].(map[string]any); ok {
			if host := firstNodeString(nodeString(headers, "Host"), nodeString(headers, "host")); host != "" {
				transport["host"] = host
			}
		}
	}
	if grpcOpts, ok := cfg["grpc-opts"].(map[string]any); ok {
		transport["type"] = "grpc"
		copyNodeIdentityString(transport, grpcOpts, "grpc-service-name", "serviceName", "service-name")
	}
	if h2Opts, ok := cfg["h2-opts"].(map[string]any); ok {
		transport["type"] = "h2"
		copyNodeIdentityString(transport, h2Opts, "path")
		if host, ok := h2Opts["host"]; ok {
			transport["host"] = host
		}
	}
	return transport
}

func nodeRealityIdentity(cfg map[string]any) map[string]any {
	reality := map[string]any{}
	if realityOpts, ok := cfg["reality-opts"].(map[string]any); ok {
		copyNodeIdentityString(reality, realityOpts, "public-key", "public_key", "pbk")
		copyNodeIdentityString(reality, realityOpts, "short-id", "short_id", "sid")
		copyNodeIdentityString(reality, realityOpts, "spider-x", "spider_x", "spx")
	}
	copyNodeIdentityString(reality, cfg, "public-key", "public_key", "pbk")
	copyNodeIdentityString(reality, cfg, "short-id", "short_id", "sid")
	copyNodeIdentityString(reality, cfg, "spider-x", "spider_x", "spx")
	return reality
}

func copyNodeIdentityString(dst map[string]any, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := nodeString(src, key); value != "" {
			dst[keys[0]] = value
			return
		}
	}
}

func copyNodeIdentityInt(dst map[string]any, src map[string]any, keys ...string) {
	for _, key := range keys {
		if value := nodeInt(src, key); value != 0 {
			dst[keys[0]] = value
			return
		}
	}
}

func nodeString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case float64:
		return strconv.FormatFloat(val, 'f', -1, 64)
	case int:
		return strconv.Itoa(val)
	default:
		return ""
	}
}

func nodeInt(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch val := v.(type) {
	case int:
		return val
	case float64:
		return int(val)
	case string:
		parsed, _ := strconv.Atoi(val)
		return parsed
	default:
		return 0
	}
}

func firstNodeString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func firstNodeInt(values ...int) int {
	for _, value := range values {
		if value != 0 {
			return value
		}
	}
	return 0
}

func shortNodeHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:16]
}

func shortNodeSuffix(n int) string {
	return hex.EncodeToString([]byte{byte(n >> 8), byte(n)})
}
