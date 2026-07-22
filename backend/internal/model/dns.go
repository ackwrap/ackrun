package model

// DNSServer DNS 服务器
type DNSServer struct {
	ID              int64  `json:"id"`
	Tag             string `json:"tag"`
	Enabled         bool   `json:"enabled"`
	ServerType      string `json:"server_type"`
	Address         string `json:"address"`
	AddressResolver string `json:"address_resolver"`
	AddressStrategy string `json:"address_strategy"`
	Strategy        string `json:"strategy"`
	Detour          string `json:"detour"`
	ClientSubnet    string `json:"client_subnet"`
	OptionsJSON     string `json:"options_json"`
	Priority        int    `json:"priority"`
	CreatedAt       int64  `json:"created_at"`
	UpdatedAt       int64  `json:"updated_at"`
}

// DNSServerRequest DNS 服务器创建/更新请求
type DNSServerRequest struct {
	Tag             string                 `json:"tag" binding:"required"`
	Enabled         bool                   `json:"enabled"`
	ServerType      string                 `json:"server_type" binding:"required"`
	Address         string                 `json:"address"`
	AddressResolver string                 `json:"address_resolver"`
	AddressStrategy string                 `json:"address_strategy"`
	Strategy        string                 `json:"strategy"`
	Detour          string                 `json:"detour"`
	ClientSubnet    string                 `json:"client_subnet"`
	Options         map[string]interface{} `json:"options"`
}

// DNSRule DNS 路由规则
type DNSRule struct {
	ID             int64  `json:"id"`
	Enabled        bool   `json:"enabled"`
	Priority       int    `json:"priority"`
	RuleType       string `json:"rule_type"`
	ConditionsJSON string `json:"conditions_json"`
	Server         string `json:"server"`
	DisableCache   bool   `json:"disable_cache"`
	RewriteTTL     int    `json:"rewrite_ttl"`
	ClientSubnet   string `json:"client_subnet"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

// DNSRuleRequest DNS 规则创建/更新请求
type DNSRuleRequest struct {
	Enabled      bool                   `json:"enabled"`
	Priority     int                    `json:"priority"`
	RuleType     string                 `json:"rule_type"`
	Conditions   map[string]interface{} `json:"conditions" binding:"required"`
	Server       string                 `json:"server" binding:"required"`
	DisableCache bool                   `json:"disable_cache"`
	RewriteTTL   int                    `json:"rewrite_ttl"`
	ClientSubnet string                 `json:"client_subnet"`
}

// DNSGlobalSettings DNS 全局设置（复用现有 settings 表）
type DNSGlobalSettings struct {
	Enabled                   bool   `json:"enabled"`
	Final                     string `json:"final"`
	ProxyFinal                string `json:"proxy_final"`
	Strategy                  string `json:"strategy"`
	DisableCache              bool   `json:"disable_cache"`
	DisableExpire             bool   `json:"disable_expire"`
	IndependentCache          bool   `json:"independent_cache"`
	IndependentCacheSupported bool   `json:"independent_cache_supported"`
	ReverseMapping            bool   `json:"reverse_mapping"`
	CacheCapacity             int    `json:"cache_capacity"`
	ClientSubnet              string `json:"client_subnet"`
	FakeIPEnabled             bool   `json:"fakeip_enabled"`
	FakeIPInet4Range          string `json:"fakeip_inet4_range"`
	FakeIPInet6Range          string `json:"fakeip_inet6_range"`
}
