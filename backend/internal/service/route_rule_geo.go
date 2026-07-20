package service

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	generatedGeoRuleSetUpdateInterval      = 24 * time.Hour
	generatedGeoRuleSetUpdateIntervalValue = "24h"
	generatedGeoRuleSetAttemptTimeout      = 10 * time.Second
	generatedGeoRuleSetDownloadTimeout     = 75 * time.Second
	generatedGeoRuleSetValidationTimeout   = 30 * time.Second
	generatedGeoRuleSetMaxUncompressedSize = 256 * 1024 * 1024
	generatedGeoRuleSetVersionCurrent      = 5
)

var generatedGeoRuleSetMagic = [3]byte{'S', 'R', 'S'}

func generatedGeoRuleSetTag(ruleType, value string) string {
	ruleType = strings.ToLower(strings.TrimSpace(ruleType))
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, ruleType+"-") {
		return value
	}
	return ruleType + "-" + value
}

func generatedGeoRuleSetURL(tag string) string {
	if strings.HasPrefix(tag, "geosite-") {
		return fmt.Sprintf("https://raw.githubusercontent.com/SagerNet/sing-geosite/rule-set/%s.srs", tag)
	}
	return fmt.Sprintf("https://raw.githubusercontent.com/SagerNet/sing-geoip/rule-set/%s.srs", tag)
}

func validateGeneratedGeoRuleSet(data []byte) error {
	return validateGeneratedGeoRuleSetSize(data, generatedGeoRuleSetMaxUncompressedSize)
}

func validateGeneratedGeoRuleSetSize(data []byte, maxUncompressedSize int64) error {
	if len(data) < 5 || !bytes.Equal(data[:3], generatedGeoRuleSetMagic[:]) {
		return fmt.Errorf("invalid sing-box rule-set header")
	}
	version := data[3]
	if version == 0 || version > generatedGeoRuleSetVersionCurrent {
		return fmt.Errorf("unsupported sing-box rule-set version: %d", version)
	}
	reader, err := zlib.NewReader(bytes.NewReader(data[4:]))
	if err != nil {
		return fmt.Errorf("open sing-box rule-set payload: %w", err)
	}
	written, readErr := io.Copy(io.Discard, io.LimitReader(reader, maxUncompressedSize+1))
	closeErr := reader.Close()
	if readErr != nil {
		return fmt.Errorf("read sing-box rule-set payload: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close sing-box rule-set payload: %w", closeErr)
	}
	if written > maxUncompressedSize {
		return fmt.Errorf("sing-box rule-set payload exceeds %d bytes", maxUncompressedSize)
	}
	return nil
}

func generatedGeoRuleSetContentURL(baseURL, tag string, accessToken ...string) string {
	path := fmt.Sprintf("/api/v1/rules/geo/rule-sets/%s/content", tag)
	rawURL := path
	if strings.TrimSpace(baseURL) == "" {
		return appendAccessToken(rawURL, accessToken)
	}
	rawURL = strings.TrimRight(baseURL, "/") + path
	return appendAccessToken(rawURL, accessToken)
}

func appendGeneratedGeoRuleSets(ruleSets []map[string]interface{}, seen map[string]bool, ruleType string, values []string, baseURL string, accessToken ...string) []map[string]interface{} {
	for _, value := range values {
		tag := generatedGeoRuleSetTag(ruleType, value)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		ruleSets = append(ruleSets, map[string]interface{}{
			"tag":             tag,
			"type":            "remote",
			"format":          "binary",
			"url":             generatedGeoRuleSetContentURL(baseURL, tag, accessToken...),
			"update_interval": generatedGeoRuleSetUpdateIntervalValue,
		})
	}
	return ruleSets
}

func generatedGeoRuleSetTags(ruleType string, values []string) []string {
	tags := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		tag := generatedGeoRuleSetTag(ruleType, value)
		if tag == "" || seen[tag] {
			continue
		}
		seen[tag] = true
		tags = append(tags, tag)
	}
	return tags
}
