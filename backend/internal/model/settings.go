package model

type UpdateSettings struct {
	Acceleration    string `json:"acceleration"`
	CustomMirrorURL string `json:"custom_mirror_url,omitempty"`
}

type UpdateSettingsResponse struct {
	Acceleration    string `json:"acceleration"`
	CustomMirrorURL string `json:"custom_mirror_url"`
}

type TrafficBypassRule struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type TrafficBypassSettings struct {
	Rules []TrafficBypassRule `json:"rules"`
}

type LogSettings struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

type LogSettingsResponse struct {
	Level     string `json:"level"`
	Timestamp bool   `json:"timestamp"`
}

// ConnectivitySettings controls automatic URLTest checks globally.
type ConnectivitySettings struct {
	TestURL         string `json:"test_url"`
	IntervalSeconds int    `json:"interval_seconds"`
}

// NTPSettings NTP 时间同步设置
type NTPSettings struct {
	Enabled    bool   `json:"enabled"`
	Server     string `json:"server,omitempty"`
	ServerPort int    `json:"server_port,omitempty"`
	Interval   string `json:"interval,omitempty"`
	Detour     string `json:"detour,omitempty"`
}

// NTPSettingsResponse NTP 时间同步设置响应
type NTPSettingsResponse struct {
	Enabled    bool   `json:"enabled"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	Interval   string `json:"interval"`
	Detour     string `json:"detour"`
}

// DNSSettings DNS 管理设置
type DNSSettings struct {
	Enabled          bool   `json:"enabled"`
	ProxyServer      string `json:"proxy_server,omitempty"`
	DirectServer     string `json:"direct_server,omitempty"`
	Resolver         string `json:"resolver,omitempty"`
	Final            string `json:"final,omitempty"`
	Strategy         string `json:"strategy,omitempty"`
	AddressStrategy  string `json:"address_strategy,omitempty"`
	DisableCache     bool   `json:"disable_cache"`
	DisableExpire    bool   `json:"disable_expire"`
	IndependentCache bool   `json:"independent_cache"`
	ReverseMapping   bool   `json:"reverse_mapping"`
	ClientSubnet     string `json:"client_subnet,omitempty"`
	FakeIPEnabled    bool   `json:"fakeip_enabled"`
	FakeIPInet4Range string `json:"fakeip_inet4_range,omitempty"`
	FakeIPInet6Range string `json:"fakeip_inet6_range,omitempty"`
	RouteCN          bool   `json:"route_cn"`
	RouteNonCN       bool   `json:"route_non_cn"`
	BlockAds         bool   `json:"block_ads"`
}

// DNSSettingsResponse DNS 管理设置响应
type DNSSettingsResponse struct {
	Enabled          bool   `json:"enabled"`
	ProxyServer      string `json:"proxy_server"`
	DirectServer     string `json:"direct_server"`
	Resolver         string `json:"resolver"`
	Final            string `json:"final"`
	Strategy         string `json:"strategy"`
	AddressStrategy  string `json:"address_strategy"`
	DisableCache     bool   `json:"disable_cache"`
	DisableExpire    bool   `json:"disable_expire"`
	IndependentCache bool   `json:"independent_cache"`
	ReverseMapping   bool   `json:"reverse_mapping"`
	ClientSubnet     string `json:"client_subnet"`
	FakeIPEnabled    bool   `json:"fakeip_enabled"`
	FakeIPInet4Range string `json:"fakeip_inet4_range"`
	FakeIPInet6Range string `json:"fakeip_inet6_range"`
	RouteCN          bool   `json:"route_cn"`
	RouteNonCN       bool   `json:"route_non_cn"`
	BlockAds         bool   `json:"block_ads"`
}

// ExperimentalSettings 实验性功能设置
type ExperimentalSettings struct {
	ClashAPIEnabled               bool   `json:"clash_api_enabled"`
	ClashAPIPort                  string `json:"clash_api_port"`
	ClashAPISecret                string `json:"clash_api_secret,omitempty"`
	ClashAPIExternalUI            string `json:"clash_api_external_ui,omitempty"`
	ClashAPIExternalUIDownloadURL string `json:"clash_api_external_ui_download_url,omitempty"`
	ClashAPIDashboard             string `json:"clash_api_dashboard,omitempty"`
	CacheFileEnabled              bool   `json:"cache_file_enabled"`
	CacheFileStoreFakeIP          bool   `json:"cache_file_store_fakeip"`
	CacheFileStoreDNS             bool   `json:"cache_file_store_dns"`
}

// ExperimentalSettingsResponse 实验性功能设置响应
type ExperimentalSettingsResponse struct {
	ClashAPIEnabled               bool   `json:"clash_api_enabled"`
	ClashAPIPort                  string `json:"clash_api_port"`
	ClashAPISecret                string `json:"clash_api_secret,omitempty"`
	ClashAPIExternalUI            string `json:"clash_api_external_ui,omitempty"`
	ClashAPIExternalUIDownloadURL string `json:"clash_api_external_ui_download_url,omitempty"`
	ClashAPIDashboard             string `json:"clash_api_dashboard,omitempty"`
	CacheFileEnabled              bool   `json:"cache_file_enabled"`
	CacheFileStoreFakeIP          bool   `json:"cache_file_store_fakeip"`
	CacheFileStoreDNS             bool   `json:"cache_file_store_dns"`
}

type NodeFilter struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	Pattern   string `json:"pattern"`
	Enabled   bool   `json:"enabled"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

type NodeFilterRequest struct {
	Name    string `json:"name" binding:"required"`
	Target  string `json:"target" binding:"required"`
	Pattern string `json:"pattern" binding:"required"`
	Enabled bool   `json:"enabled"`
}
