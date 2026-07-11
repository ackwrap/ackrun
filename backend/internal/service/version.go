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

func fetchLatestSingboxVersion(client *http.Client, apiURL, githubToken string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, apiURL, nil)
	if err != nil {
		return "", fmt.Errorf("create release request: %w", err)
	}
	req.Header.Set("User-Agent", "Ackwrap/1.0")
	req.Header.Set("Accept", "application/vnd.github+json")
	if strings.TrimSpace(githubToken) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(githubToken))
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request release API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		switch resp.StatusCode {
		case http.StatusUnauthorized:
			return "", fmt.Errorf("GitHub Token 无效，请在设置中更新或清空 Token")
		case http.StatusForbidden:
			if resp.Header.Get("X-RateLimit-Remaining") == "0" {
				return "", fmt.Errorf("GitHub API 匿名请求次数已用完，请在设置中填写 GitHub Token 或启用本地代理")
			}
			return "", fmt.Errorf("GitHub 拒绝访问，请在设置中启用本地代理或填写 GitHub Token")
		case http.StatusTooManyRequests:
			return "", fmt.Errorf("GitHub 请求过于频繁，请稍后重试或在设置中填写 GitHub Token")
		default:
			return "", fmt.Errorf("GitHub Release API 返回 HTTP %d，请检查网络或更新加速设置", resp.StatusCode)
		}
	}
	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("decode release response: %w", err)
	}
	version := strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")
	if !isSingboxVersion(version) {
		return "", fmt.Errorf("release API returned invalid version %q", release.TagName)
	}
	return version, nil
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
