package store

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"

	"github.com/ackwrap/ackwrap/internal/logging"
)

type Store struct {
	db             *sql.DB
	nodeRefsMu     sync.Mutex
	configUpdateMu sync.RWMutex
}

func Open(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	logging.Info("store", "database opened: %s", dbPath)
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) DB() *sql.DB {
	return s.db
}

// HoldConfigUpdate marks a node mutation that must finish before a configuration
// snapshot is generated. Multiple subscription syncs may run concurrently.
func (s *Store) HoldConfigUpdate() func() {
	s.configUpdateMu.RLock()
	return s.configUpdateMu.RUnlock
}

// HoldConfigSnapshot blocks new configuration-visible mutations until the
// generated configuration has been applied and its core lifecycle action ends.
func (s *Store) HoldConfigSnapshot() func() {
	s.configUpdateMu.Lock()
	return s.configUpdateMu.Unlock
}
