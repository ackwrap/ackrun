package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var singboxVersionPattern = regexp.MustCompile(`v?(\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?)`)
var exactSingboxVersionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:[-+][A-Za-z0-9.-]+)?$`)

type singboxRelease struct {
	Version string
	Assets  []singboxReleaseAsset
}

type singboxReleaseAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

func isSingboxVersion(s string) bool {
	return exactSingboxVersionPattern.MatchString(s)
}

func readSingboxVersion(binaryPath string) string {
	output, err := exec.Command(binaryPath, "version").CombinedOutput()
	if err != nil {
		return ""
	}

	match := singboxVersionPattern.FindStringSubmatch(string(output))
	if len(match) < 2 {
		return ""
	}
	return match[1]
}

func fetchLatestSingboxVersion(client *http.Client, apiURL string) (string, error) {
	release, err := fetchLatestSingboxRelease(client, apiURL)
	if err != nil {
		return "", err
	}
	return release.Version, nil
}

func fetchLatestSingboxRelease(client *http.Client, apiURL string) (*singboxRelease, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("User-Agent", "Ackwrap/1.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request release API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return nil, fmt.Errorf("GitHub Release API 拒绝未认证请求")
		case http.StatusForbidden:
			if resp.Header.Get("X-RateLimit-Remaining") == "0" {
				return nil, fmt.Errorf("GitHub API 匿名请求次数已用完，请稍后重试")
			}
			return nil, fmt.Errorf("GitHub 拒绝访问，请检查网络或更新加速设置")
		case http.StatusTooManyRequests:
			return nil, fmt.Errorf("GitHub 请求过于频繁，请稍后重试")
		default:
			return nil, fmt.Errorf("GitHub Release API 返回 HTTP %d，请检查网络或更新加速设置", resp.StatusCode)
		}
	}
	var release struct {
		TagName string                `json:"tag_name"`
		Assets  []singboxReleaseAsset `json:"assets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !isSingboxVersion(version) {
		return nil, fmt.Errorf("release API returned invalid version %q", release.TagName)
	}
	return &singboxRelease{Version: version, Assets: release.Assets}, nil
}

func compareSingboxVersions(left, right string) int {
	l, leftPrerelease := parseSingboxVersion(left)
	r, rightPrerelease := parseSingboxVersion(right)
	for i := range 3 {
		if l[i] < r[i] {
			return -1
		}
		if l[i] > r[i] {
			return 1
		}
	}
	if leftPrerelease == rightPrerelease {
		return 0
	}
	if leftPrerelease != "" && rightPrerelease == "" {
		return -1
	}
	if leftPrerelease == "" && rightPrerelease != "" {
		return 1
	}
	if leftPrerelease < rightPrerelease {
		return -1
	}
	if leftPrerelease > rightPrerelease {
		return 1
	}
	return 0
}

func singboxSupportsDNSIndependentCache(version string) bool {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if !isSingboxVersion(version) {
		return true
	}
	parsed, _ := parseSingboxVersion(version)
	return parsed[0] < 1 || parsed[0] == 1 && parsed[1] < 14
}

func parseSingboxVersion(version string) ([3]int, string) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	version = strings.SplitN(version, "+", 2)[0]
	versionParts := strings.SplitN(version, "-", 2)
	parts := strings.Split(versionParts[0], ".")
	var result [3]int
	for i := 0; i < len(parts) && i < len(result); i++ {
		result[i], _ = strconv.Atoi(parts[i])
	}
	prerelease := ""
	if len(versionParts) == 2 {
		prerelease = versionParts[1]
	}
	return result, prerelease
}
