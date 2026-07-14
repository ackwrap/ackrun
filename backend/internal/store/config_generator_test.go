package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestConfigGenerateRequestRoundTrip(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	request := &model.ConfigGenerateRequest{
		DefaultOutbound: "proxy",
		InboundListen:   "127.0.0.1",
		InboundPort:     2080,
		LogLevel:        "warn",
	}
	if err := db.SetConfigGenerateRequest(request); err != nil {
		t.Fatalf("set request: %v", err)
	}
	got, err := db.GetConfigGenerateRequest()
	if err != nil {
		t.Fatalf("get request: %v", err)
	}
	if got == nil || *got != *request {
		t.Fatalf("request = %#v, want %#v", got, request)
	}
}
