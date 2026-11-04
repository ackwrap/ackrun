package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func waitTestSFTPProbe(t *testing.T, svc *SSHHostService, hostID int64) *model.SSHSFTPProfile {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		svc.sessionMu.Lock()
		active := svc.sftpProbes[hostID] != nil
		svc.sessionMu.Unlock()
		p, err := svc.store.GetSSHSFTPProfile(hostID)
		if err != nil {
			t.Fatal(err)
		}
		if !active && p != nil && p.Status != "probing" {
			return p
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("SFTP probe did not finish")
	return nil
}

// A real pkg/sftp server behind a packet-aware channel. It can reject oversized
// WRITE requests by disconnecting, independently advertise OpenSSH limits, and
// change its limit between uploads. All addresses and data are synthetic.
type testSFTPLimits struct {
	writeLimit atomic.Int64
	maxWrite   atomic.Int64
	writes     atomic.Int64
	advertise  bool
	packetMax  uint64
	writeMax   uint64
	malformed  bool
}

type limitedTestSFTPChannel struct {
	ssh.Channel
	limits  *testSFTPLimits
	pending []byte
	writeMu sync.Mutex
}

func (c *limitedTestSFTPChannel) Read(data []byte) (int, error) {
	for len(c.pending) == 0 {
		var size uint32
		if err := binary.Read(c.Channel, binary.BigEndian, &size); err != nil {
			return 0, err
		}
		if size < 1 || size > 1<<20 {
			return 0, io.ErrUnexpectedEOF
		}
		packet := make([]byte, size)
		if _, err := io.ReadFull(c.Channel, packet); err != nil {
			return 0, err
		}
		if packet[0] == 200 && c.limits.advertise {
			var request struct {
				Type      byte
				ID        uint32
				Extension string
			}
			if ssh.Unmarshal(packet, &request) == nil && request.Extension == "limits@openssh.com" {
				reply := ssh.Marshal(struct {
					Type                         byte
					ID                           uint32
					Packet, Read, Write, Handles uint64
				}{201, request.ID, c.limits.packetMax, 32 << 10, c.limits.writeMax, 64})
				if c.limits.malformed {
					reply = reply[:6]
				}
				c.writeMu.Lock()
				err := writeSFTPProbePacket(c.Channel, reply)
				c.writeMu.Unlock()
				if err != nil {
					return 0, err
				}
				continue
			}
		}
		if packet[0] == 6 {
			var write struct {
				Type   byte
				ID     uint32
				Handle string
				Offset uint64
				Data   string
			}
			if err := ssh.Unmarshal(packet, &write); err != nil {
				return 0, err
			}
			n := int64(len(write.Data))
			c.limits.writes.Add(1)
			for previous := c.limits.maxWrite.Load(); n > previous; previous = c.limits.maxWrite.Load() {
				if c.limits.maxWrite.CompareAndSwap(previous, n) {
					break
				}
			}
			if n > c.limits.writeLimit.Load() {
				return 0, io.EOF
			}
		}
		c.pending = make([]byte, 4+len(packet))
		binary.BigEndian.PutUint32(c.pending, size)
		copy(c.pending[4:], packet)
	}
	n := copy(data, c.pending)
	c.pending = c.pending[n:]
	return n, nil
}

func (c *limitedTestSFTPChannel) Write(data []byte) (int, error) {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.limits.advertise && len(data) >= 9 && data[4] == 2 {
		packet := append([]byte(nil), data[4:]...)
		packet = append(packet, ssh.Marshal(struct{ Name, Version string }{"limits@openssh.com", "1"})...)
		return len(data), writeSFTPProbePacket(c.Channel, packet)
	}
	return c.Channel.Write(data)
}

func newLimitedSFTPFixture(t *testing.T, limits *testSFTPLimits) (*SSHHostService, int64, string, *testSSHServer) {
	t.Helper()
	root := t.TempDir()
	server := newConfiguredTestSSHServer(t, testSSHServerOptions{password: "test-password", sftpHandler: func(channel ssh.Channel) {
		files, err := sftp.NewServer(&limitedTestSFTPChannel{Channel: channel, limits: limits}, sftp.WithServerWorkingDirectory(root))
		if err == nil {
			_ = files.Serve()
			_ = files.Close()
		}
	}})
	t.Cleanup(server.Close)
	svc, _ := newSSHHostShareTestService(t)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "probe", AuthType: "password", Secret: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{Name: "probe", Host: "127.0.0.1", Port: server.Port(), Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	return svc, host.ID, root, server
}

func assertNoSFTPProbeFiles(t *testing.T, root string) {
	t.Helper()
	names, err := filepath.Glob(filepath.Join(root, ".ackwrap-sftp-probe-*"))
	if err != nil || len(names) != 0 {
		t.Fatalf("leftover probes: %d err=%v", len(names), err)
	}
}

func TestSSHSFTPProbeTrustPersistenceAndReuse(t *testing.T) {
	limits := &testSFTPLimits{advertise: true, writeMax: 128 << 10, packetMax: 256 << 10}
	limits.writeLimit.Store(128 << 10)
	svc, hostID, root, server := newLimitedSFTPFixture(t, limits)
	p := waitTestSFTPProbe(t, svc, hostID)
	if p.Status != "ready" || p.PacketSize != 128<<10 || limits.maxWrite.Load() != 128<<10 {
		t.Fatalf("advertised profile: %+v max_write=%d", p, limits.maxWrite.Load())
	}
	assertNoSFTPProbeFiles(t, root)
	var sequence int
	var databaseName, databasePath string
	if err := svc.store.DB().QueryRow(`PRAGMA database_list`).Scan(&sequence, &databaseName, &databasePath); err != nil {
		t.Fatal(err)
	}
	svc.Close()
	if err := svc.store.Close(); err != nil {
		t.Fatal(err)
	}
	// Reopen the actual database and reload credentials from disk, as at restart.
	db, err := store.Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	restarted, err := NewSSHHostService(db, &paths.Paths{DataDir: filepath.Dir(databasePath)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(restarted.Close)
	before := server.ConnectionCount()
	for i, name := range []string{"first.bin", "second.bin"} {
		reader := &mcpPatternReader{remaining: 1 << 20}
		result, err := restarted.UploadMCPStream(context.Background(), model.SSHMCPTransferRequest{HostID: hostID, Path: name}, io.NopCloser(reader), -1)
		if err != nil || !result.Success || reader.maxRead != 128<<10 {
			t.Fatalf("cached upload %d: %+v max_read=%d err=%v", i, result, reader.maxRead, err)
		}
	}
	after, err := db.GetSSHSFTPProfile(hostID)
	if err != nil || after.Generation != p.Generation || server.ConnectionCount()-before != 2 {
		t.Fatalf("uploads renegotiated profile: %+v err=%v", after, err)
	}
	assertNoSFTPProbeFiles(t, root)
}

func TestSSHSFTPProbeFindsRejectedLimitAndCleansFiles(t *testing.T) {
	limits := &testSFTPLimits{}
	limits.writeLimit.Store(96 << 10)
	svc, hostID, root, _ := newLimitedSFTPFixture(t, limits)
	p := waitTestSFTPProbe(t, svc, hostID)
	if p.Status != "ready" || p.PacketSize != 96<<10 || limits.maxWrite.Load() <= 96<<10 {
		t.Fatalf("probed profile: %+v max_write=%d", p, limits.maxWrite.Load())
	}
	assertNoSFTPProbeFiles(t, root)
}

func TestSSHSFTPProbeUploadFailureReprobes(t *testing.T) {
	limits := &testSFTPLimits{}
	limits.writeLimit.Store(128 << 10)
	svc, hostID, root, _ := newLimitedSFTPFixture(t, limits)
	before := waitTestSFTPProbe(t, svc, hostID)
	limits.writeLimit.Store(32 << 10)
	if err := os.WriteFile(filepath.Join(root, "target.bin"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := model.SSHMCPTransferRequest{HostID: hostID, Path: "target.bin", Overwrite: true}
	_, err := svc.UploadMCPStream(context.Background(), request, io.NopCloser(bytes.NewReader(make([]byte, 512<<10))), -1)
	if err == nil {
		t.Fatal("oversized upload unexpectedly succeeded")
	}
	after := waitTestSFTPProbe(t, svc, hostID)
	if after.Generation == before.Generation || after.PacketSize != 32<<10 || after.Status != "ready" {
		t.Fatalf("reprobe: %+v", after)
	}
	content, err := os.ReadFile(filepath.Join(root, "target.bin"))
	if err != nil || string(content) != "original" {
		t.Fatal("failed upload replaced original")
	}
	result, err := svc.UploadMCPStream(context.Background(), request, io.NopCloser(strings.NewReader("replacement")), -1)
	if err != nil || !result.Success {
		t.Fatalf("upload after reprobe: %+v %v", result, err)
	}
	assertNoSFTPProbeFiles(t, root)
}

func TestSSHSFTPProbeFallbackIsNotRepeated(t *testing.T) {
	limits := &testSFTPLimits{advertise: true, malformed: true}
	limits.writeLimit.Store(32 << 10)
	svc, hostID, root, server := newLimitedSFTPFixture(t, limits)
	p := waitTestSFTPProbe(t, svc, hostID)
	if p.Status != "fallback" || p.PacketSize != 32<<10 || limits.writes.Load() != 0 {
		t.Fatalf("fallback: %+v writes=%d", p, limits.writes.Load())
	}
	before := server.ConnectionCount()
	for range 2 {
		_, size, concurrency, err := svc.sftpUploadProfile(hostID)
		if err != nil || size != 32<<10 || concurrency != sshMCPUploadConcurrency {
			t.Fatalf("fallback settings: %d %d %v", size, concurrency, err)
		}
	}
	if server.ConnectionCount() != before {
		t.Fatal("fallback reprobed without transfer failure")
	}
	assertNoSFTPProbeFiles(t, root)
}

func TestSSHSFTPProbeLimitsAndInvalidReplies(t *testing.T) {
	for _, tc := range []struct {
		name          string
		packet, write uint64
		want          int
	}{
		{"write", 256 << 10, 64 << 10, 64 << 10},
		{"packet_overhead", 128 << 10, 128 << 10, (128 << 10) - 1024},
		{"unspecified", 0, 0, sshSFTPMaxPacket},
	} {
		t.Run(tc.name, func(t *testing.T) {
			limits := &testSFTPLimits{advertise: true, packetMax: tc.packet, writeMax: tc.write}
			limits.writeLimit.Store(sshSFTPMaxPacket)
			svc, hostID, root, _ := newLimitedSFTPFixture(t, limits)
			p := waitTestSFTPProbe(t, svc, hostID)
			// pkg/sftp's test server also caps total request bytes at 256 KiB.
			if p.Status != "ready" || p.PacketSize > tc.want || tc.want-p.PacketSize > 4<<10 {
				t.Fatalf("bounded profile: %+v want<=%d", p, tc.want)
			}
			assertNoSFTPProbeFiles(t, root)
		})
	}
	for _, data := range [][]byte{{0, 0, 0, 0}, {0, 2, 0, 0}, {0, 0, 0, 5, 2}} {
		if _, err := readSFTPProbePacket(bytes.NewReader(data)); err == nil {
			t.Fatal("accepted malformed frame")
		}
	}
}

func TestSSHSFTPProbeDeduplicationAndShutdown(t *testing.T) {
	svc, hostID, _ := newMCPTransferFixture(t)
	p := waitTestSFTPProbe(t, svc, hostID)
	// Occupy both host slots, so a scheduled probe waits without dialing.
	for range sshMaximumHostSessions {
		if err := svc.reserveSessionSlot(hostID); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { svc.scheduleSFTPProbe(hostID, p.Generation) })
	}
	wg.Wait()
	pending, err := svc.store.GetSSHSFTPProfile(hostID)
	if err != nil || pending.Status != "probing" || pending.Generation == p.Generation {
		t.Fatalf("pending profile: %+v %v", pending, err)
	}
	svc.scheduleSFTPProbe(hostID, p.Generation)
	again, _ := svc.store.GetSSHSFTPProfile(hostID)
	if again.Generation != pending.Generation {
		t.Fatal("duplicate probe replaced active generation")
	}
	started := time.Now()
	svc.Close()
	if time.Since(started) > time.Second {
		t.Fatal("shutdown did not cancel waiting probe")
	}
	for range sshMaximumHostSessions {
		svc.releaseSessionSlot(hostID)
	}
}

func TestSSHSFTPProbeRejectsUnrelatedFailures(t *testing.T) {
	for _, err := range []error{nil, os.ErrPermission, os.ErrNotExist, context.Canceled, context.DeadlineExceeded, &sftp.StatusError{Code: 4}, &sftp.StatusError{Code: 3}} {
		if shouldReprobeSFTP(err) {
			t.Fatalf("unrelated failure triggers probe: %v", err)
		}
	}
	for _, err := range []error{io.EOF, sftp.ErrSSHFxConnectionLost, &sftp.StatusError{Code: 5}, errors.New("connection reset")} {
		if !shouldReprobeSFTP(err) {
			t.Fatalf("transport failure does not trigger probe: %v", err)
		}
	}
}
