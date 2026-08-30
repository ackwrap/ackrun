package service

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/paths"
	"github.com/ackwrap/ackrun/internal/store"
)

func newAlertTestService(t *testing.T) (*AlertService, *store.Store) {
	t.Helper()
	root := t.TempDir()
	db, err := store.Open(filepath.Join(root, "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewAlertService(db, &paths.Paths{DataDir: root})
	if err != nil {
		db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		svc.Close()
		if err := db.Close(); err != nil {
			t.Errorf("close alert store: %v", err)
		}
	})
	return svc, db
}

func createWebhookAlertChannel(t *testing.T, svc *AlertService, endpoint string) *model.AlertChannel {
	t.Helper()
	channel, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Webhook channel", Type: model.AlertChannelWebhook, Enabled: true, Secret: endpoint,
		Config: model.AlertChannelConfig{WebhookAllowPrivate: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	return channel
}

func waitForAlertDeliveryCount(t *testing.T, svc *AlertService, expected int) []model.AlertDelivery {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		page, err := svc.ListDeliveries(1, 100, 0, "", "")
		if err == nil && len(page.Items) >= expected {
			return page.Items
		}
		time.Sleep(10 * time.Millisecond)
	}
	page, err := svc.ListDeliveries(1, 100, 0, "", "")
	t.Fatalf("delivery count = %d, want %d, err = %v", len(page.Items), expected, err)
	return nil
}

func TestAlertWebhookSecretsStatusAndTestDelivery(t *testing.T) {
	var status atomic.Int32
	status.Store(http.StatusNoContent)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(int(status.Load()))
	}))
	defer server.Close()
	svc, db := newAlertTestService(t)
	endpoint := server.URL + "/secret-webhook-token"
	channel := createWebhookAlertChannel(t, svc, endpoint)
	encoded, err := json.Marshal(channel)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "secret-webhook-token") || channel.Destination != "地址已加密" || !channel.HasSecret {
		t.Fatalf("unsafe channel response = %s, destination = %q", encoded, channel.Destination)
	}
	stored, err := db.GetAlertChannel(channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stored.SecretCiphertext), endpoint) {
		t.Fatal("webhook endpoint was stored in plaintext")
	}
	delivery, err := svc.TestChannel(context.Background(), channel.ID)
	if err != nil || !delivery.Success || delivery.StatusCode != http.StatusNoContent {
		t.Fatalf("successful test delivery = %+v, err = %v", delivery, err)
	}
	updated, err := svc.UpdateChannel(channel.ID, model.AlertChannelRequest{
		Name: "Webhook renamed", Type: model.AlertChannelWebhook, Enabled: true, Config: channel.Config,
	})
	if err != nil || !updated.HasSecret {
		t.Fatalf("secret-preserving update = %+v, err = %v", updated, err)
	}
	if _, err := svc.UpdateChannel(channel.ID, model.AlertChannelRequest{
		Name: "Webhook renamed", Type: model.AlertChannelWebhook, Enabled: true,
	}); !errors.Is(err, ErrAlertInvalid) {
		t.Fatalf("private webhook opt-out update error = %v", err)
	}
	status.Store(http.StatusBadGateway)
	delivery, err = svc.TestChannel(context.Background(), channel.ID)
	if !errors.Is(err, ErrAlertDelivery) || delivery.Success || delivery.StatusCode != http.StatusBadGateway {
		t.Fatalf("failed test delivery = %+v, err = %v", delivery, err)
	}
	updated, err = db.GetAlertChannel(channel.ID)
	if err != nil || updated.LastStatus != model.AlertDeliveryFailed || updated.LastError == "" {
		t.Fatalf("failed channel state = %+v, err = %v", updated, err)
	}
}

func TestAlertSMTPTimeoutInterruptsUnresponsiveServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan struct{})
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		close(accepted)
		defer connection.Close()
		buffer := make([]byte, 1)
		_, _ = connection.Read(buffer)
	}()
	host, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	port, _ := strconv.Atoi(portText)
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err = sendAlertEmail(ctx, model.AlertChannelConfig{
		SMTPHost: host, SMTPPort: port, SMTPFrom: "alerts@example.com",
		SMTPRecipients: []string{"ops@example.com"}, SMTPTLSMode: "none",
	}, "", model.AlertEvent{Title: "Test", Message: "Message", OccurredAt: time.Now().UnixMilli()})
	if err == nil || time.Since(started) > time.Second {
		t.Fatalf("unresponsive SMTP err = %v, duration = %s", err, time.Since(started))
	}
	select {
	case <-accepted:
	default:
		t.Fatal("SMTP test server never accepted the connection")
	}
}

func TestAlertDispatchCooldownAndRedaction(t *testing.T) {
	var requests atomic.Int32
	var bodiesMu sync.Mutex
	bodies := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer request.Body.Close()
		var payload map[string]any
		_ = json.NewDecoder(request.Body).Decode(&payload)
		data, _ := json.Marshal(payload)
		bodiesMu.Lock()
		bodies = append(bodies, string(data))
		bodiesMu.Unlock()
		requests.Add(1)
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	svc, _ := newAlertTestService(t)
	channel := createWebhookAlertChannel(t, svc, server.URL+"/hook-token")
	if _, err := svc.CreateRule(model.AlertRuleRequest{
		Name: "Circuit alerts", Enabled: true, EventTypes: []string{model.AlertEventCircuitOpen},
		ChannelIDs: []int64{channel.ID}, CooldownMinutes: 60,
	}); err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, time.August, 30, 10, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return current }
	event := model.AlertEvent{
		Type: model.AlertEventCircuitOpen, TargetKey: "node:1", SourceName: "Primary",
		Title: "Circuit open", Message: "upstream https://example.invalid/path?token=secret failed",
	}
	svc.Notify(event)
	svc.Notify(event)
	waitForAlertDeliveryCount(t, svc, 1)
	if requests.Load() != 1 {
		t.Fatalf("requests during cooldown = %d", requests.Load())
	}
	bodiesMu.Lock()
	firstBody := strings.Join(bodies, "\n")
	bodiesMu.Unlock()
	if strings.Contains(firstBody, "token=secret") || !strings.Contains(firstBody, "[URL]") {
		t.Fatalf("webhook event was not redacted: %s", firstBody)
	}
	current = current.Add(61 * time.Minute)
	svc.Notify(event)
	waitForAlertDeliveryCount(t, svc, 2)
	if requests.Load() != 2 {
		t.Fatalf("requests after cooldown = %d", requests.Load())
	}
}

func TestAlertTelegramResponseAndEmailSecretPreservation(t *testing.T) {
	telegram := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/sendMessage") {
			t.Errorf("telegram path = %q", request.URL.Path)
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"ok":true,"result":{}}`))
	}))
	defer telegram.Close()
	svc, _ := newAlertTestService(t)
	svc.telegramBaseURL = telegram.URL
	code, err := svc.sendTelegram(context.Background(), "12345:abcdefghijklmnopqrstuvwxyz", "-100123", model.AlertEvent{Title: "Test", Message: "Message"})
	if err != nil || code != http.StatusOK {
		t.Fatalf("telegram delivery code = %d, err = %v", code, err)
	}
	password := "  password with spaces  "
	channel, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Email", Type: model.AlertChannelEmail, Enabled: true, Secret: password,
		Config: model.AlertChannelConfig{
			SMTPHost: "smtp.example.com", SMTPPort: 587, SMTPUsername: "alerts",
			SMTPFrom: "alerts@example.com", SMTPRecipients: []string{"ops@example.com"}, SMTPTLSMode: "starttls",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := svc.store.GetAlertChannel(channel.ID)
	if err != nil {
		t.Fatal(err)
	}
	decrypted, err := svc.decryptChannelSecret(stored)
	if err != nil || decrypted != password {
		t.Fatalf("email password = %q, err = %v", decrypted, err)
	}
	changedConfig := channel.Config
	changedConfig.SMTPUsername = "other-user"
	if _, err := svc.UpdateChannel(channel.ID, model.AlertChannelRequest{
		Name: channel.Name, Type: channel.Type, Enabled: true, Config: changedConfig,
	}); !errors.Is(err, ErrAlertInvalid) {
		t.Fatalf("SMTP username change without password error = %v", err)
	}
	if _, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Invalid webhook", Type: model.AlertChannelWebhook, Enabled: true,
		Secret: "https://user:password@example.com/hook",
	}); !errors.Is(err, ErrAlertInvalid) {
		t.Fatalf("credential-bearing webhook error = %v", err)
	}
	if _, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Private webhook", Type: model.AlertChannelWebhook, Enabled: true,
		Secret: "http://127.0.0.1/hook",
	}); !errors.Is(err, ErrAlertInvalid) {
		t.Fatalf("private webhook without opt-in error = %v", err)
	}
	if _, err := svc.CreateChannel(model.AlertChannelRequest{
		Name: "Unsafe SMTP", Type: model.AlertChannelEmail, Enabled: true, Secret: "password",
		Config: model.AlertChannelConfig{
			SMTPHost: "smtp.example.com", SMTPPort: 25, SMTPUsername: "alerts",
			SMTPFrom: "alerts@example.com", SMTPRecipients: []string{"ops@example.com"}, SMTPTLSMode: "none",
		},
	}); !errors.Is(err, ErrAlertInvalid) {
		t.Fatalf("plaintext authenticated SMTP error = %v", err)
	}
}

func TestSubscriptionFailureBroadcastDispatchesAlert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	alerts, db := newAlertTestService(t)
	channel := createWebhookAlertChannel(t, alerts, server.URL)
	if _, err := alerts.CreateRule(model.AlertRuleRequest{
		Name: "Subscription failures", Enabled: true,
		EventTypes: []string{model.AlertEventSubscriptionFailed}, ChannelIDs: []int64{channel.ID},
	}); err != nil {
		t.Fatal(err)
	}
	subscription, err := db.EnsureManualSubscription()
	if err != nil {
		t.Fatal(err)
	}
	subscriptions := NewSubscriptionService(db, nil)
	subscriptions.SetAlertService(alerts)
	subscriptions.broadcastSync(subscription.ID, "failed", 0, "fetch [URL] failed")
	deliveries := waitForAlertDeliveryCount(t, alerts, 1)
	if deliveries[0].EventType != model.AlertEventSubscriptionFailed || !deliveries[0].Success {
		t.Fatalf("subscription alert delivery = %+v", deliveries[0])
	}
}
