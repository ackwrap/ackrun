package model

import "time"

const (
	AdvancedTargetNode       = "node"
	AdvancedTargetCollection = "collection"
	AdvancedTargetDirect     = "direct"

	AdvancedHealthUnknown             = "unknown"
	AdvancedHealthHealthy             = "healthy"
	AdvancedHealthUnhealthy           = "unhealthy"
	AdvancedHealthCircuitOpen         = "circuit_open"
	AdvancedHealthEventProbeFailed    = "probe_failed"
	AdvancedHealthEventCircuitOpen    = "circuit_open"
	AdvancedHealthEventRecovered      = "recovered"
	AdvancedHealthEventProbeSucceeded = "probe_succeeded"

	SessionLeaseActive   = "active"
	SessionLeaseExpired  = "expired"
	SessionLeaseDisabled = "disabled"

	AdvancedPrivacyStrict   = "strict"
	AdvancedPrivacyBalanced = "balanced"
)

type PlatformRoute struct {
	ID                     int64    `json:"id"`
	Name                   string   `json:"name"`
	Platform               string   `json:"platform"`
	Enabled                bool     `json:"enabled"`
	Priority               int      `json:"priority"`
	InboundExposureIDs     []int64  `json:"inbound_exposure_ids"`
	SourceCIDRs            []string `json:"source_cidrs"`
	Domains                []string `json:"domains"`
	DomainSuffixes         []string `json:"domain_suffixes"`
	DomainKeywords         []string `json:"domain_keywords"`
	DestinationCIDRs       []string `json:"destination_cidrs"`
	TargetType             string   `json:"target_type"`
	TargetSubscriptionID   *int64   `json:"target_subscription_id,omitempty"`
	TargetNodeUID          *string  `json:"target_node_uid,omitempty"`
	TargetCollectionID     *int64   `json:"target_collection_id,omitempty"`
	FallbackType           string   `json:"fallback_type"`
	FallbackSubscriptionID *int64   `json:"fallback_subscription_id,omitempty"`
	FallbackNodeUID        *string  `json:"fallback_node_uid,omitempty"`
	FallbackCollectionID   *int64   `json:"fallback_collection_id,omitempty"`
	CreatedAt              int64    `json:"created_at"`
	UpdatedAt              int64    `json:"updated_at"`
}

type SessionLease struct {
	ID                     int64   `json:"id"`
	Name                   string  `json:"name"`
	Enabled                bool    `json:"enabled"`
	ClientCIDR             string  `json:"client_cidr"`
	InboundExposureIDs     []int64 `json:"inbound_exposure_ids"`
	PlatformRouteID        *int64  `json:"platform_route_id,omitempty"`
	TargetType             string  `json:"target_type"`
	TargetSubscriptionID   *int64  `json:"target_subscription_id,omitempty"`
	TargetNodeUID          *string `json:"target_node_uid,omitempty"`
	TargetCollectionID     *int64  `json:"target_collection_id,omitempty"`
	FallbackType           string  `json:"fallback_type"`
	FallbackSubscriptionID *int64  `json:"fallback_subscription_id,omitempty"`
	FallbackNodeUID        *string `json:"fallback_node_uid,omitempty"`
	FallbackCollectionID   *int64  `json:"fallback_collection_id,omitempty"`
	ExpiresAt              int64   `json:"expires_at"`
	CreatedAt              int64   `json:"created_at"`
	UpdatedAt              int64   `json:"updated_at"`
	Status                 string  `json:"status"`
}

func (l SessionLease) IsExpiredAt(now time.Time) bool {
	return l.ExpiresAt <= now.UTC().UnixMilli()
}

func (l SessionLease) StatusAt(now time.Time) string {
	if !l.Enabled {
		return SessionLeaseDisabled
	}
	if l.IsExpiredAt(now) {
		return SessionLeaseExpired
	}
	return SessionLeaseActive
}

type AdvancedHealthState struct {
	TargetKey            string `json:"target_key"`
	TargetType           string `json:"target_type"`
	TargetRef            string `json:"target_ref"`
	DisplayName          string `json:"display_name"`
	Status               string `json:"status"`
	LatencyMS            int    `json:"latency_ms"`
	ConsecutiveFailures  int    `json:"consecutive_failures"`
	ConsecutiveSuccesses int    `json:"consecutive_successes"`
	CircuitOpenUntil     int64  `json:"circuit_open_until"`
	LastError            string `json:"last_error"`
	LastCheckedAt        int64  `json:"last_checked_at"`
	UpdatedAt            int64  `json:"updated_at"`
}

type AdvancedHealthEvent struct {
	ID          int64  `json:"id"`
	TargetKey   string `json:"target_key"`
	TargetType  string `json:"target_type"`
	DisplayName string `json:"display_name"`
	EventType   string `json:"event_type"`
	Message     string `json:"message"`
	LatencyMS   int    `json:"latency_ms"`
	CreatedAt   int64  `json:"created_at"`
}

type AdvancedAccessLog struct {
	ID                 int64  `json:"id"`
	CoreEventID        string `json:"core_event_id"`
	EventTime          int64  `json:"event_time"`
	Network            string `json:"network"`
	Inbound            string `json:"inbound"`
	SourceHash         string `json:"source_hash"`
	DestinationSummary string `json:"destination_summary"`
	DomainSummary      string `json:"domain_summary"`
	OutboundLabel      string `json:"outbound_label"`
	Platform           string `json:"platform"`
	PlatformRouteID    *int64 `json:"platform_route_id,omitempty"`
	SessionLeaseID     *int64 `json:"session_lease_id,omitempty"`
	Decision           string `json:"decision"`
	ErrorSummary       string `json:"error_summary"`
	CreatedAt          int64  `json:"created_at"`
}

type AdvancedAccessLogFilter struct {
	Platform string `json:"platform"`
	Decision string `json:"decision"`
	Keyword  string `json:"keyword"`
	FromTime int64  `json:"from_time"`
	ToTime   int64  `json:"to_time"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

type AdvancedAccessLogPage struct {
	Items  []AdvancedAccessLog `json:"items"`
	Total  int64               `json:"total"`
	Limit  int                 `json:"limit"`
	Offset int                 `json:"offset"`
}

type AdvancedSettings struct {
	RoutingEnabled         bool   `json:"routing_enabled"`
	LeasesEnabled          bool   `json:"leases_enabled"`
	HealthEnabled          bool   `json:"health_enabled"`
	HealthIntervalSeconds  int    `json:"health_interval_seconds"`
	HealthTimeoutSeconds   int    `json:"health_timeout_seconds"`
	FailureThreshold       int    `json:"failure_threshold"`
	RecoveryThreshold      int    `json:"recovery_threshold"`
	CircuitOpenSeconds     int    `json:"circuit_open_seconds"`
	AccessLogsEnabled      bool   `json:"access_logs_enabled"`
	AccessLogRetentionDays int    `json:"access_log_retention_days"`
	AccessLogMaxEntries    int    `json:"access_log_max_entries"`
	AccessLogPrivacyMode   string `json:"access_log_privacy_mode"`
}
