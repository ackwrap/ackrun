package model

type SimpleSetupRequest struct {
	SubscriptionURL string `json:"subscription_url"`
}

type SimpleSetupStatus struct {
	Status            string `json:"status"`
	Stage             string `json:"stage"`
	Error             string `json:"error,omitempty"`
	Configured        bool   `json:"configured"`
	Supported         bool   `json:"supported"`
	HasExistingConfig bool   `json:"has_existing_config"`
	CanRetry          bool   `json:"can_retry"`
	OptionsLocked     bool   `json:"options_locked"`
}

type SimpleSetupOptions struct {
	AdBlock         bool              `json:"ad_block"`
	CNOutbound      string            `json:"cn_outbound"`
	DefaultOutbound string            `json:"default_outbound"`
	AppRouting      map[string]string `json:"app_routing"`
	LocalDNS        string            `json:"local_dns"`
	ProxyDNS        string            `json:"proxy_dns"`
	DNSStrategy     string            `json:"dns_strategy"`
	DirectDevices   []string          `json:"direct_devices"`
	AutoStartCore   bool              `json:"auto_start_core"`
}

func DefaultSimpleSetupOptions() *SimpleSetupOptions {
	return &SimpleSetupOptions{
		AdBlock: true, CNOutbound: "bypass", DefaultOutbound: "proxy",
		AppRouting: map[string]string{"AI": "proxy", "视频": "proxy", "Google": "proxy", "社交": "proxy", "开发": "proxy"},
		LocalDNS:   "223.5.5.5", ProxyDNS: "1.1.1.1", DNSStrategy: "prefer_ipv4",
		DirectDevices: []string{}, AutoStartCore: true,
	}
}
