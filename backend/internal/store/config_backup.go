package store

import "github.com/ackwrap/ackwrap/internal/model"

func (s *Store) ReplaceConfigBackups(backups []model.ConfigBackup) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`DELETE FROM config_backups`); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO config_backups (config_name, file_name, path, backup_date, size_bytes, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, backup := range backups {
		if _, err := stmt.Exec(backup.ConfigName, backup.FileName, backup.Path, backup.BackupDate, backup.SizeBytes, backup.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListConfigBackups() ([]model.ConfigBackup, error) {
	rows, err := s.db.Query(`
		SELECT id, config_name, file_name, path, backup_date, size_bytes, created_at
		FROM config_backups
		ORDER BY backup_date DESC, created_at DESC, config_name ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	backups := make([]model.ConfigBackup, 0)
	for rows.Next() {
		var backup model.ConfigBackup
		if err := rows.Scan(&backup.ID, &backup.ConfigName, &backup.FileName, &backup.Path, &backup.BackupDate, &backup.SizeBytes, &backup.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, backup)
	}
	return backups, rows.Err()
}
