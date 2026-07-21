package service

import (
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

const (
	realtimeQueueSize = 256
	writeWait         = 10 * time.Second
	pongWait          = 60 * time.Second
	pingPeriod        = pongWait * 9 / 10
)

type outboundMessage struct {
	event       *model.WSEvent
	messageType int
	data        []byte
}

type realtimeClient struct {
	conn      *websocket.Conn
	send      chan outboundMessage
	done      chan struct{}
	stopOnce  sync.Once
	closeOnce sync.Once
}

func newRealtimeClient(conn *websocket.Conn) *realtimeClient {
	return &realtimeClient{
		conn: conn,
		send: make(chan outboundMessage, realtimeQueueSize),
		done: make(chan struct{}),
	}
}

func (client *realtimeClient) signalStop() {
	client.stopOnce.Do(func() {
		close(client.done)
	})
}

func (client *realtimeClient) closeConnection() {
	client.closeOnce.Do(func() {
		if client.conn != nil {
			_ = client.conn.Close()
		}
	})
}

type RealtimeService struct {
	mu      sync.Mutex
	clients map[*websocket.Conn]*realtimeClient
}

func NewRealtimeService() *RealtimeService {
	return &RealtimeService{
		clients: make(map[*websocket.Conn]*realtimeClient),
	}
}

func (svc *RealtimeService) AddClient(conn *websocket.Conn, initialEvents ...model.WSEvent) bool {
	if conn == nil || len(initialEvents) > realtimeQueueSize {
		if conn != nil {
			_ = conn.Close()
		}
		return false
	}

	client := newRealtimeClient(conn)
	if err := conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		client.closeConnection()
		return false
	}
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	conn.SetPingHandler(func(data string) error {
		svc.enqueueControl(client, websocket.PongMessage, []byte(data))
		return nil
	})
	conn.SetCloseHandler(func(int, string) error { return nil })

	svc.mu.Lock()
	if _, exists := svc.clients[conn]; exists {
		svc.mu.Unlock()
		client.closeConnection()
		return false
	}
	for i := range initialEvents {
		event := initialEvents[i]
		client.send <- outboundMessage{event: &event}
	}
	svc.clients[conn] = client
	total := len(svc.clients)
	svc.mu.Unlock()

	logging.Info("websocket.connect", "client connected, total=%d", total)
	go svc.writePump(client)
	return true
}

func (svc *RealtimeService) RemoveClient(conn *websocket.Conn) {
	svc.mu.Lock()
	client, exists := svc.clients[conn]
	if exists {
		delete(svc.clients, conn)
		client.signalStop()
	}
	total := len(svc.clients)
	svc.mu.Unlock()

	if !exists {
		return
	}
	client.closeConnection()
	logging.Info("websocket.connect", "client disconnected, total=%d", total)
}

func (svc *RealtimeService) writePump(client *realtimeClient) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-client.done:
			return
		default:
		}

		select {
		case <-client.done:
			return
		case message := <-client.send:
			select {
			case <-client.done:
				return
			default:
			}
			if err := svc.writeMessage(client, message); err != nil {
				svc.handleWriteError(client, err)
				return
			}
		case <-ticker.C:
			if err := client.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)); err != nil {
				svc.handleWriteError(client, err)
				return
			}
		}
	}
}

func (svc *RealtimeService) writeMessage(client *realtimeClient, message outboundMessage) error {
	if message.event == nil {
		return client.conn.WriteControl(message.messageType, message.data, time.Now().Add(writeWait))
	}
	if err := client.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return client.conn.WriteJSON(message.event)
}

func (svc *RealtimeService) handleWriteError(client *realtimeClient, writeErr error) {
	removed, total := svc.detachClient(client)
	if !removed {
		return
	}
	client.closeConnection()
	logging.Info("websocket.connect", "client disconnected, total=%d", total)
	logging.Error("websocket.broadcast", "write error: %v", writeErr)
}

func (svc *RealtimeService) detachClient(client *realtimeClient) (bool, int) {
	svc.mu.Lock()
	current, exists := svc.clients[client.conn]
	if exists && current == client {
		delete(svc.clients, client.conn)
		client.signalStop()
	}
	total := len(svc.clients)
	svc.mu.Unlock()
	return exists && current == client, total
}

func (svc *RealtimeService) enqueueControl(client *realtimeClient, messageType int, data []byte) {
	svc.mu.Lock()
	current, exists := svc.clients[client.conn]
	if !exists || current != client {
		svc.mu.Unlock()
		return
	}
	select {
	case client.send <- outboundMessage{messageType: messageType, data: data}:
		svc.mu.Unlock()
		return
	default:
		delete(svc.clients, client.conn)
		client.signalStop()
	}
	total := len(svc.clients)
	svc.mu.Unlock()
	go svc.finishSlowClients([]*realtimeClient{client}, total)
}

func (svc *RealtimeService) Broadcast(eventType string, data any) {
	event := model.WSEvent{
		Type: eventType,
		Time: time.Now().UnixMilli(),
		Data: data,
	}

	var slowClients []*realtimeClient
	svc.mu.Lock()
	for conn, client := range svc.clients {
		select {
		case client.send <- outboundMessage{event: &event}:
		default:
			delete(svc.clients, conn)
			client.signalStop()
			slowClients = append(slowClients, client)
		}
	}
	total := len(svc.clients)
	svc.mu.Unlock()

	if len(slowClients) > 0 {
		go svc.finishSlowClients(slowClients, total)
	}
	if eventType != "core.log" && eventType != "tool.log" {
		logging.Info("websocket.broadcast", "broadcasting event: %s", eventType)
	}
}

func (svc *RealtimeService) finishSlowClients(clients []*realtimeClient, total int) {
	for _, client := range clients {
		client.closeConnection()
	}
	logging.Info("websocket.connect", "disconnected %d slow client(s), total=%d", len(clients), total)
}
