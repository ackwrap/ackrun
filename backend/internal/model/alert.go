package model

const (
	AlertChannelWebhook  = "webhook"
	AlertChannelTelegram = "telegram"
	AlertChannelEmail    = "email"

	AlertEventCircuitOpen        = "circuit_open"
	AlertEventRecovered          = "recovered"
	AlertEventSubscriptionFailed = "subscription_failed"
	AlertEventTest               = "test"

	AlertDeliverySuccess = "success"
	AlertDeliveryFailed  = "failed"
	AlertDeliveryNever   = "never"
)

type AlertChannelConfig struct {
	WebhookAllowPrivate bool     `json:"webhook_allow_private,omitempty"`
	TelegramChatID      string   `json:"telegram_chat_id,omitempty"`
	SMTPHost            string   `json:"smtp_host,omitempty"`
	SMTPPort            int      `json:"smtp_port,omitempty"`
	SMTPUsername        string   `json:"smtp_username,omitempty"`
	SMTPFrom            string   `json:"smtp_from,omitempty"`
	SMTPRecipients      []string `json:"smtp_recipients,omitempty"`
	SMTPTLSMode         string   `json:"smtp_tls_mode,omitempty"`
}

type AlertChannel struct {
	ID               int64              `json:"id"`
	Name             string             `json:"name"`
	Type             string             `json:"type"`
	Enabled          bool               `json:"enabled"`
	Config           AlertChannelConfig `json:"config"`
	Destination      string             `json:"destination"`
	HasSecret        bool               `json:"has_secret"`
	LastStatus       string             `json:"last_status"`
	LastError        string             `json:"last_error"`
	LastDeliveredAt  int64              `json:"last_delivered_at"`
	CreatedAt        int64              `json:"created_at"`
	UpdatedAt        int64              `json:"updated_at"`
	SecretContext    string             `json:"-"`
	SecretCiphertext []byte             `json:"-"`
	SecretNonce      []byte             `json:"-"`
}

type AlertChannelRequest struct {
	Name    string             `json:"name"`
	Type    string             `json:"type"`
	Enabled bool               `json:"enabled"`
	Config  AlertChannelConfig `json:"config"`
	Secret  string             `json:"secret"`
}

type AlertRule struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	EventTypes      []string `json:"event_types"`
	ChannelIDs      []int64  `json:"channel_ids"`
	CooldownMinutes int      `json:"cooldown_minutes"`
	CreatedAt       int64    `json:"created_at"`
	UpdatedAt       int64    `json:"updated_at"`
}

type AlertRuleRequest struct {
	Name            string   `json:"name"`
	Enabled         bool     `json:"enabled"`
	EventTypes      []string `json:"event_types"`
	ChannelIDs      []int64  `json:"channel_ids"`
	CooldownMinutes int      `json:"cooldown_minutes"`
}

type AlertEvent struct {
	Type       string `json:"type"`
	TargetKey  string `json:"target_key"`
	SourceName string `json:"source_name"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	OccurredAt int64  `json:"occurred_at"`
}

type AlertDelivery struct {
	ID          int64  `json:"id"`
	RuleID      *int64 `json:"rule_id,omitempty"`
	ChannelID   *int64 `json:"channel_id,omitempty"`
	ChannelName string `json:"channel_name"`
	ChannelType string `json:"channel_type"`
	EventType   string `json:"event_type"`
	EventTitle  string `json:"event_title"`
	Success     bool   `json:"success"`
	StatusCode  int    `json:"status_code"`
	Error       string `json:"error"`
	IsTest      bool   `json:"is_test"`
	DeliveredAt int64  `json:"delivered_at"`
}

type AlertDeliveryFilter struct {
	ChannelID int64
	EventType string
	Status    string
	Limit     int
	Offset    int
}

type AlertDeliveryPage struct {
	Items    []AlertDelivery `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}
