package store

import (
	"database/sql"
	"strconv"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func (s *Store) GetInstallState() (*model.InstallStateResponse, error) {
	var r model.InstallStateResponse
	var version, message, errorMsg sql.NullString
	var progress sql.NullString
	var updatedAt sql.NullInt64
	err := s.db.QueryRow(`
		SELECT status, version, message, error, progress, updated_at
		FROM install_state WHERE id = 1
	`).Scan(&r.Status, &version, &message, &errorMsg, &progress, &updatedAt)
	if err == sql.ErrNoRows {
		return &model.InstallStateResponse{Status: model.InstallIdle}, nil
	}
	if err != nil {
		return nil, err
	}
	if version.Valid {
		r.Version = version.String
	}
	if message.Valid {
		r.Message = message.String
	}
	if errorMsg.Valid {
		r.Error = errorMsg.String
	}
	if progress.Valid {
		if v, err := strconv.ParseFloat(progress.String, 64); err == nil {
			r.Progress = v
		}
	}
	return &r, nil
}

func (s *Store) SetInstallState(r *model.InstallStateResponse) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
		INSERT INTO install_state (id, status, version, message, error, progress, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			version = excluded.version,
			message = excluded.message,
			error = excluded.error,
			progress = excluded.progress,
			updated_at = excluded.updated_at
	`, r.Status, r.Version, r.Message, r.Error, r.Progress, now)
	return err
}
