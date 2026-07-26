package model

type NodeExposure struct {
	ID             int64  `json:"id"`
	Name           string `json:"name"`
	SubscriptionID int64  `json:"subscription_id"`
	NodeUID        string `json:"node_uid"`
	InboundType    string `json:"inbound_type"`
	Listen         string `json:"listen"`
	ListenPort     int    `json:"listen_port"`
	Username       string `json:"username"`
	Password       string `json:"-"`
	HasPassword    bool   `json:"has_password"`
	Enabled        bool   `json:"enabled"`
	CreatedAt      int64  `json:"created_at"`
	UpdatedAt      int64  `json:"updated_at"`
}

type NodeExposureWithNode struct {
	NodeExposure
	NodeName         string `json:"node_name"`
	NodeType         string `json:"node_type"`
	SubscriptionName string `json:"subscription_name"`
	NodeExists       bool   `json:"node_exists"`
	NodeEnabled      bool   `json:"node_enabled"`
}

type NodeExposureRequest struct {
	Name           string `json:"name"`
	SubscriptionID int64  `json:"subscription_id"`
	NodeUID        string `json:"node_uid"`
	InboundType    string `json:"inbound_type"`
	Listen         string `json:"listen"`
	ListenPort     int    `json:"listen_port"`
	Username       string `json:"username"`
	Password       string `json:"password"`
	ClearPassword  bool   `json:"clear_password"`
	Enabled        bool   `json:"enabled"`
}
