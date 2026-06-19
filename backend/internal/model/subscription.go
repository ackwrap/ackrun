package model

type Subscription struct {
	ID                int64   `json:"id"`
	Name              string  `json:"name"`
	URL               string  `json:"url"`
	UserAgent         string  `json:"user_agent"`
	SyncIntervalMins  int     `json:"sync_interval_minutes"`
	SyncMode          string  `json:"sync_mode"`
	SyncTime          string  `json:"sync_time"`
	SyncWeekday       int     `json:"sync_weekday"`
	SyncStatus        string  `json:"sync_status"`
	SyncProgress      float64 `json:"sync_progress"`
	SyncTimeoutSecs   int     `json:"sync_timeout_seconds"`
	NodeCount         int     `json:"node_count"`
	TrafficUsedBytes  int64   `json:"traffic_used_bytes"`
	TrafficTotalBytes int64   `json:"traffic_total_bytes"`
	ExpireAt          int64   `json:"expire_at"`
	LastSyncAt        int64   `json:"last_sync_at"`
	CreatedAt         int64   `json:"created_at"`
	UpdatedAt         int64   `json:"updated_at"`
}

type SubscriptionRequest struct {
	Name             string `json:"name" binding:"required"`
	URL              string `json:"url" binding:"required"`
	UserAgent        string `json:"user_agent,omitempty"`
	ExpireAt         int64  `json:"expire_at,omitempty"`
	SyncIntervalMins int    `json:"sync_interval_minutes,omitempty"`
	SyncMode         string `json:"sync_mode,omitempty"`
	SyncTime         string `json:"sync_time,omitempty"`
	SyncWeekday      int    `json:"sync_weekday,omitempty"`
	SyncTimeoutSecs  int    `json:"sync_timeout_seconds,omitempty"`
}

type UserAgentOption struct {
	Label       string `json:"label"`
	Value       string `json:"value"`
	Description string `json:"description"`
}
