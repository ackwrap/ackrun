package model

type SSHSingboxDeployRequest struct {
	ServerAddress         string `json:"server_address"`
	Protocol              string `json:"protocol"`
	ListenPort            int    `json:"listen_port"`
	RealityServerName     string `json:"reality_server_name"`
	TLSServerName         string `json:"tls_server_name"`
	ACMEEmail             string `json:"acme_email"`
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
