package mcpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
)

func registerSettingsTools(s *mcp.Server, api *apiExecutor) {
	fixedTool(s, api, "get_general_settings", "读取常规设置。", http.MethodGet, "/settings/general")
	bodyTool[model.GeneralSettingsRequest](s, api, "set_general_settings", "修改常规设置，省略的字段保留原值。", http.MethodPut, "/settings/general")
	fixedTool(s, api, "get_inbound_mode", "读取 TUN/Mixed 运行模式。", http.MethodGet, "/settings/inbound-mode")
	bodyTool[struct {
		Mode string `json:"mode" jsonschema:"tun, mixed, or tun_mixed"`
	}](s, api, "set_inbound_mode", "设置运行模式；正在运行时按现有规则拒绝不允许的切换。", http.MethodPut, "/settings/inbound-mode")
	fixedTool(s, api, "get_proxy_mode", "读取规则、全局或直连代理模式。", http.MethodGet, "/settings/proxy-mode")
	bodyTool[struct {
		Mode string `json:"mode" jsonschema:"rule, global, or direct"`
	}](s, api, "set_proxy_mode", "设置代理模式。", http.MethodPut, "/settings/proxy-mode")
	fixedTool(s, api, "get_mixed_auth", "读取 Mixed 认证用户名，密码隐藏。", http.MethodGet, "/settings/mixed-inbound")
	bodyTool[model.MixedInboundSettings](s, api, "set_mixed_auth", "设置 Mixed 认证，用户名与密码均为空时关闭认证。", http.MethodPut, "/settings/mixed-inbound")
	fixedTool(s, api, "get_log_settings", "读取核心日志设置。", http.MethodGet, "/settings/log")
	bodyTool[model.LogSettings](s, api, "set_log_settings", "修改核心日志设置。", http.MethodPut, "/settings/log")
	fixedTool(s, api, "get_ntp_settings", "读取 NTP 设置。", http.MethodGet, "/settings/ntp")
	bodyTool[model.NTPSettings](s, api, "set_ntp_settings", "修改 NTP 设置。", http.MethodPut, "/settings/ntp")
	fixedTool(s, api, "get_connectivity_settings", "读取自动测速 URL 和周期。", http.MethodGet, "/settings/connectivity")
	bodyTool[model.ConnectivitySettings](s, api, "set_connectivity_settings", "修改自动测速设置。", http.MethodPut, "/settings/connectivity")
	fixedTool(s, api, "get_traffic_bypass", "读取流量排除规则。", http.MethodGet, "/settings/traffic-bypass")
	bodyTool[model.TrafficBypassSettings](s, api, "set_traffic_bypass", "保存完整流量排除规则。", http.MethodPut, "/settings/traffic-bypass")
	fixedTool(s, api, "get_experimental_settings", "读取实验性设置，Clash API 凭据隐藏。", http.MethodGet, "/settings/experimental")
	bodyTool[model.ExperimentalSettings](s, api, "set_experimental_settings", "保存完整实验性设置；先读取并保留其余字段。读取结果不含 Clash API Secret，保留外部面板认证时须明确提供现有 secret，空值会清除。", http.MethodPut, "/settings/experimental")
	fixedTool(s, api, "get_update_settings", "读取更新下载设置。", http.MethodGet, "/settings/update")
	bodyTool[model.UpdateSettings](s, api, "set_update_settings", "修改更新下载设置。", http.MethodPut, "/settings/update")
	fixedTool(s, api, "list_node_filters", "列出订阅和本地导入使用的节点过滤规则。", http.MethodGet, "/settings/node-filters")
	bodyTool[model.NodeFilterRequest](s, api, "create_node_filter", "新增节点过滤规则，使用 Go 正则表达式。", http.MethodPost, "/settings/node-filters")
	updateTool[model.NodeFilterRequest](s, api, "update_node_filter", "修改节点过滤规则。", "/settings/node-filters/:id")
	idTool(s, api, "delete_node_filter", "删除节点过滤规则。", http.MethodDelete, "/settings/node-filters/:id")
}
