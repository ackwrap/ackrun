package model

type RouteRule struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Enabled   bool     `json:"enabled"`
	Priority  int      `json:"priority"`
	RuleType  string   `json:"rule_type"`
	Values    []string `json:"values"`
	Outbound  string   `json:"outbound"`
	Invert    bool     `json:"invert"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

type RouteRuleRequest struct {
	Name     string   `json:"name" binding:"required"`
	Enabled  bool     `json:"enabled"`
	Priority int      `json:"priority"`
	RuleType string   `json:"rule_type" binding:"required"`
	Values   []string `json:"values" binding:"required"`
	Outbound string   `json:"outbound" binding:"required"`
	Invert   bool     `json:"invert"`
}

type RouteRuleReorderRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

type RouteRulePreviewResponse struct {
	Rules    []map[string]any `json:"rules"`
	RuleSets []map[string]any `json:"rule_sets"`
}

type RouteRuleSubscription struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Enabled         bool    `json:"enabled"`
	Tag             string  `json:"tag"`
	URL             string  `json:"url"`
	Format          string  `json:"format"`
	UseProxy        bool    `json:"use_proxy"`
	SyncMode        string  `json:"sync_mode"`
	SyncTime        string  `json:"sync_time"`
	SyncWeekday     int     `json:"sync_weekday"`
	SyncStatus      string  `json:"sync_status"`
	SyncProgress    float64 `json:"sync_progress"`
	SyncError       string  `json:"sync_error"`
	LastSyncAt      int64   `json:"last_sync_at"`
	CachedPath      string  `json:"cached_path"`
	CachedUpdatedAt int64   `json:"cached_updated_at"`
	CreatedAt       int64   `json:"created_at"`
	UpdatedAt       int64   `json:"updated_at"`
}

type RouteRuleSubscriptionRequest struct {
	Name        string `json:"name" binding:"required"`
	Enabled     bool   `json:"enabled"`
	Tag         string `json:"tag"`
	URL         string `json:"url" binding:"required"`
	Format      string `json:"format"`
	UseProxy    bool   `json:"use_proxy"`
	SyncMode    string `json:"sync_mode"`
	SyncTime    string `json:"sync_time"`
	SyncWeekday int    `json:"sync_weekday"`
}

type SingboxRuleSetSource struct {
	Version int              `json:"version"`
	Rules   []map[string]any `json:"rules"`
}

type GeoAsset struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	URL             string `json:"url"`
	UseProxy        bool   `json:"use_proxy"`
	SyncMode        string `json:"sync_mode"`
	SyncTime        string `json:"sync_time"`
	SyncWeekday     int    `json:"sync_weekday"`
	SyncStatus      string `json:"sync_status"`
	SyncError       string `json:"sync_error"`
	LastSyncAt      int64  `json:"last_sync_at"`
	LocalPath       string `json:"local_path"`
	CachedUpdatedAt int64  `json:"cached_updated_at"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

type GeoAssetRequest struct {
	URL         string `json:"url" binding:"required"`
	UseProxy    bool   `json:"use_proxy"`
	SyncMode    string `json:"sync_mode"`
	SyncTime    string `json:"sync_time"`
	SyncWeekday int    `json:"sync_weekday"`
}

type GeoLookupAssetStatus struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Ready     bool   `json:"ready"`
	LocalPath string `json:"local_path"`
	UpdatedAt int64  `json:"updated_at"`
	Error     string `json:"error"`
}

type GeoLookupResponse struct {
	Target         string                 `json:"target"`
	TargetType     string                 `json:"target_type"`
	DNSServer      string                 `json:"dns_server"`
	ResolvedIPs    []string               `json:"resolved_ips"`
	GeoAssets      []GeoLookupAssetStatus `json:"geo_assets"`
	Capabilities   []string               `json:"capabilities"`
	GeoIPMatches   []string               `json:"geoip_matches"`
	GeositeMatches []string               `json:"geosite_matches"`
	Message        string                 `json:"message"`
}

type GeoTagsResponse struct {
	Type    string   `json:"type"`
	Tags    []string `json:"tags"`
	Total   int      `json:"total"`
	Ready   bool     `json:"ready"`
	Message string   `json:"message"`
}

type GeoDomainItem struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type GeoDomainsResponse struct {
	Tag         string          `json:"tag"`
	Items       []GeoDomainItem `json:"items"`
	Suggestions []string        `json:"suggestions"`
	Total       int             `json:"total"`
	Limit       int             `json:"limit"`
	Offset      int             `json:"offset"`
	Ready       bool            `json:"ready"`
	Message     string          `json:"message"`
}
