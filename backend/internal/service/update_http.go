package service

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

type updateRequestAttempt struct {
	name   string
	url    string
	client *http.Client
}

var jsDelivrAccelerationBases = map[string]string{
	"jsdelivr_fastly":    "https://fastly.jsdelivr.net",
	"jsdelivr_testingcf": "https://testingcf.jsdelivr.net",
	"jsdelivr_cdn":       "https://cdn.jsdelivr.net",
}

func buildUpdateRequestAttempts(settings *model.UpdateSettingsResponse, rawURL string) ([]updateRequestAttempt, error) {
	direct := &http.Client{Timeout: 30 * time.Second, Transport: directHTTPTransport()}
	if settings == nil {
		return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
	}
	switch settings.Acceleration {
	case "ghproxy":
		return []updateRequestAttempt{
			{name: "ghproxy", url: "https://gh-proxy.com/" + rawURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	case "ghproxy_vip":
		return []updateRequestAttempt{
			{name: "ghproxy_vip", url: "https://ghproxy.vip/" + rawURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	case "jsdelivr_fastly", "jsdelivr_testingcf", "jsdelivr_cdn":
		acceleratedURL, ok := githubFileToJSDelivrURL(jsDelivrAccelerationBases[settings.Acceleration], rawURL)
		if !ok {
			return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
		}
		return []updateRequestAttempt{
			{name: settings.Acceleration, url: acceleratedURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	case "custom":
		mirror := strings.TrimRight(strings.TrimSpace(settings.CustomMirrorURL), "/")
		if mirror == "" {
			return nil, fmt.Errorf("自定义镜像 URL 为空，请到设置页面检查")
		}
		if isJSDelivrBase(mirror) {
			acceleratedURL, ok := githubFileToJSDelivrURL(mirror, rawURL)
			if !ok {
				return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
			}
			return []updateRequestAttempt{
				{name: "custom_jsdelivr", url: acceleratedURL, client: direct},
				{name: "direct_fallback", url: rawURL, client: direct},
			}, nil
		}
		return []updateRequestAttempt{
			{name: "custom_mirror", url: mirror + "/" + rawURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	default:
		return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
	}
}

// buildGitHubDownloadAttempts keeps the official URL as the canonical source
// while trying the configured accelerator and the remaining built-in mirrors.
func buildGitHubDownloadAttempts(settings *model.UpdateSettingsResponse, rawURL string) []updateRequestAttempt {
	direct := &http.Client{Timeout: generatedGeoRuleSetAttemptTimeout, Transport: directHTTPTransport()}
	attempts := make([]updateRequestAttempt, 0, 8)
	seen := make(map[string]bool)
	appendAttempt := func(attempt updateRequestAttempt) {
		key := attempt.url
		if attempt.url == "" || seen[key] {
			return
		}
		seen[key] = true
		attempts = append(attempts, attempt)
	}

	isGitHubURL := isGitHubFileURL(rawURL)
	if isGitHubURL || (settings != nil && settings.Acceleration == "custom") {
		if preferred, err := buildUpdateRequestAttempts(settings, rawURL); err == nil {
			for _, attempt := range preferred {
				if attempt.url == rawURL {
					continue
				}
				client := *attempt.client
				client.Timeout = generatedGeoRuleSetAttemptTimeout
				attempt.client = &client
				appendAttempt(attempt)
			}
		}
	}
	if isGitHubURL {
		appendAttempt(updateRequestAttempt{name: "ghproxy", url: "https://gh-proxy.com/" + rawURL, client: direct})
		appendAttempt(updateRequestAttempt{name: "ghproxy_vip", url: "https://ghproxy.vip/" + rawURL, client: direct})
		for _, name := range []string{"jsdelivr_fastly", "jsdelivr_testingcf", "jsdelivr_cdn"} {
			if acceleratedURL, ok := githubFileToJSDelivrURL(jsDelivrAccelerationBases[name], rawURL); ok {
				appendAttempt(updateRequestAttempt{name: name, url: acceleratedURL, client: direct})
			}
		}
	}
	appendAttempt(updateRequestAttempt{name: "official_direct", url: rawURL, client: direct})
	return attempts
}

func isGitHubFileURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	switch strings.ToLower(parsed.Hostname()) {
	case "raw.githubusercontent.com", "github.com", "api.github.com":
		return true
	default:
		return false
	}
}

func isJSDelivrBase(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "fastly.jsdelivr.net" || host == "testingcf.jsdelivr.net" || host == "cdn.jsdelivr.net"
}

func githubFileToJSDelivrURL(baseURL, rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	parts := strings.Split(strings.Trim(parsed.EscapedPath(), "/"), "/")
	var owner, repo, ref string
	var fileParts []string
	switch strings.ToLower(parsed.Hostname()) {
	case "raw.githubusercontent.com":
		if len(parts) < 4 {
			return "", false
		}
		owner, repo, ref, fileParts = parts[0], parts[1], parts[2], parts[3:]
	case "github.com":
		if len(parts) < 5 || (parts[2] != "raw" && parts[2] != "blob") {
			return "", false
		}
		owner, repo, ref, fileParts = parts[0], parts[1], parts[3], parts[4:]
	default:
		return "", false
	}
	if owner == "" || repo == "" || ref == "" || len(fileParts) == 0 {
		return "", false
	}
	return strings.TrimRight(baseURL, "/") + "/gh/" + owner + "/" + repo + "@" + ref + "/" + strings.Join(fileParts, "/"), true
}

func directHTTPTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

func fetchLatestSingboxVersionWithSettings(settings *model.UpdateSettingsResponse, apiURL string) (string, error) {
	release, err := fetchLatestSingboxReleaseWithSettings(settings, apiURL)
	if err != nil {
		return "", err
	}
	return release.Version, nil
}

func fetchLatestSingboxReleaseWithSettings(settings *model.UpdateSettingsResponse, apiURL string) (*singboxRelease, error) {
	attempts, err := buildUpdateRequestAttempts(settings, apiURL)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for _, attempt := range attempts {
		release, err := fetchLatestSingboxRelease(attempt.client, attempt.url)
		if err == nil {
			return release, nil
		}
		lastErr = err
	}
	return nil, lastErr
}
