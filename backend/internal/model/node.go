package model

type Node struct {
	ID               int64  `json:"id"`
	UID              string `json:"uid"`
	SubscriptionID   int64  `json:"subscription_id"`
	SubscriptionName string `json:"subscription_name"`
	Name             string `json:"name"`
	NameOverridden   bool   `json:"name_overridden"`
	Type             string `json:"type"`
	Server           string `json:"server"`
	ServerPort       int    `json:"server_port"`
	Raw              string `json:"raw"`
	RawJSON          string `json:"raw_json"`
	Enabled          bool   `json:"enabled"`
	Preferred        bool   `json:"preferred"`
	LatencyMS        int    `json:"latency_ms"`
	Status           string `json:"status"`
	LastTestAt       int64  `json:"last_test_at"`
	TestLatencyMS    int    `json:"test_latency_ms"`
	TestSuccess      bool   `json:"test_success"`
	CreatedAt        int64  `json:"created_at"`
	UpdatedAt        int64  `json:"updated_at"`
}

type NodeListRequest struct {
	SubscriptionID int64
	Keyword        string
	Type           string
	Status         string
	Enabled        *bool
	Preferred      *bool
	Limit          int
	Offset         int
}

type NodeListResponse struct {
	Items []Node `json:"items"`
	Total int    `json:"total"`
}

type NodeFacetItem struct {
	Value string `json:"value"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

type NodeFacetsResponse struct {
	Total         int             `json:"total"`
	Types         []NodeFacetItem `json:"types"`
	Subscriptions []NodeFacetItem `json:"subscriptions"`
}

type NodeToggleRequest struct {
	Value bool `json:"value"`
}

type NodeUIDsRequest struct {
	UIDs []string `json:"uids"`
}

type NodeFlagRequest struct {
	Name   string `json:"name"`
	Server string `json:"server"`
}

type NodeFlagResponse struct {
	Flag string `json:"flag"`
}

type NodeFlagBatchItem struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Server string `json:"server"`
}

type NodeFlagBatchRequest struct {
	Items []NodeFlagBatchItem `json:"items"`
}

type NodeFlagBatchResult struct {
	Key  string `json:"key"`
	Flag string `json:"flag"`
}

type NodeFlagBatchResponse struct {
	Items []NodeFlagBatchResult `json:"items"`
}

type NodeBatchRenameRequest struct {
	UIDs    []string `json:"uids"`
	Mode    string   `json:"mode"`
	Names   []string `json:"names,omitempty"`
	Find    string   `json:"find,omitempty"`
	Replace string   `json:"replace,omitempty"`
	Prefix  string   `json:"prefix,omitempty"`
	Suffix  string   `json:"suffix,omitempty"`
}

type NodeBatchResult struct {
	Success int `json:"success"`
	Failed  int `json:"failed"`
}

type NodeTCPingResult struct {
	UID       string `json:"uid"`
	Success   bool   `json:"success"`
	LatencyMS int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type NodeImportRequest struct {
	Content string `json:"content" binding:"required"`
}

type NodeImportResponse struct {
	Imported       int   `json:"imported"`
	SubscriptionID int64 `json:"subscription_id"`
}

type NodeImportPreviewItem struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Server     string `json:"server"`
	ServerPort int    `json:"server_port"`
	UID        string `json:"uid"`
	RawJSON    string `json:"raw_json"`
}

type NodeImportPreviewResponse struct {
	Count int                     `json:"count"`
	Items []NodeImportPreviewItem `json:"items"`
}

type ParsedNode struct {
	UID               string
	Name              string
	Type              string
	Server            string
	ServerPort        int
	Raw               string
	RawJSON           string
	UnsupportedReason string
}
