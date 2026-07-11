package model

// ProxyCollection 代理集合
type ProxyCollection struct {
	ID                 int    `json:"id"`
	Name               string `json:"name"`
	Type               string `json:"type"`                 // selector, urltest
	SourceType         string `json:"source_type"`          // 'node_groups', 'manual'
	ReferencedGroupIDs string `json:"referenced_group_ids"` // JSON: [1,2,3]
	RouteRuleIDs       string `json:"route_rule_ids"`       // JSON: [1,2,3]
	NodeUIDs           string `json:"node_uids"`            // JSON: ["uid1","uid2"]
	TestURL            string `json:"test_url"`
	TestInterval       int    `json:"test_interval"` // 秒
	Tolerance          int    `json:"tolerance"`     // 毫秒
	Enabled            bool   `json:"enabled"`
	CreatedAt          int64  `json:"created_at"`
	UpdatedAt          int64  `json:"updated_at"`
}

// ProxyCollectionWithNodes 代理集合及其包含的节点 UID 列表
type ProxyCollectionWithNodes struct {
	ProxyCollection
	NodeUIDs         []string    `json:"node_uids"`
	ReferencedGroups []NodeGroup `json:"referenced_groups"`
	RouteRuleIDs     []int64     `json:"route_rule_ids"`
}

// ProxyCollectionRequest 创建/更新代理集合的请求
type ProxyCollectionRequest struct {
	Name               string   `json:"name"`
	Type               string   `json:"type"`
	SourceType         string   `json:"source_type"`
	ReferencedGroupIDs []int64  `json:"referenced_group_ids"`
	RouteRuleIDs       []int64  `json:"route_rule_ids"`
	NodeUIDs           []string `json:"node_uids"`
	TestURL            string   `json:"test_url"`
	TestInterval       int      `json:"test_interval"`
	Tolerance          int      `json:"tolerance"`
	Enabled            bool     `json:"enabled"`
}

type CollectionTestNodeResult struct {
	UID       string `json:"uid"`
	Success   bool   `json:"success"`
	LatencyMS int    `json:"latency_ms"`
	Error     string `json:"error,omitempty"`
}

type CollectionTestResponse struct {
	CollectionID   int                        `json:"collection_id"`
	Tested         int                        `json:"tested"`
	Available      int                        `json:"available"`
	FastestUID     string                     `json:"fastest_uid,omitempty"`
	FastestLatency int                        `json:"fastest_latency,omitempty"`
	Error          string                     `json:"error,omitempty"`
	Results        []CollectionTestNodeResult `json:"results"`
}
