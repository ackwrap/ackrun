package service

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	_ "time/tzdata"

	"github.com/robfig/cron/v3"

	"github.com/ackwrap/ackrun/internal/logging"
)

const schedulerTimezoneEnv = "ACKWRAP_TIMEZONE"

var (
	posixNamedTimezonePattern = regexp.MustCompile(`^([A-Za-z]{3,})([+-]?\d{1,2})(?::(\d{2}))?(?::(\d{2}))?$`)
	posixAngleTimezonePattern = regexp.MustCompile(`^<([^>]+)>([+-]?\d{1,2})(?::(\d{2}))?(?::(\d{2}))?$`)
	schedulerTimezoneLogOnce  sync.Once
)

func newSchedulerCron() *cron.Cron {
	location, source := schedulerLocation()
	schedulerTimezoneLogOnce.Do(func() {
		zone, offset := time.Now().In(location).Zone()
		logging.Info("scheduler.timezone", "using timezone=%s zone=%s offset=%s source=%s", location, zone, formatTimezoneOffset(offset), source)
	})
	return cron.New(cron.WithSeconds(), cron.WithLocation(location))
}

func schedulerLocation() (*time.Location, string) {
	etcTimezone := ""
	if content, err := os.ReadFile("/etc/TZ"); err == nil {
		etcTimezone = string(content)
	}
	location, source := resolveSchedulerLocation(os.Getenv(schedulerTimezoneEnv), os.Getenv("TZ"), etcTimezone, time.Local)
	if configured := strings.TrimSpace(os.Getenv(schedulerTimezoneEnv)); configured != "" && source != schedulerTimezoneEnv {
		logging.Error("scheduler.timezone", "invalid %s=%q, using source=%s", schedulerTimezoneEnv, configured, source)
	}
	return location, source
}

func resolveSchedulerLocation(explicit, processTimezone, etcTimezone string, fallback *time.Location) (*time.Location, string) {
	candidates := []struct {
		value  string
		source string
	}{
		{value: explicit, source: schedulerTimezoneEnv},
		{value: processTimezone, source: "TZ"},
		{value: etcTimezone, source: "/etc/TZ"},
	}
	for _, candidate := range candidates {
		if location, ok := loadSchedulerLocation(candidate.value); ok {
			return location, candidate.source
		}
	}
	if fallback == nil {
		fallback = time.Local
	}
	return fallback, "time.Local"
}

func loadSchedulerLocation(value string) (*time.Location, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, false
	}
	if location, err := time.LoadLocation(strings.TrimPrefix(value, ":")); err == nil {
		return location, true
	}
	return parsePOSIXFixedLocation(value)
}

func parsePOSIXFixedLocation(value string) (*time.Location, bool) {
	matches := posixNamedTimezonePattern.FindStringSubmatch(value)
	if matches == nil {
		matches = posixAngleTimezonePattern.FindStringSubmatch(value)
	}
	if matches == nil {
		return nil, false
	}
	offsetText := matches[2]
	negative := strings.HasPrefix(offsetText, "-")
	hours, err := strconv.Atoi(strings.TrimLeft(offsetText, "+-"))
	if err != nil || hours > 24 {
		return nil, false
	}
	minutes, seconds := 0, 0
	if matches[3] != "" {
		minutes, err = strconv.Atoi(matches[3])
		if err != nil || minutes > 59 {
			return nil, false
		}
	}
	if matches[4] != "" {
		seconds, err = strconv.Atoi(matches[4])
		if err != nil || seconds > 59 {
			return nil, false
		}
	}
	if hours == 24 && (minutes != 0 || seconds != 0) {
		return nil, false
	}
	posixOffset := hours*60*60 + minutes*60 + seconds
	if negative {
		posixOffset = -posixOffset
	}
	// POSIX TZ offsets are reversed: CST-8 means UTC+08:00.
	return time.FixedZone(matches[1], -posixOffset), true
}

func formatTimezoneOffset(offset int) string {
	sign := "+"
	if offset < 0 {
		sign = "-"
		offset = -offset
	}
	return fmt.Sprintf("%s%02d:%02d", sign, offset/3600, offset%3600/60)
}
