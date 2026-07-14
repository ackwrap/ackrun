package parser

import (
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func normalizeHysteriaPortRanges(value any) ([]string, error) {
	if value == nil {
		return nil, nil
	}
	var rawRanges []string
	switch typed := value.(type) {
	case string:
		rawRanges = strings.Split(typed, ",")
	case []string:
		rawRanges = typed
	case []any:
		rawRanges = make([]string, 0, len(typed))
		for _, item := range typed {
			rawRanges = append(rawRanges, fmt.Sprint(item))
		}
	default:
		return nil, fmt.Errorf("port hopping range must be a string or list")
	}

	ranges := make([]string, 0, len(rawRanges))
	for _, rawRange := range rawRanges {
		rawRange = strings.TrimSpace(rawRange)
		if rawRange == "" {
			return nil, fmt.Errorf("port hopping range contains an empty item")
		}
		separator := ""
		if strings.Contains(rawRange, "-") {
			separator = "-"
		} else if strings.Contains(rawRange, ":") {
			separator = ":"
		}
		startText, endText := rawRange, rawRange
		if separator != "" {
			parts := strings.SplitN(rawRange, separator, 2)
			startText, endText = strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
		start, startErr := strconv.Atoi(startText)
		end, endErr := strconv.Atoi(endText)
		if startErr != nil || endErr != nil || start < 1 || end > 65535 || start > end {
			return nil, fmt.Errorf("invalid port hopping range")
		}
		ranges = append(ranges, fmt.Sprintf("%d:%d", start, end))
	}
	return ranges, nil
}

func normalizeHysteriaHopInterval(value string) (string, string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", nil
	}
	if !strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") && strings.Contains(value, "-") {
		parts := strings.SplitN(value, "-", 2)
		minimum, maximum := durationInSeconds(parts[0]), durationInSeconds(parts[1])
		minimumDuration, minimumErr := time.ParseDuration(minimum)
		maximumDuration, maximumErr := time.ParseDuration(maximum)
		if minimumErr != nil || maximumErr != nil || minimumDuration <= 0 || maximumDuration < minimumDuration {
			return "", "", fmt.Errorf("invalid hop interval range")
		}
		return minimum, maximum, nil
	}
	interval := durationInSeconds(value)
	duration, err := time.ParseDuration(interval)
	if err != nil || duration <= 0 {
		return "", "", fmt.Errorf("invalid hop interval")
	}
	return interval, "", nil
}

func durationInSeconds(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.ContainsAny(value, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ") {
		return value
	}
	if _, err := strconv.ParseUint(value, 10, 64); err != nil {
		return ""
	}
	return value + "s"
}

func normalizeWireGuardReserved(value any) []int {
	if items, ok := value.([]any); ok {
		reserved := make([]int, 0, len(items))
		for _, item := range items {
			value := getInt(map[string]any{"value": item}, "value")
			if value < 0 || value > 255 {
				return nil
			}
			reserved = append(reserved, value)
		}
		if len(reserved) == 3 {
			return reserved
		}
	}
	raw, ok := value.(string)
	if !ok {
		return nil
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(strings.TrimSpace(raw))
		if err != nil || len(decoded) != 3 {
			continue
		}
		return []int{int(decoded[0]), int(decoded[1]), int(decoded[2])}
	}
	return nil
}

func normalizeClashECHConfig(value any) ([]string, error) {
	raw, ok := value.(string)
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("ECH config must be a non-empty base64 string")
	}
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "-----BEGIN") {
		block, rest := pem.Decode([]byte(raw))
		if block == nil || block.Type != "ECH CONFIGS" || len(strings.TrimSpace(string(rest))) > 0 {
			return nil, fmt.Errorf("invalid ECH configs PEM")
		}
		return []string{raw}, nil
	}
	for _, encoding := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		decoded, err := encoding.DecodeString(raw)
		if err != nil || len(decoded) == 0 {
			continue
		}
		encoded := pem.EncodeToMemory(&pem.Block{Type: "ECH CONFIGS", Bytes: decoded})
		return []string{strings.TrimSpace(string(encoded))}, nil
	}
	return nil, fmt.Errorf("invalid ECH config base64")
}

func stringList(value any) []string {
	switch typed := value.(type) {
	case string:
		if typed = strings.TrimSpace(typed); typed != "" {
			return []string{typed}
		}
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				items = append(items, text)
			}
		}
		return items
	}
	return nil
}

func mapClashQUICOptions(result, proxy map[string]any) {
	streamWindow := getInt(proxy, "max-stream-receive-window")
	if streamWindow == 0 {
		streamWindow = getInt(proxy, "recv-window-conn")
	}
	if streamWindow > 0 {
		result["stream_receive_window"] = streamWindow
	}
	connectionWindow := getInt(proxy, "max-connection-receive-window")
	if connectionWindow == 0 {
		connectionWindow = getInt(proxy, "recv-window")
	}
	if connectionWindow > 0 {
		result["connection_receive_window"] = connectionWindow
	}
	if streams := getInt(proxy, "max-open-streams"); streams > 0 {
		result["max_concurrent_streams"] = streams
	}
	if size := getInt(proxy, "initial-packet-size"); size > 0 {
		result["initial_packet_size"] = size
	}
	if disabled, ok := proxy["disable-mtu-discovery"]; ok {
		result["disable_path_mtu_discovery"] = boolOrString(disabled)
	}
}

func normalizeClashMultiplex(smux map[string]any) map[string]any {
	multiplex := map[string]any{"enabled": true}
	for source, target := range map[string]string{
		"protocol":        "protocol",
		"max-connections": "max_connections",
		"min-streams":     "min_streams",
		"max-streams":     "max_streams",
		"padding":         "padding",
	} {
		if value, ok := smux[source]; ok {
			multiplex[target] = value
		}
	}
	if options, ok := smux["brutal-opts"].(map[string]any); ok && boolOrString(options["enabled"]) {
		brutal := map[string]any{"enabled": true}
		if up := parseBandwidth(getString(options, "up")); up > 0 {
			brutal["up_mbps"] = up
		}
		if down := parseBandwidth(getString(options, "down")); down > 0 {
			brutal["down_mbps"] = down
		}
		multiplex["brutal"] = brutal
	}
	return multiplex
}

func clashIPVersionStrategy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ipv4", "ipv4-only":
		return "ipv4_only"
	case "ipv6", "ipv6-only":
		return "ipv6_only"
	case "ipv4-prefer", "prefer-ipv4":
		return "prefer_ipv4"
	case "ipv6-prefer", "prefer-ipv6":
		return "prefer_ipv6"
	default:
		return ""
	}
}

func clashSupportsNetworkField(protocol string) bool {
	switch protocol {
	case "vmess", "vless", "trojan", "shadowsocks", "ssr", "socks", "hysteria", "hysteria2", "tuic", "snell":
		return true
	default:
		return false
	}
}

func isSupportedClashProtocol(protocol string) bool {
	switch protocol {
	case "shadowsocks", "ssr", "vmess", "vless", "trojan", "hysteria", "hysteria2", "tuic", "wireguard", "socks", "http", "anytls", "snell", "naive", "mieru":
		return true
	default:
		return false
	}
}
