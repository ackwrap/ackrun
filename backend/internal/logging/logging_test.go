package logging

import (
	"fmt"
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
