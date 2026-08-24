package service

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestSSHHostShareRoundTripAndRejectsWrongPassword(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	credential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "shared credential", AuthType: "password", Secret: "credential-secret-value",
	})
	if err != nil {
		t.Fatal(err)
	}
	host, err := svc.CreateHost(model.SSHHostRequest{
		Name: "shared host", GroupName: "production", Host: "host.example.invalid", Port: 2222,
		Username: "operator", CredentialID: credential.ID, ConnectionMode: "direct",
		TerminalType: "xterm-256color", Enabled: true, Tags: []string{"linux", "edge"}, Notes: "shared notes",
	})
	if err != nil {
		t.Fatal(err)
	}
	shared, err := svc.ShareHost(host.ID, "strong-share-password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(shared.Code, sshHostShareCodePrefix) {
		t.Fatal("share code prefix is missing")
	}
	for _, plaintext := range []string{"credential-secret-value", "host.example.invalid", "operator"} {
		if strings.Contains(shared.Code, plaintext) {
			t.Fatal("share code exposed plaintext host or credential data")
		}
	}

	assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
		Code: shared.Code, Password: "wrong-share-password",
	}, "SSH_SHARE_DECRYPT_FAILED")
	tampered := []byte(shared.Code)
	index := len(sshHostShareCodePrefix) + 12
	if tampered[index] == 'A' {
		tampered[index] = 'B'
	} else {
		tampered[index] = 'A'
	}
	assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
		Code: string(tampered), Password: "strong-share-password",
	}, "")

	imported, err := svc.ImportHost(model.SSHHostImportRequest{
		Code: shared.Code, Password: "strong-share-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if imported.Host.ID == host.ID || imported.Host.Name != "shared host (导入)" {
		t.Fatalf("unexpected imported host identity: id=%d name=%q", imported.Host.ID, imported.Host.Name)
	}
	if imported.Host.CredentialName != "shared credential (导入)" || imported.Host.HostKeyStatus != "unknown" {
		t.Fatalf("unexpected imported credential or Host Key status: %+v", imported.Host)
	}
	if imported.Host.Host != host.Host || imported.Host.Port != host.Port || imported.Host.Username != host.Username ||
		imported.Host.GroupName != host.GroupName || imported.Host.Notes != host.Notes ||
		!equalStringSlices(imported.Host.Tags, host.Tags) || imported.ConvertedToDirect {
		t.Fatalf("imported host fields differ from source: %+v", imported.Host)
	}
	storedCredential, err := db.GetSSHCredential(imported.Host.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := svc.cipher.decrypt(storedCredential.SecretContext, "secret", storedCredential.SecretCiphertext, storedCredential.SecretNonce)
	if err != nil {
		t.Fatal(err)
	}
	defer clearBytes(secret)
	if !bytes.Equal(secret, []byte("credential-secret-value")) {
		t.Fatal("imported credential secret does not match")
	}
}

func TestSSHHostSharePrivateKeyAndNodeExposureConversion(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKeyWithPassphrase(privateKey, "shared", []byte("private-key-passphrase"))
	if err != nil {
		t.Fatal(err)
	}
	privateKeyPEM := pem.EncodeToMemory(block)
	payload := sshHostSharePayload{
		Format: sshHostShareFormat, Version: sshHostShareVersion,
		Host: sshHostShareHost{
			Name: "proxied host", Host: "proxy.example.invalid", Port: 22, Username: "root",
			ConnectionMode: "node_exposure", TerminalType: "xterm-256color", Enabled: true,
		},
		Credential: sshHostShareCredential{
			Name: "private key", AuthType: "private_key", Secret: privateKeyPEM,
			Passphrase: []byte("private-key-passphrase"),
		},
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	code, err := encryptSSHHostShare(plaintext, "private-share-password")
	clearBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(code, "OPENSSH PRIVATE KEY") || strings.Contains(code, "private-key-passphrase") {
		t.Fatal("private key share code exposed plaintext secret material")
	}
	imported, err := svc.ImportHost(model.SSHHostImportRequest{Code: code, Password: "private-share-password"})
	if err != nil {
		t.Fatal(err)
	}
	if !imported.ConvertedToDirect || imported.Host.ConnectionMode != "direct" || imported.Host.NodeExposureID != nil {
		t.Fatalf("node exposure reference was not converted to direct: %+v", imported)
	}
	credential, err := db.GetSSHCredential(imported.Host.CredentialID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.authMethod(credential); err != nil {
		t.Fatalf("imported private key credential is unusable: %v", err)
	}
	passphrase, err := svc.cipher.decrypt(credential.SecretContext, "passphrase", credential.PassphraseCiphertext, credential.PassphraseNonce)
	if err != nil {
		t.Fatal(err)
	}
	defer clearBytes(passphrase)
	if !bytes.Equal(passphrase, []byte("private-key-passphrase")) {
		t.Fatal("imported private key passphrase does not match")
	}
}

func TestSSHHostShareValidatesPasswordAndImportedPayload(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	for _, password := range []string{"short", strings.Repeat("a", sshHostShareMaxPassword+1)} {
		_, err := svc.ImportHost(model.SSHHostImportRequest{Code: "unused", Password: password})
		if sshShareErrorCode(err) != "SSH_SHARE_PASSWORD_INVALID" {
			t.Fatalf("expected password validation error, got %v", err)
		}
	}
	assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
		Code: strings.Repeat("a", model.MaxSSHHostShareCodeSize+1), Password: "valid-share-password",
	}, "SSH_SHARE_TOO_LARGE")
	payload := sshHostSharePayload{
		Format: sshHostShareFormat, Version: sshHostShareVersion,
		Host: sshHostShareHost{
			Name: "invalid host", Host: "host.example.invalid", Port: 0, Username: "root",
			ConnectionMode: "direct", TerminalType: "xterm-256color", Enabled: true,
		},
		Credential: sshHostShareCredential{Name: "credential", AuthType: "password", Secret: []byte("secret")},
	}
	plaintext, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	code, err := encryptSSHHostShare(plaintext, "valid-share-password")
	clearBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
		Code: code, Password: "valid-share-password",
	}, "SSH_SHARE_INVALID")
}

func TestSSHHostBatchShareDeduplicatesCredentialsAndImportsAtomically(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	sharedCredential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "batch shared credential", AuthType: "password", Secret: "batch-shared-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	uniqueCredential, err := svc.CreateCredential(model.SSHCredentialRequest{
		Name: "batch unique credential", AuthType: "password", Secret: "batch-unique-secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	hosts := make([]*model.SSHHost, 0, 3)
	for index, credentialID := range []int64{sharedCredential.ID, sharedCredential.ID, uniqueCredential.ID} {
		host, err := svc.CreateHost(model.SSHHostRequest{
			Name: fmt.Sprintf("batch host %d", index+1), Host: fmt.Sprintf("batch-%d.example.invalid", index+1),
			Port: 22, Username: "batch", CredentialID: credentialID, ConnectionMode: "direct",
			TerminalType: "xterm-256color", Enabled: true,
		})
		if err != nil {
			t.Fatal(err)
		}
		hosts = append(hosts, host)
	}
	shared, err := svc.ShareHosts([]int64{hosts[0].ID, hosts[1].ID, hosts[2].ID, hosts[0].ID}, "batch-share-password")
	if err != nil {
		t.Fatal(err)
	}
	if shared.HostCount != 3 {
		t.Fatalf("expected 3 deduplicated hosts, got %d", shared.HostCount)
	}
	imported, err := svc.ImportHost(model.SSHHostImportRequest{Code: shared.Code, Password: "batch-share-password"})
	if err != nil {
		t.Fatal(err)
	}
	if imported.HostCount != 3 || imported.CredentialCount != 2 || len(imported.Hosts) != 3 {
		t.Fatalf("unexpected batch import counts: %+v", imported)
	}
	if imported.Hosts[0].CredentialID != imported.Hosts[1].CredentialID ||
		imported.Hosts[0].CredentialID == imported.Hosts[2].CredentialID {
		t.Fatalf("credential references were not preserved: %+v", imported.Hosts)
	}
	for _, host := range imported.Hosts {
		if !strings.Contains(host.Name, "(导入)") || !strings.Contains(host.CredentialName, "(导入)") {
			t.Fatalf("batch import did not resolve name conflicts: host=%q credential=%q", host.Name, host.CredentialName)
		}
	}

	invalidPayload := sshHostShareBundlePayload{
		Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion,
		Credentials: []sshHostShareBundleCredential{{
			Ref: "credential-1", Credential: sshHostShareCredential{Name: "rollback credential", AuthType: "password", Secret: []byte("rollback-secret")},
		}},
		Hosts: []sshHostShareBundleHost{
			{CredentialRef: "credential-1", Host: sshHostShareHost{Name: "rollback valid", Host: "valid.example.invalid", Port: 22, Username: "root", ConnectionMode: "direct", TerminalType: "xterm-256color"}},
			{CredentialRef: "credential-1", Host: sshHostShareHost{Name: "rollback invalid", Host: "invalid.example.invalid", Port: 0, Username: "root", ConnectionMode: "direct", TerminalType: "xterm-256color"}},
		},
	}
	plaintext, err := json.Marshal(invalidPayload)
	clearSSHHostShareBundle(&invalidPayload)
	if err != nil {
		t.Fatal(err)
	}
	invalidCode, err := encryptSSHHostShare(plaintext, "batch-share-password")
	clearBytes(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
		Code: invalidCode, Password: "batch-share-password",
	}, "SSH_SHARE_INVALID")
}

func TestSSHHostBatchShareValidatesSelection(t *testing.T) {
	svc, _ := newSSHHostShareTestService(t)
	for _, ids := range [][]int64{nil, {0}, make([]int64, model.MaxSSHHostShareHosts+1)} {
		_, err := svc.ShareHosts(ids, "batch-share-password")
		if sshShareErrorCode(err) != "SSH_SHARE_INVALID" {
			t.Fatalf("expected invalid batch selection error, got %v", err)
		}
	}
}

func TestSSHHostBatchShareRejectsInvalidReferences(t *testing.T) {
	svc, db := newSSHHostShareTestService(t)
	credential := sshHostShareBundleCredential{
		Ref: "credential-1", Credential: sshHostShareCredential{Name: "credential", AuthType: "password", Secret: []byte("secret")},
	}
	host := sshHostShareBundleHost{
		CredentialRef: "credential-1",
		Host:          sshHostShareHost{Name: "host", Host: "host.example.invalid", Port: 22, Username: "root", ConnectionMode: "direct", TerminalType: "xterm-256color"},
	}
	secondHost := host
	secondHost.Host.Name = "host two"
	tooManyHosts := make([]sshHostShareBundleHost, model.MaxSSHHostShareHosts+1)
	for index := range tooManyHosts {
		tooManyHosts[index] = host
		tooManyHosts[index].Host.Name = fmt.Sprintf("host %d", index+1)
	}
	cases := []sshHostShareBundlePayload{
		{Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion, Credentials: []sshHostShareBundleCredential{credential, credential}, Hosts: []sshHostShareBundleHost{host, secondHost}},
		{Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion, Credentials: []sshHostShareBundleCredential{credential}, Hosts: []sshHostShareBundleHost{{CredentialRef: "missing", Host: host.Host}}},
		{Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion, Credentials: []sshHostShareBundleCredential{credential, {Ref: "unused", Credential: sshHostShareCredential{Name: "unused", AuthType: "password", Secret: []byte("unused")}}}, Hosts: []sshHostShareBundleHost{host, secondHost}},
		{Format: sshHostShareBundleFormat, Version: sshHostShareBundleVersion, Credentials: []sshHostShareBundleCredential{credential}, Hosts: tooManyHosts},
	}
	for index := range cases {
		plaintext, err := json.Marshal(cases[index])
		clearSSHHostShareBundle(&cases[index])
		if err != nil {
			t.Fatal(err)
		}
		code, err := encryptSSHHostShare(plaintext, "batch-share-password")
		clearBytes(plaintext)
		if err != nil {
			t.Fatal(err)
		}
		assertSSHShareImportRejectedWithoutWrites(t, svc, db, model.SSHHostImportRequest{
			Code: code, Password: "batch-share-password",
		}, "SSH_SHARE_INVALID")
	}
}

func newSSHHostShareTestService(t *testing.T) (*SSHHostService, *store.Store) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	svc, err := NewSSHHostService(db, &paths.Paths{DataDir: root}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(svc.Close)
	return svc, db
}

func assertSSHShareImportRejectedWithoutWrites(t *testing.T, svc *SSHHostService, db *store.Store, request model.SSHHostImportRequest, expectedCode string) {
	t.Helper()
	hostsBefore, err := db.ListSSHHosts()
	if err != nil {
		t.Fatal(err)
	}
	credentialsBefore, err := db.ListSSHCredentials()
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ImportHost(request)
	code := sshShareErrorCode(err)
	if expectedCode != "" && code != expectedCode {
		t.Fatalf("expected %s, got %v", expectedCode, err)
	}
	if expectedCode == "" && code != "SSH_SHARE_INVALID" && code != "SSH_SHARE_DECRYPT_FAILED" {
		t.Fatalf("expected tampered share rejection, got %v", err)
	}
	hostsAfter, listErr := db.ListSSHHosts()
	if listErr != nil {
		t.Fatal(listErr)
	}
	credentialsAfter, listErr := db.ListSSHCredentials()
	if listErr != nil {
		t.Fatal(listErr)
	}
	if len(hostsAfter) != len(hostsBefore) || len(credentialsAfter) != len(credentialsBefore) {
		t.Fatal("rejected share import wrote host or credential data")
	}
}

func sshShareErrorCode(err error) string {
	var serviceErr *SSHServiceError
	if errors.As(err, &serviceErr) {
		return serviceErr.Code
	}
	return ""
}

func equalStringSlices(first, second []string) bool {
	if len(first) != len(second) {
		return false
	}
	for index := range first {
		if first[index] != second[index] {
			return false
		}
	}
	return true
}
