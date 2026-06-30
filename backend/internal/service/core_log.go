package service

import "sync"

const defaultCoreLogLimit = 1000

type CoreLogEntry struct {
	ID     int64  `json:"id"`
	Time   int64  `json:"time"`
	Source string `json:"source"`
	Line   string `json:"line"`
}

type CoreLogService struct {
	mu      sync.Mutex
	entries []CoreLogEntry
	nextID  int64
	limit   int
}

func NewCoreLogService() *CoreLogService {
	return &CoreLogService{limit: defaultCoreLogLimit}
}

func (svc *CoreLogService) Append(source string, time int64, line string) CoreLogEntry {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.nextID++
	entry := CoreLogEntry{ID: svc.nextID, Time: time, Source: source, Line: line}
	svc.entries = append(svc.entries, entry)
	if len(svc.entries) > svc.limit {
		copy(svc.entries, svc.entries[len(svc.entries)-svc.limit:])
		svc.entries = svc.entries[:svc.limit]
	}
	return entry
}

func (svc *CoreLogService) List(limit int) []CoreLogEntry {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if limit <= 0 || limit > svc.limit {
		limit = svc.limit
	}
	start := len(svc.entries) - limit
	if start < 0 {
		start = 0
	}
	out := make([]CoreLogEntry, len(svc.entries[start:]))
	copy(out, svc.entries[start:])
	return out
}

func (svc *CoreLogService) Clear() {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.entries = nil
}
