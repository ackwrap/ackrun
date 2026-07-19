package logging

import (
	"fmt"
	"log"
	"net/url"
	"strings"
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

var toolLogSubscribers = struct {
	sync.RWMutex
	channels map[chan ToolLogEntry]struct{}
}{channels: make(map[chan ToolLogEntry]struct{})}

func Info(tag string, format string, args ...any) {
	message := RedactAccessToken(fmt.Sprintf(format, args...))
	appendToolLog("info", tag, message)
	log.Printf("[%s] %s", tag, message)
}

func Error(tag string, format string, args ...any) {
	message := RedactAccessToken(fmt.Sprintf(format, args...))
	appendToolLog("error", tag, message)
	log.Printf("[ERROR][%s] %s", tag, message)
}

func RedactAccessToken(value string) string {
	var redacted strings.Builder
	lastWritten := 0
	for searchFrom := 0; searchFrom < len(value); {
		relativeEqual := strings.IndexByte(value[searchFrom:], '=')
		if relativeEqual < 0 {
			break
		}
		equalIndex := searchFrom + relativeEqual
		keyStart := equalIndex
		for keyStart > 0 && isQueryKeyCharacter(value[keyStart-1]) {
			keyStart--
		}
		decodedKey, err := url.QueryUnescape(value[keyStart:equalIndex])
		if err != nil || !strings.EqualFold(decodedKey, "access_token") {
			searchFrom = equalIndex + 1
			continue
		}

		valueEnd := equalIndex + 1
		for valueEnd < len(value) && !isQueryValueDelimiter(value[valueEnd]) {
			valueEnd++
		}
		redacted.WriteString(value[lastWritten : equalIndex+1])
		redacted.WriteString("[REDACTED]")
		lastWritten = valueEnd
		searchFrom = valueEnd
	}
	if lastWritten == 0 {
		return value
	}
	redacted.WriteString(value[lastWritten:])
	return redacted.String()
}

func isQueryKeyCharacter(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || strings.ContainsRune("_-.~%+", rune(value))
}

func isQueryValueDelimiter(value byte) bool {
	return value == '&' || value == '\\' || value == '"' || value == '\'' || value == '<' || value == '>' || value == ' ' || value == '\t' || value == '\r' || value == '\n'
}

func appendToolLog(level, tag, message string) {
	toolLogs.Lock()
	toolLogs.nextID++
	entry := ToolLogEntry{
		ID:      toolLogs.nextID,
		Time:    time.Now().UnixMilli(),
		Level:   level,
		Tag:     tag,
		Message: message,
	}
	toolLogs.entries = append(toolLogs.entries, entry)
	if len(toolLogs.entries) > defaultToolLogLimit {
		copy(toolLogs.entries, toolLogs.entries[len(toolLogs.entries)-defaultToolLogLimit:])
		toolLogs.entries = toolLogs.entries[:defaultToolLogLimit]
	}
	toolLogs.Unlock()

	toolLogSubscribers.RLock()
	defer toolLogSubscribers.RUnlock()
	for channel := range toolLogSubscribers.channels {
		select {
		case channel <- entry:
		default:
		}
	}
}

func SubscribeToolLogs(buffer int) (<-chan ToolLogEntry, func()) {
	if buffer < 1 {
		buffer = 1
	}
	channel := make(chan ToolLogEntry, buffer)
	toolLogSubscribers.Lock()
	toolLogSubscribers.channels[channel] = struct{}{}
	toolLogSubscribers.Unlock()
	var once sync.Once
	return channel, func() {
		once.Do(func() {
			toolLogSubscribers.Lock()
			delete(toolLogSubscribers.channels, channel)
			close(channel)
			toolLogSubscribers.Unlock()
		})
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
