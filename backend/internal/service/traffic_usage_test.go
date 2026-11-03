package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

func testTrafficRecorder(t *testing.T, server *httptest.Server) *TrafficUsageService {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	address, _ := url.Parse(server.URL)
	if err := db.SetExperimentalSettings(&model.ExperimentalSettings{ClashAPIEnabled: true, ClashAPIPort: address.Port(), ClashAPISecret: "synthetic-secret"}); err != nil {
		t.Fatal(err)
	}
	core := &SingboxService{pid: 123, cmd: &exec.Cmd{Process: &os.Process{}}, coreStartedAt: time.Now().Add(-time.Minute)}
	return NewTrafficUsageService(db, core)
}

func TestTrafficRecorderCollectsCountersAndPreservesUsageOnFailure(t *testing.T) {
	var payload atomic.Value
	payload.Store(`{"uploadTotal":1000,"downloadTotal":2000,"connections":[]}`)
	var status atomic.Int64
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/connections" || r.Header.Get("Authorization") != "Bearer synthetic-secret" {
			t.Error("traffic request path/auth mismatch")
		}
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(payload.Load().(string)))
	}))
	defer server.Close()
	svc := testTrafficRecorder(t, server)
	svc.Sample()
	payload.Store(`{"uploadTotal":1100,"downloadTotal":2300}`)
	svc.Sample()
	got, err := svc.store.GetMonthlyTraffic(time.Now().In(svc.location))
	if err != nil || got.UploadBytes != 100 || got.DownloadBytes != 300 || svc.Error() != "" {
		t.Fatalf("usage = %+v, error = %v, collection error = %q", got, err, svc.Error())
	}
	status.Store(http.StatusBadGateway)
	payload.Store("synthetic-secret must never reach the error")
	svc.Sample()
	if svc.Error() == "" || strings.Contains(svc.Error(), "synthetic-secret") {
		t.Fatalf("unsafe or missing collection error: %q", svc.Error())
	}
	status.Store(http.StatusOK)
	payload.Store(`{"uploadTotal":1120,"downloadTotal":2330}`)
	svc.Sample()
	got, err = svc.store.GetMonthlyTraffic(time.Now().In(svc.location))
	if err != nil || got.UploadBytes != 120 || got.DownloadBytes != 330 || svc.Error() != "" {
		t.Fatalf("recovered usage = %+v, error = %v, collection error = %q", got, err, svc.Error())
	}
	for _, invalid := range []string{`{}`, `{"uploadTotal":-1,"downloadTotal":10}`, `not json`} {
		payload.Store(invalid)
		svc.Sample()
		if svc.Error() == "" {
			t.Fatalf("accepted invalid counters %s", invalid)
		}
	}
}

func TestTrafficRecorderShutdownCancelsInflightRequest(t *testing.T) {
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	svc := testTrafficRecorder(t, server)
	svc.Start()
	defer svc.Close()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("collector did not start")
	}
	closed := make(chan struct{})
	go func() { svc.Close(); close(closed) }()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("collector shutdown did not cancel the request")
	}
	if svc.Error() != "" {
		t.Fatalf("shutdown reported a collection failure: %s", svc.Error())
	}
}

func TestTrafficRecorderDiscardsSampleFromReplacedCore(t *testing.T) {
	var svc *TrafficUsageService
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		svc.core.mu.Lock()
		svc.core.coreStartedAt = time.Now()
		svc.core.mu.Unlock()
		_, _ = w.Write([]byte(`{"uploadTotal":1000,"downloadTotal":2000}`))
	}))
	defer server.Close()
	svc = testTrafficRecorder(t, server)
	if err := svc.collect(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := svc.store.GetMonthlyTraffic(time.Now().In(svc.location))
	if err != nil || got.UploadBytes != 0 || got.DownloadBytes != 0 {
		t.Fatalf("stale session was recorded: %+v, error = %v", got, err)
	}
}

func TestSubscriptionSummaryIgnoresManualAndFindsNextExpiry(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	items := []model.Subscription{
		{Name: "local", URL: "manual://local", TrafficTotalBytes: 1000, ExpireAt: now.Add(time.Hour).UnixMilli()},
		{Name: "expired", TrafficTotalBytes: 100, ExpireAt: now.Add(-time.Hour).UnixMilli()},
		{Name: "later", TrafficTotalBytes: 200, ExpireAt: now.Add(48 * time.Hour).UnixMilli()},
		{Name: "next", TrafficTotalBytes: 300, ExpireAt: now.Add(24 * time.Hour).UnixMilli()},
		{Name: "unknown", TrafficTotalBytes: 0},
	}
	got := summarizeSubscriptions(items, now)
	if got.SubscriptionTotalBytes != 600 || got.SubscriptionCount != 4 || got.TrafficReportedCount != 3 || got.ExpiredSubscriptionCount != 1 || got.NextExpiry == nil || got.NextExpiry.Name != "next" {
		t.Fatalf("subscription summary = %+v", got)
	}
	if empty := summarizeSubscriptions(nil, now); empty.NextExpiry != nil || empty.SubscriptionCount != 0 {
		t.Fatalf("empty summary = %+v", empty)
	}
}

func TestSubscriptionSummaryUsesProviderExpiryMilliseconds(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	parseExpiry := func(at time.Time) int64 {
		return parseSubscriptionUserInfo("expire=" + strconv.FormatInt(at.Unix(), 10)).ExpireAt
	}
	items := []model.Subscription{
		{Name: "expired", ExpireAt: parseExpiry(now.Add(-time.Second))},
		{Name: "due-now", ExpireAt: parseExpiry(now)},
		{Name: "next", ExpireAt: parseExpiry(now.Add(time.Hour))},
		{Name: "later", ExpireAt: parseExpiry(now.Add(24 * time.Hour))},
	}
	got := summarizeSubscriptions(items, now)
	if got.ExpiredSubscriptionCount != 2 || got.NextExpiry == nil || got.NextExpiry.Name != "next" || got.NextExpiry.ExpireAt != now.Add(time.Hour).UnixMilli() {
		t.Fatalf("provider expiry summary = %+v", got)
	}
	if expired := summarizeSubscriptions(items[:2], now); expired.NextExpiry != nil || expired.ExpiredSubscriptionCount != 2 {
		t.Fatalf("expired subscriptions were returned as upcoming: %+v", expired)
	}
}
