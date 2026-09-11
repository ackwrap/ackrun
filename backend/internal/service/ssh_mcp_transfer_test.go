package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func newMCPTransferFixture(t *testing.T) (*SSHHostService, int64, string) {
	t.Helper()
	root := t.TempDir()
	server := newSFTPTestSSHServer(t, "test-password", root)
	t.Cleanup(server.Close)
	svc, _ := newSSHHostShareTestService(t)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "transfer", AuthType: "password", Secret: "test-password"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{Name: "transfer", Host: "127.0.0.1", Port: server.Port(), Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	return svc, host.ID, root
}

type mcpPatternReader struct{ remaining, maxRead int64 }

func (reader *mcpPatternReader) Read(data []byte) (int, error) {
	reader.maxRead = max(reader.maxRead, int64(len(data)))
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	n := int(min(int64(len(data)), reader.remaining))
	for i := range data[:n] {
		data[i] = byte((reader.remaining - int64(i)) % 251)
	}
	reader.remaining -= int64(n)
	return n, nil
}

func TestSSHMCPTransferLargeRoundTrip(t *testing.T) {
	svc, hostID, _ := newMCPTransferFixture(t)
	const size = 32<<20 + 17
	source := &mcpPatternReader{remaining: size}
	request := model.SSHMCPTransferRequest{HostID: hostID, Path: "package.bin"}
	result, err := svc.UploadMCPStream(context.Background(), request, io.NopCloser(source), -1)
	if err != nil || !result.Success || result.Bytes != size {
		t.Fatalf("large upload: result=%+v err=%v", result, err)
	}
	if source.maxRead > 64<<10 {
		t.Fatalf("unbounded source read: %d", source.maxRead)
	}
	hash := sha256.New()
	_, _ = io.Copy(hash, &mcpPatternReader{remaining: size})
	wantHash := hex.EncodeToString(hash.Sum(nil))
	if result.SHA256 != wantHash {
		t.Fatal("upload checksum mismatch")
	}
	hash.Reset()
	err = svc.DownloadMCPStream(context.Background(), model.SSHMCPFileRequest{HostID: hostID, Path: request.Path}, func(_ context.Context, reader io.Reader, info os.FileInfo) error {
		if info.Size() != size {
			t.Errorf("remote size = %d", info.Size())
		}
		n, err := io.Copy(hash, reader)
		if n != size {
			t.Errorf("download size = %d", n)
		}
		return err
	})
	if err != nil || hex.EncodeToString(hash.Sum(nil)) != wantHash {
		t.Fatalf("large download checksum: %v", err)
	}
}

type mcpFailingReader struct{}

func (mcpFailingReader) Read([]byte) (int, error) { return 0, errors.New("injected read error") }

func TestSSHMCPTransferFailurePreservesDestination(t *testing.T) {
	svc, hostID, root := newMCPTransferFixture(t)
	for _, test := range []struct {
		name, checksum, code string
		size                 int64
		reader               io.Reader
	}{
		{"length", "", "SSH_MCP_TRANSFER_INCOMPLETE", 100, strings.NewReader("partial")},
		{"checksum", strings.Repeat("0", 64), "SSH_MCP_CHECKSUM_MISMATCH", 7, strings.NewReader("partial")},
		{"read error", "", "SSH_MCP_TRANSFER_FAILED", -1, io.MultiReader(strings.NewReader("partial"), mcpFailingReader{})},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "target"), []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}
			result, err := svc.UploadMCPStream(context.Background(), model.SSHMCPTransferRequest{HostID: hostID, Path: "target", Overwrite: true, SHA256: test.checksum}, io.NopCloser(test.reader), test.size)
			if sshServiceCode(err) != test.code || result.Success {
				t.Fatalf("failure result=%+v err=%v", result, err)
			}
			data, err := os.ReadFile(filepath.Join(root, "target"))
			if err != nil || string(data) != "original" {
				t.Fatalf("original changed: %v", err)
			}
			partials, _ := filepath.Glob(filepath.Join(root, ".ackwrap-mcp-*"))
			if len(partials) != 0 {
				t.Fatal("temporary upload was not removed")
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	defer writer.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := svc.UploadMCPStream(ctx, model.SSHMCPTransferRequest{HostID: hostID, Path: "target", Overwrite: true}, reader, -1)
		finished <- err
	}()
	if _, err := writer.Write([]byte("partial")); err != nil {
		t.Fatal(err)
	}
	cancel()
	select {
	case err := <-finished:
		if sshServiceCode(err) != "SSH_MCP_CANCELLED" {
			t.Fatalf("cancelled result = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not unblock the upload")
	}
	data, _ := os.ReadFile(filepath.Join(root, "target"))
	if string(data) != "original" {
		t.Fatal("cancelled upload replaced original")
	}
}

func TestSSHMCPTransferSourceURLAndPublish(t *testing.T) {
	svc, hostID, root := newMCPTransferFixture(t)
	content := bytes.Repeat([]byte{0, 0xff, 0x80, 3}, 1<<20)
	hash := sha256.Sum256(content)
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("authorization leaked to source")
		}
		if r.URL.Path == "/redirect" {
			http.Redirect(w, r, "/file", http.StatusFound)
			return
		}
		if r.URL.Path != "/file" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write(content)
	}))
	defer source.Close()
	request := model.SSHMCPUploadRequest{SSHMCPTransferRequest: model.SSHMCPTransferRequest{HostID: hostID, Path: "url.bin", SHA256: hex.EncodeToString(hash[:])}, SourceURL: source.URL + "/redirect"}
	result, err := svc.uploadMCPFromURL(context.Background(), request.SSHMCPTransferRequest, request.SourceURL, source.Client())
	if err != nil || !result.Success {
		t.Fatalf("URL upload: %v", err)
	}
	actual, err := os.ReadFile(filepath.Join(root, "url.bin"))
	if err != nil || !bytes.Equal(actual, content) {
		t.Fatalf("URL contents: %v", err)
	}
	request.Overwrite = true
	if _, err := svc.uploadMCPFromURL(context.Background(), request.SSHMCPTransferRequest, request.SourceURL, source.Client()); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	for _, sourceURL := range []string{"file:///file", "http://user:password@127.0.0.1/file", source.URL + "/file"} {
		request.SourceURL = sourceURL
		if _, err := svc.UploadMCP(context.Background(), request); sshServiceCode(err) != "SSH_MCP_INVALID" {
			t.Fatalf("invalid/private source result = %v", err)
		}
	}
	request.SourceURL = source.URL + "/missing"
	if _, err := svc.uploadMCPFromURL(context.Background(), request.SSHMCPTransferRequest, request.SourceURL, source.Client()); sshServiceCode(err) != "SSH_MCP_SOURCE_FAILED" {
		t.Fatalf("failed source result = %v", err)
	}
	request.SourceURL = ""
	prepared, err := svc.UploadMCP(context.Background(), request)
	if err != nil || prepared.(map[string]any)["status"] != "ready" {
		t.Fatalf("prepare: %v", err)
	}

	reader, writer := io.Pipe()
	finished := make(chan error, 1)
	go func() {
		_, err := svc.UploadMCPStream(context.Background(), model.SSHMCPTransferRequest{HostID: hostID, Path: "race.bin"}, reader, -1)
		finished <- err
	}()
	if _, err := writer.Write([]byte("incoming")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "race.bin"), []byte("concurrent"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = writer.Close()
	if err := <-finished; err == nil {
		t.Fatal("upload overwrote a concurrently created destination")
	}
	actual, _ = os.ReadFile(filepath.Join(root, "race.bin"))
	if string(actual) != "concurrent" {
		t.Fatal("concurrent file was replaced")
	}
}

func TestMCPSourceHTTPClientRejectsPrivateAddresses(t *testing.T) {
	for _, address := range []string{
		"127.0.0.1", "::1", "10.0.0.1", "fd00::1", "169.254.169.254", "fe80::1", "100.64.0.1", "0.0.0.0", "ff02::1",
	} {
		if !isDisallowedMCPSourceAddress(netip.MustParseAddr(address)) {
			t.Fatalf("private or special address %s was allowed", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if isDisallowedMCPSourceAddress(netip.MustParseAddr(address)) {
			t.Fatalf("public address %s was rejected", address)
		}
	}

	var requested atomic.Bool
	private := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { requested.Store(true) }))
	defer private.Close()
	response, err := newMCPSourceHTTPClient().Get(private.URL)
	if response != nil {
		response.Body.Close()
	}
	if err == nil || requested.Load() {
		t.Fatal("MCP source client connected to a loopback address")
	}
}

func TestMCPSourceHTTPClientRejectsPrivateRedirect(t *testing.T) {
	var secretRequested atomic.Bool
	var private *httptest.Server
	private = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/redirect" {
			http.Redirect(w, request, private.URL+"/secret", http.StatusFound)
			return
		}
		secretRequested.Store(true)
		_, _ = w.Write([]byte("secret"))
	}))
	defer private.Close()
	_, port, err := net.SplitHostPort(private.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	dialer := &net.Dialer{}
	expectedDialAddress := net.JoinHostPort("8.8.8.8", port)
	var dialCalls atomic.Int32
	client := newMCPSourceHTTPClientWithDial(
		func(ctx context.Context, network, host string) ([]netip.Addr, error) {
			if host == "public.example" {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
			}
			return net.DefaultResolver.LookupNetIP(ctx, network, host)
		},
		func(ctx context.Context, network, address string) (net.Conn, error) {
			dialCalls.Add(1)
			if address != expectedDialAddress {
				return nil, errors.New("source client dialed an unverified address")
			}
			return dialer.DialContext(ctx, network, private.Listener.Addr().String())
		},
	)
	response, err := client.Get("http://public.example:" + port + "/redirect")
	if response != nil {
		response.Body.Close()
	}
	if err == nil || secretRequested.Load() || dialCalls.Load() != 1 {
		t.Fatal("MCP source client followed a redirect to a loopback address")
	}
}
