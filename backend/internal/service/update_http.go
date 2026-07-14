package service

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ackwrap/ackwrap/internal/model"
)

type updateRequestAttempt struct {
	name   string
	url    string
	client *http.Client
}

func buildUpdateRequestAttempts(settings *model.UpdateSettingsResponse, rawURL string) ([]updateRequestAttempt, error) {
	direct := &http.Client{Timeout: 30 * time.Second, Transport: directHTTPTransport()}
	if settings == nil {
		return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
	}
	switch settings.Acceleration {
	case "proxy":
		proxyURL := strings.TrimSpace(settings.ProxyURL)
		if proxyURL == "" {
			proxyURL = "http://127.0.0.1:2080"
		}
		parsed, err := url.Parse(proxyURL)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("本地代理 URL 无效，请到设置页面检查")
		}
		transport := directHTTPTransport()
		transport.Proxy = http.ProxyURL(parsed)
		return []updateRequestAttempt{
			{name: "local_proxy", url: rawURL, client: &http.Client{Timeout: 30 * time.Second, Transport: transport}},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	case "ghproxy":
		return []updateRequestAttempt{
			{name: "ghproxy", url: "https://gh-proxy.com/" + rawURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	case "custom":
		mirror := strings.TrimRight(strings.TrimSpace(settings.CustomMirrorURL), "/")
		if mirror == "" {
			return nil, fmt.Errorf("自定义镜像 URL 为空，请到设置页面检查")
		}
		return []updateRequestAttempt{
			{name: "custom_mirror", url: mirror + "/" + rawURL, client: direct},
			{name: "direct_fallback", url: rawURL, client: direct},
		}, nil
	default:
		return []updateRequestAttempt{{name: "direct", url: rawURL, client: direct}}, nil
	}
}

func directHTTPTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return transport
}

func fetchLatestSingboxVersionWithSettings(settings *model.UpdateSettingsResponse, apiURL string) (string, error) {
	attempts, err := buildUpdateRequestAttempts(settings, apiURL)
	if err != nil {
		return "", err
	}
	token := ""
	if settings != nil {
		token = settings.GithubToken
	}
	var lastErr error
	for _, attempt := range attempts {
		version, err := fetchLatestSingboxVersion(attempt.client, attempt.url, token)
		if err == nil {
			return version, nil
		}
		lastErr = err
	}
	return "", lastErr
}
