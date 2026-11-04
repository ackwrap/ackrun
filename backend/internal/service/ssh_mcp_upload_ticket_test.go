package service

import (
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func newMCPUploadTicketFixture(t *testing.T) (*SSHMCPService, model.SSHMCPTransferRequest) {
	t.Helper()
	hosts, db := newSSHHostShareTestService(t)
	credential, err := hosts.CreateCredential(model.SSHCredentialRequest{Name: "upload fixture", AuthType: "password", Secret: "synthetic"})
	if err != nil {
		t.Fatal(err)
	}
	host, err := hosts.CreateHost(model.SSHHostRequest{Name: "upload fixture", Host: "192.0.2.53", Port: 22, Username: "tester", CredentialID: credential.ID, Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	svc := NewSSHMCPService(db)
	if _, err := svc.UpdateSettings(model.SSHMCPSettingsRequest{Enabled: true, GenerateToken: true}); err != nil {
		t.Fatal(err)
	}
	return svc, model.SSHMCPTransferRequest{HostID: host.ID, Path: "/tmp/package.bin", Overwrite: true, SHA256: strings.Repeat("a", 64)}
}

func ticketCredentials(ticket *model.SSHMCPUploadTicket) (string, string) {
	return strings.TrimPrefix(ticket.EndpointPath, SSHMCPEndpoint+"/uploads/"), strings.TrimPrefix(ticket.Headers["Authorization"], "Bearer ")
}

func TestSSHMCPUploadTicketScopeAndSingleUse(t *testing.T) {
	svc, request := newMCPUploadTicketFixture(t)
	ticket, err := svc.PrepareUpload(request)
	if err != nil {
		t.Fatal(err)
	}
	other, err := svc.PrepareUpload(request)
	if err != nil {
		t.Fatal(err)
	}
	if ticket.Status != "ready" || ticket.Method != http.MethodPut || !ticket.SingleUse || ticket.ExpiresInSeconds != 900 || ticket.TimeoutSeconds != 3600 || ticket.Headers["Content-Type"] != "application/octet-stream" || ticket.EndpointPath == other.EndpointPath || ticket.Headers["Authorization"] == other.Headers["Authorization"] {
		t.Fatal("invalid or non-unique upload ticket")
	}
	id, secret := ticketCredentials(ticket)
	for _, bad := range []string{"", "invalid"} {
		if _, err := svc.ClaimUpload(id, bad); sshServiceCode(err) != "SSH_MCP_UPLOAD_INVALID" {
			t.Fatal("accepted invalid upload secret")
		}
	}
	var successes atomic.Int32
	var wait sync.WaitGroup
	for range 8 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			claimed, err := svc.ClaimUpload(id, secret)
			if err == nil {
				successes.Add(1)
				if *claimed != request {
					t.Error("upload target/options changed")
				}
			} else if sshServiceCode(err) != "SSH_MCP_UPLOAD_INVALID" {
				t.Errorf("claim: %v", err)
			}
		}()
	}
	wait.Wait()
	if successes.Load() != 1 {
		t.Fatalf("single-use claims = %d", successes.Load())
	}
}

func TestSSHMCPUploadTicketExpiryAndRevocation(t *testing.T) {
	svc, request := newMCPUploadTicketFixture(t)
	now := time.Now()
	svc.now = func() time.Time { return now }
	ticket, err := svc.PrepareUpload(request)
	if err != nil {
		t.Fatal(err)
	}
	id, secret := ticketCredentials(ticket)
	now = now.Add(sshMCPUploadTicketTTL)
	if _, err := svc.ClaimUpload(id, secret); sshServiceCode(err) != "SSH_MCP_UPLOAD_EXPIRED" {
		t.Fatal("expired ticket accepted")
	}
	for _, change := range []model.SSHMCPSettingsRequest{{Enabled: true, GenerateToken: true}, {Enabled: false}} {
		ticket, err = svc.PrepareUpload(request)
		if err != nil {
			t.Fatal(err)
		}
		id, secret = ticketCredentials(ticket)
		if _, err := svc.UpdateSettings(change); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.ClaimUpload(id, secret); err == nil {
			t.Fatal("revoked ticket accepted")
		}
	}
}

func TestSSHMCPUploadTicketBoundedAndValidated(t *testing.T) {
	svc, request := newMCPUploadTicketFixture(t)
	for _, bad := range []model.SSHMCPTransferRequest{{HostID: request.HostID, Path: ""}, {HostID: request.HostID, Path: "/tmp/file", SHA256: "invalid"}, {HostID: -1, Path: "/tmp/file"}} {
		if _, err := svc.PrepareUpload(bad); err == nil {
			t.Fatal("invalid upload request accepted")
		}
	}
	now := time.Now()
	svc.now = func() time.Time { return now }
	for range sshMCPMaximumUploadTickets {
		if _, err := svc.PrepareUpload(request); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.PrepareUpload(request); sshServiceCode(err) != "SSH_MCP_BUSY" {
		t.Fatal("unbounded upload tickets")
	}
	now = now.Add(sshMCPUploadTicketTTL)
	if _, err := svc.PrepareUpload(request); err != nil {
		t.Fatalf("expired tickets not reclaimed: %v", err)
	}
}
