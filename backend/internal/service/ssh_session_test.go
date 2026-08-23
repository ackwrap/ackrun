package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHAttachRejectsDisconnectedRealtimeClient(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := &SSHHostService{
		store:         db,
		sessions:      make(map[string]*managedSSHSession),
		pendingByHost: make(map[int64]int),
		realtime:      NewRealtimeService(),
		now:           time.Now,
	}
	managed := &managedSSHSession{
		id: "session", attachToken: "token", host: testManagedSSHHost(),
		done: make(chan struct{}), input: make(chan []byte, 1), expiresAt: time.Now().Add(time.Minute),
	}
	svc.sessions[managed.id] = managed
	conn := &websocket.Conn{}
	svc.attachSession(conn, sshSessionAttachCommand{
		SessionID: managed.id, AttachToken: managed.attachToken, Columns: 80, Rows: 24,
	})
	if _, exists := svc.sessions[managed.id]; exists {
		t.Fatal("disconnected realtime client retained SSH session")
	}
	if !managed.terminated {
		t.Fatal("disconnected realtime client did not terminate SSH session")
	}
}

func testManagedSSHHost() *model.SSHHost {
	return &model.SSHHost{ID: 1, Name: "test", ConnectionMode: "direct"}
}
