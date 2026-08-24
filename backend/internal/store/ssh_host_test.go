package store

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSSHCredentialAndHostReferences(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	credential := &model.SSHCredential{
		Name: "test credential", AuthType: "password", SecretContext: "context-1",
		SecretCiphertext: []byte{1, 2, 3}, SecretNonce: []byte{4, 5, 6}, KeyVersion: 1,
	}
	if err := db.CreateSSHCredential(credential); err != nil {
		t.Fatal(err)
	}
	host := &model.SSHHost{
		Name: "test host", Host: "localhost", Port: 22, Username: "tester",
		CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm-256color",
		Enabled: true, Tags: []string{"test"},
	}
	if err := db.CreateSSHHost(host); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSSHCredential(credential.ID); !errors.Is(err, ErrSSHReferenceInUse) {
		t.Fatalf("expected credential reference protection, got %v", err)
	}
	loaded, err := db.GetSSHHost(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.CredentialName != credential.Name || len(loaded.Tags) != 1 || loaded.Tags[0] != "test" {
		t.Fatalf("unexpected SSH host round trip: %+v", loaded)
	}
	if err := db.DeleteSSHHost(host.ID); err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteSSHCredential(credential.ID); err != nil {
		t.Fatal(err)
	}
}

func TestSSHCredentialCiphertextStoredAsBlob(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ciphertext := []byte{0, 1, 2, 3, 255}
	credential := &model.SSHCredential{
		Name: "blob credential", AuthType: "password", SecretContext: "context-2",
		SecretCiphertext: ciphertext, SecretNonce: []byte{4, 5, 6}, KeyVersion: 1,
	}
	if err := db.CreateSSHCredential(credential); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := db.DB().QueryRow(`SELECT secret_ciphertext FROM ssh_credentials WHERE id = ?`, credential.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(stored, ciphertext) {
		t.Fatalf("ciphertext mismatch: %v", stored)
	}
}

func TestCreateSSHHostsWithCredentialsRollsBackBatch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	credential := &model.SSHCredential{
		Name: "batch credential", AuthType: "password", SecretContext: "batch-context",
		SecretCiphertext: []byte{1}, SecretNonce: []byte{2}, KeyVersion: 1,
	}
	hosts := []*model.SSHHost{
		{Name: "duplicate batch host", Host: "one.example.invalid", Port: 22, Username: "root", ConnectionMode: "direct", TerminalType: "xterm-256color", Tags: []string{}},
		{Name: "duplicate batch host", Host: "two.example.invalid", Port: 22, Username: "root", ConnectionMode: "direct", TerminalType: "xterm-256color", Tags: []string{}},
	}
	if err := db.CreateSSHHostsWithCredentials([]*model.SSHCredential{credential}, hosts, []int{0, 0}); err == nil {
		t.Fatal("expected duplicate host name batch to fail")
	}
	credentials, err := db.ListSSHCredentials()
	if err != nil {
		t.Fatal(err)
	}
	loadedHosts, err := db.ListSSHHosts()
	if err != nil {
		t.Fatal(err)
	}
	if len(credentials) != 0 || len(loadedHosts) != 0 {
		t.Fatal("failed SSH import batch was not rolled back")
	}
}
