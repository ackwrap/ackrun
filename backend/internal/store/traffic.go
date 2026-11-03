package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

type TrafficSample struct {
	Session       string
	StartedAt     int64
	UploadTotal   int64
	DownloadTotal int64
}

func (s *Store) EnsureTrafficMonth(now time.Time) error {
	_, err := s.db.Exec(`INSERT INTO monthly_traffic (month, tracking_since) VALUES (?, ?) ON CONFLICT(month) DO NOTHING`, now.Format("2006-01"), now.Unix())
	return err
}

func (s *Store) GetMonthlyTraffic(now time.Time) (model.MonthlyTraffic, error) {
	result := model.MonthlyTraffic{Month: now.Format("2006-01")}
	err := s.db.QueryRow(`SELECT upload_bytes, download_bytes, tracking_since FROM monthly_traffic WHERE month = ?`, result.Month).
		Scan(&result.UploadBytes, &result.DownloadBytes, &result.TrackingSince)
	if errors.Is(err, sql.ErrNoRows) {
		return result, nil
	}
	return result, err
}

// RecordTrafficSample commits the counter delta and its checkpoint atomically.
// A newly observed existing core is baselined, so its lifetime total is never
// presented as this month's usage. The first sample across a month boundary is
// also baselined: bytes from an unsampled interval cannot be dated accurately.
func (s *Store) RecordTrafficSample(sample TrafficSample, now time.Time) error {
	if sample.Session == "" || sample.UploadTotal < 0 || sample.DownloadTotal < 0 {
		return fmt.Errorf("invalid traffic counters")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	month := now.Format("2006-01")
	if _, err = tx.Exec(`INSERT INTO monthly_traffic (month, tracking_since) VALUES (?, ?) ON CONFLICT(month) DO NOTHING`, month, now.Unix()); err != nil {
		return err
	}
	var since int64
	if err = tx.QueryRow(`SELECT tracking_since FROM monthly_traffic WHERE month = ?`, month).Scan(&since); err != nil {
		return err
	}
	var previous TrafficSample
	var previousMonth string
	err = tx.QueryRow(`SELECT month, session, upload_total, download_total FROM traffic_checkpoint WHERE id = 1`).
		Scan(&previousMonth, &previous.Session, &previous.UploadTotal, &previous.DownloadTotal)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var upload, download int64
	if previous.Session == sample.Session && previousMonth == month {
		upload = counterDelta(sample.UploadTotal, previous.UploadTotal)
		download = counterDelta(sample.DownloadTotal, previous.DownloadTotal)
	} else if sample.StartedAt >= since {
		// A core launched after recording began has no historical bytes to exclude.
		upload, download = sample.UploadTotal, sample.DownloadTotal
	}
	if _, err = tx.Exec(`UPDATE monthly_traffic SET upload_bytes = upload_bytes + ?, download_bytes = download_bytes + ? WHERE month = ?`, upload, download, month); err != nil {
		return err
	}
	if _, err = tx.Exec(`INSERT INTO traffic_checkpoint (id, month, session, upload_total, download_total) VALUES (1, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET month = excluded.month, session = excluded.session, upload_total = excluded.upload_total, download_total = excluded.download_total`, month, sample.Session, sample.UploadTotal, sample.DownloadTotal); err != nil {
		return err
	}
	return tx.Commit()
}

func counterDelta(current, previous int64) int64 {
	if current < previous {
		return current // The core reset its counter within the same process.
	}
	return current - previous
}
