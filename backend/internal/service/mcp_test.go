package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func TestMCPTokenLifecycleAndSeparateSSHScope(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	svc := NewMCPService(db)
	initial, err := svc.Settings()
	if err != nil || initial.Enabled || initial.TokenConfigured || initial.EndpointPath != "/mcp" {
		t.Fatalf("unexpected default MCP configuration: %v", err)
	}
	if _, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: true}); err == nil {
		t.Fatal("enabled without a token")
	}
	issued, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil || len(issued.Token) < 32 {
		t.Fatalf("issue token: %v", err)
	}
	stored, err := db.GetMCPConfig()
	if err != nil || stored.TokenHash == issued.Token || strings.Contains(stored.TokenHash, issued.Token) {
		t.Fatal("plaintext token persisted")
	}
	assertAuth := func(token string, wantEnabled, wantAuthenticated bool) {
		t.Helper()
		enabled, authenticated, err := svc.Authenticate(token)
		if err != nil || enabled != wantEnabled || authenticated != wantAuthenticated {
			t.Fatalf("authentication mismatch: enabled=%t authenticated=%t err=%v", enabled, authenticated, err)
		}
	}
	assertAuth(issued.Token, true, true)
	assertAuth("", true, false)
	assertAuth("invalid", true, false)
	settings, _ := svc.Settings()
	if settings.Token != "" {
		t.Fatal("settings read returned plaintext token")
	}
	ssh := NewSSHMCPService(db)
	sshToken, err := ssh.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil {
		t.Fatal(err)
	}
	assertAuth(sshToken.Token, true, false)
	_, accepted, _ := ssh.Authenticate(issued.Token)
	if accepted {
		t.Fatal("application MCP token authenticated SSH MCP")
	}
	for _, request := range []model.MCPSettingsRequest{
		{Enabled: true, Token: "short"},
		{Enabled: true, Token: strings.Repeat("a", 257)},
		{Enabled: true, Token: strings.Repeat("a", 32) + " "},
		{Enabled: true, Token: strings.Repeat("a", 32), GenerateToken: true},
	} {
		if _, err := svc.UpdateSettings(request); err == nil {
			t.Fatal("invalid token update accepted")
		}
		assertAuth(issued.Token, true, true)
	}
	rotated, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: true, GenerateToken: true})
	if err != nil {
		t.Fatal(err)
	}
	assertAuth(issued.Token, true, false)
	assertAuth(rotated.Token, true, true)
	if _, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	assertAuth(rotated.Token, false, false)
	if _, err := svc.UpdateSettings(model.MCPSettingsRequest{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	assertAuth(rotated.Token, true, true)
}
