package service

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHSFTPFileLifecycleAndSessionIsolation(t *testing.T) {
	remoteRoot := t.TempDir()
	server := newSFTPTestSSHServer(t, "correct-password", remoteRoot)
	defer server.Close()

	dataRoot := t.TempDir()
	db, err := store.Open(filepath.Join(dataRoot, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: dataRoot}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "sftp password", AuthType: "password", Secret: "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "sftp host", Host: "127.0.0.1", Port: server.Port(), Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	trustTestSSHHostKey(t, svc, host.ID)
	session, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{Columns: 100, Rows: 30})
	if err != nil {
		t.Fatal(err)
	}
	if session.SFTPToken == "" || session.SFTPToken == session.AttachToken {
		t.Fatal("SFTP session token is missing or reuses the one-time attach token")
	}
	if _, err := svc.ListSFTP(context.Background(), session.SessionID, "wrong-token", "."); sshServiceCode(err) != "SSH_SESSION_NOT_FOUND" {
		t.Fatalf("wrong SFTP token was not rejected: %v", err)
	}
	otherSession, err := svc.CreateSession(context.Background(), host.ID, model.SSHSessionCreateRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListSFTP(context.Background(), session.SessionID, otherSession.SFTPToken, "."); sshServiceCode(err) != "SSH_SESSION_NOT_FOUND" {
		t.Fatalf("another session's SFTP token was accepted: %v", err)
	}
	if err := svc.CloseSession(otherSession.SessionID); err != nil {
		t.Fatal(err)
	}

	listing, err := svc.ListSFTP(context.Background(), session.SessionID, session.SFTPToken, ".")
	if err != nil {
		t.Fatal(err)
	}
	directory := path.Join(listing.Home, "documents")
	if err := svc.CreateSFTPDirectory(session.SessionID, session.SFTPToken, directory); err != nil {
		t.Fatal(err)
	}
	emptyPath := path.Join(directory, "empty.txt")
	if err := svc.CreateSFTPFile(session.SessionID, session.SFTPToken, path.Join(directory, `..\escape.txt`)); sshServiceCode(err) != "SSH_SFTP_INVALID_PATH" {
		t.Fatalf("SFTP file path containing a backslash was not rejected: %v", err)
	}
	if err := svc.CreateSFTPFile(session.SessionID, session.SFTPToken, emptyPath); err != nil {
		t.Fatal(err)
	}
	if err := svc.CreateSFTPFile(session.SessionID, session.SFTPToken, emptyPath); sshServiceCode(err) != "SSH_SFTP_EXISTS" {
		t.Fatalf("duplicate SFTP file creation was not rejected: %v", err)
	}
	emptyDownload, err := svc.OpenSFTPDownload(session.SessionID, session.SFTPToken, emptyPath)
	if err != nil {
		t.Fatal(err)
	}
	if emptyDownload.Size != 0 {
		t.Fatalf("new SFTP file is not empty: %d", emptyDownload.Size)
	}
	_ = emptyDownload.Reader.Close()
	if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, emptyPath, false); err != nil {
		t.Fatal(err)
	}
	filePath := path.Join(directory, "notes.txt")
	written, err := svc.UploadSFTP(session.SessionID, session.SFTPToken, filePath, false, io.NopCloser(bytes.NewBufferString("sftp-content")))
	if err != nil {
		t.Fatal(err)
	}
	if written != int64(len("sftp-content")) {
		t.Fatalf("unexpected uploaded size: %d", written)
	}
	listing, err = svc.ListSFTP(context.Background(), session.SessionID, session.SFTPToken, directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(listing.Entries) != 1 || listing.Entries[0].Name != "notes.txt" || listing.Entries[0].IsDir {
		t.Fatalf("unexpected SFTP listing: %+v", listing.Entries)
	}
	download, err := svc.OpenSFTPDownload(session.SessionID, session.SFTPToken, filePath)
	if err != nil {
		t.Fatal(err)
	}
	content, readErr := io.ReadAll(download.Reader)
	closeErr := download.Reader.Close()
	if readErr != nil || closeErr != nil {
		t.Fatalf("download failed: read=%v close=%v", readErr, closeErr)
	}
	if string(content) != "sftp-content" {
		t.Fatalf("unexpected downloaded content: %q", content)
	}
	if _, err := svc.UploadSFTP(session.SessionID, session.SFTPToken, filePath, false, io.NopCloser(bytes.NewBufferString("replacement"))); sshServiceCode(err) != "SSH_SFTP_EXISTS" {
		t.Fatalf("unconfirmed overwrite was not rejected: %v", err)
	}
	limitedRequest := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString("streamed-content"))
	limitedRequest.ContentLength = -1
	limitedBody := http.MaxBytesReader(httptest.NewRecorder(), limitedRequest.Body, 4)
	_, err = svc.UploadSFTP(session.SessionID, session.SFTPToken, filePath, true, limitedBody)
	var maxBytesError *http.MaxBytesError
	if !errors.As(err, &maxBytesError) {
		t.Fatalf("streaming upload limit error was not preserved: %v", err)
	}
	if _, err := svc.UploadSFTP(session.SessionID, session.SFTPToken, filePath, true, io.NopCloser(&failingSSHUploadReader{})); err == nil {
		t.Fatal("interrupted overwrite unexpectedly succeeded")
	}
	download, err = svc.OpenSFTPDownload(session.SessionID, session.SFTPToken, filePath)
	if err != nil {
		t.Fatal(err)
	}
	content, err = io.ReadAll(download.Reader)
	_ = download.Reader.Close()
	if err != nil || string(content) != "sftp-content" {
		t.Fatalf("failed overwrite changed the original file: content=%q err=%v", content, err)
	}
	listing, err = svc.ListSFTP(context.Background(), session.SessionID, session.SFTPToken, directory)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range listing.Entries {
		if len(entry.Name) >= len(".ackwrap-") && entry.Name[:len(".ackwrap-")] == ".ackwrap-" {
			t.Fatalf("failed upload left a temporary file: %s", entry.Name)
		}
	}
	if _, err := svc.UploadSFTP(session.SessionID, session.SFTPToken, filePath, true, io.NopCloser(bytes.NewBufferString("replacement"))); err != nil {
		t.Fatal(err)
	}
	targetPath := path.Join(directory, "target.txt")
	if _, err := svc.UploadSFTP(session.SessionID, session.SFTPToken, targetPath, false, io.NopCloser(bytes.NewBufferString("target"))); err != nil {
		t.Fatal(err)
	}
	if err := svc.RenameSFTP(session.SessionID, session.SFTPToken, filePath, targetPath); sshServiceCode(err) != "SSH_SFTP_EXISTS" {
		t.Fatalf("rename over an existing target was not rejected: %v", err)
	}

	renamedPath := path.Join(directory, "renamed.txt")
	if err := svc.RenameSFTP(session.SessionID, session.SFTPToken, filePath, renamedPath); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, renamedPath, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, targetPath, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, directory, true); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, "/", true); sshServiceCode(err) != "SSH_SFTP_INVALID_PATH" {
		t.Fatalf("SFTP root deletion was not rejected: %v", err)
	}
	for _, unsafePath := range []string{"..", "../..", "child/../../.."} {
		if err := svc.DeleteSFTP(session.SessionID, session.SFTPToken, unsafePath, true); sshServiceCode(err) != "SSH_SFTP_INVALID_PATH" {
			t.Fatalf("unsafe parent path %q was not rejected: %v", unsafePath, err)
		}
	}
	gatedReader := &gatedSSHUploadReader{started: make(chan struct{}), closed: make(chan struct{})}
	uploadDone := make(chan error, 1)
	go func() {
		_, err := svc.UploadSFTP(
			session.SessionID, session.SFTPToken, path.Join(listing.Home, "closing.txt"), false, gatedReader,
		)
		uploadDone <- err
	}()
	select {
	case <-gatedReader.started:
	case <-time.After(time.Second):
		t.Fatal("SFTP upload did not start")
	}
	closeDone := make(chan error, 1)
	go func() { closeDone <- svc.CloseSession(session.SessionID) }()
	select {
	case <-gatedReader.closed:
	case <-time.After(time.Second):
		t.Fatal("session close did not interrupt the active upload body")
	}
	if err := <-uploadDone; err == nil {
		t.Fatal("interrupted SFTP upload unexpectedly succeeded")
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListSFTP(context.Background(), session.SessionID, session.SFTPToken, "."); sshServiceCode(err) != "SSH_SESSION_NOT_FOUND" {
		t.Fatalf("closed session still accepted SFTP requests: %v", err)
	}
}

type failingSSHUploadReader struct{ read bool }

func (reader *failingSSHUploadReader) Read(content []byte) (int, error) {
	if !reader.read {
		reader.read = true
		return copy(content, "partial"), nil
	}
	return 0, errors.New("interrupted upload")
}

type gatedSSHUploadReader struct {
	started chan struct{}
	closed  chan struct{}
	read    bool
	once    sync.Once
}

func (reader *gatedSSHUploadReader) Read(content []byte) (int, error) {
	if reader.read {
		return 0, io.EOF
	}
	reader.read = true
	close(reader.started)
	<-reader.closed
	return 0, errors.New("upload body closed")
}

func (reader *gatedSSHUploadReader) Close() error {
	reader.once.Do(func() { close(reader.closed) })
	return nil
}

func sshServiceCode(err error) string {
	var serviceErr *SSHServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code
	}
	return ""
}
