package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

var (
	ErrSSHCredentialNotFound = errors.New("SSH credential not found")
	ErrSSHHostNotFound       = errors.New("SSH host not found")
	ErrSSHReferenceInUse     = errors.New("SSH resource is still referenced")
)

const sshCredentialColumns = `id, name, auth_type, secret_context, secret_ciphertext, secret_nonce,
	passphrase_ciphertext, passphrase_nonce, key_fingerprint, key_version, created_at, updated_at`

func (s *Store) ListSSHCredentials() ([]model.SSHCredential, error) {
	rows, err := s.db.Query(`SELECT ` + sshCredentialColumns + ` FROM ssh_credentials ORDER BY name COLLATE NOCASE, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.SSHCredential, 0)
	for rows.Next() {
		item, err := scanSSHCredential(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetSSHCredential(id int64) (*model.SSHCredential, error) {
	item, err := scanSSHCredential(s.db.QueryRow(`SELECT `+sshCredentialColumns+` FROM ssh_credentials WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSSHCredentialNotFound
	}
	return item, err
}

func (s *Store) CreateSSHCredential(item *model.SSHCredential) error {
	now := time.Now().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	result, err := s.db.Exec(`INSERT INTO ssh_credentials
		(name, auth_type, secret_context, secret_ciphertext, secret_nonce, passphrase_ciphertext,
		 passphrase_nonce, key_fingerprint, key_version, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.Name, item.AuthType, item.SecretContext,
		item.SecretCiphertext, item.SecretNonce, nullableBytes(item.PassphraseCiphertext),
		nullableBytes(item.PassphraseNonce), item.KeyFingerprint, item.KeyVersion, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) UpdateSSHCredential(item *model.SSHCredential) error {
	item.UpdatedAt = time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE ssh_credentials SET name = ?, auth_type = ?, secret_context = ?,
		secret_ciphertext = ?, secret_nonce = ?, passphrase_ciphertext = ?, passphrase_nonce = ?,
		key_fingerprint = ?, key_version = ?, updated_at = ? WHERE id = ?`, item.Name, item.AuthType,
		item.SecretContext, item.SecretCiphertext, item.SecretNonce, nullableBytes(item.PassphraseCiphertext),
		nullableBytes(item.PassphraseNonce), item.KeyFingerprint, item.KeyVersion, item.UpdatedAt, item.ID)
	if err != nil {
		return err
	}
	return requireSSHRowsAffected(result, ErrSSHCredentialNotFound)
}

func (s *Store) DeleteSSHCredential(id int64) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM ssh_hosts WHERE credential_id = ?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: %d host(s)", ErrSSHReferenceInUse, count)
	}
	result, err := s.db.Exec(`DELETE FROM ssh_credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireSSHRowsAffected(result, ErrSSHCredentialNotFound)
}

const sshHostSelect = `SELECT h.id, h.name, h.group_name, h.host, h.port, h.username,
	h.credential_id, c.name, h.connection_mode, h.node_exposure_id, COALESCE(e.name, ''),
	h.terminal_type, h.enabled, h.tags_json, h.notes,
	CASE WHEN k.host_id IS NULL THEN 'unknown' ELSE 'trusted' END,
	h.last_status, h.last_latency_ms, h.last_error_code, h.last_error_message,
	h.last_checked_at, h.created_at, h.updated_at
	FROM ssh_hosts h
	JOIN ssh_credentials c ON c.id = h.credential_id
	LEFT JOIN node_exposures e ON e.id = h.node_exposure_id
	LEFT JOIN ssh_host_keys k ON k.host_id = h.id`

func (s *Store) ListSSHHosts() ([]model.SSHHost, error) {
	rows, err := s.db.Query(sshHostSelect + ` ORDER BY h.group_name COLLATE NOCASE, h.name COLLATE NOCASE, h.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]model.SSHHost, 0)
	for rows.Next() {
		item, err := scanSSHHost(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, *item)
	}
	return items, rows.Err()
}

func (s *Store) GetSSHHost(id int64) (*model.SSHHost, error) {
	item, err := scanSSHHost(s.db.QueryRow(sshHostSelect+` WHERE h.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSSHHostNotFound
	}
	return item, err
}

func (s *Store) CreateSSHHost(item *model.SSHHost) error {
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	item.CreatedAt, item.UpdatedAt = now, now
	result, err := s.db.Exec(`INSERT INTO ssh_hosts
		(name, group_name, host, port, username, credential_id, connection_mode, node_exposure_id,
		 terminal_type, enabled, tags_json, notes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, item.Name, item.GroupName, item.Host,
		item.Port, item.Username, item.CredentialID, item.ConnectionMode, item.NodeExposureID,
		item.TerminalType, boolToInt(item.Enabled), string(tags), item.Notes, now, now)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func (s *Store) CreateSSHHostWithCredential(credential *model.SSHCredential, host *model.SSHHost) error {
	return s.CreateSSHHostsWithCredentials([]*model.SSHCredential{credential}, []*model.SSHHost{host}, []int{0})
}

func (s *Store) CreateSSHHostsWithCredentials(credentials []*model.SSHCredential, hosts []*model.SSHHost, credentialIndexes []int) error {
	if len(credentials) == 0 || len(hosts) == 0 || len(hosts) != len(credentialIndexes) {
		return errors.New("invalid SSH host import batch")
	}
	tags := make([]string, len(hosts))
	for index, host := range hosts {
		if credentialIndexes[index] < 0 || credentialIndexes[index] >= len(credentials) {
			return errors.New("invalid SSH credential index")
		}
		encoded, err := json.Marshal(host.Tags)
		if err != nil {
			return err
		}
		tags[index] = string(encoded)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().UnixMilli()
	for _, credential := range credentials {
		credential.CreatedAt, credential.UpdatedAt = now, now
		result, err := tx.Exec(`INSERT INTO ssh_credentials
			(name, auth_type, secret_context, secret_ciphertext, secret_nonce, passphrase_ciphertext,
			 passphrase_nonce, key_fingerprint, key_version, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, credential.Name, credential.AuthType,
			credential.SecretContext, credential.SecretCiphertext, credential.SecretNonce,
			nullableBytes(credential.PassphraseCiphertext), nullableBytes(credential.PassphraseNonce),
			credential.KeyFingerprint, credential.KeyVersion, now, now)
		if err != nil {
			return err
		}
		credential.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}

	for index, host := range hosts {
		host.CredentialID = credentials[credentialIndexes[index]].ID
		host.CreatedAt, host.UpdatedAt = now, now
		result, err := tx.Exec(`INSERT INTO ssh_hosts
			(name, group_name, host, port, username, credential_id, connection_mode, node_exposure_id,
			 terminal_type, enabled, tags_json, notes, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, host.Name, host.GroupName, host.Host,
			host.Port, host.Username, host.CredentialID, host.ConnectionMode, host.NodeExposureID,
			host.TerminalType, boolToInt(host.Enabled), tags[index], host.Notes, now, now)
		if err != nil {
			return err
		}
		host.ID, err = result.LastInsertId()
		if err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) UpdateSSHHost(item *model.SSHHost) error {
	tags, err := json.Marshal(item.Tags)
	if err != nil {
		return err
	}
	item.UpdatedAt = time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE ssh_hosts SET name = ?, group_name = ?, host = ?, port = ?,
		username = ?, credential_id = ?, connection_mode = ?, node_exposure_id = ?, terminal_type = ?,
		enabled = ?, tags_json = ?, notes = ?, updated_at = ? WHERE id = ?`, item.Name, item.GroupName,
		item.Host, item.Port, item.Username, item.CredentialID, item.ConnectionMode, item.NodeExposureID,
		item.TerminalType, boolToInt(item.Enabled), string(tags), item.Notes, item.UpdatedAt, item.ID)
	if err != nil {
		return err
	}
	return requireSSHRowsAffected(result, ErrSSHHostNotFound)
}

func (s *Store) DeleteSSHHost(id int64) error {
	result, err := s.db.Exec(`DELETE FROM ssh_hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	return requireSSHRowsAffected(result, ErrSSHHostNotFound)
}

func (s *Store) DeleteSSHHostKey(hostID int64) error {
	_, err := s.db.Exec(`DELETE FROM ssh_host_keys WHERE host_id = ?`, hostID)
	return err
}

func (s *Store) GetSSHHostKey(hostID int64) (*model.SSHHostKey, error) {
	var item model.SSHHostKey
	err := s.db.QueryRow(`SELECT host_id, key_type, public_key, fingerprint_sha256,
		first_seen_at, last_seen_at, updated_at FROM ssh_host_keys WHERE host_id = ?`, hostID).Scan(
		&item.HostID, &item.KeyType, &item.PublicKey, &item.FingerprintSHA256,
		&item.FirstSeenAt, &item.LastSeenAt, &item.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &item, err
}

func (s *Store) UpsertSSHHostKey(item *model.SSHHostKey) error {
	now := time.Now().UnixMilli()
	if item.FirstSeenAt == 0 {
		item.FirstSeenAt = now
	}
	item.LastSeenAt, item.UpdatedAt = now, now
	_, err := s.db.Exec(`INSERT INTO ssh_host_keys
		(host_id, key_type, public_key, fingerprint_sha256, first_seen_at, last_seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(host_id) DO UPDATE SET key_type = excluded.key_type, public_key = excluded.public_key,
		fingerprint_sha256 = excluded.fingerprint_sha256, last_seen_at = excluded.last_seen_at,
		updated_at = excluded.updated_at`, item.HostID, item.KeyType, item.PublicKey,
		item.FingerprintSHA256, item.FirstSeenAt, item.LastSeenAt, item.UpdatedAt)
	return err
}

func (s *Store) TouchSSHHostKey(hostID int64) error {
	_, err := s.db.Exec(`UPDATE ssh_host_keys SET last_seen_at = ? WHERE host_id = ?`, time.Now().UnixMilli(), hostID)
	return err
}

func (s *Store) UpdateSSHHostTestResult(hostID int64, status string, latencyMS int64, errorCode, errorMessage string) error {
	now := time.Now().UnixMilli()
	result, err := s.db.Exec(`UPDATE ssh_hosts SET last_status = ?, last_latency_ms = ?,
		last_error_code = ?, last_error_message = ?, last_checked_at = ?, updated_at = ? WHERE id = ?`,
		status, latencyMS, errorCode, errorMessage, now, now, hostID)
	if err != nil {
		return err
	}
	return requireSSHRowsAffected(result, ErrSSHHostNotFound)
}

func (s *Store) CountSSHHostsByNodeExposure(id int64) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM ssh_hosts WHERE node_exposure_id = ?`, id).Scan(&count)
	return count, err
}

func (s *Store) CreateSSHSessionAudit(item *model.SSHSessionAudit) error {
	item.CreatedAt = time.Now().UnixMilli()
	result, err := s.db.Exec(`INSERT INTO ssh_session_audits
		(session_id_hash, host_id, host_name, event_type, connection_mode, node_exposure_id,
		 result, error_code, duration_ms, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.SessionIDHash, item.HostID, item.HostName, item.EventType, item.ConnectionMode,
		item.NodeExposureID, item.Result, item.ErrorCode, item.DurationMS, item.CreatedAt)
	if err != nil {
		return err
	}
	item.ID, err = result.LastInsertId()
	return err
}

func scanSSHCredential(scanner interface{ Scan(...any) error }) (*model.SSHCredential, error) {
	var item model.SSHCredential
	var passphraseCiphertext, passphraseNonce []byte
	err := scanner.Scan(&item.ID, &item.Name, &item.AuthType, &item.SecretContext,
		&item.SecretCiphertext, &item.SecretNonce, &passphraseCiphertext, &passphraseNonce,
		&item.KeyFingerprint, &item.KeyVersion, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.PassphraseCiphertext, item.PassphraseNonce = passphraseCiphertext, passphraseNonce
	item.HasSecret = len(item.SecretCiphertext) > 0
	return &item, nil
}

func scanSSHHost(scanner interface{ Scan(...any) error }) (*model.SSHHost, error) {
	var item model.SSHHost
	var enabled int
	var nodeExposureID sql.NullInt64
	var tagsJSON string
	err := scanner.Scan(&item.ID, &item.Name, &item.GroupName, &item.Host, &item.Port, &item.Username,
		&item.CredentialID, &item.CredentialName, &item.ConnectionMode, &nodeExposureID,
		&item.NodeExposureName, &item.TerminalType, &enabled, &tagsJSON, &item.Notes,
		&item.HostKeyStatus, &item.LastStatus, &item.LastLatencyMS, &item.LastErrorCode,
		&item.LastErrorMessage, &item.LastCheckedAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return nil, err
	}
	item.Enabled = enabled != 0
	if nodeExposureID.Valid {
		item.NodeExposureID = &nodeExposureID.Int64
	}
	if err := json.Unmarshal([]byte(tagsJSON), &item.Tags); err != nil {
		return nil, err
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	return &item, nil
}

func nullableBytes(value []byte) any {
	if len(value) == 0 {
		return nil
	}
	return value
}

func requireSSHRowsAffected(result sql.Result, notFound error) error {
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return notFound
	}
	return nil
}
