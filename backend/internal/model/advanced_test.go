package model

import (
	"testing"
	"time"
)

func TestSessionLeaseStatusAt(t *testing.T) {
	now := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	lease := SessionLease{Enabled: true, ExpiresAt: now.Add(time.Minute).UnixMilli()}
	if status := lease.StatusAt(now); status != "active" {
		t.Fatalf("future lease status = %q", status)
	}
	lease.ExpiresAt = now.UnixMilli()
	if status := lease.StatusAt(now); status != "expired" {
		t.Fatalf("expired lease status = %q", status)
	}
	lease.ExpiresAt = 0
	if status := lease.StatusAt(now); status != "expired" {
		t.Fatalf("zero-expiry lease status = %q", status)
	}
	lease.Enabled = false
	if status := lease.StatusAt(now); status != "disabled" {
		t.Fatalf("disabled lease status = %q", status)
	}
}
