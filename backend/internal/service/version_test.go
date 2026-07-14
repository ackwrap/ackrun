package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestFetchLatestSingboxVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected authorization header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.13.14"}`))
	}))
	defer server.Close()
	version, err := fetchLatestSingboxVersion(server.Client(), server.URL, "test-token")
	if err != nil {
		t.Fatalf("fetch latest version: %v", err)
	}
	if version != "1.13.14" {
		t.Fatalf("version = %q, want 1.13.14", version)
	}
}

func TestFetchLatestSingboxVersionRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"latest"}`))
	}))
	defer server.Close()
	if _, err := fetchLatestSingboxVersion(server.Client(), server.URL, ""); err == nil {
		t.Fatal("expected invalid version error")
	}
}

func TestFetchLatestSingboxVersionUsesLocalProxyFirst(t *testing.T) {
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Host != "github.test" {
			t.Errorf("unexpected proxy target: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"tag_name":"v1.13.14"}`))
	}))
	defer proxy.Close()
	settings := &model.UpdateSettingsResponse{Acceleration: "proxy", ProxyURL: proxy.URL}
	version, err := fetchLatestSingboxVersionWithSettings(settings, "http://github.test/releases/latest")
	if err != nil {
		t.Fatalf("fetch through proxy: %v", err)
	}
	if version != "1.13.14" {
		t.Fatalf("version = %q, want 1.13.14", version)
	}
}

func TestFetchLatestSingboxVersionReturnsFriendlyRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := fetchLatestSingboxVersion(server.Client(), server.URL, "")
	if err == nil || !strings.Contains(err.Error(), "GitHub Token") || !strings.Contains(err.Error(), "本地代理") {
		t.Fatalf("unexpected rate limit error: %v", err)
	}
}

func TestBuildUpdateRequestAttemptsUsesMirrorAndFallback(t *testing.T) {
	attempts, err := buildUpdateRequestAttempts(&model.UpdateSettingsResponse{Acceleration: "custom", CustomMirrorURL: "https://mirror.example/"}, "https://github.com/ackwrap/release.zip")
	if err != nil {
		t.Fatalf("build attempts: %v", err)
	}
	if len(attempts) != 2 || attempts[0].url != "https://mirror.example/https://github.com/ackwrap/release.zip" || attempts[1].url != "https://github.com/ackwrap/release.zip" {
		t.Fatalf("unexpected attempts: %+v", attempts)
	}
}

func TestCompareSingboxVersions(t *testing.T) {
	tests := []struct {
		left  string
		right string
		want  int
	}{
		{left: "1.13.13", right: "1.13.14", want: -1},
		{left: "1.13.14", right: "1.13.14", want: 0},
		{left: "1.14.0", right: "1.13.14", want: 1},
		{left: "2.0.0", right: "1.99.99", want: 1},
		{left: "1.13.14-beta.1", right: "1.13.14", want: -1},
	}
	for _, test := range tests {
		if got := compareSingboxVersions(test.left, test.right); got != test.want {
			t.Errorf("compareSingboxVersions(%q, %q) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}
