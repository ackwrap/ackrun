package mcpserver

import (
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ackwrap/ackrun/internal/model"
)

func registerDNSTools(s *mcp.Server, api *apiExecutor) {
	fixedTool(s, api, "list_dns_servers", "列出 DNS 服务器。", http.MethodGet, "/dns/servers")
	idTool(s, api, "get_dns_server", "读取 DNS 服务器配置。", http.MethodGet, "/dns/servers/:id")
	bodyTool[model.DNSServerRequest](s, api, "create_dns_server", "新增 DNS 服务器。", http.MethodPost, "/dns/servers")
	updateTool[model.DNSServerRequest](s, api, "update_dns_server", "修改 DNS 服务器。", "/dns/servers/:id")
	idTool(s, api, "delete_dns_server", "删除 DNS 服务器。", http.MethodDelete, "/dns/servers/:id")
	orderTool(s, api, "reorder_dns_servers", "按完整 ID 顺序排列 DNS 服务器。", "/dns/servers/reorder")
	fixedTool(s, api, "list_dns_hosts", "列出固定域名解析。", http.MethodGet, "/dns/hosts")
	idTool(s, api, "get_dns_host", "读取固定域名解析配置。", http.MethodGet, "/dns/hosts/:id")
	bodyTool[model.DNSHostRequest](s, api, "create_dns_host", "新增固定域名解析。", http.MethodPost, "/dns/hosts")
	updateTool[model.DNSHostRequest](s, api, "update_dns_host", "修改固定域名解析。", "/dns/hosts/:id")
	idTool(s, api, "delete_dns_host", "删除固定域名解析。", http.MethodDelete, "/dns/hosts/:id")
	fixedTool(s, api, "list_dns_rules", "列出 DNS 分流规则。", http.MethodGet, "/dns/rules")
	idTool(s, api, "get_dns_rule", "读取 DNS 分流规则。", http.MethodGet, "/dns/rules/:id")
	bodyTool[model.DNSRuleRequest](s, api, "create_dns_rule", "新增 DNS 分流规则。", http.MethodPost, "/dns/rules")
	updateTool[model.DNSRuleRequest](s, api, "update_dns_rule", "修改 DNS 分流规则。", "/dns/rules/:id")
	idTool(s, api, "delete_dns_rule", "删除 DNS 分流规则。", http.MethodDelete, "/dns/rules/:id")
	orderTool(s, api, "reorder_dns_rules", "按完整 ID 顺序排列 DNS 分流规则。", "/dns/rules/reorder")
	fixedTool(s, api, "get_dns_global", "读取 DNS 全局配置。", http.MethodGet, "/dns/global")
	bodyTool[model.DNSGlobalSettings](s, api, "set_dns_global", "保存 DNS 全局配置；先读取现有值并保留无关字段。", http.MethodPut, "/dns/global")
}
