package service

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHCredentialEncryptionAndHostKeyTrust(t *testing.T) {
	server := newTestSSHServer(t, "correct-password")
	defer server.Close()

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
		Name: "password credential", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("correct-password")) || bytes.Contains(encoded, []byte("ciphertext")) {
		t.Fatalf("credential response exposed secret material: %s", encoded)
	}
	var ciphertext []byte
	if err := db.DB().QueryRow(`SELECT secret_ciphertext FROM ssh_credentials WHERE id = ?`, credential.ID).Scan(&ciphertext); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("correct-password")) {
		t.Fatal("credential secret was stored as plaintext")
	}

	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "local SSH", Host: "127.0.0.1", Port: server.Port(), Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.TestHost(context.Background(), host.ID)
	var serviceErr *SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_UNKNOWN" {
		t.Fatalf("expected unknown host key, got %v", err)
	}
	challenge, ok := serviceErr.Details.(*model.SSHHostKeyChallenge)
	if !ok || challenge.ChallengeID == "" || challenge.FingerprintSHA256 == "" {
		t.Fatalf("missing host key challenge: %#v", serviceErr.Details)
	}
	if _, err := svc.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{
		ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256,
	}, false); err != nil {
		t.Fatal(err)
	}
	result, err := svc.TestHost(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Success || result.FingerprintSHA256 != challenge.FingerprintSHA256 {
		t.Fatalf("unexpected connection result: %+v", result)
	}
	loaded, err := svc.GetHost(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.LastStatus != "available" || loaded.HostKeyStatus != "trusted" {
		t.Fatalf("unexpected persisted host status: %+v", loaded)
	}
}

func TestSSHCredentialUpdatePreservesSecretWhenOmitted(t *testing.T) {
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
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "old", AuthType: "password", Secret: "secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	before, err := db.GetSSHCredential(credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.UpdateCredential(credential.ID, model.SSHCredentialRequest{Name: "new", AuthType: "password"})
	if err != nil {
		t.Fatal(err)
	}
	after, err := db.GetSSHCredential(updated.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before.SecretCiphertext, after.SecretCiphertext) || !bytes.Equal(before.SecretNonce, after.SecretNonce) {
		t.Fatal("omitted secret did not preserve existing encrypted value")
	}
}

func TestSSHEncryptedPrivateKeyCredential(t *testing.T) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	clientSigner, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	server := newPublicKeyTestSSHServer(t, clientSigner.PublicKey())
	defer server.Close()
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
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", []byte("key-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := pem.EncodeToMemory(block)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "encrypted key", AuthType: "private_key", Secret: string(privateKeyPEM), Passphrase: "key-passphrase",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.KeyFingerprint == "" {
		t.Fatal("private key fingerprint was not populated")
	}
	encoded, err := json.Marshal(credential)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("OPENSSH PRIVATE KEY")) || bytes.Contains(encoded, []byte("key-passphrase")) {
		t.Fatal("private key credential response exposed secret material")
	}
	stored, err := db.GetSSHCredential(credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.authMethod(stored); err != nil {
		t.Fatalf("stored encrypted private key could not be used: %v", err)
	}
	if bytes.Contains(stored.SecretCiphertext, privateKeyPEM) || bytes.Contains(stored.PassphraseCiphertext, []byte("key-passphrase")) {
		t.Fatal("private key credential was stored without effective encryption")
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "private key host", Host: "127.0.0.1", Port: server.Port(), Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	if _, err := svc.TestHost(context.Background(), host.ID); err != nil {
		t.Fatalf("encrypted private key SSH authentication failed: %v", err)
	}
}

func TestSSHHostKeyChangeRequiresExplicitRotation(t *testing.T) {
	server := newTestSSHServer(t, "correct-password")
	defer server.Close()
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
		Name: "password", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "changed key host", Host: "127.0.0.1", Port: server.Port(), Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	oldSigner := newTestSSHSigner(t)
	if err := db.UpsertSSHHostKey(&model.SSHHostKey{
		HostID: host.ID, KeyType: oldSigner.PublicKey().Type(),
		PublicKey:         string(ssh.MarshalAuthorizedKey(oldSigner.PublicKey())),
		FingerprintSHA256: ssh.FingerprintSHA256(oldSigner.PublicKey()),
	}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.TestHost(context.Background(), host.ID)
	var serviceErr *SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_CHANGED" {
		t.Fatalf("expected changed host key rejection, got %v", err)
	}
	challenge, ok := serviceErr.Details.(*model.SSHHostKeyChallenge)
	if !ok || challenge.TrustedFingerprint != ssh.FingerprintSHA256(oldSigner.PublicKey()) {
		t.Fatalf("unexpected changed host key challenge: %#v", serviceErr.Details)
	}
	if _, err := svc.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{
		ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256,
	}, false); err == nil {
		t.Fatal("changed Host Key was accepted without explicit rotation")
	}
	trusted, err := db.GetSSHHostKey(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if trusted.FingerprintSHA256 != ssh.FingerprintSHA256(oldSigner.PublicKey()) {
		t.Fatal("non-rotation confirmation changed the trusted Host Key")
	}
	_, err = svc.TestHost(context.Background(), host.ID)
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_CHANGED" {
		t.Fatalf("expected a fresh changed Host Key challenge, got %v", err)
	}
	challenge, ok = serviceErr.Details.(*model.SSHHostKeyChallenge)
	if !ok {
		t.Fatal("fresh Host Key rotation challenge is missing")
	}
	if _, err := svc.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{
		ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256,
	}, true); err != nil {
		t.Fatal(err)
	}
	result, err := svc.TestHost(context.Background(), host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.FingerprintSHA256 != ssh.FingerprintSHA256(server.HostKey()) {
		t.Fatalf("rotated host key mismatch: %+v", result)
	}
}

func TestSSHServiceRejectsMissingKeyWhenCredentialsExist(t *testing.T) {
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
	if _, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "saved", AuthType: "password", Secret: "secret-value"}); err != nil {
		t.Fatal(err)
	}
	svc.Close()
	if err := os.Remove((&paths.Paths{DataDir: root}).SSHSecretKeyPath()); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, nil); !errors.Is(err, ErrSSHSecretKeyUnavailable) {
		t.Fatalf("expected missing SSH secret key failure, got %v", err)
	}
}

func TestSSHHostKeyRotationRejectsStaleChallenge(t *testing.T) {
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
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "saved", AuthType: "password", Secret: "secret-value"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "host", Host: "localhost", Port: 22, Username: "tester", CredentialID: credential.ID,
		ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.UpsertSSHHostKey(&model.SSHHostKey{HostID: host.ID, KeyType: "ssh-ed25519", PublicKey: "key-a", FingerprintSHA256: "fingerprint-a"}); err != nil {
		t.Fatal(err)
	}
	challengeID := "stale-challenge"
	svc.challenges[challengeID] = sshHostKeyChallenge{
		SSHHostKeyChallenge: model.SSHHostKeyChallenge{
			ChallengeID: challengeID, KeyType: "ssh-ed25519", FingerprintSHA256: "fingerprint-b",
			TrustedFingerprint: "fingerprint-a", ExpiresAt: time.Now().Add(time.Minute).UnixMilli(),
		},
		HostID: host.ID, HostAddress: "localhost:22", PublicKey: "key-b",
	}
	if err := db.UpsertSSHHostKey(&model.SSHHostKey{HostID: host.ID, KeyType: "ssh-ed25519", PublicKey: "key-c", FingerprintSHA256: "fingerprint-c"}); err != nil {
		t.Fatal(err)
	}
	_, err = svc.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{ChallengeID: challengeID, FingerprintSHA256: "fingerprint-b"}, true)
	var serviceErr *SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_CHALLENGE_INVALID" {
		t.Fatalf("expected stale challenge rejection, got %v", err)
	}
	trusted, err := db.GetSSHHostKey(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if trusted.FingerprintSHA256 != "fingerprint-c" {
		t.Fatalf("stale challenge overwrote current trust: %+v", trusted)
	}
}

func TestSSHSessionReservationsEnforceLimits(t *testing.T) {
	svc := &SSHHostService{sessions: make(map[string]*managedSSHSession), pendingByHost: make(map[int64]int)}
	if err := svc.reserveSessionSlot(1); err != nil {
		t.Fatal(err)
	}
	if err := svc.reserveSessionSlot(1); err != nil {
		t.Fatal(err)
	}
	if err := svc.reserveSessionSlot(1); err == nil {
		t.Fatal("expected per-host session reservation limit")
	}
	if err := svc.reserveSessionSlot(2); err != nil {
		t.Fatal(err)
	}
	if err := svc.reserveSessionSlot(3); err != nil {
		t.Fatal(err)
	}
	if err := svc.reserveSessionSlot(4); err == nil {
		t.Fatal("expected global session reservation limit")
	}
	svc.releaseSessionSlot(1)
	if err := svc.reserveSessionSlot(4); err != nil {
		t.Fatalf("released slot was not reusable: %v", err)
	}
}

func TestSSHSessionTerminationPreventsAttachCommit(t *testing.T) {
	managed := &managedSSHSession{done: make(chan struct{}), input: make(chan []byte, 1), terminated: true}
	if managed.commitAttach(&ssh.Session{}, nil) || managed.session != nil {
		t.Fatal("terminated session accepted a late attach commit")
	}
}

type testSSHServer struct {
	listener      net.Listener
	config        *ssh.ServerConfig
	hostKey       ssh.PublicKey
	echoShell     bool
	sftpRoot      string
	windowChanges atomic.Int64
	connections   atomic.Int64
	done          chan struct{}
	once          sync.Once
}

func newTestSSHServer(t *testing.T, password string) *testSSHServer {
	return newConfiguredTestSSHServer(t, testSSHServerOptions{password: password, echoShell: true})
}

func newPublicKeyTestSSHServer(t *testing.T, publicKey ssh.PublicKey) *testSSHServer {
	return newConfiguredTestSSHServer(t, testSSHServerOptions{publicKey: publicKey, echoShell: true})
}

func newSFTPTestSSHServer(t *testing.T, password, root string) *testSSHServer {
	return newConfiguredTestSSHServer(t, testSSHServerOptions{password: password, echoShell: true, sftpRoot: root})
}

type testSSHServerOptions struct {
	password  string
	publicKey ssh.PublicKey
	echoShell bool
	sftpRoot  string
}

func newConfiguredTestSSHServer(t *testing.T, options testSSHServerOptions) *testSSHServer {
	t.Helper()
	signer := newTestSSHSigner(t)
	config := &ssh.ServerConfig{}
	if options.password != "" {
		config.PasswordCallback = func(metadata ssh.ConnMetadata, content []byte) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && string(content) == options.password {
				return nil, nil
			}
			return nil, errors.New("authentication failed")
		}
	}
	if options.publicKey != nil {
		config.PublicKeyCallback = func(metadata ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && sshKeysEqual(options.publicKey, key) {
				return nil, nil
			}
			return nil, errors.New("authentication failed")
		}
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &testSSHServer{
		listener: listener, config: config, hostKey: signer.PublicKey(),
		echoShell: options.echoShell, sftpRoot: options.sftpRoot, done: make(chan struct{}),
	}
	go server.serve()
	return server
}

func newTestSSHSigner(t *testing.T) ssh.Signer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signer
}

func (server *testSSHServer) Port() int {
	_, port, _ := net.SplitHostPort(server.listener.Addr().String())
	value, _ := strconv.Atoi(port)
	return value
}

func (server *testSSHServer) HostKey() ssh.PublicKey {
	return server.hostKey
}

func (server *testSSHServer) WindowChangeCount() int64 {
	return server.windowChanges.Load()
}

func (server *testSSHServer) ConnectionCount() int64 {
	return server.connections.Load()
}

func (server *testSSHServer) Close() {
	server.once.Do(func() {
		close(server.done)
		_ = server.listener.Close()
	})
}

func (server *testSSHServer) serve() {
	for {
		conn, err := server.listener.Accept()
		if err != nil {
			return
		}
		server.connections.Add(1)
		go server.serveConnection(conn)
	}
}

func (server *testSSHServer) serveConnection(conn net.Conn) {
	serverConn, channels, requests, err := ssh.NewServerConn(conn, server.config)
	if err != nil {
		_ = conn.Close()
		return
	}
	defer serverConn.Close()
	go ssh.DiscardRequests(requests)
	for channel := range channels {
		if channel.ChannelType() != "session" {
			_ = channel.Reject(ssh.UnknownChannelType, "unsupported")
			continue
		}
		accepted, requests, err := channel.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer accepted.Close()
			var shellOnce sync.Once
			for request := range requests {
				supported := request.Type == "pty-req" || request.Type == "shell" || request.Type == "window-change"
				if request.Type == "subsystem" {
					var payload struct{ Name string }
					supported = ssh.Unmarshal(request.Payload, &payload) == nil && payload.Name == "sftp" && server.sftpRoot != ""
					_ = request.Reply(supported, nil)
					if supported {
						sftpServer, err := sftp.NewServer(accepted, sftp.WithServerWorkingDirectory(server.sftpRoot))
						if err == nil {
							_ = sftpServer.Serve()
							_ = sftpServer.Close()
						}
					}
					return
				}
				_ = request.Reply(supported, nil)
				if request.Type == "shell" && server.echoShell {
					shellOnce.Do(func() { go serveTestSSHEchoShell(accepted) })
				}
				if request.Type == "window-change" {
					server.windowChanges.Add(1)
				}
			}
		}()
	}
}

func serveTestSSHEchoShell(channel ssh.Channel) {
	_, _ = channel.Write([]byte("ready\r\n"))
	buffer := make([]byte, 4096)
	for {
		count, err := channel.Read(buffer)
		if count > 0 {
			_, _ = channel.Write(buffer[:count])
		}
		if err != nil {
			return
		}
	}
}

func trustTestSSHHostKey(t *testing.T, svc *SSHHostService, hostID int64) {
	t.Helper()
	_, err := svc.TestHost(context.Background(), hostID)
	var serviceErr *SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_UNKNOWN" {
		t.Fatalf("expected unknown host key, got %v", err)
	}
	challenge, ok := serviceErr.Details.(*model.SSHHostKeyChallenge)
	if !ok {
		t.Fatalf("missing host key challenge: %#v", serviceErr.Details)
	}
	if _, err := svc.TrustHostKey(hostID, model.SSHHostKeyTrustRequest{
		ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256,
	}, false); err != nil {
		t.Fatal(err)
	}
}
