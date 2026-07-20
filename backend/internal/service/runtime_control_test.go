package service

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestRequestClashAPIReportsEmptyFakeIPCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"bucket not found"}`))
	}))
	defer server.Close()

	err := requestClashAPI(http.MethodPost, server.URL+"/cache/fakeip/flush", "")
	var responseErr *clashAPIResponseError
	if !errors.As(err, &responseErr) {
		t.Fatalf("requestClashAPI() error = %v, want clashAPIResponseError", err)
	}
	if !isEmptyFakeIPCacheError(err) {
		t.Fatalf("isEmptyFakeIPCacheError() = false, want true for %v", err)
	}
}

func TestCleanLogLineRemovesANSIColorCodes(t *testing.T) {
	got := cleanLogLine("\x1b[31mFATAL\x1b[0m TLS required")
	if got != "FATAL TLS required" {
		t.Fatalf("cleanLogLine() = %q", got)
	}
}

func TestCleanLogLineRedactsAPIAccessToken(t *testing.T) {
	got := cleanLogLine(`FATAL fetch http://127.0.0.1:8080/api/v1/rules/content?access_token=secret-value&v=1 failed`)
	if strings.Contains(got, "secret-value") || !strings.Contains(got, "access_token=[REDACTED]") {
		t.Fatalf("cleanLogLine() did not redact token: %q", got)
	}
}
