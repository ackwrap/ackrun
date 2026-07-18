package logging

import (
	"fmt"
	"log"
	"sync"
	"time"
)

const defaultToolLogLimit = 1000

type ToolLogEntry struct {
	ID      int64  `json:"id"`
	Time    int64  `json:"time"`
	Level   string `json:"level"`
	Tag     string `json:"tag"`
	Message string `json:"message"`
}

var toolLogs = struct {
	sync.Mutex
	entries []ToolLogEntry
	nextID  int64
}{entries: make([]ToolLogEntry, 0, defaultToolLogLimit)}

func Info(tag string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	appendToolLog("info", tag, message)
	log.Printf("[%s] %s", tag, message)
}

func Error(tag string, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	appendToolLog("error", tag, message)
	log.Printf("[ERROR][%s] %s", tag, message)
}

func appendToolLog(level, tag, message string) {
	toolLogs.Lock()
	defer toolLogs.Unlock()
	toolLogs.nextID++
	toolLogs.entries = append(toolLogs.entries, ToolLogEntry{
		ID:      toolLogs.nextID,
		Time:    time.Now().UnixMilli(),
		Level:   level,
		Tag:     tag,
		Message: message,
	})
	if len(toolLogs.entries) > defaultToolLogLimit {
		copy(toolLogs.entries, toolLogs.entries[len(toolLogs.entries)-defaultToolLogLimit:])
		toolLogs.entries = toolLogs.entries[:defaultToolLogLimit]
	}
}

func ListToolLogs(limit int) []ToolLogEntry {
	toolLogs.Lock()
	defer toolLogs.Unlock()
	if limit <= 0 || limit > defaultToolLogLimit {
		limit = defaultToolLogLimit
	}
	start := len(toolLogs.entries) - limit
	if start < 0 {
		start = 0
	}
	entries := make([]ToolLogEntry, len(toolLogs.entries[start:]))
	copy(entries, toolLogs.entries[start:])
	return entries
}

func ClearToolLogs() {
	toolLogs.Lock()
	defer toolLogs.Unlock()
	toolLogs.entries = toolLogs.entries[:0]
}
