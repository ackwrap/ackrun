package service

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHRealtimeTerminalLifecycle(t *testing.T) {
	sshServer := newTestSSHServer(t, "correct-password")
	defer sshServer.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	realtime := NewRealtimeService()
	svc.SetRealtimeService(realtime)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "terminal password", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "terminal host", Host: "127.0.0.1", Port: sshServer.Port(), Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)

	websocketServer := newTestRealtimeWebSocketServer(t, realtime)
	defer websocketServer.Close()
	client := dialTestRealtimeWebSocket(t, websocketServer.URL)
	observer := dialTestRealtimeWebSocket(t, websocketServer.URL)
	waitForRealtimeClientCount(t, realtime, 2)

	first, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{Columns: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	writeTestSSHCommand(t, client, "ssh.session.attach", map[string]any{
		"session_id": first.SessionID, "attach_token": first.AttachToken, "columns": 100, "rows": 30,
	})
	waitForTestTerminalContent(t, client, first.SessionID, "ready")
	writeTestSSHCommand(t, observer, "ssh.session.attach", map[string]any{
		"session_id": first.SessionID, "attach_token": first.AttachToken, "columns": 80, "rows": 24,
	})
	observerEvent := readTestSSHEvent(t, observer, time.Now().Add(5*time.Second))
	observerData := testSSHEventData(observerEvent)
	if observerEvent.Type != "ssh.session.error" || observerData["code"] != "SSH_SESSION_ATTACH_EXPIRED" {
		t.Fatalf("non-owner received session data or reused attach token: type=%s data=%#v", observerEvent.Type, observerData)
	}
	writeTestSSHCommand(t, client, "ssh.session.input", map[string]any{
		"session_id": first.SessionID,
		"content":    base64.StdEncoding.EncodeToString([]byte("hello from websocket\n")),
	})
	waitForTestTerminalContent(t, client, first.SessionID, "hello from websocket")
	assertNoTestSSHOutput(t, observer, first.SessionID, 250*time.Millisecond)
	if err := observer.Close(); err != nil {
		t.Fatal(err)
	}
	waitForRealtimeClientCount(t, realtime, 1)
	writeTestSSHCommand(t, client, "ssh.session.resize", map[string]any{
		"session_id": first.SessionID, "columns": 120, "rows": 40,
	})
	waitForTestCondition(t, func() bool { return sshServer.WindowChangeCount() > 0 }, "SSH terminal resize was not forwarded")
	writeTestSSHCommand(t, client, "ssh.session.close", map[string]any{"session_id": first.SessionID})
	waitForTestSSHEvent(t, client, "ssh.session.closed", first.SessionID)
	waitForSSHSessionCount(t, svc, 0)

	second, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	writeTestSSHCommand(t, client, "ssh.session.attach", map[string]any{
		"session_id": second.SessionID, "attach_token": second.AttachToken, "columns": 80, "rows": 24,
	})
	waitForTestSSHEvent(t, client, "ssh.session.status", second.SessionID)
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	waitForSSHSessionCount(t, svc, 0)

	shutdownClient := dialTestRealtimeWebSocket(t, websocketServer.URL)
	third, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{Columns: 80, Rows: 24})
	if err != nil {
		t.Fatal(err)
	}
	writeTestSSHCommand(t, shutdownClient, "ssh.session.attach", map[string]any{
		"session_id": third.SessionID, "attach_token": third.AttachToken, "columns": 80, "rows": 24,
	})
	waitForTestSSHEvent(t, shutdownClient, "ssh.session.status", third.SessionID)
	svc.Close()
	waitForTestSSHEvent(t, shutdownClient, "ssh.session.closed", third.SessionID)
	waitForSSHSessionCount(t, svc, 0)
	if _, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{}); sshServiceErrorCode(err) != "SSH_SESSION_CLOSED" {
		t.Fatalf("expected stopped SSH service to reject new session, got %v", err)
	}
	_ = shutdownClient.Close()
}

func TestSSHCreateSessionEnforcesLimitsAndReleases(t *testing.T) {
	sshServer := newTestSSHServer(t, "correct-password")
	defer sshServer.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "limit password", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	createHost := func(name string) *model.SSHHost {
		host, err := svc.CreateHost(model.SSHHostRequest{
			Name: name, Host: "127.0.0.1", Port: sshServer.Port(), Username: "tester",
			CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		trustTestSSHHostKey(t, svc, host.ID)
		return host
	}
	hostOne := createHost("limit host one")
	hostTwo := createHost("limit host two")
	hostThree := createHost("limit host three")
	first, err := svc.CreateSession(context.Background(), hostOne.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CreateSession(context.Background(), hostOne.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(context.Background(), hostOne.ID, model.SSHSessionCreateRequest{}); sshServiceErrorCode(err) != "SSH_SESSION_LIMIT" {
		t.Fatalf("expected per-host session limit, got %v", err)
	}
	third, err := svc.CreateSession(context.Background(), hostTwo.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	fourth, err := svc.CreateSession(context.Background(), hostTwo.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateSession(context.Background(), hostThree.ID, model.SSHSessionCreateRequest{}); sshServiceErrorCode(err) != "SSH_SESSION_LIMIT" {
		t.Fatalf("expected global session limit, got %v", err)
	}
	if err := svc.CloseSession(first.SessionID); err != nil {
		t.Fatal(err)
	}
	replacement, err := svc.CreateSession(context.Background(), hostThree.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatalf("released session slot was not reusable: %v", err)
	}
	for _, sessionID := range []string{second.SessionID, third.SessionID, fourth.SessionID, replacement.SessionID} {
		if err := svc.CloseSession(sessionID); err != nil {
			t.Fatal(err)
		}
	}
	waitForSSHSessionCount(t, svc, 0)
}

func newTestRealtimeWebSocketServer(t *testing.T, realtime *RealtimeService) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		conn, err := upgrader.Upgrade(writer, request, nil)
		if err != nil {
			return
		}
		if !realtime.AddClient(conn) {
			return
		}
		defer realtime.RemoveClient(conn)
		for {
			var command model.WSCommand
			if err := conn.ReadJSON(&command); err != nil {
				return
			}
			realtime.HandleCommand(conn, command)
		}
	}))
	return server
}

func dialTestRealtimeWebSocket(t *testing.T, serverURL string) *websocket.Conn {
	t.Helper()
	url := "ws" + strings.TrimPrefix(serverURL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func writeTestSSHCommand(t *testing.T, conn *websocket.Conn, eventType string, data any) {
	t.Helper()
	if err := conn.WriteJSON(model.WSEvent{Type: eventType, Time: time.Now().UnixMilli(), Data: data}); err != nil {
		t.Fatal(err)
	}
}

func waitForTestTerminalContent(t *testing.T, conn *websocket.Conn, sessionID, expected string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	content := ""
	for time.Now().Before(deadline) {
		event := readTestSSHEvent(t, conn, deadline)
		data := testSSHEventData(event)
		if data["session_id"] != sessionID {
			continue
		}
		if event.Type == "ssh.session.error" {
			t.Fatalf("terminal error: %#v", data)
		}
		if event.Type != "ssh.session.output" {
			continue
		}
		encoded, _ := data["content"].(string)
		chunk, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		content += string(chunk)
		if strings.Contains(content, expected) {
			return
		}
	}
	t.Fatalf("terminal output did not contain %q: %q", expected, content)
}

func waitForTestSSHEvent(t *testing.T, conn *websocket.Conn, eventType, sessionID string) model.WSEvent {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		event := readTestSSHEvent(t, conn, deadline)
		data := testSSHEventData(event)
		if event.Type == "ssh.session.error" && data["session_id"] == sessionID {
			t.Fatalf("terminal error: %#v", data)
		}
		if event.Type == eventType && data["session_id"] == sessionID {
			return event
		}
	}
	t.Fatalf("event %s for session %s was not received", eventType, sessionID)
	return model.WSEvent{}
}

func readTestSSHEvent(t *testing.T, conn *websocket.Conn, deadline time.Time) model.WSEvent {
	t.Helper()
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	var event model.WSEvent
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatal(err)
	}
	return event
}

func testSSHEventData(event model.WSEvent) map[string]any {
	data, _ := event.Data.(map[string]any)
	return data
}

func assertNoTestSSHOutput(t *testing.T, conn *websocket.Conn, sessionID string, duration time.Duration) {
	t.Helper()
	deadline := time.Now().Add(duration)
	if err := conn.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	for {
		var event model.WSEvent
		err := conn.ReadJSON(&event)
		if err != nil {
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				return
			}
			t.Fatalf("reading observer websocket: %v", err)
		}
		data := testSSHEventData(event)
		if event.Type == "ssh.session.output" && data["session_id"] == sessionID {
			t.Fatal("non-owner received SSH terminal output")
		}
		if time.Now().After(deadline) {
			return
		}
	}
}

func sshServiceErrorCode(err error) string {
	var serviceErr *SSHServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code
	}
	return ""
}

func waitForSSHSessionCount(t *testing.T, svc *SSHHostService, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		svc.sessionMu.Lock()
		count := len(svc.sessions)
		svc.sessionMu.Unlock()
		if count == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(fmt.Sprintf("SSH session count did not become %d", expected))
}

func waitForRealtimeClientCount(t *testing.T, realtime *RealtimeService, expected int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		realtime.mu.Lock()
		count := len(realtime.clients)
		realtime.mu.Unlock()
		if count == expected {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("realtime client count did not become %d", expected)
}

func waitForTestCondition(t *testing.T, condition func() bool, message string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(message)
}
