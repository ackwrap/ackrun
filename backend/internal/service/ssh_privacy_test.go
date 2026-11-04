package service

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestMaskSSHAddress(t *testing.T) {
	for _, test := range []struct{ address, masked string }{
		{"192.0.2.53", "192.0.*.*"},
		{"::ffff:192.0.2.53", "192.0.*.*"},
		{"2001:db8::53", "2001:0db8:*"},
		{"[fe80::53%eth0]", "fe80:0000:*"},
		{"::1", "0000:0000:*"},
		{"server.example.com", "***"},
	} {
		t.Run(test.address, func(t *testing.T) {
			if got := maskSSHAddress(test.address); got != test.masked {
				t.Fatalf("mask = %q, want %q", got, test.masked)
			}
		})
	}
}

func TestSSHPrivacyPreservesSavedHostsAndConnections(t *testing.T) {
	server := newTestSSHServer(t, "synthetic-password")
	defer server.Close()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	settings, err := svc.PrivacySettings()
	if err != nil || settings.HideHostAddress {
		t.Fatalf("unexpected privacy default: %v", err)
	}
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{Name: "fixture", AuthType: "password", Secret: "synthetic-password"})
	if err != nil {
		t.Fatal(err)
	}
	request := model.SSHHostRequest{
		Name: "fixture 127.0.0.1", Host: "127.0.0.1", Port: server.Port(), Username: "tester",
		CredentialID: credential.ID, Enabled: true, Notes: "address: 127.0.0.1", Tags: []string{"127.0.0.1"},
	}
	host, err := svc.CreateHost(request)
	if err != nil || host.Host != request.Host || host.HostHidden {
		t.Fatalf("unexpected visible host: %v", err)
	}
	if _, err := svc.UpdatePrivacySettings(model.SSHPrivacySettings{HideHostAddress: true}); err != nil {
		t.Fatal(err)
	}
	settings, err = db.GetSSHPrivacySettings()
	if err != nil || !settings.HideHostAddress {
		t.Fatalf("privacy was not persisted: %v", err)
	}
	visible, err := svc.GetHost(host.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertSSHHostAddressHidden(t, visible, request.Host)
	listed, err := svc.ListHosts()
	if err != nil || len(listed) != 1 {
		t.Fatalf("list hosts: %v", err)
	}
	assertSSHHostAddressHidden(t, listed, request.Host)
	update := request
	update.Host, update.KeepHostAddress = "", true
	update.Name, update.Notes, update.Tags = visible.Name, visible.Notes, visible.Tags
	update.Port = host.Port
	updated, err := svc.UpdateHost(host.ID, update)
	if err != nil {
		t.Fatal(err)
	}
	assertSSHHostAddressHidden(t, updated, request.Host)
	saved, err := db.GetSSHHost(host.ID)
	if err != nil || saved.Host != request.Host || saved.Name != request.Name || saved.Notes != request.Notes || saved.Tags[0] != request.Tags[0] {
		t.Fatalf("masked update changed saved fields: %v", err)
	}
	editedText := update
	editedText.Notes += " appended"
	editedText.Tags = append(append([]string{}, update.Tags...), "new-tag")
	if _, err := svc.UpdateHost(host.ID, editedText); err != nil {
		t.Fatal(err)
	}
	saved, err = db.GetSSHHost(host.ID)
	if err != nil || saved.Notes != request.Notes+" appended" || saved.Tags[0] != request.Tags[0] || saved.Tags[1] != "new-tag" {
		t.Fatalf("editing masked text overwrote hidden originals: %v", err)
	}
	bad := update
	bad.Host, bad.KeepHostAddress = visible.Host, false
	if _, err := svc.UpdateHost(host.ID, bad); err == nil {
		t.Fatal("masked address accepted as real address")
	}
	if _, err := svc.CreateHost(update); err == nil {
		t.Fatal("address preservation accepted for new host")
	}
	_, err = svc.TestHost(context.Background(), host.ID)
	var serviceErr *SSHServiceError
	if !errors.As(err, &serviceErr) || serviceErr.Code != "SSH_HOST_KEY_UNKNOWN" {
		t.Fatalf("test host: %v", err)
	}
	challenge := serviceErr.Details.(*model.SSHHostKeyChallenge)
	if _, err := svc.TrustHostKey(host.ID, model.SSHHostKeyTrustRequest{ChallengeID: challenge.ChallengeID, FingerprintSHA256: challenge.FingerprintSHA256}, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.withMCPHost(context.Background(), host.ID, 5*time.Second, "privacy_test", func(context.Context, *ssh.Client) error { return nil }); err != nil {
		t.Fatalf("MCP must connect with the stored address: %v", err)
	}
	trusted, err := db.GetSSHHostKey(host.ID)
	if err != nil || trusted == nil {
		t.Fatalf("host key was lost: %v", err)
	}
	if _, err := svc.UpdateHost(host.ID, update); err != nil {
		t.Fatal(err)
	}
	trusted, err = db.GetSSHHostKey(host.ID)
	if err != nil || trusted == nil {
		t.Fatalf("masked update reset host key: %v", err)
	}
	sharedOriginal, err := svc.ShareHost(host.ID, "synthetic-share-password")
	if err != nil || strings.Contains(sharedOriginal.HostName, request.Host) {
		t.Fatalf("share response exposed the address in its label: %v", err)
	}
	request.Name = "created while hidden"
	created, err := svc.CreateHost(request)
	if err != nil {
		t.Fatal(err)
	}
	assertSSHHostAddressHidden(t, created, request.Host)
	shared, err := svc.ShareHost(created.ID, "synthetic-share-password")
	if err != nil {
		t.Fatal(err)
	}
	imported, err := svc.ImportHost(model.SSHHostImportRequest{Code: shared.Code, Password: "synthetic-share-password"})
	if err != nil {
		t.Fatal(err)
	}
	assertSSHHostAddressHidden(t, imported, request.Host)
	if _, err := svc.UpdatePrivacySettings(model.SSHPrivacySettings{}); err != nil {
		t.Fatal(err)
	}
	// A form fetched before the switch must still preserve the saved values.
	if _, err := svc.UpdateHost(host.ID, update); err != nil {
		t.Fatal(err)
	}
	visible, err = svc.GetHost(host.ID)
	if err != nil || visible.Host != request.Host || visible.Name != "fixture 127.0.0.1" || visible.HostHidden {
		t.Fatalf("restore visible address: %v", err)
	}
	if _, err := db.DB().Exec(`UPDATE app_settings SET value = 'invalid' WHERE key = 'ssh.privacy'`); err != nil {
		t.Fatal(err)
	}
	if value, err := svc.GetHost(host.ID); err == nil || value != nil {
		t.Fatal("invalid privacy settings must fail closed")
	}
}

func assertSSHHostAddressHidden(t *testing.T, value any, address string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), address) || !strings.Contains(string(encoded), `"host_hidden":true`) {
		t.Fatal("SSH presentation did not hide the address")
	}
}
