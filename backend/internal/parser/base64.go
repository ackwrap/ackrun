package parser

import (
	"encoding/base64"
	"strings"
)

func decodeBase64(text string) (string, bool) {
	compact := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, text)
	if compact == "" {
		return "", false
	}
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.URLEncoding} {
		decoded, err := enc.DecodeString(withPadding(compact))
		if err == nil && strings.Contains(string(decoded), "://") {
			return string(decoded), true
		}
	}
	return "", false
}

func withPadding(value string) string {
	if rem := len(value) % 4; rem != 0 {
		value += strings.Repeat("=", 4-rem)
	}
	return value
}
