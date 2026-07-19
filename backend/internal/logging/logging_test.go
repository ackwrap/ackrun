package logging

import (
	"fmt"
	"strings"
	"testing"
)

func TestToolLogsListAndClear(t *testing.T) {
	ClearToolLogs()
	Info("test.info", "value=%d", 1)
	Error("test.error", "failed: %s", "reason")

	entries := ListToolLogs(1)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].Level != "error" || entries[0].Tag != "test.error" || entries[0].Message != "failed: reason" {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}

	ClearToolLogs()
	if entries := ListToolLogs(10); len(entries) != 0 {
		t.Fatalf("entries after clear = %d, want 0", len(entries))
	}
}

func TestToolLogsRetainBoundedTail(t *testing.T) {
	ClearToolLogs()
	for i := 0; i < defaultToolLogLimit+2; i++ {
		appendToolLog("info", "test.bound", fmt.Sprintf("entry-%d", i))
	}

	entries := ListToolLogs(defaultToolLogLimit + 100)
	if len(entries) != defaultToolLogLimit {
		t.Fatalf("entries = %d, want %d", len(entries), defaultToolLogLimit)
	}
	if entries[0].Message != "entry-2" {
		t.Fatalf("first retained message = %q, want entry-2", entries[0].Message)
	}
	ClearToolLogs()
}

func TestSubscribeToolLogsPublishesAndCancels(t *testing.T) {
	ClearToolLogs()
	defer ClearToolLogs()
	events, cancel := SubscribeToolLogs(1)
	defer cancel()
	appendToolLog("info", "test.stream", "streamed")
	entry, ok := <-events
	if !ok || entry.Level != "info" || entry.Tag != "test.stream" || entry.Message != "streamed" {
		t.Fatalf("unexpected streamed entry: %+v, open=%t", entry, ok)
	}
	cancel()
	if _, ok := <-events; ok {
		t.Fatal("tool log subscription must close when cancelled")
	}
}

func TestRedactAccessToken(t *testing.T) {
	values := []string{
		`GET /api/v1/rules/content?access_token=secret-value&format=source`,
		`GET /api/v1/rules/content?access%5Ftoken=secret-value&format=source`,
		`GET /api/v1/rules/content?%61%63%63%65%73%73%5f%74%6f%6b%65%6e=secret-value&format=source`,
	}
	for _, value := range values {
		redacted := RedactAccessToken(value)
		if strings.Contains(redacted, "secret-value") || !strings.Contains(redacted, "[REDACTED]") {
			t.Fatalf("RedactAccessToken(%q) = %q", value, redacted)
		}
	}

	ClearToolLogs()
	Info("test.token", "%s", values[1])
	entries := ListToolLogs(1)
	if len(entries) != 1 || strings.Contains(entries[0].Message, "secret-value") {
		t.Fatalf("tool log exposed access token: %+v", entries)
	}
	ClearToolLogs()
}
