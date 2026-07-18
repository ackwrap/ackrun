package service

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadMixedInboundPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := []byte(`{"inbounds":[{"type":"tun"},{"type":"mixed","listen_port":8888}]}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	if got := readMixedInboundPort(path); got != 8888 {
		t.Fatalf("readMixedInboundPort() = %d, want 8888", got)
	}
}

func TestRequestClashAPIClosesConnectionsWithAuthentication(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-secret" {
			t.Errorf("authorization = %q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := requestClashAPI(http.MethodDelete, server.URL+"/connections", "test-secret"); err != nil {
		t.Fatal(err)
	}
}

func TestRequestClashAPIRejectsFailureStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	if err := requestClashAPI(http.MethodDelete, server.URL+"/connections", "bad-secret"); err == nil {
		t.Fatal("requestClashAPI() error = nil, want HTTP status error")
	}
}

func TestCleanLogLineRemovesANSIColorCodes(t *testing.T) {
	got := cleanLogLine("\x1b[31mFATAL\x1b[0m TLS required")
	if got != "FATAL TLS required" {
		t.Fatalf("cleanLogLine() = %q", got)
	}
}
