package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ackwrap/ackrun/internal/httpclient"
	"github.com/ackwrap/ackrun/internal/logging"
	"github.com/ackwrap/ackrun/internal/model"
)

type AdvancedHealthSnapshot struct {
	Settings *model.AdvancedSettings     `json:"settings"`
	States   []model.AdvancedHealthState `json:"states"`
	Events   []model.AdvancedHealthEvent `json:"events"`
	Running  bool                        `json:"running"`
}

type AdvancedAccessLogResponse struct {
	Items    []model.AdvancedAccessLog `json:"items"`
	Total    int64                     `json:"total"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"page_size"`
}

func (svc *AdvancedRoutingService) Start() {
	svc.lifecycleMu.Lock()
	defer svc.lifecycleMu.Unlock()
	if svc.started {
		return
	}
	svc.stop = make(chan struct{})
	svc.runContext, svc.runCancel = context.WithCancel(context.Background())
	svc.started = true
	svc.done.Add(2)
	go svc.healthLoop(svc.stop)
	go svc.accessLoop(svc.stop)
	logging.Info("advanced.lifecycle", "高级路由调度与访问事件采集已启动")
}

func (svc *AdvancedRoutingService) Stop() {
	svc.lifecycleMu.Lock()
	if !svc.started {
		svc.lifecycleMu.Unlock()
		return
	}
	stop := svc.stop
	svc.started = false
	close(stop)
	if svc.runCancel != nil {
		svc.runCancel()
	}
	svc.lifecycleMu.Unlock()
	svc.done.Wait()
	svc.healthRuns.Wait()
	logging.Info("advanced.lifecycle", "高级路由调度与访问事件采集已停止")
}

func (svc *AdvancedRoutingService) healthLoop(stop <-chan struct{}) {
	defer svc.done.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			settings, err := svc.store.GetAdvancedSettings()
			if err != nil || !settings.HealthEnabled {
				continue
			}
			svc.healthMu.Lock()
			due := svc.lastHealthRun.IsZero() || svc.now().Sub(svc.lastHealthRun) >= time.Duration(settings.HealthIntervalSeconds)*time.Second
			svc.healthMu.Unlock()
			if due {
				_, _ = svc.RunHealth()
			}
		}
	}
}

func (svc *AdvancedRoutingService) accessLoop(stop <-chan struct{}) {
	defer svc.done.Done()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			svc.collectAccessEvents()
		}
	}
}

func (svc *AdvancedRoutingService) HealthSnapshot() (*AdvancedHealthSnapshot, error) {
	settings, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return nil, err
	}
	states, err := svc.store.ListAdvancedHealthStates()
	if err != nil {
		return nil, err
	}
	events, err := svc.store.ListAdvancedHealthEvents(200, 0)
	if err != nil {
		return nil, err
	}
	svc.healthMu.Lock()
	running := svc.healthRunning
	svc.healthMu.Unlock()
	return &AdvancedHealthSnapshot{Settings: settings, States: states, Events: events, Running: running}, nil
}

func (svc *AdvancedRoutingService) RunHealth() (*model.ActionResponse, error) {
	settings, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return nil, err
	}
	if !settings.HealthEnabled {
		return nil, fmt.Errorf("%w: 健康调度未启用", ErrAdvancedInvalid)
	}
	if svc.core == nil || !svc.core.IsRunning() {
		return nil, fmt.Errorf("%w: sing-box 核心未运行", ErrAdvancedInvalid)
	}
	svc.lifecycleMu.Lock()
	if svc.stop != nil && !svc.started {
		svc.lifecycleMu.Unlock()
		return nil, fmt.Errorf("%w: 高级路由服务正在停止", ErrAdvancedInvalid)
	}
	svc.healthMu.Lock()
	if svc.healthRunning {
		svc.healthMu.Unlock()
		svc.lifecycleMu.Unlock()
		return nil, fmt.Errorf("%w: 健康检查正在运行", ErrAdvancedInvalid)
	}
	svc.healthRunning = true
	svc.lastHealthRun = svc.now()
	svc.healthRuns.Add(1)
	svc.healthMu.Unlock()
	svc.lifecycleMu.Unlock()
	go func() {
		defer svc.healthRuns.Done()
		defer func() {
			svc.healthMu.Lock()
			svc.healthRunning = false
			svc.healthMu.Unlock()
		}()
		if err := svc.runHealth(); err != nil {
			if errors.Is(err, ErrAdvancedApply) {
				logging.Error("advanced.health.run", "高级健康检查运行时同步失败")
			} else {
				logging.Error("advanced.health.run", "高级健康检查失败: %v", err)
				_ = svc.store.AppendAdvancedHealthEvent(&model.AdvancedHealthEvent{
					TargetKey: "scheduler", TargetType: "system", DisplayName: "健康调度",
					EventType: model.AdvancedHealthEventProbeFailed, Message: "健康检查执行失败",
				})
			}
		}
	}()
	return &model.ActionResponse{Success: true, Message: "advanced health check started"}, nil
}

func (svc *AdvancedRoutingService) runHealth() error {
	settings, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return err
	}
	if !settings.HealthEnabled {
		return fmt.Errorf("%w: 健康调度未启用", ErrAdvancedInvalid)
	}
	if svc.core == nil || !svc.core.IsRunning() {
		return fmt.Errorf("%w: sing-box 核心未运行", ErrAdvancedInvalid)
	}
	targets, err := svc.healthTargets(settings)
	if err != nil {
		return err
	}
	states, err := svc.store.ListAdvancedHealthStates()
	if err != nil {
		return err
	}
	stateByKey := make(map[string]model.AdvancedHealthState, len(states))
	for _, state := range states {
		stateByKey[state.TargetKey] = state
	}
	now := svc.now()
	runtimeChanged := false
	updatedStates := make([]model.AdvancedHealthState, 0, len(targets))
	healthEvents := make([]model.AdvancedHealthEvent, 0, len(targets))
	for _, target := range targets {
		state := stateByKey[target.targetKey]
		if state.TargetKey == "" {
			state = model.AdvancedHealthState{
				TargetKey: target.targetKey, TargetType: target.targetType, TargetRef: target.targetRef,
				DisplayName: target.displayName, Status: model.AdvancedHealthUnknown,
			}
		}
		state.TargetType, state.TargetRef, state.DisplayName = target.targetType, target.targetRef, target.displayName
		if state.Status == model.AdvancedHealthCircuitOpen && state.CircuitOpenUntil > now.UnixMilli() {
			continue
		}
		previousStatus := state.Status
		latency, probeErr := svc.probe(target, time.Duration(settings.HealthTimeoutSeconds)*time.Second)
		state.LastCheckedAt = now.UnixMilli()
		event := model.AdvancedHealthEvent{TargetKey: target.targetKey, TargetType: target.targetType, DisplayName: target.displayName, LatencyMS: latency}
		if probeErr != nil {
			if svc.core == nil || !svc.core.IsRunning() {
				return fmt.Errorf("%w: sing-box 核心已停止", ErrAdvancedInvalid)
			}
			state.LatencyMS = 0
			state.ConsecutiveFailures++
			state.ConsecutiveSuccesses = 0
			state.LastError = healthProbeErrorSummary(probeErr)
			state.Status = model.AdvancedHealthUnhealthy
			state.CircuitOpenUntil = 0
			event.EventType = model.AdvancedHealthEventProbeFailed
			event.Message = "目标健康探测失败"
			if previousStatus == model.AdvancedHealthCircuitOpen || state.ConsecutiveFailures >= settings.FailureThreshold {
				state.Status = model.AdvancedHealthCircuitOpen
				state.CircuitOpenUntil = now.Add(time.Duration(settings.CircuitOpenSeconds) * time.Second).UnixMilli()
				event.EventType = model.AdvancedHealthEventCircuitOpen
				if previousStatus == model.AdvancedHealthCircuitOpen {
					event.Message = "熔断恢复探测失败，已延长熔断"
				} else {
					event.Message = "连续失败达到阈值，已打开熔断"
				}
			}
		} else {
			state.LatencyMS = latency
			state.ConsecutiveFailures = 0
			state.ConsecutiveSuccesses++
			state.LastError = ""
			event.EventType = model.AdvancedHealthEventProbeSucceeded
			event.Message = "目标健康探测成功"
			if previousStatus == model.AdvancedHealthUnknown || previousStatus == model.AdvancedHealthHealthy || state.ConsecutiveSuccesses >= settings.RecoveryThreshold {
				state.Status = model.AdvancedHealthHealthy
				state.CircuitOpenUntil = 0
			} else if previousStatus == model.AdvancedHealthCircuitOpen {
				state.Status = model.AdvancedHealthCircuitOpen
				state.CircuitOpenUntil = now.Add(time.Duration(settings.HealthIntervalSeconds) * time.Second).UnixMilli()
			} else {
				state.Status = model.AdvancedHealthUnhealthy
				state.CircuitOpenUntil = 0
			}
			if previousStatus == model.AdvancedHealthCircuitOpen || previousStatus == model.AdvancedHealthUnhealthy {
				if state.Status == model.AdvancedHealthHealthy {
					event.EventType = model.AdvancedHealthEventRecovered
					event.Message = "连续成功达到阈值，目标已恢复"
				}
			}
		}
		updatedStates = append(updatedStates, state)
		healthEvents = append(healthEvents, event)
		if (previousStatus == model.AdvancedHealthCircuitOpen) != (state.Status == model.AdvancedHealthCircuitOpen) {
			runtimeChanged = true
		}
	}
	if err := svc.applyHealthResults(updatedStates, healthEvents, runtimeChanged); err != nil {
		return err
	}
	if svc.alerts != nil {
		for _, event := range healthEvents {
			switch event.EventType {
			case model.AdvancedHealthEventCircuitOpen:
				svc.alerts.Notify(model.AlertEvent{
					Type: model.AlertEventCircuitOpen, TargetKey: event.TargetKey, SourceName: event.DisplayName,
					Title: "[Ackwrap] 出口熔断", Message: "目标「" + event.DisplayName + "」已进入熔断状态。" + event.Message,
					OccurredAt: event.CreatedAt,
				})
			case model.AdvancedHealthEventRecovered:
				svc.alerts.Notify(model.AlertEvent{
					Type: model.AlertEventRecovered, TargetKey: event.TargetKey, SourceName: event.DisplayName,
					Title: "[Ackwrap] 出口恢复", Message: "目标「" + event.DisplayName + "」已恢复可用。" + event.Message,
					OccurredAt: event.CreatedAt,
				})
			}
		}
	}
	_, _ = svc.store.PruneAdvancedHealthEvents(1000)
	logging.Info("advanced.health.run", "高级健康检查完成: targets=%d", len(targets))
	return nil
}

func (svc *AdvancedRoutingService) applyHealthResults(states []model.AdvancedHealthState, events []model.AdvancedHealthEvent, runtimeChanged bool) error {
	svc.mu.Lock()
	defer svc.mu.Unlock()
	if !runtimeChanged {
		return svc.store.ApplyAdvancedHealthResults(states, events)
	}
	if svc.runtime == nil || svc.core == nil || !svc.core.IsRunning() {
		return fmt.Errorf("%w: sing-box 核心已停止", ErrAdvancedInvalid)
	}
	previous, err := svc.runtime.GetRuntimeRouting()
	if err != nil {
		return fmt.Errorf("%w: 读取旧运行时快照失败: %v", ErrAdvancedApply, err)
	}
	overrides := make(map[string]model.AdvancedHealthState, len(states))
	for _, state := range states {
		overrides[state.TargetKey] = state
	}
	desired, err := svc.buildRuntimeConfigWithHealthLocked(overrides)
	if err != nil {
		return err
	}
	if err := svc.runtime.PutRuntimeRouting(desired); err != nil {
		if restoreErr := svc.runtime.PutRuntimeRouting(previous); restoreErr != nil {
			return fmt.Errorf("%w: 发布健康状态失败: %v；恢复旧运行时快照失败: %v", ErrAdvancedApply, err, restoreErr)
		}
		return fmt.Errorf("%w: 发布健康状态失败，运行时已恢复: %v", ErrAdvancedApply, err)
	}
	if err := svc.store.ApplyAdvancedHealthResults(states, events); err != nil {
		if restoreErr := svc.runtime.PutRuntimeRouting(previous); restoreErr != nil {
			return fmt.Errorf("%w: 持久化健康状态失败: %v；恢复旧运行时快照失败: %v", ErrAdvancedApply, err, restoreErr)
		}
		return fmt.Errorf("%w: 持久化健康状态失败，运行时已恢复: %v", ErrAdvancedApply, err)
	}
	return nil
}

func (svc *AdvancedRoutingService) healthTargets(settings *model.AdvancedSettings) ([]advancedResolvedTarget, error) {
	unique := make(map[string]advancedResolvedTarget)
	appendTarget := func(targetType string, subscriptionID *int64, nodeUID *string, collectionID *int64) error {
		target, err := svc.resolveTarget(targetType, subscriptionID, nodeUID, collectionID)
		if err != nil {
			return err
		}
		if target.targetType != model.AdvancedTargetDirect {
			unique[target.targetKey] = target
		}
		return nil
	}
	if settings.RoutingEnabled {
		routes, err := svc.store.ListPlatformRoutes()
		if err != nil {
			return nil, err
		}
		for _, route := range routes {
			if !route.Enabled {
				continue
			}
			if err := appendTarget(route.TargetType, route.TargetSubscriptionID, route.TargetNodeUID, route.TargetCollectionID); err != nil {
				return nil, err
			}
			if err := appendTarget(route.FallbackType, route.FallbackSubscriptionID, route.FallbackNodeUID, route.FallbackCollectionID); err != nil {
				return nil, err
			}
		}
	}
	if settings.LeasesEnabled {
		leases, err := svc.store.ListSessionLeases()
		if err != nil {
			return nil, err
		}
		now := svc.now()
		for _, lease := range leases {
			if !lease.Enabled || lease.IsExpiredAt(now) {
				continue
			}
			if err := appendTarget(lease.TargetType, lease.TargetSubscriptionID, lease.TargetNodeUID, lease.TargetCollectionID); err != nil {
				return nil, err
			}
			if err := appendTarget(lease.FallbackType, lease.FallbackSubscriptionID, lease.FallbackNodeUID, lease.FallbackCollectionID); err != nil {
				return nil, err
			}
		}
	}
	result := make([]advancedResolvedTarget, 0, len(unique))
	for _, target := range unique {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].targetKey < result[j].targetKey })
	return result, nil
}

func (svc *AdvancedRoutingService) probeTarget(target advancedResolvedTarget, timeout time.Duration) (int, error) {
	experimental, err := svc.store.GetExperimentalSettings()
	if err != nil || experimental == nil || strings.TrimSpace(experimental.ClashAPIPort) == "" {
		return 0, errors.New("Clash API 未配置")
	}
	connectivity, err := svc.store.GetConnectivitySettings()
	if err != nil {
		return 0, errors.New("连通性地址不可用")
	}
	endpoint := fmt.Sprintf("http://127.0.0.1:%s/proxies/%s/delay?timeout=%d&url=%s",
		experimental.ClashAPIPort, url.PathEscape(target.tag), timeout.Milliseconds(), url.QueryEscape(connectivity.TestURL))
	svc.lifecycleMu.Lock()
	baseContext := svc.runContext
	svc.lifecycleMu.Unlock()
	if baseContext == nil {
		baseContext = context.Background()
	}
	ctx, cancel := context.WithTimeout(baseContext, timeout+time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, errors.New("无法创建健康探测请求")
	}
	httpclient.SetBrowserUserAgent(request)
	if experimental.ClashAPISecret != "" {
		request.Header.Set("Authorization", "Bearer "+experimental.ClashAPISecret)
	}
	response, err := svc.healthClient.Do(request)
	if err != nil {
		return 0, errors.New("核心测速接口不可用")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("核心测速返回 HTTP %d", response.StatusCode)
	}
	var payload struct {
		Delay int `json:"delay"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil || payload.Delay <= 0 {
		return 0, errors.New("核心测速响应无效")
	}
	return payload.Delay, nil
}

func healthProbeErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	if len(message) > 160 || strings.ContainsAny(message, "\r\n\x00") {
		return "目标健康探测失败"
	}
	return message
}

func (svc *AdvancedRoutingService) ListAccessLogs(page, pageSize int, platform, decision, keyword string) (*AdvancedAccessLogResponse, error) {
	if page < 1 || pageSize < 1 || pageSize > 500 {
		return nil, fmt.Errorf("%w: page 必须大于 0，page_size 必须在 1 到 500 之间", ErrAdvancedInvalid)
	}
	filter := model.AdvancedAccessLogFilter{
		Platform: strings.TrimSpace(platform), Decision: strings.TrimSpace(decision), Keyword: strings.TrimSpace(keyword),
		Limit: pageSize, Offset: (page - 1) * pageSize,
	}
	result, err := svc.store.ListAdvancedAccessLogs(filter)
	if err != nil {
		return nil, err
	}
	return &AdvancedAccessLogResponse{Items: result.Items, Total: result.Total, Page: page, PageSize: result.Limit}, nil
}

func (svc *AdvancedRoutingService) ClearAccessLogs() (*model.ActionResponse, error) {
	svc.accessMu.Lock()
	defer svc.accessMu.Unlock()
	logging.Info("advanced.access_log.clear", "清空高级访问日志")
	if err := svc.store.ClearAdvancedAccessLogs(); err != nil {
		return nil, err
	}
	return &model.ActionResponse{Success: true, Message: "access logs cleared"}, nil
}

func (svc *AdvancedRoutingService) collectAccessEvents() {
	svc.accessMu.Lock()
	defer svc.accessMu.Unlock()
	settings, err := svc.store.GetAdvancedSettings()
	if err != nil {
		return
	}
	now := svc.now()
	cleanupDue := svc.lastCleanup.IsZero() || now.Sub(svc.lastCleanup) >= time.Minute
	if cleanupDue {
		if _, err := svc.store.CleanupAdvancedAccessLogs(settings.AccessLogRetentionDays, settings.AccessLogMaxEntries); err == nil {
			svc.lastCleanup = now
		}
	}
	if !settings.AccessLogsEnabled {
		return
	}
	if svc.runtime == nil || svc.core == nil || !svc.core.IsRunning() {
		return
	}
	events, err := svc.runtime.GetAccessEvents(svc.accessCursor, 500)
	if err != nil {
		return
	}
	nextCursor := svc.accessCursor
	for _, event := range events.Items {
		if event.ID > nextCursor {
			nextCursor = event.ID
		}
	}
	if len(events.Items) == 0 {
		if events.LatestID > nextCursor {
			svc.accessCursor = events.LatestID
		}
		return
	}
	items := svc.safeAccessLogs(events.Items, settings.AccessLogPrivacyMode)
	inserted, err := svc.store.InsertAdvancedAccessLogs(items)
	if err != nil {
		return
	}
	svc.accessCursor = nextCursor
	if inserted > 0 {
		if _, err := svc.store.CleanupAdvancedAccessLogs(settings.AccessLogRetentionDays, settings.AccessLogMaxEntries); err == nil {
			svc.lastCleanup = now
		}
	}
	if inserted > 0 && svc.realtime != nil {
		svc.realtime.Broadcast("access_log.created", map[string]int{"count": inserted})
	}
}

func (svc *AdvancedRoutingService) safeAccessLogs(events []RuntimeAccessEvent, privacyMode string) []model.AdvancedAccessLog {
	outboundLabels := svc.outboundDisplayLabels()
	result := make([]model.AdvancedAccessLog, 0, len(events))
	for _, event := range events {
		item := model.AdvancedAccessLog{
			CoreEventID: strconv.FormatUint(event.ID, 10), EventTime: event.Time, Network: safeAccessText(event.Network, 16),
			Inbound: safeAccessText(event.Inbound, 128), OutboundLabel: outboundLabels[event.OutboundTag],
			Decision: safeAccessText(event.Decision, 64),
		}
		if item.OutboundLabel == "" {
			item.OutboundLabel = "unknown"
		} else {
			item.OutboundLabel = safeAccessText(item.OutboundLabel, 128)
			if item.OutboundLabel == "" {
				item.OutboundLabel = "unknown"
			}
		}
		item.PlatformRouteID = parseRuntimeDatabaseID(event.RouteID, "route-")
		item.SessionLeaseID = parseRuntimeDatabaseID(event.LeaseID, "lease-")
		if event.Error != "" {
			item.ErrorSummary = "runtime routing decision failed"
		}
		if privacyMode == model.AdvancedPrivacyBalanced {
			item.Platform = safeAccessPlatform(event.Platform)
			item.SourceHash = svc.shortHash("source", event.SourceIP)
			item.DestinationSummary = anonymizeIPAddress(event.DestinationIP)
			item.DomainSummary = svc.shortHash("domain", strings.ToLower(strings.TrimSuffix(event.Domain, ".")))
		}
		result = append(result, item)
	}
	return result
}

func (svc *AdvancedRoutingService) outboundDisplayLabels() map[string]string {
	labels := map[string]string{"direct": "direct"}
	if nodes, err := svc.store.ListEnabledNodes(); err == nil {
		tags := buildNodeOutboundTags(nodes)
		for _, node := range nodes {
			labels[tags[node.UID]] = "node-" + svc.shortHash("outbound-label", node.UID)
		}
	}
	if collections, err := svc.store.ListProxyCollectionsWithNodes(); err == nil {
		for _, collection := range collections {
			if collection != nil && collection.Enabled {
				labels[collection.Name] = "collection-" + strconv.Itoa(collection.ID)
			}
		}
	}
	return labels
}

func (svc *AdvancedRoutingService) shortHash(kind, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(svc.hashSecret + "\x00" + kind + "\x00" + value))
	return hex.EncodeToString(sum[:8])
}

func anonymizeIPAddress(value string) string {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || address.Zone() != "" {
		return ""
	}
	address = address.Unmap()
	bits := 48
	if address.Is4() {
		bits = 24
	}
	return netip.PrefixFrom(address, bits).Masked().String()
}

func parseRuntimeDatabaseID(value, prefix string) *int64 {
	if !strings.HasPrefix(value, prefix) {
		return nil
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(value, prefix), 10, 64)
	if err != nil || id <= 0 {
		return nil
	}
	return &id
}

func safeAccessText(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if len(value) > maxLength || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func safeAccessPlatform(value string) string {
	value = strings.TrimSpace(value)
	if !validAdvancedPlatformLabel(value) {
		return ""
	}
	return value
}
