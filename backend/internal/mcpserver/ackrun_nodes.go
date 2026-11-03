package mcpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
)

func registerNodeTools(s *mcp.Server, api *apiExecutor) {
	fixedTool(s, api, "list_subscriptions", "列出订阅、节点数、用量与同步结果；订阅链接属于敏感数据。", http.MethodGet, "/subscriptions")
	fixedTool(s, api, "list_subscription_user_agents", "列出支持的订阅 User-Agent。", http.MethodGet, "/subscriptions/user-agents")
	bodyTool[model.SubscriptionRequest](s, api, "create_subscription", "新增订阅并异步同步，完成后自动按地域创建节点分组。", http.MethodPost, "/subscriptions")
	updateTool[model.SubscriptionRequest](s, api, "update_subscription", "修改订阅，URL 变化会异步同步。", "/subscriptions/:id")
	idTool(s, api, "delete_subscription", "删除订阅及关联节点。", http.MethodDelete, "/subscriptions/:id")
	idTool(s, api, "sync_subscription", "同步指定订阅，返回已启动不代表同步完成；用 list_subscriptions 查询结果。", http.MethodPost, "/subscriptions/:id/sync")
	fixedTool(s, api, "sync_all_subscriptions", "同步全部外部订阅，跳过本地手动节点。", http.MethodPost, "/subscriptions/sync")
	queryTool[struct {
		SubscriptionID int64  `json:"subscription_id,omitempty"`
		Keyword        string `json:"keyword,omitempty"`
		Type           string `json:"type,omitempty"`
		Status         string `json:"status,omitempty"`
		Enabled        *bool  `json:"enabled,omitempty"`
		Preferred      *bool  `json:"preferred,omitempty"`
		Limit          int    `json:"limit,omitempty"`
		Offset         int    `json:"offset,omitempty"`
	}](s, api, "list_nodes", "分页查询节点，默认 50 条；不返回 raw/raw_json 凭据。", "/nodes")
	fixedTool(s, api, "get_node_facets", "读取全部节点的协议和订阅分类计数。", http.MethodGet, "/nodes/facets")
	addAPITool(s, api, "preview_node_import", "预览 URI、Clash YAML 或 sing-box JSON 导入，不保存节点。", http.MethodPost, true, func(input model.NodeImportRequest) (apiCall, error) {
		return apiCall{path: "/nodes/import/preview", body: input}, nil
	})
	bodyTool[model.NodeImportRequest](s, api, "import_nodes", "导入本地节点，按稳定 UID 添加或更新；应用节点过滤和自动地域分组。", http.MethodPost, "/nodes/import")
	idTool(s, api, "share_node", "导出指定节点的完整连接 URI，包含凭据，仅在用户要求分享时调用。", http.MethodGet, "/nodes/:id/share")
	for _, field := range []string{"enabled", "preferred"} {
		addAPITool(s, api, "set_node_"+field, "设置指定 UID 节点的 "+field+" 状态。", http.MethodPut, false, func(input struct {
			UID   string `json:"uid"`
			Value bool   `json:"value"`
		}) (apiCall, error) {
			if input.UID == "" || strings.IndexFunc(input.UID, func(r rune) bool {
				return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-')
			}) >= 0 {
				return apiCall{}, errors.New("无效的节点 UID")
			}
			return apiCall{path: "/nodes/" + input.UID + "/" + field, body: model.NodeToggleRequest{Value: input.Value}}, nil
		})
	}
	bodyTool[model.NodeUIDsRequest](s, api, "test_nodes", "对指定 UID 节点执行现有测速流程并保存结果。", http.MethodPost, "/nodes/tcping")
	bodyTool[model.NodeUIDsRequest](s, api, "delete_nodes", "批量删除指定 UID 节点。", http.MethodPost, "/nodes/batch-delete")
	bodyTool[model.NodeUIDsRequest](s, api, "add_node_emoji", "为指定 UID 节点名称添加地域 emoji，保留订阅更新时的自定义名称。", http.MethodPost, "/nodes/add-emoji")
	bodyTool[model.NodeBatchRenameRequest](s, api, "rename_nodes", "批量修改节点名称，保留订阅更新时的自定义名称。", http.MethodPost, "/nodes/batch-rename")

	fixedTool(s, api, "list_node_groups", "列出节点分组和匹配节点数。", http.MethodGet, "/node-groups")
	idTool(s, api, "get_node_group", "读取节点分组配置。", http.MethodGet, "/node-groups/:id")
	bodyTool[model.NodeGroupRequest](s, api, "create_node_group", "创建节点分组；普通分组使用关键词筛选节点。", http.MethodPost, "/node-groups")
	updateTool[model.NodeGroupRequest](s, api, "update_node_group", "修改节点分组。", "/node-groups/:id")
	idTool(s, api, "delete_node_group", "删除节点分组，内置分组受保护。", http.MethodDelete, "/node-groups/:id")
	orderTool(s, api, "reorder_node_groups", "按给定的完整 ID 顺序排列节点分组。", "/node-groups/reorder")
	bodyTool[model.NodeGroupQuickSetupRequest](s, api, "group_nodes_by_region", "调用现有智能快速配置，按地域建立节点分组。", http.MethodPost, "/node-groups/quick-setup")

	fixedTool(s, api, "list_node_entries", "列出固定指向节点的代理入口，不返回密码。", http.MethodGet, "/advanced/node-exposures")
	bodyTool[model.NodeExposureRequest](s, api, "create_node_entry", "创建节点入口，为节点分配 Mixed 端口。", http.MethodPost, "/advanced/node-exposures")
	updateTool[model.NodeExposureRequest](s, api, "update_node_entry", "修改节点入口。", "/advanced/node-exposures/:id")
	idTool(s, api, "delete_node_entry", "删除节点入口。", http.MethodDelete, "/advanced/node-exposures/:id")
	idTool(s, api, "share_node_entry", "导出入口代理链接，包含认证凭据，仅在用户要求复制或分享时调用。", http.MethodGet, "/advanced/node-exposures/:id/share")
}
