package model

// NodeGroup 节点组（地域划分）
type NodeGroup struct {
	ID                  int64  `json:"id"`
	Name                string `json:"name"`
	Type                string `json:"type"`                 // selector, urltest
	FilterProtocols     string `json:"filter_protocols"`     // 逗号分隔: trojan,vless,shadowsocks
	FilterSubscriptions string `json:"filter_subscriptions"` // 逗号分隔: 1,2,3
	FilterInclude       string `json:"filter_include"`
	FilterExclude       string `json:"filter_exclude"`
	NodeUIDs            string `json:"node_uids"`
	TestURL             string `json:"test_url"`
	TestInterval        int    `json:"test_interval"`
	Tolerance           int    `json:"tolerance"`
	Enabled             bool   `json:"enabled"`
	Priority            int    `json:"priority"`
	CreatedAt           int64  `json:"created_at"`
	UpdatedAt           int64  `json:"updated_at"`
}

// NodeGroupWithStats 节点组及统计信息
type NodeGroupWithStats struct {
	NodeGroup
	MatchedNodeCount int `json:"matched_node_count"`
}

// NodeGroupRequest 创建/更新节点组请求
type NodeGroupRequest struct {
	Name                string   `json:"name" binding:"required"`
	Type                string   `json:"type" binding:"required"`
	FilterProtocols     string   `json:"filter_protocols"`
	FilterSubscriptions string   `json:"filter_subscriptions"`
	FilterInclude       string   `json:"filter_include" binding:"required"`
	FilterExclude       string   `json:"filter_exclude"`
	NodeUIDs            []string `json:"node_uids"`
	TestURL             string   `json:"test_url"`
	TestInterval        int      `json:"test_interval"`
	Tolerance           int      `json:"tolerance"`
	Enabled             bool     `json:"enabled"`
	Priority            int      `json:"priority"`
}

type NodeGroupIDsRequest struct {
	IDs []int64 `json:"ids" binding:"required"`
}

type NodeGroupQuickSetupRequest struct {
	FilterProtocols     string `json:"filter_protocols"`
	FilterSubscriptions string `json:"filter_subscriptions"`
}
