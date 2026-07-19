package service

import (
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/store"
)

func TestDNSGlobalSettingsFakeIPFollowsTUNMode(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	svc := NewDNSService(db)
	settings, err := svc.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !settings.FakeIPEnabled {
		t.Fatal("default TUN + Mixed mode must enable FakeIP")
	}

	if err := db.SetInboundMode("mixed"); err != nil {
		t.Fatal(err)
	}
	settings.FakeIPEnabled = true
	if err := svc.SetDNSGlobalSettings(settings); err != nil {
		t.Fatal(err)
	}
	stored, err := db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if stored.FakeIPEnabled {
		t.Fatal("Mixed mode must persist FakeIP as disabled")
	}

	if err := db.SetInboundMode("tun"); err != nil {
		t.Fatal(err)
	}
	stored.FakeIPEnabled = false
	if err := svc.SetDNSGlobalSettings(stored); err != nil {
		t.Fatal(err)
	}
	stored, err = db.GetDNSGlobalSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !stored.FakeIPEnabled {
		t.Fatal("TUN mode must persist FakeIP as enabled")
	}
}
