package service

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackwrap/internal/logging"
	"github.com/ackwrap/ackwrap/internal/model"
)

type RealtimeService struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]struct{}
}

func NewRealtimeService() *RealtimeService {
	return &RealtimeService{
		clients: make(map[*websocket.Conn]struct{}),
	}
}

func (svc *RealtimeService) AddClient(conn *websocket.Conn) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.clients[conn] = struct{}{}
	logging.Info("websocket.connect", "client connected, total=%d", len(svc.clients))
}

func (svc *RealtimeService) RemoveClient(conn *websocket.Conn) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	delete(svc.clients, conn)
	conn.Close()
	logging.Info("websocket.connect", "client disconnected, total=%d", len(svc.clients))
}

func (svc *RealtimeService) Broadcast(eventType string, data any) {
	svc.mu.Lock()
	defer svc.mu.Unlock()

	event := model.WSEvent{
		Type: eventType,
		Time: time.Now().UnixMilli(),
		Data: data,
	}

	if eventType != "core.log" {
		logging.Info("websocket.broadcast", "broadcasting event: %s", eventType)
	}
	for conn := range svc.clients {
		if err := conn.WriteJSON(event); err != nil {
			logging.Error("websocket.broadcast", "write error: %v", err)
			delete(svc.clients, conn)
			conn.Close()
		}
	}
}
