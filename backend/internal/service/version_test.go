package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestFetchLatestSingboxVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Errorf("unexpected authorization header: %q", authorization)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.13.14"}`))
	}))
	defer server.Close()
	version, err := fetchLatestSingboxVersion(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetch latest version: %v", err)
	}
	if version != "1.13.14" {
		t.Fatalf("version = %q, want 1.13.14", version)
	}
}

func TestFetchLatestSingboxReleaseIncludesAssetDigest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"tag_name":"v1.13.14",
			"assets":[{
				"name":"sing-wrap-1.13.14-windows-amd64.zip",
				"digest":"sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				"size":123
			}]
		}`))
	}))
	defer server.Close()

	release, err := fetchLatestSingboxRelease(server.Client(), server.URL)
	if err != nil {
		t.Fatalf("fetch latest release: %v", err)
	}
	asset, ok := releaseAssetByName(release, "sing-wrap-1.13.14-windows-amd64.zip")
	if !ok {
		t.Fatal("expected release asset")
	}
	if asset.Digest != "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" || asset.Size != 123 {
		t.Fatalf("unexpected release asset: %+v", asset)
	}
}

func TestFetchLatestSingboxVersionRejectsInvalidResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"latest"}`))
	}))
	defer server.Close()
	if _, err := fetchLatestSingboxVersion(server.Client(), server.URL); err == nil {
		t.Fatal("expected invalid version error")
	}
}

func TestBuildUpdateRequestAttemptsDefaultsToDirect(t *testing.T) {
	attempts, err := buildUpdateRequestAttempts(&model.UpdateSettingsResponse{}, "https://api.github.com/releases/latest")
	if err != nil {
		t.Fatal(err)
	}
	if len(attempts) != 1 || attempts[0].name != "direct" || attempts[0].url != "https://api.github.com/releases/latest" {
		t.Fatalf("default update attempts = %+v", attempts)
	}
	transport, ok := attempts[0].client.Transport.(*http.Transport)
	if !ok || transport.Proxy != nil {
		t.Fatalf("default update transport must bypass environment proxies: %#v", attempts[0].client.Transport)
	}
}

func TestFetchLatestSingboxVersionReturnsFriendlyRateLimitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()
	_, err := fetchLatestSingboxVersion(server.Client(), server.URL)
	if err == nil || !strings.Contains(err.Error(), "匿名请求次数已用完") {
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

func TestBuildUpdateRequestAttemptsUsesGHProxyVIPAndFallback(t *testing.T) {
	upstream := "https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/geosite-google.srs"
	attempts, err := buildUpdateRequestAttempts(&model.UpdateSettingsResponse{Acceleration: "ghproxy_vip"}, upstream)
	if err != nil {
		t.Fatalf("build attempts: %v", err)
	}
	if len(attempts) != 2 || attempts[0].url != "https://ghproxy.vip/"+upstream || attempts[1].url != upstream {
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
