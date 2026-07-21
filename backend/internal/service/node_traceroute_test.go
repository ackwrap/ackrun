package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/store"
)

func TestTracerouteRejectsUnknownNode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	_, err = NewNodeService(db).StartTraceroute("missing", "12345678", "ipapi.is")
	if !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestTracerouteRejectsInvalidTraceID(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	_, err = NewNodeService(db).StartTraceroute("missing", "bad", "ipapi.is")
	if !errors.Is(err, ErrTracerouteInvalid) {
		t.Fatalf("expected ErrTracerouteInvalid, got %v", err)
	}
}

func TestTracerouteRejectsInvalidGeoProvider(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	_, err = NewNodeService(db).StartTraceroute("missing", "12345678", "unknown")
	if !errors.Is(err, ErrTracerouteInvalid) {
		t.Fatalf("expected ErrTracerouteInvalid, got %v", err)
	}
}

func TestCancelTraceroute(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	svc := NewNodeService(db)
	ctx, cancel := context.WithCancel(context.Background())
	svc.traces["12345678"] = nodeTracerouteTask{uid: "node-uid", cancel: cancel}
	resp, err := svc.CancelTraceroute("node-uid", "12345678")
	if err != nil || resp == nil || !resp.Success {
		t.Fatalf("cancel traceroute: resp=%+v err=%v", resp, err)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("traceroute context was not canceled")
	}
}
