package service

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

type advancedRuntimeStub struct {
	snapshot           RuntimeRoutingConfig
	puts               []RuntimeRoutingConfig
	failPuts           int
	applyBeforeFailure bool
	events             RuntimeAccessEventList
	accesses           []uint64
}

func (stub *advancedRuntimeStub) GetRuntimeRouting() (RuntimeRoutingConfig, error) {
	return stub.snapshot, nil
}

func (stub *advancedRuntimeStub) PutRuntimeRouting(config RuntimeRoutingConfig) error {
	stub.puts = append(stub.puts, config)
	if stub.failPuts > 0 {
		stub.failPuts--
		if stub.applyBeforeFailure {
			stub.snapshot = config
		}
		return errors.New("runtime rejected snapshot")
	}
	stub.snapshot = config
	return nil
}

func (stub *advancedRuntimeStub) GetAccessEvents(after uint64, _ int) (RuntimeAccessEventList, error) {
	stub.accesses = append(stub.accesses, after)
	return stub.events, nil
}

type advancedCoreStub struct{ running bool }

func (stub *advancedCoreStub) IsRunning() bool { return stub.running }

type advancedTestData struct {
	service      *AdvancedRoutingService
	store        *store.Store
	runtime      *advancedRuntimeStub
	core         *advancedCoreStub
	subscription int64
	nodeUID      string
	exposureID   int64
}

func newAdvancedTestData(t *testing.T) advancedTestData {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "ackwrap.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	subscription, err := db.EnsureManualSubscription()
	if err != nil {
		t.Fatal(err)
	}
	const uid = "advanced-test-node"
	if err := db.ReplaceSubscriptionNodes(subscription.ID, []model.ParsedNode{{
		UID: uid, Name: "Test Node", Type: "socks", Server: "192.0.2.10", ServerPort: 443,
	}}); err != nil {
		t.Fatal(err)
	}
	exposure := &model.NodeExposure{
		Name: "Advanced Test Inbound", SubscriptionID: subscription.ID, NodeUID: uid,
		InboundType: "mixed", Listen: "127.0.0.1", ListenPort: 19090, Enabled: true,
	}
	if err := db.CreateNodeExposure(exposure); err != nil {
		t.Fatal(err)
	}
	runtime := &advancedRuntimeStub{snapshot: RuntimeRoutingConfig{
		Routes: []RuntimeRoute{}, Leases: []RuntimeLease{}, UnhealthyOutbounds: []string{},
		AccessEventsPrivacyMode: model.AdvancedPrivacyStrict,
	}}
	core := &advancedCoreStub{}
	return advancedTestData{
		service: NewAdvancedRoutingService(db, runtime, core, nil, "persistent-test-secret"),
		store:   db, runtime: runtime, core: core, subscription: subscription.ID, nodeUID: uid, exposureID: exposure.ID,
	}
}

func TestAdvancedRouteValidationAndExactTargetTags(t *testing.T) {
	data := newAdvancedTestData(t)
	invalid := model.PlatformRoute{
		Name: "invalid", Platform: "windows", Enabled: true, Priority: 10,
		SourceCIDRs: []string{"not-a-prefix"}, TargetType: model.AdvancedTargetDirect,
	}
	if _, err := data.service.CreatePlatformRoute(invalid); !errors.Is(err, ErrAdvancedInvalid) {
		t.Fatalf("invalid CIDR error = %v", err)
	}
	invalid.SourceCIDRs = nil
	invalid.Platform = "https://secret.example"
	if _, err := data.service.CreatePlatformRoute(invalid); !errors.Is(err, ErrAdvancedInvalid) {
		t.Fatalf("unsafe platform error = %v", err)
	}
	nodeRoute := model.PlatformRoute{
		Name: "Node route", Platform: "windows", Enabled: true, Priority: 10,
		InboundExposureIDs: []int64{data.exposureID}, SourceCIDRs: []string{"10.1.2.3"},
		Domains: []string{"Example.Invalid."}, TargetType: model.AdvancedTargetNode,
		TargetSubscriptionID: int64TestPointer(data.subscription), TargetNodeUID: stringTestPointer(data.nodeUID),
		FallbackType: model.AdvancedTargetDirect,
	}
	created, err := data.service.CreatePlatformRoute(nodeRoute)
	if err != nil {
		t.Fatal(err)
	}
	collection := &model.ProxyCollection{
		Name: "Advanced Collection", Type: "selector", SourceType: "manual", NodeUIDs: `["advanced-test-node"]`,
		ReferencedGroupIDs: "[]", RouteRuleIDs: "[]", Enabled: true,
	}
	if err := data.store.CreateProxyCollection(collection); err != nil {
		t.Fatal(err)
	}
	if _, err := data.service.CreatePlatformRoute(model.PlatformRoute{
		Name: "Collection route", Platform: "linux", Enabled: true, Priority: 20,
		TargetType: model.AdvancedTargetCollection, TargetCollectionID: int64TestPointer(int64(collection.ID)),
		FallbackType: model.AdvancedTargetDirect,
	}); err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultAdvancedSettings()
	settings.RoutingEnabled = true
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	config, err := data.service.buildRuntimeConfigLocked()
	if err != nil {
		t.Fatal(err)
	}
	node, err := data.store.GetNodeByReference(data.subscription, data.nodeUID)
	if err != nil {
		t.Fatal(err)
	}
	expectedNodeTag := buildNodeOutboundTags([]model.Node{*node})[data.nodeUID]
	if len(config.Routes) != 2 || config.Routes[0].ID != "route-"+strconvFormat(created.ID) || config.Routes[0].OutboundTag != expectedNodeTag {
		t.Fatalf("node runtime route = %+v", config.Routes)
	}
	if config.Routes[0].InboundTags[0] != "ackwrap-node-exposure-in-"+strconvFormat(data.exposureID) || config.Routes[0].SourcePrefixes[0] != "10.1.2.3/32" || config.Routes[0].Domains[0] != "example.invalid" {
		t.Fatalf("normalized runtime route = %+v", config.Routes[0])
	}
	if config.Routes[1].OutboundTag != collection.Name {
		t.Fatalf("collection outbound tag = %q", config.Routes[1].OutboundTag)
	}
	if config.AccessEventsEnabled || config.AccessEventsPrivacyMode != model.AdvancedPrivacyStrict {
		t.Fatalf("unsafe access event defaults: %+v", config)
	}
}

func TestAdvancedCreateRollsBackDatabaseAndRuntime(t *testing.T) {
	data := newAdvancedTestData(t)
	settings := store.DefaultAdvancedSettings()
	settings.RoutingEnabled = true
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	data.core.running = true
	data.runtime.failPuts = 1
	_, err := data.service.CreatePlatformRoute(model.PlatformRoute{
		Name: "Rollback route", Platform: "test", Enabled: true, Priority: 10,
		TargetType: model.AdvancedTargetDirect, FallbackType: model.AdvancedTargetDirect,
	})
	if !errors.Is(err, ErrAdvancedApply) {
		t.Fatalf("create error = %v", err)
	}
	routes, listErr := data.store.ListPlatformRoutes()
	if listErr != nil || len(routes) != 0 {
		t.Fatalf("routes after rollback = %+v, err = %v", routes, listErr)
	}
	if len(data.runtime.puts) != 2 || len(data.runtime.snapshot.Routes) != 0 {
		t.Fatalf("runtime rollback calls = %+v snapshot=%+v", data.runtime.puts, data.runtime.snapshot)
	}
}

func TestAdvancedLeaseExpiryStatusAndRuntimeFiltering(t *testing.T) {
	data := newAdvancedTestData(t)
	route := &model.PlatformRoute{Name: "Lease platform", Platform: "mobile", Enabled: true, Priority: 10, TargetType: model.AdvancedTargetDirect}
	if err := data.store.CreatePlatformRoute(route); err != nil {
		t.Fatal(err)
	}
	expired := &model.SessionLease{
		Name: "Expired lease", Enabled: true, ClientCIDR: "198.51.100.7/32", InboundExposureIDs: []int64{data.exposureID},
		PlatformRouteID: int64TestPointer(route.ID), TargetType: model.AdvancedTargetDirect,
		ExpiresAt: data.service.now().Add(-time.Minute).UnixMilli(),
	}
	if err := data.store.CreateSessionLease(expired); err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultAdvancedSettings()
	settings.LeasesEnabled = true
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	config, err := data.service.buildRuntimeConfigLocked()
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Leases) != 0 {
		t.Fatalf("expired lease reached runtime: %+v", config.Leases)
	}
	leases, err := data.service.ListSessionLeases()
	if err != nil || len(leases) != 1 || leases[0].Status != model.SessionLeaseExpired {
		t.Fatalf("lease response = %+v, err = %v", leases, err)
	}
}

func TestAdvancedSettingsAndAccessPrivacy(t *testing.T) {
	data := newAdvancedTestData(t)
	invalid := store.DefaultAdvancedSettings()
	invalid.HealthIntervalSeconds = 0
	if _, err := data.service.UpdateSettings(&invalid); !errors.Is(err, ErrAdvancedInvalid) {
		t.Fatalf("invalid settings error = %v", err)
	}
	event := RuntimeAccessEvent{
		ID: 7, Time: 100, Network: "tcp", Inbound: "ackwrap-node-exposure-in-1",
		SourceIP: "203.0.113.9", DestinationIP: "198.51.100.42", Domain: "Sensitive.Example",
		OutboundTag: "direct", Platform: "desktop", RouteID: "route-3", LeaseID: "lease-4", Decision: "route",
	}
	strict := data.service.safeAccessLogs([]RuntimeAccessEvent{event}, model.AdvancedPrivacyStrict)[0]
	if strict.SourceHash != "" || strict.DestinationSummary != "" || strict.DomainSummary != "" || strict.Platform != "" {
		t.Fatalf("strict privacy retained endpoint data: %+v", strict)
	}
	balanced := data.service.safeAccessLogs([]RuntimeAccessEvent{event}, model.AdvancedPrivacyBalanced)[0]
	if balanced.SourceHash == "" || balanced.DestinationSummary != "198.51.100.0/24" || balanced.DomainSummary == "" || strings.Contains(balanced.DomainSummary, "example") || balanced.Platform != "desktop" {
		t.Fatalf("balanced privacy result = %+v", balanced)
	}
	if balanced.PlatformRouteID == nil || *balanced.PlatformRouteID != 3 || balanced.SessionLeaseID == nil || *balanced.SessionLeaseID != 4 {
		t.Fatalf("runtime IDs were not parsed: %+v", balanced)
	}
	if err := data.store.UpdateNodeName(data.nodeUID, "password=sentinel"); err != nil {
		t.Fatal(err)
	}
	node, err := data.store.GetNodeByReference(data.subscription, data.nodeUID)
	if err != nil {
		t.Fatal(err)
	}
	event.OutboundTag = buildNodeOutboundTags([]model.Node{*node})[data.nodeUID]
	protected := data.service.safeAccessLogs([]RuntimeAccessEvent{event}, model.AdvancedPrivacyBalanced)[0]
	if !strings.HasPrefix(protected.OutboundLabel, "node-") || strings.Contains(protected.OutboundLabel, "sentinel") || strings.Contains(protected.OutboundLabel, "password") {
		t.Fatalf("node name reached access persistence: %+v", protected)
	}
	target, err := data.service.resolveTarget(model.AdvancedTargetNode, &data.subscription, &data.nodeUID, nil)
	if err != nil || !strings.HasPrefix(target.displayName, "node-") || strings.Contains(target.displayName, "sentinel") {
		t.Fatalf("node name reached health display: target=%+v err=%v", target, err)
	}
	event.Platform = "https://secret.example"
	if unsafe := data.service.safeAccessLogs([]RuntimeAccessEvent{event}, model.AdvancedPrivacyBalanced)[0]; unsafe.Platform != "" {
		t.Fatalf("unsafe platform reached access persistence: %+v", unsafe)
	}
}

func TestAdvancedAccessCollectorUsesIncrementalCursorAndDedupe(t *testing.T) {
	data := newAdvancedTestData(t)
	settings := store.DefaultAdvancedSettings()
	settings.AccessLogsEnabled = true
	settings.AccessLogPrivacyMode = model.AdvancedPrivacyBalanced
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	data.core.running = true
	data.runtime.events = RuntimeAccessEventList{
		LatestID: 9,
		Items: []RuntimeAccessEvent{
			{ID: 2, Time: time.Now().UnixMilli(), SourceIP: "203.0.113.2", DestinationIP: "198.51.100.2", Domain: "one.invalid", OutboundTag: "direct", Decision: "route"},
			{ID: 3, Time: time.Now().UnixMilli(), SourceIP: "203.0.113.3", DestinationIP: "198.51.100.3", Domain: "two.invalid", OutboundTag: "direct", Decision: "route"},
		},
	}
	data.service.collectAccessEvents()
	if data.service.accessCursor != 3 {
		t.Fatalf("access cursor = %d, want last consumed item 3", data.service.accessCursor)
	}
	if len(data.runtime.accesses) != 1 || data.runtime.accesses[0] != 0 {
		t.Fatalf("access event requests = %+v", data.runtime.accesses)
	}
	data.service.collectAccessEvents()
	page, err := data.store.ListAdvancedAccessLogs(model.AdvancedAccessLogFilter{Limit: 10})
	if err != nil || page.Total != 2 || len(page.Items) != 2 {
		t.Fatalf("deduplicated access logs = %+v, err = %v", page, err)
	}
}

func TestAdvancedAccessCollectorSkipsDisabledAndRetriesPersistenceFailure(t *testing.T) {
	data := newAdvancedTestData(t)
	data.core.running = true
	data.runtime.events = RuntimeAccessEventList{Items: []RuntimeAccessEvent{{
		ID: 11, Time: time.Now().UnixMilli(), OutboundTag: "direct", RouteID: "route-999", Decision: "route",
	}}}
	data.service.collectAccessEvents()
	if len(data.runtime.accesses) != 0 || data.service.accessCursor != 0 {
		t.Fatalf("disabled collector fetched events: calls=%+v cursor=%d", data.runtime.accesses, data.service.accessCursor)
	}
	settings := store.DefaultAdvancedSettings()
	settings.AccessLogsEnabled = true
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	data.service.collectAccessEvents()
	if data.service.accessCursor != 0 {
		t.Fatalf("failed persistence advanced cursor to %d", data.service.accessCursor)
	}
	data.runtime.events.Items[0].RouteID = ""
	data.service.collectAccessEvents()
	if data.service.accessCursor != 11 {
		t.Fatalf("successful retry cursor = %d", data.service.accessCursor)
	}
}

func TestAdvancedRunHealthRejectsDisabledAndStoppedCore(t *testing.T) {
	data := newAdvancedTestData(t)
	if _, err := data.service.RunHealth(); !errors.Is(err, ErrAdvancedInvalid) {
		t.Fatalf("disabled health error = %v", err)
	}
	settings := store.DefaultAdvancedSettings()
	settings.HealthEnabled = true
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	if _, err := data.service.RunHealth(); !errors.Is(err, ErrAdvancedInvalid) {
		t.Fatalf("stopped core health error = %v", err)
	}
}

func TestAdvancedHealthThresholdAndRecovery(t *testing.T) {
	data := newAdvancedTestData(t)
	if _, err := data.service.CreatePlatformRoute(model.PlatformRoute{
		Name: "Health route", Platform: "health", Enabled: true, Priority: 10,
		TargetType: model.AdvancedTargetNode, TargetSubscriptionID: int64TestPointer(data.subscription),
		TargetNodeUID: stringTestPointer(data.nodeUID), FallbackType: model.AdvancedTargetDirect,
	}); err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultAdvancedSettings()
	settings.RoutingEnabled, settings.HealthEnabled = true, true
	settings.FailureThreshold, settings.RecoveryThreshold, settings.CircuitOpenSeconds = 2, 2, 10
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, time.July, 27, 12, 0, 0, 0, time.UTC)
	data.service.now = func() time.Time { return current }
	data.core.running = true
	results := []error{errors.New("probe unavailable"), errors.New("probe unavailable"), nil, nil}
	data.service.probe = func(advancedResolvedTarget, time.Duration) (int, error) {
		err := results[0]
		results = results[1:]
		if err != nil {
			return 0, err
		}
		return 25, nil
	}
	if err := data.service.runHealth(); err != nil {
		t.Fatal(err)
	}
	assertAdvancedHealthStatus(t, data.store, model.AdvancedHealthUnhealthy)
	if err := data.service.runHealth(); err != nil {
		t.Fatal(err)
	}
	assertAdvancedHealthStatus(t, data.store, model.AdvancedHealthCircuitOpen)
	config, err := data.service.buildRuntimeConfigLocked()
	if err != nil || len(config.UnhealthyOutbounds) != 1 {
		t.Fatalf("circuit runtime config = %+v, err = %v", config, err)
	}
	current = current.Add(11 * time.Second)
	config, err = data.service.buildRuntimeConfigLocked()
	if err != nil || len(config.UnhealthyOutbounds) != 1 {
		t.Fatalf("expired cooldown re-enabled target before probe: %+v, err = %v", config, err)
	}
	if err := data.service.runHealth(); err != nil {
		t.Fatal(err)
	}
	assertAdvancedHealthStatus(t, data.store, model.AdvancedHealthCircuitOpen)
	config, err = data.service.buildRuntimeConfigLocked()
	if err != nil || len(config.UnhealthyOutbounds) != 1 {
		t.Fatalf("partial recovery re-enabled target: %+v, err = %v", config, err)
	}
	current = current.Add(61 * time.Second)
	if err := data.service.runHealth(); err != nil {
		t.Fatal(err)
	}
	assertAdvancedHealthStatus(t, data.store, model.AdvancedHealthHealthy)
}

func TestAdvancedHealthRuntimeFailureDoesNotPersistTransition(t *testing.T) {
	data := newAdvancedTestData(t)
	if _, err := data.service.CreatePlatformRoute(model.PlatformRoute{
		Name: "Health rollback route", Platform: "health", Enabled: true, Priority: 10,
		TargetType: model.AdvancedTargetNode, TargetSubscriptionID: int64TestPointer(data.subscription),
		TargetNodeUID: stringTestPointer(data.nodeUID), FallbackType: model.AdvancedTargetDirect,
	}); err != nil {
		t.Fatal(err)
	}
	settings := store.DefaultAdvancedSettings()
	settings.RoutingEnabled, settings.HealthEnabled = true, true
	settings.FailureThreshold = 1
	if err := data.store.SetAdvancedSettings(&settings); err != nil {
		t.Fatal(err)
	}
	data.core.running = true
	data.runtime.failPuts = 1
	data.runtime.applyBeforeFailure = true
	data.service.probe = func(advancedResolvedTarget, time.Duration) (int, error) {
		return 0, errors.New("probe unavailable")
	}
	if _, err := data.service.RunHealth(); err != nil {
		t.Fatalf("start health check = %v", err)
	}
	data.service.healthRuns.Wait()
	states, err := data.store.ListAdvancedHealthStates()
	if err != nil || len(states) != 0 {
		t.Fatalf("failed runtime transition persisted states: %+v, err=%v", states, err)
	}
	events, err := data.store.ListAdvancedHealthEvents(10, 0)
	if err != nil || len(events) != 0 {
		t.Fatalf("failed runtime transition persisted events: %+v, err=%v", events, err)
	}
	if len(data.runtime.snapshot.UnhealthyOutbounds) != 0 {
		t.Fatalf("failed transition changed runtime: %+v", data.runtime.snapshot)
	}
	if len(data.runtime.puts) != 2 {
		t.Fatalf("failed transition did not restore runtime: puts=%+v", data.runtime.puts)
	}
}

func TestAdvancedDeleteRemovesInactiveUnhealthyTarget(t *testing.T) {
	t.Run("platform route", func(t *testing.T) {
		data := newAdvancedTestData(t)
		route, err := data.service.CreatePlatformRoute(model.PlatformRoute{
			Name: "Delete route", Platform: "delete", Enabled: true, Priority: 10,
			TargetType: model.AdvancedTargetNode, TargetSubscriptionID: int64TestPointer(data.subscription),
			TargetNodeUID: stringTestPointer(data.nodeUID), FallbackType: model.AdvancedTargetDirect,
		})
		if err != nil {
			t.Fatal(err)
		}
		settings := store.DefaultAdvancedSettings()
		settings.RoutingEnabled, settings.HealthEnabled = true, true
		if err := data.store.SetAdvancedSettings(&settings); err != nil {
			t.Fatal(err)
		}
		upsertOpenHealthState(t, data)
		data.core.running = true
		if err := data.service.SyncRuntime(); err != nil {
			t.Fatal(err)
		}
		if len(data.runtime.snapshot.Routes) != 1 || len(data.runtime.snapshot.UnhealthyOutbounds) != 1 {
			t.Fatalf("pre-delete runtime = %+v", data.runtime.snapshot)
		}
		if _, err := data.service.DeletePlatformRoute(route.ID); err != nil {
			t.Fatal(err)
		}
		if len(data.runtime.snapshot.Routes) != 0 || len(data.runtime.snapshot.UnhealthyOutbounds) != 0 {
			t.Fatalf("deleted route retained unhealthy target: %+v", data.runtime.snapshot)
		}
	})

	t.Run("session lease", func(t *testing.T) {
		data := newAdvancedTestData(t)
		route, err := data.service.CreatePlatformRoute(model.PlatformRoute{
			Name: "Lease label", Platform: "lease", Enabled: true, Priority: 10,
			TargetType: model.AdvancedTargetDirect, FallbackType: model.AdvancedTargetDirect,
		})
		if err != nil {
			t.Fatal(err)
		}
		lease, err := data.service.CreateSessionLease(model.SessionLease{
			Name: "Delete lease", Enabled: true, ClientCIDR: "198.51.100.8/32",
			InboundExposureIDs: []int64{data.exposureID}, PlatformRouteID: int64TestPointer(route.ID),
			TargetType: model.AdvancedTargetNode, TargetSubscriptionID: int64TestPointer(data.subscription),
			TargetNodeUID: stringTestPointer(data.nodeUID), FallbackType: model.AdvancedTargetDirect,
			ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		})
		if err != nil {
			t.Fatal(err)
		}
		settings := store.DefaultAdvancedSettings()
		settings.LeasesEnabled, settings.HealthEnabled = true, true
		if err := data.store.SetAdvancedSettings(&settings); err != nil {
			t.Fatal(err)
		}
		upsertOpenHealthState(t, data)
		data.core.running = true
		if err := data.service.SyncRuntime(); err != nil {
			t.Fatal(err)
		}
		if len(data.runtime.snapshot.Leases) != 1 || len(data.runtime.snapshot.UnhealthyOutbounds) != 1 {
			t.Fatalf("pre-delete runtime = %+v", data.runtime.snapshot)
		}
		if _, err := data.service.DeleteSessionLease(lease.ID); err != nil {
			t.Fatal(err)
		}
		if len(data.runtime.snapshot.Leases) != 0 || len(data.runtime.snapshot.UnhealthyOutbounds) != 0 {
			t.Fatalf("deleted lease retained unhealthy target: %+v", data.runtime.snapshot)
		}
	})
}

func upsertOpenHealthState(t *testing.T, data advancedTestData) {
	t.Helper()
	targetRef := strconvFormat(data.subscription) + ":" + data.nodeUID
	if err := data.store.UpsertAdvancedHealthState(&model.AdvancedHealthState{
		TargetKey: "node:" + targetRef, TargetType: model.AdvancedTargetNode, TargetRef: targetRef,
		DisplayName: "Test Node", Status: model.AdvancedHealthCircuitOpen,
	}); err != nil {
		t.Fatal(err)
	}
}

func assertAdvancedHealthStatus(t *testing.T, db *store.Store, want string) {
	t.Helper()
	states, err := db.ListAdvancedHealthStates()
	if err != nil || len(states) != 1 || states[0].Status != want {
		t.Fatalf("health states = %+v, err = %v, want %s", states, err, want)
	}
}

func int64TestPointer(value int64) *int64    { return &value }
func stringTestPointer(value string) *string { return &value }
func strconvFormat(value int64) string       { return strconv.FormatInt(value, 10) }
