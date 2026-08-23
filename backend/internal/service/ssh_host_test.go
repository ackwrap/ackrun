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
	"testing"
	"time"

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
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "test", []byte("key-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "encrypted key", AuthType: "private_key", Secret: string(pem.EncodeToMemory(block)), Passphrase: "key-passphrase",
	})
	if err != nil {
		t.Fatal(err)
	}
	if credential.KeyFingerprint == "" {
		t.Fatal("private key fingerprint was not populated")
	}
	stored, err := db.GetSSHCredential(credential.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.authMethod(stored); err != nil {
		t.Fatalf("stored encrypted private key could not be used: %v", err)
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
	listener net.Listener
	config   *ssh.ServerConfig
	done     chan struct{}
	once     sync.Once
}

func newTestSSHServer(t *testing.T, password string) *testSSHServer {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(metadata ssh.ConnMetadata, content []byte) (*ssh.Permissions, error) {
			if metadata.User() == "tester" && string(content) == password {
				return nil, nil
			}
			return nil, errors.New("authentication failed")
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := &testSSHServer{listener: listener, config: config, done: make(chan struct{})}
	go server.serve()
	return server
}

func (server *testSSHServer) Port() int {
	_, port, _ := net.SplitHostPort(server.listener.Addr().String())
	value, _ := strconv.Atoi(port)
	return value
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
			for request := range requests {
				_ = request.Reply(request.Type == "pty-req" || request.Type == "shell", nil)
			}
		}()
	}
}
