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

func TestRealtimeTargetedSendDropsSlowSSHClient(t *testing.T) {
	realtime := NewRealtimeService()
	key := &websocket.Conn{}
	client := newRealtimeClient(nil)
	realtime.clients[key] = client
	closed := make(chan struct{}, 1)
	realtime.SetCommandHandlers(nil, func(conn *websocket.Conn) {
		if conn == nil {
			closed <- struct{}{}
		}
	})
	for index := 0; index < realtimeQueueSize; index++ {
		if !realtime.SendTo(key, "ssh.session.output", map[string]any{"index": index}) {
			t.Fatalf("targeted message %d was dropped before queue reached its limit", index)
		}
	}
	if realtime.SendTo(key, "ssh.session.output", map[string]any{"overflow": true}) {
		t.Fatal("slow SSH client remained connected after its bounded queue filled")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("slow SSH client close handler was not called")
	}
	if realtime.HasClient(key) {
		t.Fatal("slow SSH client remained registered")
	}
}

func testManagedSSHHost() *model.SSHHost {
	return &model.SSHHost{ID: 1, Name: "test", ConnectionMode: "direct"}
}
