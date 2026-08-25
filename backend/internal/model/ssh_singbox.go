package model

type SSHSingboxDeployRequest struct {
	ServerAddress         string `json:"server_address"`
	VLESSRealityEnabled   bool   `json:"vless_reality_enabled"`
	VLESSRealityPort      int    `json:"vless_reality_port"`
	RealityServerName     string `json:"reality_server_name"`
	ShadowsocksEnabled    bool   `json:"shadowsocks_enabled"`
	ShadowsocksPort       int    `json:"shadowsocks_port"`
	ReplaceExistingConfig bool   `json:"replace_existing_config"`
}

type SSHSingboxDeployResponse struct {
	Success      bool                   `json:"success"`
	Message      string                 `json:"message"`
	Version      string                 `json:"version"`
	ConfigPath   string                 `json:"config_path"`
	BackupPath   string                 `json:"backup_path,omitempty"`
	Nodes        []SSHSingboxNode       `json:"nodes"`
	ClientConfig map[string]interface{} `json:"client_config"`
}

type SSHSingboxNode struct {
	Type           string                 `json:"type"`
	Name           string                 `json:"name"`
	ListenPort     int                    `json:"listen_port"`
	Network        string                 `json:"network"`
	ShareURI       string                 `json:"share_uri"`
	ClientOutbound map[string]interface{} `json:"client_outbound"`
}
