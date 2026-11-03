package model

type MonthlyTraffic struct {
	Month         string `json:"month"`
	UploadBytes   int64  `json:"upload_bytes"`
	DownloadBytes int64  `json:"download_bytes"`
	TrackingSince int64  `json:"tracking_since"`
}

type SubscriptionExpiry struct {
	Name     string `json:"name"`
	ExpireAt int64  `json:"expire_at"`
}

type ControlSummary struct {
	MonthlyTraffic           MonthlyTraffic      `json:"monthly_traffic"`
	TrafficError             string              `json:"traffic_error,omitempty"`
	SubscriptionTotalBytes   int64               `json:"subscription_total_bytes"`
	SubscriptionCount        int                 `json:"subscription_count"`
	TrafficReportedCount     int                 `json:"traffic_reported_count"`
	ExpiredSubscriptionCount int                 `json:"expired_subscription_count"`
	NextExpiry               *SubscriptionExpiry `json:"next_expiry"`
}
