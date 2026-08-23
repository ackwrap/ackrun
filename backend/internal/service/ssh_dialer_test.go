package service

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHNodeExposureProxyConnections(t *testing.T) {
	sshServer := newTestSSHServer(t, "correct-password")
	defer sshServer.Close()
	for _, inboundType := range []string{"socks", "mixed", "http"} {
		t.Run(inboundType, func(t *testing.T) {
			proxyServer := newTestSSHProxy(t, inboundType, "proxy-user", "proxy-password")
			defer proxyServer.Close()
			svc, host := setupNodeExposureSSHHost(t, sshServer.Port(), inboundType, proxyServer.Port(), true)
			trustTestSSHHostKey(t, svc, host.ID)
			result, err := svc.TestHost(context.Background(), host.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !result.Success || result.ConnectionMode != "node_exposure" {
				t.Fatalf("unexpected proxied SSH result: %+v", result)
			}
		})
	}
}

func TestSSHNodeExposureFailureDoesNotFallbackToDirect(t *testing.T) {
	sshServer := newTestSSHServer(t, "correct-password")
	defer sshServer.Close()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	svc, host := setupNodeExposureSSHHost(t, sshServer.Port(), "socks", port, true)
	_, err = svc.TestHost(context.Background(), host.ID)
	code, _ := sshErrorInfo(err)
	if code != "SSH_PROXY_CONNECT_FAILED" {
		t.Fatalf("expected proxy failure without direct fallback, got %s: %v", code, err)
	}
	if sshServer.ConnectionCount() != 0 {
		t.Fatal("proxy failure unexpectedly connected directly to the SSH target")
	}
}

func TestSSHNodeExposureRequiresRunningCore(t *testing.T) {
	sshServer := newTestSSHServer(t, "correct-password")
	defer sshServer.Close()
	proxyServer := newTestSSHProxy(t, "socks", "proxy-user", "proxy-password")
	defer proxyServer.Close()
	svc, host := setupNodeExposureSSHHost(t, sshServer.Port(), "socks", proxyServer.Port(), false)
	_, err := svc.TestHost(context.Background(), host.ID)
	code, _ := sshErrorInfo(err)
	if code != "SSH_CORE_NOT_RUNNING" {
		t.Fatalf("expected stopped core failure, got %s: %v", code, err)
	}
	if sshServer.ConnectionCount() != 0 {
		t.Fatal("stopped core unexpectedly connected directly to the SSH target")
	}
}

type sshTestCore struct{ running bool }

func (core sshTestCore) IsRunning() bool { return core.running }

func setupNodeExposureSSHHost(t *testing.T, sshPort int, inboundType string, proxyPort int, coreRunning bool) (*SSHHostService, *model.SSHHost) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	now := time.Now().UnixMilli()
	result, err := db.DB().Exec(`INSERT INTO subscriptions (name, url, created_at, updated_at) VALUES (?, ?, ?, ?)`, "proxy source", "manual://proxy-test", now, now)
	if err != nil {
		t.Fatal(err)
	}
	subscriptionID, err := result.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.DB().Exec(`INSERT INTO nodes
		(uid, subscription_id, name, type, server, server_port, raw, raw_json, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)`, "proxy-node", subscriptionID, "proxy node", "socks", "127.0.0.1", 1, "test", `{}`, now, now); err != nil {
		t.Fatal(err)
	}
	exposure := &model.NodeExposure{
		Name: "SSH proxy entry", SubscriptionID: subscriptionID, NodeUID: "proxy-node",
		InboundType: inboundType, Listen: "127.0.0.1", ListenPort: proxyPort,
		Username: "proxy-user", Password: "proxy-password", Enabled: true,
	}
	if err := db.CreateNodeExposure(exposure); err != nil {
		t.Fatal(err)
	}
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, sshTestCore{running: coreRunning})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "SSH password", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "proxied SSH", Host: "127.0.0.1", Port: sshPort, Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "node_exposure", NodeExposureID: &exposure.ID,
		TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return svc, host
}

type testSSHProxy struct {
	listener net.Listener
	typeName string
	username string
	password string
	once     sync.Once
}

func newTestSSHProxy(t *testing.T, typeName, username, password string) *testSSHProxy {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &testSSHProxy{listener: listener, typeName: typeName, username: username, password: password}
	go server.serve()
	return server
}

func (server *testSSHProxy) Port() int {
	return server.listener.Addr().(*net.TCPAddr).Port
}

func (server *testSSHProxy) Close() {
	server.once.Do(func() { _ = server.listener.Close() })
}

func (server *testSSHProxy) serve() {
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer conn.Close()
			if server.typeName == "http" {
				server.handleHTTP(conn)
			} else {
				server.handleSOCKS5(conn)
			}
		}()
	}
}

func (server *testSSHProxy) handleHTTP(conn net.Conn) {
	reader := bufio.NewReader(conn)
	request, err := http.ReadRequest(reader)
	if err != nil || request.Method != http.MethodConnect {
		return
	}
	expected := "Basic " + base64.StdEncoding.EncodeToString([]byte(server.username+":"+server.password))
	if request.Header.Get("Proxy-Authorization") != expected {
		_, _ = io.WriteString(conn, "HTTP/1.1 407 Proxy Authentication Required\r\nContent-Length: 0\r\n\r\n")
		return
	}
	upstream, err := net.DialTimeout("tcp", request.Host, 5*time.Second)
	if err != nil {
		_, _ = io.WriteString(conn, "HTTP/1.1 502 Bad Gateway\r\nContent-Length: 0\r\n\r\n")
		return
	}
	defer upstream.Close()
	_, _ = io.WriteString(conn, "HTTP/1.1 200 Connection Established\r\n\r\n")
	proxyBidirectional(conn, upstream, reader)
}

func (server *testSSHProxy) handleSOCKS5(conn net.Conn) {
	reader := bufio.NewReader(conn)
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 5 {
		return
	}
	methods := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, methods); err != nil || !byteSliceContains(methods, 2) {
		return
	}
	_, _ = conn.Write([]byte{5, 2})
	if !server.readSOCKS5Auth(reader) {
		_, _ = conn.Write([]byte{1, 1})
		return
	}
	_, _ = conn.Write([]byte{1, 0})
	requestHeader := make([]byte, 4)
	if _, err := io.ReadFull(reader, requestHeader); err != nil || requestHeader[0] != 5 || requestHeader[1] != 1 {
		return
	}
	host, err := readSOCKS5Host(reader, requestHeader[3])
	if err != nil {
		return
	}
	portBytes := make([]byte, 2)
	if _, err := io.ReadFull(reader, portBytes); err != nil {
		return
	}
	target := net.JoinHostPort(host, strconv.Itoa(int(binary.BigEndian.Uint16(portBytes))))
	upstream, err := net.DialTimeout("tcp", target, 5*time.Second)
	if err != nil {
		_, _ = conn.Write([]byte{5, 5, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	defer upstream.Close()
	_, _ = conn.Write([]byte{5, 0, 0, 1, 127, 0, 0, 1, 0, 0})
	proxyBidirectional(conn, upstream, reader)
}

func (server *testSSHProxy) readSOCKS5Auth(reader *bufio.Reader) bool {
	header := make([]byte, 2)
	if _, err := io.ReadFull(reader, header); err != nil || header[0] != 1 {
		return false
	}
	username := make([]byte, int(header[1]))
	if _, err := io.ReadFull(reader, username); err != nil {
		return false
	}
	passwordLength, err := reader.ReadByte()
	if err != nil {
		return false
	}
	password := make([]byte, int(passwordLength))
	if _, err := io.ReadFull(reader, password); err != nil {
		return false
	}
	return string(username) == server.username && string(password) == server.password
}

func readSOCKS5Host(reader *bufio.Reader, addressType byte) (string, error) {
	switch addressType {
	case 1:
		content := make([]byte, net.IPv4len)
		_, err := io.ReadFull(reader, content)
		return net.IP(content).String(), err
	case 4:
		content := make([]byte, net.IPv6len)
		_, err := io.ReadFull(reader, content)
		return net.IP(content).String(), err
	case 3:
		length, err := reader.ReadByte()
		if err != nil {
			return "", err
		}
		content := make([]byte, int(length))
		_, err = io.ReadFull(reader, content)
		return string(content), err
	default:
		return "", fmt.Errorf("unsupported SOCKS5 address type %d", addressType)
	}
}

func proxyBidirectional(client, upstream net.Conn, clientReader io.Reader) {
	completed := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(upstream, clientReader)
		completed <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(client, upstream)
		completed <- struct{}{}
	}()
	<-completed
}

func byteSliceContains(values []byte, expected byte) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
