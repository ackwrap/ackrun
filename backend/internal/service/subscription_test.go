package service

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ackwrap/ackwrap/internal/model"
	"github.com/ackwrap/ackwrap/internal/store"
)

func TestParseSubscriptionUserInfo(t *testing.T) {
	result := parseSubscriptionUserInfo("upload=10; download=20; total=100; expire=2000000000")
	if result.TrafficUsedBytes != 30 {
		t.Fatalf("expected used bytes 30, got %d", result.TrafficUsedBytes)
	}
	if result.TrafficTotalBytes != 100 {
		t.Fatalf("expected total bytes 100, got %d", result.TrafficTotalBytes)
	}
	if result.ExpireAt != 2000000000000 {
		t.Fatalf("expected expire ms 2000000000000, got %d", result.ExpireAt)
	}
}

func TestSubscriptionUserAgentOptions(t *testing.T) {
	svc := NewSubscriptionService(nil, nil)
	options := svc.UserAgentOptions()
	if len(options) == 0 {
		t.Fatalf("expected user agent options")
	}
	if options[0].Value == "" || options[0].Label == "" {
		t.Fatalf("expected populated option, got %+v", options[0])
	}
}

func TestFetchAndParseSubscription(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "clash-meta/2.4.0" {
			t.Fatalf("expected user agent clash-meta/2.4.0, got %q", got)
		}
		w.Header().Set("Subscription-Userinfo", "upload=10; download=20; total=100; expire=2000000000")
		_, _ = w.Write([]byte("ss://aes-128-gcm:pass@example.com:8388#SS-01\ntrojan://password@trojan.example.com:443#Trojan-01\n"))
	}))
	defer server.Close()

	svc := NewSubscriptionService(nil, nil)
	result, err := svc.fetchAndParse(server.URL, "clash-meta/2.4.0", 5)
	if err != nil {
		t.Fatalf("fetch and parse: %v", err)
	}
	if result.NodeCount != 2 || result.TrafficUsedBytes != 30 || result.TrafficTotalBytes != 100 || result.ExpireAt != 2000000000000 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Nodes) != 2 || result.Nodes[0].Type != "ss" || result.Nodes[1].Type != "trojan" {
		t.Fatalf("unexpected parsed nodes: %+v", result.Nodes)
	}
}

func TestFetchAndParseEmptySubscriptionFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not a subscription"))
	}))
	defer server.Close()

	svc := NewSubscriptionService(nil, nil)
	if _, err := svc.fetchAndParse(server.URL, "clash-meta/2.4.0", 5); err == nil {
		t.Fatalf("expected empty subscription error")
	}
}

func TestApplyNodeFilters(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()
	if _, err := db.CreateNodeFilter(&model.NodeFilterRequest{Name: "filter hk", Target: "name", Pattern: "HK", Enabled: true}); err != nil {
		t.Fatalf("create filter: %v", err)
	}
	svc := NewSubscriptionService(db, nil)
	result := &subscriptionSyncResult{NodeCount: 2, Nodes: []model.ParsedNode{
		{Name: "HK-01", Type: "vless", Server: "hk.example.com", ServerPort: 443, RawJSON: `{"name":"HK-01"}`},
		{Name: "JP-01", Type: "vless", Server: "jp.example.com", ServerPort: 443, RawJSON: `{"name":"JP-01"}`},
	}}
	if err := svc.applyNodeFilters(result); err != nil {
		t.Fatalf("apply filters: %v", err)
	}
	if result.NodeCount != 1 || result.Nodes[0].Name != "JP-01" {
		t.Fatalf("unexpected filtered result: %+v", result)
	}
}

func TestValidateSubscriptionScheduleAndTimeout(t *testing.T) {
	req := &model.SubscriptionRequest{
		Name:            "sub",
		URL:             "https://example.com/sub",
		SyncMode:        "weekly",
		SyncTime:        "03:04:05",
		SyncWeekday:     3,
		SyncTimeoutSecs: 60,
	}
	if err := validateSubscription(req); err != nil {
		t.Fatalf("validate weekly schedule: %v", err)
	}

	badTime := *req
	badTime.SyncTime = "25:00:00"
	if err := validateSubscription(&badTime); err == nil {
		t.Fatalf("expected invalid time error")
	}

	badWeekday := *req
	badWeekday.SyncWeekday = 8
	if err := validateSubscription(&badWeekday); err == nil {
		t.Fatalf("expected invalid weekday error")
	}

	monthly := *req
	monthly.SyncMode = "monthly"
	monthly.SyncWeekday = 31
	if err := validateSubscription(&monthly); err != nil {
		t.Fatalf("validate monthly schedule: %v", err)
	}

	badMonthly := monthly
	badMonthly.SyncWeekday = 32
	if err := validateSubscription(&badMonthly); err == nil {
		t.Fatalf("expected invalid monthly day error")
	}

	badTimeout := *req
	badTimeout.SyncTimeoutSecs = 301
	if err := validateSubscription(&badTimeout); err == nil {
		t.Fatalf("expected invalid timeout error")
	}

	defaultTimeout := &model.SubscriptionRequest{Name: "sub", URL: "https://example.com/sub"}
	if err := validateSubscription(defaultTimeout); err != nil {
		t.Fatalf("validate default timeout: %v", err)
	}
	if defaultTimeout.SyncTimeoutSecs != 60 || defaultTimeout.SyncMode != "off" {
		t.Fatalf("expected defaults to be applied, got %+v", defaultTimeout)
	}
}

func TestScheduleJobRegistersCronEntries(t *testing.T) {
	svc := NewSubscriptionService(nil, nil)
	daily := &model.Subscription{ID: 1, Name: "daily", SyncMode: "daily", SyncTime: "03:04:05"}
	svc.scheduleJob(daily)
	if _, ok := svc.entries[daily.ID]; !ok {
		t.Fatalf("expected daily cron entry")
	}

	weekly := &model.Subscription{ID: 2, Name: "weekly", SyncMode: "weekly", SyncTime: "03:04:05", SyncWeekday: 7}
	svc.scheduleJob(weekly)
	if _, ok := svc.entries[weekly.ID]; !ok {
		t.Fatalf("expected weekly cron entry")
	}

	monthly := &model.Subscription{ID: 4, Name: "monthly", SyncMode: "monthly", SyncTime: "03:04:05", SyncWeekday: 15}
	svc.scheduleJob(monthly)
	if _, ok := svc.entries[monthly.ID]; !ok {
		t.Fatalf("expected monthly cron entry")
	}

	off := &model.Subscription{ID: 3, Name: "off", SyncMode: "off"}
	svc.scheduleJob(off)
	if _, ok := svc.entries[off.ID]; ok {
		t.Fatalf("did not expect off cron entry")
	}
}
