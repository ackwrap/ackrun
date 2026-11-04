package store

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSSHSFTPProfileTrustBindingAndCompareAndSwap(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "profiles.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	credential := &model.SSHCredential{Name: "profile", AuthType: "password", SecretContext: "profile", SecretCiphertext: []byte{1}, SecretNonce: []byte{2}, KeyVersion: 1}
	if err := db.CreateSSHCredential(credential); err != nil {
		t.Fatal(err)
	}
	host := &model.SSHHost{Name: "profile", Host: "localhost", Port: 22, Username: "tester", CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm", Enabled: true, Tags: []string{}}
	if err := db.CreateSSHHost(host); err != nil {
		t.Fatal(err)
	}
	begin := func(generation, previous string, want bool) {
		t.Helper()
		started, err := db.BeginSSHSFTPProbe(host.ID, generation, previous)
		if err != nil || started != want {
			t.Fatalf("begin %s: %v %v", generation, started, err)
		}
	}
	begin("untrusted", "", false)
	key := &model.SSHHostKey{HostID: host.ID, KeyType: "test", PublicKey: "synthetic", FingerprintSHA256: "synthetic-a"}
	if err := db.UpsertSSHHostKey(key); err != nil {
		t.Fatal(err)
	}
	begin("first", "", true)
	begin("duplicate", "", false)
	if err := db.FinishSSHSFTPProbe(host.ID, "first", 128<<10, "ready"); err != nil {
		t.Fatal(err)
	}
	begin("retry", "first", true)
	// A late upload failure using the older settings cannot start another job.
	begin("stale-retry", "first", false)
	if err := db.FinishSSHSFTPProbe(host.ID, "first", 256<<10, "ready"); err != nil {
		t.Fatal(err)
	}
	p, err := db.GetSSHSFTPProfile(host.ID)
	if err != nil || p.Generation != "retry" || p.Status != "probing" || p.PacketSize != 32<<10 {
		t.Fatalf("stale completion replaced retry: %+v %v", p, err)
	}
	if err := db.FinishSSHSFTPProbe(host.ID, "retry", 64<<10, "ready"); err != nil {
		t.Fatal(err)
	}
	key.FingerprintSHA256 = "synthetic-b"
	if err := db.UpsertSSHHostKey(key); err != nil {
		t.Fatal(err)
	}
	if p, err := db.GetSSHSFTPProfile(host.ID); err != nil || p != nil {
		t.Fatalf("rotation retained profile: %+v %v", p, err)
	}
	begin("rotated", "", true)
	if err := db.FinishSSHSFTPProbe(host.ID, "retry", 128<<10, "ready"); err != nil {
		t.Fatal(err)
	}
	p, err = db.GetSSHSFTPProfile(host.ID)
	if err != nil || p.Generation != "rotated" || p.Fingerprint != "synthetic-b" || p.Status != "probing" {
		t.Fatalf("old key completion applied: %+v %v", p, err)
	}
	if err := db.DeleteSSHHostKey(host.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM ssh_sftp_profiles`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("trust removal left profiles: %d %v", count, err)
	}
}
