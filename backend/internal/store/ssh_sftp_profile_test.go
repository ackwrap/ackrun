package store

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
)

func TestSSHSFTPProfileMigratesLegacyPacketLimit(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "legacy-profiles.db")
	db, err := Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if db != nil {
			_ = db.Close()
		}
	}()
	credential := &model.SSHCredential{Name: "migration", AuthType: "password", SecretContext: "migration", SecretCiphertext: []byte{1}, SecretNonce: []byte{2}, KeyVersion: 1}
	if err := db.CreateSSHCredential(credential); err != nil {
		t.Fatal(err)
	}
	host := &model.SSHHost{Name: "migration", Host: "localhost", Port: 22, Username: "tester", CredentialID: credential.ID, ConnectionMode: "direct", TerminalType: "xterm", Enabled: true, Tags: []string{}}
	if err := db.CreateSSHHost(host); err != nil {
		t.Fatal(err)
	}
	key := &model.SSHHostKey{HostID: host.ID, KeyType: "test", PublicKey: "synthetic", FingerprintSHA256: "synthetic"}
	if err := db.UpsertSSHHostKey(key); err != nil {
		t.Fatal(err)
	}
	if started, err := db.BeginSSHSFTPProbe(host.ID, "retained", ""); err != nil || !started {
		t.Fatalf("begin: %v %v", started, err)
	}
	if err := db.FinishSSHSFTPProbe(host.ID, "retained", 128<<10, "ready"); err != nil {
		t.Fatal(err)
	}
	original, err := db.GetSSHSFTPProfile(host.ID)
	if err != nil || original == nil {
		t.Fatalf("original: %+v %v", original, err)
	}
	// Recreate the previous release's constraint while retaining its data.
	for _, statement := range []string{
		`ALTER TABLE ssh_sftp_profiles RENAME TO profile_fixture`,
		strings.Replace(sshSFTPProfilesSchema, "1048576", "262144", 1),
		`INSERT INTO ssh_sftp_profiles SELECT * FROM profile_fixture`,
		`DROP TABLE profile_fixture`,
	} {
		if _, err := db.DB().Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.FinishSSHSFTPProbe(host.ID, "retained", 1<<20, "ready"); err == nil {
		t.Fatal("legacy fixture accepted 1 MiB before migration")
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	got, err := db.GetSSHSFTPProfile(host.ID)
	if err != nil || got == nil || *got != *original {
		t.Fatalf("migration changed existing profile: %+v err=%v", got, err)
	}
	if err := db.FinishSSHSFTPProbe(host.ID, "retained", 1<<20, "ready"); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []int{0, (1 << 20) + 1} {
		if err := db.FinishSSHSFTPProbe(host.ID, "retained", invalid, "ready"); err == nil {
			t.Fatalf("accepted invalid packet size %d", invalid)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(filename)
	if err != nil {
		t.Fatal(err)
	}
	got, err = db.GetSSHSFTPProfile(host.ID)
	if err != nil || got == nil || got.PacketSize != 1<<20 || got.Generation != "retained" {
		t.Fatalf("reopen lost 1 MiB profile: %+v err=%v", got, err)
	}
	if err := db.DeleteSSHHostKey(host.ID); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM ssh_sftp_profiles`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration lost cascading trust cleanup: count=%d err=%v", count, err)
	}
}

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
