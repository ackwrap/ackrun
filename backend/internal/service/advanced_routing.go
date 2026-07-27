package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
	"github.com/ackwrap/ackrun/internal/store"
)

var (
	ErrAdvancedInvalid  = errors.New("高级路由配置无效")
	ErrAdvancedNotFound = errors.New("高级路由资源不存在")
	ErrAdvancedApply    = errors.New("应用高级路由运行时失败")
)

type advancedRuntime interface {
	GetRuntimeRouting() (RuntimeRoutingConfig, error)
	PutRuntimeRouting(RuntimeRoutingConfig) error
	GetAccessEvents(after uint64, limit int) (RuntimeAccessEventList, error)
}

type advancedCore interface {
	IsRunning() bool
}

type AdvancedRoutingService struct {
	store      *store.Store
	runtime    advancedRuntime
	core       advancedCore
	realtime   *RealtimeService
	hashSecret string
	now        func() time.Time

	mu sync.Mutex

	lifecycleMu sync.Mutex
	stop        chan struct{}
	runContext  context.Context
	runCancel   context.CancelFunc
	done        sync.WaitGroup
	started     bool

	healthMu      sync.Mutex
	healthRunning bool
	lastHealthRun time.Time
	healthRuns    sync.WaitGroup

	accessMu     sync.Mutex
	accessCursor uint64
	lastCleanup  time.Time
	probe        func(target advancedResolvedTarget, timeout time.Duration) (int, error)
	healthClient *http.Client
}

type advancedResolvedTarget struct {
	targetType  string
	targetRef   string
	targetKey   string
	tag         string
	displayName string
}

func NewAdvancedRoutingService(db *store.Store, runtime advancedRuntime, core advancedCore, realtime *RealtimeService, hashSecret string) *AdvancedRoutingService {
	svc := &AdvancedRoutingService{
		store: db, runtime: runtime, core: core, realtime: realtime, hashSecret: hashSecret,
		now: time.Now,
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	svc.healthClient = &http.Client{Transport: transport}
	svc.probe = svc.probeTarget
	return svc
}

func (svc *AdvancedRoutingService) ListPlatformRoutes() ([]model.PlatformRoute, error) {
	logging.Info("advanced.platform_route.list", "读取平台路由")
	return svc.store.ListPlatformRoutes()
}

func (svc *AdvancedRoutingService) CreatePlatformRoute(request model.PlatformRoute) (*model.PlatformRoute, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	item := request
	if err := svc.normalizePlatformRoute(&item, false); err != nil {
		return nil, err
	}
	logging.Info("advanced.platform_route.create", "创建平台路由")
	if err := svc.store.CreatePlatformRoute(&item); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.DeletePlatformRoute(item.ID) }); err != nil {
		return nil, err
	}
	return svc.store.GetPlatformRoute(item.ID)
}

func (svc *AdvancedRoutingService) UpdatePlatformRoute(id int64, request model.PlatformRoute) (*model.PlatformRoute, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	existing, err := svc.store.GetPlatformRoute(id)
	if err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	item := request
	item.ID = id
	item.CreatedAt = existing.CreatedAt
	if item.Priority == 0 {
		item.Priority = existing.Priority
	}
	if err := svc.normalizePlatformRoute(&item, true); err != nil {
		return nil, err
	}
	logging.Info("advanced.platform_route.update", "更新平台路由: %d", id)
	if err := svc.store.UpdatePlatformRoute(id, &item); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.UpdatePlatformRoute(id, existing) }); err != nil {
		return nil, err
	}
	return svc.store.GetPlatformRoute(id)
}

func (svc *AdvancedRoutingService) DeletePlatformRoute(id int64) (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, err := svc.store.GetPlatformRoute(id); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	leases, err := svc.store.ListSessionLeases()
	if err != nil {
		return nil, err
	}
	for _, lease := range leases {
		if lease.PlatformRouteID != nil && *lease.PlatformRouteID == id {
			return nil, fmt.Errorf("%w: 平台路由仍被会话租约引用", ErrAdvancedInvalid)
		}
	}
	logging.Info("advanced.platform_route.delete", "删除平台路由: %d", id)
	if err := svc.deleteWithRuntimeFirstLocked(func(config RuntimeRoutingConfig) RuntimeRoutingConfig {
		filtered := config.Routes[:0]
		for _, route := range config.Routes {
			if route.ID != "route-"+strconv.FormatInt(id, 10) {
				filtered = append(filtered, route)
			}
		}
		config.Routes = filtered
		return config
	}, func() error { return svc.store.DeletePlatformRoute(id) }); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "platform route deleted"}, nil
}

func (svc *AdvancedRoutingService) ReorderPlatformRoutes(ids []int64) (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: ids 不能为空", ErrAdvancedInvalid)
	}
	old, err := svc.store.ListPlatformRoutes()
	if err != nil {
		return nil, err
	}
	oldIDs := make([]int64, len(old))
	for index := range old {
		oldIDs[index] = old[index].ID
	}
	logging.Info("advanced.platform_route.reorder", "调整 %d 项平台路由顺序", len(ids))
	if err := svc.store.ReorderPlatformRoutes(ids); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.ReorderPlatformRoutes(oldIDs) }); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "platform routes reordered"}, nil
}

func (svc *AdvancedRoutingService) ListSessionLeases() ([]model.SessionLease, error) {
	logging.Info("advanced.session_lease.list", "读取会话租约")
	items, err := svc.store.ListSessionLeases()
	if err != nil {
		return nil, err
	}
	now := svc.now()
	for index := range items {
		items[index].Status = items[index].StatusAt(now)
	}
	return items, nil
}

func (svc *AdvancedRoutingService) CreateSessionLease(request model.SessionLease) (*model.SessionLease, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	item := request
	if err := svc.normalizeSessionLease(&item); err != nil {
		return nil, err
	}
	logging.Info("advanced.session_lease.create", "创建会话租约")
	if err := svc.store.CreateSessionLease(&item); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.DeleteSessionLease(item.ID) }); err != nil {
		return nil, err
	}
	return svc.getSessionLease(item.ID)
}

func (svc *AdvancedRoutingService) UpdateSessionLease(id int64, request model.SessionLease) (*model.SessionLease, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	existing, err := svc.store.GetSessionLease(id)
	if err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	item := request
	item.ID, item.CreatedAt = id, existing.CreatedAt
	if err := svc.normalizeSessionLease(&item); err != nil {
		return nil, err
	}
	logging.Info("advanced.session_lease.update", "更新会话租约: %d", id)
	if err := svc.store.UpdateSessionLease(id, &item); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.UpdateSessionLease(id, existing) }); err != nil {
		return nil, err
	}
	return svc.getSessionLease(id)
}

func (svc *AdvancedRoutingService) DeleteSessionLease(id int64) (*model.ActionResponse, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, err := svc.store.GetSessionLease(id); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	logging.Info("advanced.session_lease.delete", "删除会话租约: %d", id)
	if err := svc.deleteWithRuntimeFirstLocked(func(config RuntimeRoutingConfig) RuntimeRoutingConfig {
		filtered := config.Leases[:0]
		for _, lease := range config.Leases {
			if lease.ID != "lease-"+strconv.FormatInt(id, 10) {
				filtered = append(filtered, lease)
			}
		}
		config.Leases = filtered
		return config
	}, func() error { return svc.store.DeleteSessionLease(id) }); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "session lease deleted"}, nil
}

func (svc *AdvancedRoutingService) RenewSessionLease(id int64, durationMinutes int) (*model.SessionLease, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if durationMinutes < 1 || durationMinutes > 525600 {
		return nil, fmt.Errorf("%w: duration_minutes 必须在 1 到 525600 之间", ErrAdvancedInvalid)
	}
	existing, err := svc.store.GetSessionLease(id)
	if err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	base := svc.now()
	if existing.ExpiresAt > base.UnixMilli() {
		base = time.UnixMilli(existing.ExpiresAt)
	}
	expiresAt := base.Add(time.Duration(durationMinutes) * time.Minute).UnixMilli()
	logging.Info("advanced.session_lease.renew", "续期会话租约: %d", id)
	if err := svc.store.UpdateSessionLeaseExpiresAt(id, expiresAt); err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.UpdateSessionLeaseExpiresAt(id, existing.ExpiresAt) }); err != nil {
		return nil, err
	}
	return svc.getSessionLease(id)
}

func (svc *AdvancedRoutingService) GetSettings() (*model.AdvancedSettings, error) {
	return svc.store.GetAdvancedSettings()
}

func (svc *AdvancedRoutingService) UpdateSettings(settings *model.AdvancedSettings) (*model.AdvancedSettings, error) {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	svc.accessMu.Lock()
	defer svc.accessMu.Unlock()
	if err := validateAdvancedSettings(settings); err != nil {
		return nil, err
	}
	previous, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return nil, err
	}
	if err := svc.store.SetAdvancedSettings(settings); err != nil {
		return nil, err
	}
	if err := svc.applyMutationLocked(func() error { return svc.store.SetAdvancedSettings(previous) }); err != nil {
		return nil, err
	}
	logging.Info("advanced.settings.update", "高级功能设置已更新")
	return svc.store.GetAdvancedSettings()
}

func (svc *AdvancedRoutingService) SyncRuntime() error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if err := svc.syncRuntimeLocked(); err != nil {
		logging.Error("advanced.runtime.sync", "同步高级路由运行时失败")
		return ErrAdvancedApply
	}
	return nil
}

func (svc *AdvancedRoutingService) syncRuntimeLocked() error {
	if svc.runtime == nil {
		return nil
	}
	config, err := svc.buildRuntimeConfigLocked()
	if err != nil {
		return err
	}
	logging.Info("advanced.runtime.sync", "同步高级路由: routes=%d leases=%d unhealthy=%d", len(config.Routes), len(config.Leases), len(config.UnhealthyOutbounds))
	return svc.runtime.PutRuntimeRouting(config)
}

func (svc *AdvancedRoutingService) applyMutationLocked(rollback func() error) error {
	if svc.runtime == nil || svc.core == nil || !svc.core.IsRunning() {
		return nil
	}
	previous, err := svc.runtime.GetRuntimeRouting()
	if err != nil {
		return svc.rollbackMutation(rollback, nil, err)
	}
	desired, err := svc.buildRuntimeConfigLocked()
	if err != nil {
		return svc.rollbackMutation(rollback, nil, err)
	}
	if err := svc.runtime.PutRuntimeRouting(desired); err != nil {
		return svc.rollbackMutation(rollback, &previous, err)
	}
	return nil
}

func (svc *AdvancedRoutingService) rollbackMutation(rollback func() error, previous *RuntimeRoutingConfig, cause error) error {
	if rollbackErr := rollback(); rollbackErr != nil {
		return fmt.Errorf("%w: %v；回滚数据库失败: %w", ErrAdvancedApply, cause, rollbackErr)
	}
	if previous != nil {
		if restoreErr := svc.runtime.PutRuntimeRouting(*previous); restoreErr != nil {
			return fmt.Errorf("%w: %v；数据库已回滚，但恢复运行时失败: %w", ErrAdvancedApply, cause, restoreErr)
		}
	}
	return fmt.Errorf("%w，数据库和运行时已恢复: %v", ErrAdvancedApply, cause)
}

func (svc *AdvancedRoutingService) deleteWithRuntimeFirstLocked(remove func(RuntimeRoutingConfig) RuntimeRoutingConfig, persist func() error) error {
	if svc.runtime == nil || svc.core == nil || !svc.core.IsRunning() {
		return normalizeAdvancedStoreError(persist())
	}
	previous, err := svc.runtime.GetRuntimeRouting()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrAdvancedApply, err)
	}
	desired, err := svc.buildRuntimeConfigLocked()
	if err != nil {
		return err
	}
	desired = remove(desired)
	desired.UnhealthyOutbounds = activeRuntimeUnhealthyOutbounds(desired)
	if err := svc.runtime.PutRuntimeRouting(desired); err != nil {
		if restoreErr := svc.runtime.PutRuntimeRouting(previous); restoreErr != nil {
			return fmt.Errorf("%w: %v；恢复旧运行时快照失败: %w", ErrAdvancedApply, err, restoreErr)
		}
		return fmt.Errorf("%w: %v", ErrAdvancedApply, err)
	}
	if err := persist(); err != nil {
		if restoreErr := svc.runtime.PutRuntimeRouting(previous); restoreErr != nil {
			return fmt.Errorf("%v；恢复运行时失败: %w", normalizeAdvancedStoreError(err), restoreErr)
		}
		return normalizeAdvancedStoreError(err)
	}
	return nil
}

func (svc *AdvancedRoutingService) buildRuntimeConfigLocked() (RuntimeRoutingConfig, error) {
	return svc.buildRuntimeConfigWithHealthLocked(nil)
}

func (svc *AdvancedRoutingService) buildRuntimeConfigWithHealthLocked(healthOverrides map[string]model.AdvancedHealthState) (RuntimeRoutingConfig, error) {
	settings, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return RuntimeRoutingConfig{}, err
	}
	config := RuntimeRoutingConfig{
		Routes: []RuntimeRoute{}, Leases: []RuntimeLease{}, UnhealthyOutbounds: []string{},
		AccessEventsEnabled: settings.AccessLogsEnabled, AccessEventsPrivacyMode: settings.AccessLogPrivacyMode,
	}
	routes, err := svc.store.ListPlatformRoutes()
	if err != nil {
		return config, err
	}
	if settings.RoutingEnabled {
		for index := range routes {
			route := &routes[index]
			if !route.Enabled {
				continue
			}
			if err := svc.normalizePlatformRoute(route, true); err != nil {
				return config, fmt.Errorf("平台路由 %d: %w", route.ID, err)
			}
			target, err := svc.resolveTarget(route.TargetType, route.TargetSubscriptionID, route.TargetNodeUID, route.TargetCollectionID)
			if err != nil {
				return config, err
			}
			fallback, err := svc.resolveTarget(route.FallbackType, route.FallbackSubscriptionID, route.FallbackNodeUID, route.FallbackCollectionID)
			if err != nil {
				return config, err
			}
			config.Routes = append(config.Routes, RuntimeRoute{
				ID: "route-" + strconv.FormatInt(route.ID, 10), Priority: route.Priority, Platform: route.Platform,
				InboundTags: exposureTags(route.InboundExposureIDs), SourcePrefixes: route.SourceCIDRs,
				Domains: route.Domains, DomainSuffixes: route.DomainSuffixes, DomainKeywords: route.DomainKeywords,
				DestinationPrefixes: route.DestinationCIDRs, OutboundTag: target.tag, FallbackOutboundTag: fallback.tag,
			})
		}
	}
	if settings.LeasesEnabled {
		leases, err := svc.store.ListSessionLeases()
		if err != nil {
			return config, err
		}
		now := svc.now()
		for index := range leases {
			lease := &leases[index]
			if !lease.Enabled || lease.IsExpiredAt(now) {
				continue
			}
			if err := svc.normalizeSessionLease(lease); err != nil {
				return config, fmt.Errorf("会话租约 %d: %w", lease.ID, err)
			}
			target, err := svc.resolveTarget(lease.TargetType, lease.TargetSubscriptionID, lease.TargetNodeUID, lease.TargetCollectionID)
			if err != nil {
				return config, err
			}
			fallback, err := svc.resolveTarget(lease.FallbackType, lease.FallbackSubscriptionID, lease.FallbackNodeUID, lease.FallbackCollectionID)
			if err != nil {
				return config, err
			}
			route, err := svc.store.GetPlatformRoute(*lease.PlatformRouteID)
			if err != nil {
				return config, normalizeAdvancedStoreError(err)
			}
			config.Leases = append(config.Leases, RuntimeLease{
				ID: "lease-" + strconv.FormatInt(lease.ID, 10), SourcePrefix: lease.ClientCIDR,
				InboundTags: exposureTags(lease.InboundExposureIDs), Platform: route.Platform,
				OutboundTag: target.tag, FallbackOutboundTag: fallback.tag, ExpiresAt: lease.ExpiresAt,
			})
		}
	}
	if settings.HealthEnabled {
		targets, err := svc.healthTargets(settings)
		if err != nil {
			return config, err
		}
		activeTargets := make(map[string]bool, len(targets))
		for _, target := range targets {
			activeTargets[target.targetKey] = true
		}
		states, err := svc.store.ListAdvancedHealthStates()
		if err != nil {
			return config, err
		}
		stateIndexes := make(map[string]int, len(states))
		for index := range states {
			stateIndexes[states[index].TargetKey] = index
		}
		for key, state := range healthOverrides {
			if index, exists := stateIndexes[key]; exists {
				states[index] = state
			} else {
				states = append(states, state)
			}
		}
		seen := make(map[string]bool)
		for _, state := range states {
			if state.Status != model.AdvancedHealthCircuitOpen || !activeTargets[state.TargetKey] {
				continue
			}
			target, err := svc.resolveHealthStateTarget(state)
			if err != nil || target.tag == "direct" || seen[target.tag] {
				continue
			}
			seen[target.tag] = true
			config.UnhealthyOutbounds = append(config.UnhealthyOutbounds, target.tag)
		}
	}
	sort.Strings(config.UnhealthyOutbounds)
	return config, nil
}

func (svc *AdvancedRoutingService) normalizePlatformRoute(item *model.PlatformRoute, updating bool) error {
	item.Name = strings.TrimSpace(item.Name)
	item.Platform = strings.TrimSpace(item.Platform)
	if err := validateAdvancedName(item.Name, "名称"); err != nil {
		return err
	}
	if !validAdvancedPlatformLabel(item.Platform) {
		return fmt.Errorf("%w: platform 必须是 64 字以内的业务标签", ErrAdvancedInvalid)
	}
	if item.Priority < 0 || item.Priority > 1000000 || updating && item.Priority == 0 {
		return fmt.Errorf("%w: priority 必须在 1 到 1000000 之间", ErrAdvancedInvalid)
	}
	var err error
	if item.InboundExposureIDs, err = svc.normalizeExposureIDs(item.InboundExposureIDs, false); err != nil {
		return err
	}
	if item.SourceCIDRs, err = normalizeAdvancedPrefixes(item.SourceCIDRs, "source_cidrs"); err != nil {
		return err
	}
	if item.DestinationCIDRs, err = normalizeAdvancedPrefixes(item.DestinationCIDRs, "destination_cidrs"); err != nil {
		return err
	}
	if item.Domains, err = normalizeAdvancedDomains(item.Domains, "domains"); err != nil {
		return err
	}
	if item.DomainSuffixes, err = normalizeAdvancedDomains(item.DomainSuffixes, "domain_suffixes"); err != nil {
		return err
	}
	if item.DomainKeywords, err = normalizeAdvancedKeywords(item.DomainKeywords); err != nil {
		return err
	}
	if _, err = svc.resolveTarget(item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID, item.TargetCollectionID); err != nil {
		return err
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	_, err = svc.resolveTarget(item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID, item.FallbackCollectionID)
	return err
}

func (svc *AdvancedRoutingService) normalizeSessionLease(item *model.SessionLease) error {
	item.Name = strings.TrimSpace(item.Name)
	if err := validateAdvancedName(item.Name, "名称"); err != nil {
		return err
	}
	prefixes, err := normalizeAdvancedPrefixes([]string{item.ClientCIDR}, "client_cidr")
	if err != nil {
		return err
	}
	item.ClientCIDR = prefixes[0]
	item.InboundExposureIDs, err = svc.normalizeExposureIDs(item.InboundExposureIDs, true)
	if err != nil {
		return err
	}
	if item.PlatformRouteID == nil || *item.PlatformRouteID <= 0 {
		return fmt.Errorf("%w: platform_route_id 必须引用现有平台路由", ErrAdvancedInvalid)
	}
	if _, err := svc.store.GetPlatformRoute(*item.PlatformRouteID); err != nil {
		return fmt.Errorf("%w: platform_route_id 不存在", ErrAdvancedInvalid)
	}
	if item.ExpiresAt <= svc.now().UnixMilli() {
		return fmt.Errorf("%w: expires_at 必须晚于当前时间", ErrAdvancedInvalid)
	}
	if _, err := svc.resolveTarget(item.TargetType, item.TargetSubscriptionID, item.TargetNodeUID, item.TargetCollectionID); err != nil {
		return err
	}
	if item.FallbackType == "" {
		item.FallbackType = model.AdvancedTargetDirect
	}
	_, err = svc.resolveTarget(item.FallbackType, item.FallbackSubscriptionID, item.FallbackNodeUID, item.FallbackCollectionID)
	return err
}

func (svc *AdvancedRoutingService) normalizeExposureIDs(ids []int64, required bool) ([]int64, error) {
	if required && len(ids) == 0 {
		return nil, fmt.Errorf("%w: 至少需要一个入口暴露", ErrAdvancedInvalid)
	}
	result := make([]int64, 0, len(ids))
	seen := make(map[int64]bool)
	for _, id := range ids {
		if id <= 0 || seen[id] {
			return nil, fmt.Errorf("%w: 入口暴露 ID 无效或重复", ErrAdvancedInvalid)
		}
		if _, err := svc.store.GetNodeExposure(id); err != nil {
			return nil, fmt.Errorf("%w: 入口暴露 %d 不存在", ErrAdvancedInvalid, id)
		}
		seen[id] = true
		result = append(result, id)
	}
	return result, nil
}

func (svc *AdvancedRoutingService) resolveTarget(targetType string, subscriptionID *int64, nodeUID *string, collectionID *int64) (advancedResolvedTarget, error) {
	targetType = strings.ToLower(strings.TrimSpace(targetType))
	switch targetType {
	case model.AdvancedTargetDirect:
		if subscriptionID != nil || nodeUID != nil || collectionID != nil {
			return advancedResolvedTarget{}, fmt.Errorf("%w: direct target 不能包含引用字段", ErrAdvancedInvalid)
		}
		return advancedResolvedTarget{targetType: targetType, targetKey: "direct", tag: "direct", displayName: "direct"}, nil
	case model.AdvancedTargetNode:
		if subscriptionID == nil || *subscriptionID <= 0 || nodeUID == nil || strings.TrimSpace(*nodeUID) == "" || collectionID != nil {
			return advancedResolvedTarget{}, fmt.Errorf("%w: node target 引用不完整", ErrAdvancedInvalid)
		}
		uid := strings.TrimSpace(*nodeUID)
		node, err := svc.store.GetNodeByReference(*subscriptionID, uid)
		if err != nil || !node.Enabled {
			return advancedResolvedTarget{}, fmt.Errorf("%w: node target 不存在或已停用", ErrAdvancedInvalid)
		}
		tag := buildNodeOutboundTags([]model.Node{*node})[node.UID]
		ref := fmt.Sprintf("%d:%s", *subscriptionID, uid)
		return advancedResolvedTarget{
			targetType: targetType, targetRef: ref, targetKey: "node:" + ref, tag: tag,
			displayName: "node-" + svc.shortHash("health-target", ref),
		}, nil
	case model.AdvancedTargetCollection:
		if collectionID == nil || *collectionID <= 0 || subscriptionID != nil || nodeUID != nil {
			return advancedResolvedTarget{}, fmt.Errorf("%w: collection target 引用不完整", ErrAdvancedInvalid)
		}
		collection, err := svc.store.GetProxyCollectionWithNodes(int(*collectionID))
		if err != nil || !collection.Enabled || strings.TrimSpace(collection.Name) == "" {
			return advancedResolvedTarget{}, fmt.Errorf("%w: collection target 不存在或已停用", ErrAdvancedInvalid)
		}
		valid, err := svc.collectionHasEnabledNodes(collection)
		if err != nil {
			return advancedResolvedTarget{}, err
		}
		if !valid {
			return advancedResolvedTarget{}, fmt.Errorf("%w: collection target 没有有效的已启用节点", ErrAdvancedInvalid)
		}
		ref := strconv.FormatInt(*collectionID, 10)
		return advancedResolvedTarget{
			targetType: targetType, targetRef: ref, targetKey: "collection:" + ref, tag: collection.Name,
			displayName: "collection-" + ref,
		}, nil
	default:
		return advancedResolvedTarget{}, fmt.Errorf("%w: target type 仅支持 node/collection/direct", ErrAdvancedInvalid)
	}
}

func (svc *AdvancedRoutingService) collectionHasEnabledNodes(collection *model.ProxyCollectionWithNodes) (bool, error) {
	if collection.SourceType != "node_groups" && collection.SourceType != "node_groups_and_nodes" {
		nodes, err := svc.store.ListNodesByUIDs(collection.NodeUIDs)
		if err != nil {
			return false, err
		}
		for _, node := range nodes {
			if node.Enabled {
				return true, nil
			}
		}
		return false, nil
	}
	for _, group := range collection.ReferencedGroups {
		var nodes []model.Node
		var err error
		if strings.TrimSpace(group.NodeUIDs) != "" && strings.TrimSpace(group.NodeUIDs) != "[]" {
			nodes, err = svc.store.PreviewNodeGroupManualMatches(group.NodeUIDs)
		} else {
			nodes, err = svc.store.PreviewNodeGroupMatches(group.FilterProtocols, group.FilterSubscriptions, group.FilterInclude, group.FilterExclude)
		}
		if err != nil {
			return false, err
		}
		for _, node := range nodes {
			if node.Enabled {
				return true, nil
			}
		}
	}
	return false, nil
}

func (svc *AdvancedRoutingService) resolveHealthStateTarget(state model.AdvancedHealthState) (advancedResolvedTarget, error) {
	switch state.TargetType {
	case model.AdvancedTargetNode:
		parts := strings.SplitN(state.TargetRef, ":", 2)
		if len(parts) != 2 {
			return advancedResolvedTarget{}, ErrAdvancedInvalid
		}
		subscriptionID, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return advancedResolvedTarget{}, err
		}
		return svc.resolveTarget(model.AdvancedTargetNode, &subscriptionID, &parts[1], nil)
	case model.AdvancedTargetCollection:
		id, err := strconv.ParseInt(state.TargetRef, 10, 64)
		if err != nil {
			return advancedResolvedTarget{}, err
		}
		return svc.resolveTarget(model.AdvancedTargetCollection, nil, nil, &id)
	default:
		return advancedResolvedTarget{}, ErrAdvancedInvalid
	}
}

func (svc *AdvancedRoutingService) getSessionLease(id int64) (*model.SessionLease, error) {
	item, err := svc.store.GetSessionLease(id)
	if err != nil {
		return nil, normalizeAdvancedStoreError(err)
	}
	item.Status = item.StatusAt(svc.now())
	return item, nil
}

func exposureTags(ids []int64) []string {
	tags := make([]string, len(ids))
	for index, id := range ids {
		tags[index] = "ackwrap-node-exposure-in-" + strconv.FormatInt(id, 10)
	}
	return tags
}

func normalizeAdvancedPrefixes(values []string, field string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			address, addressErr := netip.ParseAddr(value)
			if addressErr != nil || address.Zone() != "" {
				return nil, fmt.Errorf("%w: %s 包含无效 CIDR/IP", ErrAdvancedInvalid, field)
			}
			prefix = netip.PrefixFrom(address, address.BitLen())
		}
		if prefix.Addr().Is4In6() {
			return nil, fmt.Errorf("%w: %s 不允许 IPv4-mapped IPv6", ErrAdvancedInvalid, field)
		}
		value = prefix.Masked().String()
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

var advancedDomainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)*$`)

func normalizeAdvancedDomains(values []string, field string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "."))
		if len(value) > 253 || !advancedDomainPattern.MatchString(value) {
			return nil, fmt.Errorf("%w: %s 包含无效域名", ErrAdvancedInvalid, field)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func normalizeAdvancedKeywords(values []string) ([]string, error) {
	result := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" || len(value) > 253 || strings.ContainsAny(value, "\r\n\x00") {
			return nil, fmt.Errorf("%w: domain_keywords 包含无效值", ErrAdvancedInvalid)
		}
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result, nil
}

func validateAdvancedName(value, field string) error {
	if value == "" || utf8.RuneCountInString(value) > 100 || strings.ContainsAny(value, "\r\n\x00") {
		return fmt.Errorf("%w: %s 必须是 100 字以内的单行文本", ErrAdvancedInvalid, field)
	}
	return nil
}

func validAdvancedPlatformLabel(value string) bool {
	if value == "" || utf8.RuneCountInString(value) > 64 {
		return false
	}
	for index, character := range value {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			continue
		}
		if index > 0 && (character == '-' || character == '_') {
			continue
		}
		return false
	}
	return true
}

func activeRuntimeUnhealthyOutbounds(config RuntimeRoutingConfig) []string {
	active := make(map[string]bool, len(config.Routes)*2+len(config.Leases)*2)
	for _, route := range config.Routes {
		active[route.OutboundTag] = true
		active[route.FallbackOutboundTag] = true
	}
	for _, lease := range config.Leases {
		active[lease.OutboundTag] = true
		active[lease.FallbackOutboundTag] = true
	}
	filtered := make([]string, 0, len(config.UnhealthyOutbounds))
	for _, tag := range config.UnhealthyOutbounds {
		if active[tag] {
			filtered = append(filtered, tag)
		}
	}
	return filtered
}

func validateAdvancedSettings(settings *model.AdvancedSettings) error {
	if settings == nil {
		return fmt.Errorf("%w: settings 不能为空", ErrAdvancedInvalid)
	}
	if settings.HealthIntervalSeconds < 1 || settings.HealthIntervalSeconds > 86400 ||
		settings.HealthTimeoutSeconds < 1 || settings.HealthTimeoutSeconds > 300 ||
		settings.FailureThreshold < 1 || settings.FailureThreshold > 100 ||
		settings.RecoveryThreshold < 1 || settings.RecoveryThreshold > 100 ||
		settings.CircuitOpenSeconds < 1 || settings.CircuitOpenSeconds > 86400 ||
		settings.AccessLogRetentionDays < 1 || settings.AccessLogRetentionDays > 3650 ||
		settings.AccessLogMaxEntries < 1 || settings.AccessLogMaxEntries > 1000000 {
		return fmt.Errorf("%w: settings 数值超出允许范围", ErrAdvancedInvalid)
	}
	if settings.AccessLogPrivacyMode != model.AdvancedPrivacyStrict && settings.AccessLogPrivacyMode != model.AdvancedPrivacyBalanced {
		return fmt.Errorf("%w: access_log_privacy_mode 仅支持 strict/balanced", ErrAdvancedInvalid)
	}
	return nil
}

func normalizeAdvancedStoreError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrAdvancedNotFound
	}
	if strings.Contains(strings.ToLower(err.Error()), "unique constraint") {
		return fmt.Errorf("%w: 名称已存在", ErrAdvancedInvalid)
	}
	return err
}
