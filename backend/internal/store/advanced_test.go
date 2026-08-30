package store

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
)

func openAdvancedTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close store: %v", err)
		}
	})
	return store
}

func TestAdvancedMigrationsAreIdempotent(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "ackwrap.db")
	store, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.migrate(); err != nil {
		store.Close()
		t.Fatalf("repeat migration: %v", err)
	}
	var foreignKeys int
	if err := store.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		store.Close()
		t.Fatal(err)
	}
	if foreignKeys != 1 {
		store.Close()
		t.Fatalf("foreign_keys = %d, want 1", foreignKeys)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(databasePath)
	if err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	defer store.Close()
	for _, table := range []string{
		"platform_routes", "session_leases", "advanced_health_states",
		"advanced_health_events", "advanced_access_logs", "alert_channels",
		"alert_rules", "alert_rule_channels", "alert_rule_cooldowns", "alert_deliveries",
	} {
		var count int
		if err := store.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatalf("table %s count = %d", table, count)
		}
	}
}

func TestPlatformRouteCRUDReorderAndLeaseCleanup(t *testing.T) {
	store := openAdvancedTestStore(t)
	routes := []*model.PlatformRoute{
		{
			Name: "Windows", Platform: "windows", Enabled: true,
			InboundExposureIDs: []int64{2, 3}, SourceCIDRs: []string{"10.0.0.0/8"},
			Domains: []string{"example.invalid"}, DomainSuffixes: []string{"invalid"},
			DomainKeywords: []string{"example"}, DestinationCIDRs: []string{"192.0.2.0/24"},
			TargetType: model.AdvancedTargetDirect, FallbackType: model.AdvancedTargetDirect,
		},
		{Name: "Linux", Platform: "linux", Enabled: true, TargetType: model.AdvancedTargetDirect},
		{Name: "Android", Platform: "android", Enabled: false, TargetType: model.AdvancedTargetDirect},
	}
	for _, route := range routes {
		if err := store.CreatePlatformRoute(route); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CreatePlatformRoute(&model.PlatformRoute{Name: "windows", Platform: "other", TargetType: model.AdvancedTargetDirect}); err == nil {
		t.Fatal("case-insensitive duplicate route name was accepted")
	}
	loaded, err := store.GetPlatformRoute(routes[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded.InboundExposureIDs, []int64{2, 3}) ||
		!reflect.DeepEqual(loaded.DestinationCIDRs, []string{"192.0.2.0/24"}) {
		t.Fatalf("route arrays did not round trip: %+v", loaded)
	}
	if err := store.ReorderPlatformRoutes([]int64{routes[2].ID, routes[0].ID}); err == nil {
		t.Fatal("incomplete reorder was accepted")
	}
	if err := store.ReorderPlatformRoutes([]int64{routes[2].ID, routes[0].ID, routes[1].ID}); err != nil {
		t.Fatal(err)
	}
	listed, err := store.ListPlatformRoutes()
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 3 || listed[0].ID != routes[2].ID || listed[2].ID != routes[1].ID {
		t.Fatalf("reordered routes = %+v", listed)
	}
	loaded.Name = "Windows updated"
	loaded.Enabled = false
	loaded.Priority = 25
	loaded.Domains = nil
	if err := store.UpdatePlatformRoute(loaded.ID, loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = store.GetPlatformRoute(loaded.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "Windows updated" || loaded.Enabled || loaded.Domains == nil || len(loaded.Domains) != 0 {
		t.Fatalf("updated route = %+v", loaded)
	}
	if _, err := store.db.Exec(`UPDATE platform_routes SET domains = 'not-json' WHERE id = ?`, loaded.ID); err == nil {
		t.Fatal("invalid JSON array bypassed the database check")
	}

	expiresAt := time.Now().UTC().Add(time.Hour).UnixMilli()
	lease := &model.SessionLease{
		Name: "temporary", Enabled: true, ClientCIDR: "198.51.100.0/24",
		InboundExposureIDs: []int64{4}, PlatformRouteID: int64Pointer(loaded.ID),
		TargetType: model.AdvancedTargetDirect, ExpiresAt: expiresAt,
	}
	if err := store.CreateSessionLease(lease); err != nil {
		t.Fatal(err)
	}
	lease.ExpiresAt = expiresAt + int64(time.Hour/time.Millisecond)
	lease.ClientCIDR = "203.0.113.0/24"
	if err := store.UpdateSessionLease(lease.ID, lease); err != nil {
		t.Fatal(err)
	}
	newExpiry := expiresAt + int64(2*time.Hour/time.Millisecond)
	if err := store.UpdateSessionLeaseExpiresAt(lease.ID, newExpiry); err != nil {
		t.Fatal(err)
	}
	loadedLease, err := store.GetSessionLease(lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedLease.ExpiresAt != newExpiry || loadedLease.ClientCIDR != "203.0.113.0/24" || loadedLease.PlatformRouteID == nil {
		t.Fatalf("updated lease = %+v", loadedLease)
	}
	if err := store.DeletePlatformRoute(loaded.ID); err != nil {
		t.Fatal(err)
	}
	loadedLease, err = store.GetSessionLease(lease.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loadedLease.PlatformRouteID != nil {
		t.Fatalf("deleted route reference was not cleared: %+v", loadedLease)
	}
	leases, err := store.ListSessionLeases()
	if err != nil || len(leases) != 1 {
		t.Fatalf("leases = %+v, err = %v", leases, err)
	}
	if err := store.DeleteSessionLease(lease.ID); err != nil {
		t.Fatal(err)
	}
}

func TestAdvancedHealthUpsertEventsAndRetention(t *testing.T) {
	store := openAdvancedTestStore(t)
	state := &model.AdvancedHealthState{
		TargetKey: "collection:1", TargetType: "collection", TargetRef: "1", DisplayName: "Primary",
		Status: model.AdvancedHealthUnhealthy, ConsecutiveFailures: 2, LastCheckedAt: 100,
	}
	if err := store.UpsertAdvancedHealthState(state); err != nil {
		t.Fatal(err)
	}
	state.Status = model.AdvancedHealthHealthy
	state.LatencyMS = 42
	state.ConsecutiveFailures = 0
	state.ConsecutiveSuccesses = 1
	if err := store.UpsertAdvancedHealthState(state); err != nil {
		t.Fatal(err)
	}
	states, err := store.ListAdvancedHealthStates()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 || states[0].Status != model.AdvancedHealthHealthy || states[0].LatencyMS != 42 {
		t.Fatalf("health states = %+v", states)
	}
	if err := store.AppendAdvancedHealthEvent(&model.AdvancedHealthEvent{
		TargetKey: "collection:1", TargetType: "collection", EventType: "probe_failed", Message: `password=secret`,
	}); !errors.Is(err, ErrSensitiveAdvancedHealthText) {
		t.Fatalf("sensitive event error = %v", err)
	}
	for index, eventType := range []string{"probe_failed", "circuit_open", "recovered"} {
		event := &model.AdvancedHealthEvent{
			TargetKey: "collection:1", TargetType: "collection", DisplayName: "Primary",
			EventType: eventType, Message: "summary only", LatencyMS: index, CreatedAt: int64(index + 1),
		}
		if err := store.AppendAdvancedHealthEvent(event); err != nil {
			t.Fatal(err)
		}
	}
	events, err := store.ListAdvancedHealthEvents(10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].EventType != "recovered" {
		t.Fatalf("newest health events = %+v", events)
	}
	deleted, err := store.PruneAdvancedHealthEvents(2)
	if err != nil || deleted != 1 {
		t.Fatalf("prune deleted = %d, err = %v", deleted, err)
	}
	events, err = store.ListAdvancedHealthEvents(10, 0)
	if err != nil || len(events) != 2 || events[1].EventType != "circuit_open" {
		t.Fatalf("retained health events = %+v, err = %v", events, err)
	}
}

func TestAdvancedAccessLogsPaginationDedupeAndCleanup(t *testing.T) {
	store := openAdvancedTestStore(t)
	now := time.Now().UTC()
	items := []model.AdvancedAccessLog{
		{CoreEventID: "event-old", EventTime: now.AddDate(0, 0, -10).UnixMilli(), Platform: "windows", Decision: "direct", DestinationSummary: "old-target"},
		{CoreEventID: "event-2", EventTime: now.Add(-2 * time.Minute).UnixMilli(), Platform: "linux", Decision: "proxy", DomainSummary: "alpha.invalid"},
		{CoreEventID: "event-3", EventTime: now.Add(-time.Minute).UnixMilli(), Platform: "linux", Decision: "proxy", DomainSummary: "beta.invalid"},
		{CoreEventID: "event-3", EventTime: now.UnixMilli(), Platform: "linux", Decision: "proxy", DomainSummary: "duplicate.invalid"},
	}
	inserted, err := store.InsertAdvancedAccessLogs(items)
	if err != nil || inserted != 3 {
		t.Fatalf("inserted = %d, err = %v", inserted, err)
	}
	filter := model.AdvancedAccessLogFilter{Platform: "linux", Decision: "proxy", Keyword: "invalid", Limit: 1}
	page, err := store.ListAdvancedAccessLogs(filter)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].CoreEventID != "event-3" {
		t.Fatalf("first access page = %+v", page)
	}
	filter.Offset = 1
	page, err = store.ListAdvancedAccessLogs(filter)
	if err != nil || len(page.Items) != 1 || page.Items[0].CoreEventID != "event-2" {
		t.Fatalf("second access page = %+v, err = %v", page, err)
	}
	count, err := store.CountAdvancedAccessLogs(model.AdvancedAccessLogFilter{
		FromTime: now.Add(-3 * time.Minute).UnixMilli(), ToTime: now.UnixMilli(),
	})
	if err != nil || count != 2 {
		t.Fatalf("time-filtered count = %d, err = %v", count, err)
	}
	deleted, err := store.CleanupAdvancedAccessLogs(7, 1)
	if err != nil || deleted != 2 {
		t.Fatalf("cleanup deleted = %d, err = %v", deleted, err)
	}
	count, err = store.CountAdvancedAccessLogs(model.AdvancedAccessLogFilter{})
	if err != nil || count != 1 {
		t.Fatalf("post-cleanup count = %d, err = %v", count, err)
	}
	if err := store.ClearAdvancedAccessLogs(); err != nil {
		t.Fatal(err)
	}
	count, err = store.CountAdvancedAccessLogs(model.AdvancedAccessLogFilter{})
	if err != nil || count != 0 {
		t.Fatalf("post-clear count = %d, err = %v", count, err)
	}
}

func TestAdvancedSettingsDefaultsTransactionalRoundTrip(t *testing.T) {
	store := openAdvancedTestStore(t)
	defaults, err := store.GetAdvancedSettings()
	if err != nil {
		t.Fatal(err)
	}
	if defaults.HealthIntervalSeconds <= 0 || defaults.AccessLogPrivacyMode != model.AdvancedPrivacyStrict {
		t.Fatalf("advanced defaults = %+v", defaults)
	}
	if _, err := store.db.Exec(`
		CREATE TRIGGER reject_advanced_setting
		BEFORE INSERT ON app_settings
		WHEN NEW.key = 'advanced.health_timeout_seconds'
		BEGIN SELECT RAISE(ABORT, 'blocked'); END
	`); err != nil {
		t.Fatal(err)
	}
	updated := model.AdvancedSettings{
		RoutingEnabled: true, LeasesEnabled: true, HealthEnabled: true,
		HealthIntervalSeconds: 30, HealthTimeoutSeconds: 3, FailureThreshold: 4,
		RecoveryThreshold: 2, CircuitOpenSeconds: 120, AccessLogsEnabled: true,
		AccessLogRetentionDays: 14, AccessLogMaxEntries: 4321,
		AccessLogPrivacyMode: model.AdvancedPrivacyBalanced,
	}
	if err := store.SetAdvancedSettings(&updated); err == nil {
		t.Fatal("triggered settings write unexpectedly succeeded")
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM app_settings WHERE key LIKE 'advanced.%'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed settings transaction persisted %d keys", count)
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_advanced_setting`); err != nil {
		t.Fatal(err)
	}
	if err := store.SetAdvancedSettings(&updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetAdvancedSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(*loaded, updated) {
		t.Fatalf("advanced settings = %+v, want %+v", loaded, updated)
	}
}

func int64Pointer(value int64) *int64 {
	return &value
}
