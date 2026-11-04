package store

import (
	"database/sql"
	"errors"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) GetSSHSFTPProfile(hostID int64) (*model.SSHSFTPProfile, error) {
	var p model.SSHSFTPProfile
	err := s.db.QueryRow(`SELECT p.host_id, p.fingerprint, p.username, p.generation,
		p.packet_size, p.status, p.probed_at FROM ssh_sftp_profiles p
		JOIN ssh_host_keys k ON k.host_id = p.host_id AND k.fingerprint_sha256 = p.fingerprint
		JOIN ssh_hosts h ON h.id = p.host_id AND h.username = p.username
		WHERE p.host_id = ?`, hostID).Scan(&p.HostID, &p.Fingerprint, &p.Username,
		&p.Generation, &p.PacketSize, &p.Status, &p.ProbedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &p, err
}

// BeginSSHSFTPProbe atomically switches to the compatible default. An empty
// previous generation only inserts missing profiles; retries use compare-and-swap.
func (s *Store) BeginSSHSFTPProbe(hostID int64, generation, previous string) (bool, error) {
	r, err := s.db.Exec(`INSERT INTO ssh_sftp_profiles
		(host_id, fingerprint, username, generation, packet_size, status, probed_at)
		SELECT h.id, k.fingerprint_sha256, h.username, ?, 32768, 'probing', 0
		FROM ssh_hosts h JOIN ssh_host_keys k ON k.host_id = h.id WHERE h.id = ? AND h.enabled = 1
		ON CONFLICT(host_id) DO UPDATE SET fingerprint = excluded.fingerprint,
		username = excluded.username, generation = excluded.generation, packet_size = 32768,
		status = 'probing', probed_at = 0 WHERE ssh_sftp_profiles.generation = ?`, generation, hostID, previous)
	if err != nil {
		return false, err
	}
	n, err := r.RowsAffected()
	return n != 0, err
}

func (s *Store) FinishSSHSFTPProbe(hostID int64, generation string, packetSize int, status string) error {
	_, err := s.db.Exec(`UPDATE ssh_sftp_profiles SET packet_size = ?, status = ?, probed_at = ?
		WHERE host_id = ? AND generation = ? AND EXISTS (
		SELECT 1 FROM ssh_host_keys k JOIN ssh_hosts h ON h.id = k.host_id
		WHERE k.host_id = ssh_sftp_profiles.host_id AND k.fingerprint_sha256 = ssh_sftp_profiles.fingerprint
		AND h.username = ssh_sftp_profiles.username)`, packetSize, status, time.Now().UnixMilli(), hostID, generation)
	return err
}

func (s *Store) DeleteSSHSFTPProfile(hostID int64) error {
	_, err := s.db.Exec(`DELETE FROM ssh_sftp_profiles WHERE host_id = ?`, hostID)
	return err
}
