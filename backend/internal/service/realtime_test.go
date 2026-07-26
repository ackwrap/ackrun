package service

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackrun/internal/model"
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

func TestRealtimeInitializationBuffersConcurrentBroadcasts(t *testing.T) {
	svc := NewRealtimeService()
	snapshotCaptured := make(chan struct{})
	releaseSnapshot := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSnapshot) }) }
	defer release()
	initialized := make(chan struct{})
	serverDone := make(chan struct{})
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer close(serverDone)
		if !svc.AddClientWithInitialState(conn, func() []model.WSEvent {
			initial := model.WSEvent{Type: "runtime.status", Time: 0, Data: map[string]any{"status": "stopped"}}
			close(snapshotCaptured)
			<-releaseSnapshot
			return []model.WSEvent{initial}
		}) {
			return
		}
		defer svc.RemoveClient(conn)
		close(initialized)
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
	select {
	case <-snapshotCaptured:
	case <-time.After(time.Second):
		t.Fatal("initial state callback did not capture a snapshot")
	}

	broadcastDone := make(chan struct{})
	go func() {
		svc.Broadcast("runtime.status", map[string]any{"status": "running"})
		close(broadcastDone)
	}()
	select {
	case <-broadcastDone:
	case <-time.After(time.Second):
		t.Fatal("broadcast blocked while the initial state was generated")
	}
	release()
	select {
	case <-initialized:
	case <-time.After(time.Second):
		t.Fatal("client initialization did not complete")
	}

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("set client read deadline: %v", err)
	}
	var initial model.WSEvent
	if err := conn.ReadJSON(&initial); err != nil {
		t.Fatalf("read initial event: %v", err)
	}
	var live model.WSEvent
	if err := conn.ReadJSON(&live); err != nil {
		t.Fatalf("read buffered event: %v", err)
	}
	if initial.Type != "runtime.status" || initial.Time != 0 {
		t.Fatalf("initial event = %#v, want snapshot first", initial)
	}
	if live.Type != "runtime.status" || live.Time <= 0 {
		t.Fatalf("live event = %#v, want buffered broadcast second", live)
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

func TestRealtimeInitializationPanicRemovesClient(t *testing.T) {
	svc := NewRealtimeService()
	serverDone := make(chan struct{})
	recoveredPanic := make(chan any, 1)
	registeredClient := make(chan *realtimeClient, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			close(serverDone)
			return
		}
		defer func() {
			recoveredPanic <- recover()
			close(serverDone)
		}()
		svc.AddClientWithInitialState(conn, func() []model.WSEvent {
			svc.mu.Lock()
			client := svc.clients[conn]
			svc.mu.Unlock()
			registeredClient <- client
			panic("initial state failed")
		})
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	defer conn.Close()

	var client *realtimeClient
	select {
	case client = <-registeredClient:
	case <-time.After(time.Second):
		t.Fatal("initializing client was not registered")
	}
	select {
	case recovered := <-recoveredPanic:
		if recovered != "initial state failed" {
			t.Fatalf("recovered panic = %v", recovered)
		}
	case <-time.After(time.Second):
		t.Fatal("initial state panic was not propagated")
	}
	if client == nil {
		t.Fatal("registered client is nil")
	}
	svc.mu.Lock()
	clientCount := len(svc.clients)
	svc.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("client count = %d, want panicking client removed", clientCount)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("panicking client was not stopped")
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("server did not finish after initial state panic")
	}
}

func TestRealtimeInitializationBufferIsBounded(t *testing.T) {
	svc := NewRealtimeService()
	client := newRealtimeClient(nil)
	client.initializing = true
	svc.clients[nil] = client

	for range realtimeQueueSize + 1 {
		svc.Broadcast("core.log", "line")
	}

	svc.mu.Lock()
	clientCount := len(svc.clients)
	svc.mu.Unlock()
	if clientCount != 0 {
		t.Fatalf("client count = %d, want overflowing initializing client removed", clientCount)
	}
	select {
	case <-client.done:
	default:
		t.Fatal("overflowing initializing client was not stopped")
	}
}
