package service

import (
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestResolveSchedulerLocation(t *testing.T) {
	fallback := time.FixedZone("fallback", 3600)
	tests := []struct {
		name       string
		explicit   string
		process    string
		etc        string
		wantSource string
		wantOffset int
	}{
		{name: "OpenWrt process POSIX timezone", process: "CST-8", wantSource: "TZ", wantOffset: 8 * 3600},
		{name: "OpenWrt timezone file", etc: "CST-8\n", wantSource: "/etc/TZ", wantOffset: 8 * 3600},
		{name: "angle POSIX timezone", etc: "<+08>-8", wantSource: "/etc/TZ", wantOffset: 8 * 3600},
		{name: "fractional POSIX timezone", explicit: "IST-5:30", process: "CST-8", wantSource: schedulerTimezoneEnv, wantOffset: 5*3600 + 30*60},
		{name: "negative zero POSIX timezone", explicit: "UTC-0:30", wantSource: schedulerTimezoneEnv, wantOffset: 30 * 60},
		{name: "positive zero POSIX timezone", explicit: "UTC+0:30", wantSource: schedulerTimezoneEnv, wantOffset: -30 * 60},
		{name: "explicit IANA-compatible timezone", explicit: "UTC", process: "CST-8", wantSource: schedulerTimezoneEnv, wantOffset: 0},
		{name: "invalid explicit falls through", explicit: "invalid timezone", process: "CST-8", wantSource: "TZ", wantOffset: 8 * 3600},
		{name: "invalid values use fallback", explicit: "invalid", process: "EST5EDT,M3.2.0,M11.1.0", etc: "invalid", wantSource: "time.Local", wantOffset: 3600},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			location, source := resolveSchedulerLocation(testCase.explicit, testCase.process, testCase.etc, fallback)
			if source != testCase.wantSource {
				t.Fatalf("source = %q, want %q", source, testCase.wantSource)
			}
			_, offset := time.Date(2026, 7, 28, 0, 0, 0, 0, location).Zone()
			if offset != testCase.wantOffset {
				t.Fatalf("offset = %d, want %d", offset, testCase.wantOffset)
			}
		})
	}
}

func TestResolveSchedulerLocationPreservesIANADaylightSaving(t *testing.T) {
	location, source := resolveSchedulerLocation("America/New_York", "", "", time.UTC)
	if source != schedulerTimezoneEnv {
		t.Fatalf("source = %q, want %q", source, schedulerTimezoneEnv)
	}
	_, winterOffset := time.Date(2026, 1, 15, 12, 0, 0, 0, location).Zone()
	_, summerOffset := time.Date(2026, 7, 15, 12, 0, 0, 0, location).Zone()
	if winterOffset != -5*3600 || summerOffset != -4*3600 {
		t.Fatalf("New York offsets = winter %d summer %d", winterOffset, summerOffset)
	}
}

func TestCronSixFieldScheduleUsesConfiguredLocation(t *testing.T) {
	location := time.FixedZone("CST", 8*3600)
	scheduler := cron.New(cron.WithSeconds(), cron.WithLocation(location))
	entryID, err := scheduler.AddFunc("0 0 4 * * *", func() {})
	if err != nil {
		t.Fatal(err)
	}
	if scheduler.Location() != location {
		t.Fatalf("scheduler location = %s, want %s", scheduler.Location(), location)
	}
	entry := scheduler.Entry(entryID)
	current := time.Date(2026, 7, 27, 18, 0, 0, 0, time.UTC).In(scheduler.Location())
	next := entry.Schedule.Next(current)
	localNext := next.In(location)
	if localNext.Day() != 28 || localNext.Hour() != 4 || localNext.Minute() != 0 || localNext.Second() != 0 {
		t.Fatalf("next local run = %s, want day 28 at 04:00:00", localNext.Format(time.RFC3339))
	}
}

func TestFormatTimezoneOffset(t *testing.T) {
	for _, testCase := range []struct {
		offset int
		want   string
	}{
		{offset: 8 * 3600, want: "+08:00"},
		{offset: -(5*3600 + 30*60), want: "-05:30"},
		{offset: 0, want: "+00:00"},
	} {
		if got := formatTimezoneOffset(testCase.offset); got != testCase.want {
			t.Fatalf("formatTimezoneOffset(%d) = %q, want %q", testCase.offset, got, testCase.want)
		}
	}
}
