package model

type MCPSettings struct {
	Enabled         bool   `json:"enabled"`
	TokenConfigured bool   `json:"token_configured"`
	EndpointPath    string `json:"endpoint_path"`
	Token           string `json:"token,omitempty"`
}

type MCPSettingsRequest struct {
	Enabled       bool   `json:"enabled"`
	Token         string `json:"token"`
	GenerateToken bool   `json:"generate_token"`
}
