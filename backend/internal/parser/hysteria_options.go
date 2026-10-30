package parser

import (
	"fmt"
	"strconv"
	"strings"
)

// NormalizeHysteriaPortRanges converts port hopping ranges to sing-box's colon
// notation, preserving native open-ended ranges. It also handles saved nodes.
func NormalizeHysteriaPortRanges(value any) ([]string, error) {
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
		start, end := uint64(0), uint64(65535)
		var startErr, endErr error
		if startText != "" || separator != ":" {
			start, startErr = strconv.ParseUint(startText, 10, 16)
		}
		if endText != "" || separator != ":" {
			end, endErr = strconv.ParseUint(endText, 10, 16)
		}
		if startErr != nil || endErr != nil || start > end || (separator != ":" && start == 0) {
			return nil, fmt.Errorf("invalid port hopping range")
		}
		if startText != "" {
			startText = strconv.FormatUint(start, 10)
		}
		if endText != "" {
			endText = strconv.FormatUint(end, 10)
		}
		ranges = append(ranges, startText+":"+endText)
	}
	return ranges, nil
}
