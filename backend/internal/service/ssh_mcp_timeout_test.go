package service

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
)

func newMCPTimeoutTestHost(t *testing.T, server *testSSHServer, mode string) (*SSHHostService, *model.SSHHost) {
	t.Helper()
	var svc *SSHHostService
	var host *model.SSHHost
	if mode != "direct" {
		proxy := newTestSSHProxy(t, mode, "proxy-user", "proxy-password")
		t.Cleanup(proxy.Close)
		svc, host = setupNodeExposureSSHHost(t, server.Port(), mode, proxy.Port(), true)
	} else {
		svc, _ = newSSHHostShareTestService(t)
		credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "timeout fixture", AuthType: "password", Secret: "correct-password"})
		if err != nil {
			t.Fatal(err)
		}
		host, err = svc.CreateHost(model.SSHHostRequest{Name: "timeout fixture", Host: "127.0.0.1", Port: server.Port(), Username: "tester", CredentialID: credential.ID, Enabled: true})
		if err != nil {
			t.Fatal(err)
		}
	}
	// Seed the synthetic server's trusted key without starting an unrelated SFTP probe.
	key := server.HostKey()
	if err := svc.store.UpsertSSHHostKey(&model.SSHHostKey{HostID: host.ID, KeyType: key.Type(), PublicKey: string(ssh.MarshalAuthorizedKey(key)), FingerprintSHA256: ssh.FingerprintSHA256(key)}); err != nil {
		t.Fatal(err)
	}
	return svc, host
}

func TestSSHMCPExecWaitsBeyondDefaultConnectionTimeout(t *testing.T) {
	for _, mode := range []string{"direct", "socks", "http"} {
		for _, stage := range []string{"handshake", "session"} {
			t.Run(mode+"/"+stage, func(t *testing.T) {
				t.Parallel()
				options := testSSHServerOptions{password: "correct-password", exec: func(channel ssh.Channel, _ string) {
					_, _ = io.WriteString(channel, "connected")
					_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
				}}
				if stage == "handshake" {
					options.handshakeDelay = sshConnectTimeout + time.Second
				} else {
					options.sessionDelay = sshConnectTimeout + time.Second
				}
				server := newConfiguredTestSSHServer(t, options)
				t.Cleanup(server.Close)
				svc, host := newMCPTimeoutTestHost(t, server, mode)
				result, err := svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: host.ID, Command: "fixture", Timeout: 20})
				if err != nil || result.ExitCode != 0 || result.Stdout != "connected" {
					t.Fatalf("explicit timeout was shortened during %s: result=%+v err=%v", stage, result, err)
				}
			})
		}
	}
}

func TestSSHMCPExecConnectionAndSessionTimeouts(t *testing.T) {
	for _, test := range []struct {
		name           string
		handshakeDelay time.Duration
		sessionDelay   time.Duration
		code           string
	}{
		{name: "handshake", handshakeDelay: 2 * time.Second, code: "SSH_CONNECT_TIMEOUT"},
		{name: "shared budget", handshakeDelay: 600 * time.Millisecond, sessionDelay: 600 * time.Millisecond, code: "SSH_SESSION_OPEN_TIMEOUT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var executed atomic.Bool
			server := newConfiguredTestSSHServer(t, testSSHServerOptions{password: "correct-password", handshakeDelay: test.handshakeDelay, sessionDelay: test.sessionDelay, exec: func(ssh.Channel, string) { executed.Store(true) }})
			t.Cleanup(server.Close)
			svc, host := newMCPTimeoutTestHost(t, server, "direct")
			started := time.Now()
			result, err := svc.ExecuteMCP(context.Background(), model.SSHMCPExecRequest{HostID: host.ID, Command: "fixture", Timeout: 1})
			if sshServiceCode(err) != test.code || executed.Load() || result.ExitCode != -1 || time.Since(started) > 3*time.Second {
				t.Fatalf("unexpected timeout phase or deadline: result=%+v err=%v executed=%v", result, err, executed.Load())
			}
			if test.name == "handshake" && !strings.Contains(err.Error(), "握手") {
				t.Fatalf("handshake timeout lost phase: %v", err)
			}
			svc.sessionMu.Lock()
			defer svc.sessionMu.Unlock()
			if svc.pendingSessions != 0 || len(svc.mcpCancels) != 0 {
				t.Fatal("timed out operation leaked a session reservation")
			}
		})
	}
}

func TestSSHMCPExecCallerDeadlineAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		t.Run(map[bool]string{false: "earlier deadline", true: "caller cancellation"}[cancelled], func(t *testing.T) {
			server := newConfiguredTestSSHServer(t, testSSHServerOptions{password: "correct-password", handshakeDelay: 2 * time.Second})
			t.Cleanup(server.Close)
			svc, host := newMCPTimeoutTestHost(t, server, "direct")
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			if cancelled {
				stop := time.AfterFunc(50*time.Millisecond, cancel)
				defer stop.Stop()
			}
			started := time.Now()
			_, err := svc.ExecuteMCP(ctx, model.SSHMCPExecRequest{HostID: host.ID, Command: "fixture", Timeout: 120})
			code := "SSH_CONNECT_TIMEOUT"
			if cancelled {
				code = "SSH_MCP_CANCELLED"
			}
			if sshServiceCode(err) != code || time.Since(started) > time.Second {
				t.Fatalf("caller limit was ignored: %v after %s", err, time.Since(started))
			}
		})
	}
}

func TestSSHConnectionTimeoutStageMessages(t *testing.T) {
	for _, test := range []struct {
		stage    string
		message  string
		classify func(context.Context, error) error
	}{
		{stage: "tcp_connect", message: "TCP", classify: classifySSHDialError},
		{stage: "proxy_connect", message: "节点入口", classify: classifySSHProxyError},
		{stage: "handshake", message: "握手", classify: classifySSHConnectError},
	} {
		t.Run(test.stage, func(t *testing.T) {
			err := test.classify(context.Background(), &net.DNSError{IsTimeout: true})
			var serviceErr *SSHServiceError
			if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_CONNECT_TIMEOUT" || !strings.Contains(serviceErr.Message, test.message) {
				t.Fatalf("timeout phase was lost: %v", err)
			}
			if serviceErr.Details.(map[string]string)["stage"] != test.stage {
				t.Fatalf("unexpected timeout details: %#v", serviceErr.Details)
			}
		})
	}
}
