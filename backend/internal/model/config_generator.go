package model

const DefaultMixedInboundPort = 7890

// ConfigGenerateRequest 配置生成请求
type ConfigGenerateRequest struct {
	DefaultOutbound string `json:"default_outbound"` // 默认出站（代理集合名称）
	InboundListen   string `json:"inbound_listen"`   // 入站监听地址
	InboundPort     int    `json:"inbound_port"`     // 入站监听端口
	TUNIPv4Address  string `json:"tun_ipv4_address"` // TUN IPv4 接口地址（CIDR）
	TUNIPv6Address  string `json:"tun_ipv6_address"` // TUN IPv6 接口地址（CIDR）
	LogLevel        string `json:"log_level"`        // 日志级别
}

// ConfigGenerateResponse 配置生成响应
type ConfigGenerateResponse struct {
	Config   map[string]interface{} `json:"config"`    // 完整配置
	Valid    bool                   `json:"valid"`     // 是否通过验证
	Error    string                 `json:"error"`     // 验证错误信息
	FilePath string                 `json:"file_path"` // 生成的配置文件路径
}

// ConfigApplyRequest 配置应用请求
type ConfigApplyRequest struct {
	FileName string `json:"file_name"` // 保存到配置目录的文件名
}

// SingboxOutbound sing-box outbound 配置
type SingboxOutbound struct {
	Type      string   `json:"type"`
	Tag       string   `json:"tag"`
	Outbounds []string `json:"outbounds,omitempty"` // selector/urltest
	URL       string   `json:"url,omitempty"`       // urltest
	Interval  string   `json:"interval,omitempty"`  // urltest
	Tolerance int      `json:"tolerance,omitempty"` // urltest
	// 节点详细配置（从 Node 转换）
	Server    string                 `json:"server,omitempty"`
	Port      int                    `json:"server_port,omitempty"`
	Method    string                 `json:"method,omitempty"`
	Password  string                 `json:"password,omitempty"`
	UUID      string                 `json:"uuid,omitempty"`
	Flow      string                 `json:"flow,omitempty"`
	TLS       map[string]interface{} `json:"tls,omitempty"`
	Transport map[string]interface{} `json:"transport,omitempty"`
	// 其他字段根据协议类型动态添加
	Extra map[string]interface{} `json:"-"` // 用于存储其他字段
}

// SingboxRoute sing-box route 配置
type SingboxRoute struct {
	Rules   []map[string]interface{} `json:"rules"`
	RuleSet []map[string]interface{} `json:"rule_set,omitempty"`
	Final   string                   `json:"final,omitempty"`
}

// SingboxInbound sing-box inbound 配置
type SingboxInbound struct {
	Type   string `json:"type"`
	Tag    string `json:"tag"`
	Listen string `json:"listen"`
	Port   int    `json:"listen_port"`
}

// DefaultInbounds 默认入站配置
func DefaultInbounds() []SingboxInbound {
	return []SingboxInbound{
		{
			Type:   "mixed",
			Tag:    "mixed-in",
			Listen: "127.0.0.1",
			Port:   DefaultMixedInboundPort,
		},
	}
}
