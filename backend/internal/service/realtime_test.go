package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackwrap/internal/model"
)

func TestRealtimeBroadcastQueuesEvent(t *testing.T) {
	svc := NewRealtimeService()
	client := newRealtimeClient(nil)
	svc.clients[nil] = client

	svc.Broadcast("core.log", map[string]any{"line": "ready"})

	select {
	case message := <-client.send:
		if message.event == nil {
			t.Fatal("queued message has no event")
		}
		if message.event.Type != "core.log" {
			t.Fatalf("event type = %q, want core.log", message.event.Type)
		}
		if message.event.Time <= 0 {
			t.Fatalf("event time = %d, want current milliseconds", message.event.Time)
		}
	default:
		t.Fatal("broadcast did not enqueue an event")
	}
}

func TestRealtimeBroadcastDisconnectsFullQueueWithoutBlocking(t *testing.T) {
	svc := NewRealtimeService()
	client := newRealtimeClient(nil)
	for range realtimeQueueSize {
		client.send <- outboundMessage{event: &model.WSEvent{Type: "core.log"}}
	}
	svc.clients[nil] = client

	returned := make(chan struct{})
	go func() {
		svc.Broadcast("core.log", "overflow")
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked on a full client queue")
	}

	svc.mu.Lock()
	clientCount := len(svc.clients)
	svc.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("client count = %d, want 0", clientCount)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("slow client was removed without being stopped")
	}
}

func TestRealtimeRemoveClientIsIdempotent(t *testing.T) {
	svc := NewRealtimeService()
	client := newRealtimeClient(nil)
	svc.clients[nil] = client

	svc.RemoveClient(nil)
	svc.RemoveClient(nil)

	select {
	case <-client.done:
	default:
		t.Fatal("removed client was not stopped")
	}
}

func TestRealtimeInitialEventsPrecedeBroadcasts(t *testing.T) {
	svc := NewRealtimeService()
	registered := make(chan struct{})
	serverDone := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(serverDone)
		initial := model.WSEvent{Type: "runtime.status", Time: 0, Data: map[string]any{"status": "stopped"}}
		if !svc.AddClient(conn, initial) {
			return
		}
		defer svc.RemoveClient(conn)
		close(registered)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()
	<-registered
	svc.Broadcast("config.status", map[string]any{"valid": true})

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set client read deadline: %v", err)
	}
	var first model.WSEvent
	if err := conn.ReadJSON(&first); err != nil {
		t.Fatalf("read initial event: %v", err)
	}
	var second model.WSEvent
	if err := conn.ReadJSON(&second); err != nil {
		t.Fatalf("read broadcast event: %v", err)
	}
	if first.Type != "runtime.status" || first.Time != 0 {
		t.Fatalf("first event = %#v, want initial runtime.status", first)
	}
	if second.Type != "config.status" || second.Time <= 0 {
		t.Fatalf("second event = %#v, want current config.status", second)
	}

	if err := conn.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("server did not remove closed client")
	}
}
