local sys = require "luci.sys"
local dispatcher = require "luci.dispatcher"

local m = Map("ackwrap", translate("Ackwrap"))
m.description = translate("Ackwrap 提供 sing-box 配置、订阅、规则与运行管理 Web 界面。")

local status = m:section(SimpleSection, translate("状态"))
status.template = "ackwrap/status"
status.running = (sys.call("pidof ackwrap >/dev/null") == 0)
status.url = dispatcher.build_url("admin", "services", "ackwrap", "open")

local settings = m:section(TypedSection, "ackwrap", translate("设置"))
settings.anonymous = true

local enabled = settings:option(Flag, "enabled", translate("启用"))
enabled.default = enabled.enabled
enabled.rmempty = false

local listen_port = settings:option(Value, "port", translate("监听端口"))
listen_port.datatype = "range(1,65535)"
listen_port.default = "8080"
listen_port.rmempty = false

local api_token = settings:option(Value, "api_token", translate("访问 Token"))
api_token.password = true
api_token.rmempty = false
function api_token.validate(self, value)
	if #value < 16 or #value > 128 or not value:match("^[A-Za-z0-9._~%-]+$") then
		return nil, translate("Token 必须为 16 至 128 位，只能包含字母、数字、点、下划线、波浪号或连字符。")
	end
	return value
end

local generate_token = settings:option(DummyValue, "_generate_token", translate("随机 Token"))
generate_token.template = "ackwrap/token_button"

local data_dir = settings:option(Value, "data_dir", translate("数据目录"))
data_dir.default = "/etc/ackwrap"
data_dir.rmempty = false

local logger = settings:option(Flag, "logger", translate("启用日志"))
logger.default = logger.enabled

return m
