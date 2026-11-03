package store

import (
	"path/filepath"
	"testing"
	"time"
)

func TestMonthlyTrafficPersistsAcrossBackendAndCoreRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "traffic.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("test", 8*3600))
	if err := db.EnsureTrafficMonth(now); err != nil {
		t.Fatal(err)
	}
	write := func(db *Store, sample TrafficSample, at time.Time) {
		t.Helper()
		if err := db.RecordTrafficSample(sample, at); err != nil {
			t.Fatal(err)
		}
	}
	// Existing lifetime counters are excluded when recording is first enabled.
	sample := TrafficSample{Session: "core-one", StartedAt: now.Add(-time.Hour).Unix(), UploadTotal: 1000, DownloadTotal: 2000}
	write(db, sample, now)
	sample.UploadTotal, sample.DownloadTotal = 1100, 2200
	write(db, sample, now.Add(5*time.Second))
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	sample.UploadTotal, sample.DownloadTotal = 1150, 2300
	write(db, sample, now.Add(10*time.Second))
	// The first bytes of a new core session count without resetting the month.
	write(db, TrafficSample{Session: "core-two", StartedAt: now.Add(12 * time.Second).Unix(), UploadTotal: 20, DownloadTotal: 40}, now.Add(15*time.Second))
	got, err := db.GetMonthlyTraffic(now)
	if err != nil || got.UploadBytes != 170 || got.DownloadBytes != 340 || got.TrackingSince != now.Unix() {
		t.Fatalf("persisted usage = %+v, error = %v", got, err)
	}
}

func TestMonthlyTrafficRolloverResetAndInvalidSamples(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	now := time.Date(2026, 11, 30, 23, 59, 50, 0, time.FixedZone("test", 8*3600))
	if err := db.EnsureTrafficMonth(now); err != nil {
		t.Fatal(err)
	}
	sample := TrafficSample{Session: "core", StartedAt: now.Unix(), UploadTotal: 100, DownloadTotal: 200}
	if err := db.RecordTrafficSample(sample, now); err != nil {
		t.Fatal(err)
	}
	// A counter reset must add positive new bytes, never subtract recorded usage.
	sample.UploadTotal, sample.DownloadTotal = 10, 20
	if err := db.RecordTrafficSample(sample, now.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	next := now.Add(15 * time.Second)
	sample.UploadTotal, sample.DownloadTotal = 500, 600
	if err := db.RecordTrafficSample(sample, next); err != nil {
		t.Fatal(err)
	}
	sample.UploadTotal, sample.DownloadTotal = 530, 640
	if err := db.RecordTrafficSample(sample, next.Add(5*time.Second)); err != nil {
		t.Fatal(err)
	}
	sample.UploadTotal = -1
	if err := db.RecordTrafficSample(sample, next.Add(10*time.Second)); err == nil {
		t.Fatal("negative traffic was accepted")
	}
	previous, err := db.GetMonthlyTraffic(now)
	if err != nil || previous.UploadBytes != 110 || previous.DownloadBytes != 220 {
		t.Fatalf("previous month = %+v, error = %v", previous, err)
	}
	got, err := db.GetMonthlyTraffic(next)
	if err != nil || got.Month != "2026-12" || got.UploadBytes != 30 || got.DownloadBytes != 40 {
		t.Fatalf("new month = %+v, error = %v", got, err)
	}
}
