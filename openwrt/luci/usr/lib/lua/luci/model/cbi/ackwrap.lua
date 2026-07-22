local sys = require "luci.sys"
local dispatcher = require "luci.dispatcher"

local function trim(value)
	return (value or ""):gsub("^%s+", ""):gsub("%s+$", "")
end

local function generate_api_token()
	local random = io.open("/dev/urandom", "rb")
	if not random then
		return nil
	end
	local bytes = random:read(24)
	random:close()
	if not bytes or #bytes ~= 24 then
		return nil
	end
	return (bytes:gsub(".", function(value)
		return string.format("%02x", value:byte())
	end))
end

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

local generate_token = settings:option(Button, "_generate_token", translate("随机 Token"))
generate_token.inputtitle = translate("随机生成")
generate_token.inputstyle = "apply"
generate_token.description = translate("生成 48 位随机 Token 后，请点击“保存并应用”使服务使用新 Token。")
function generate_token.write(self, section)
	local token = generate_api_token()
	if not token then
		m.message = translate("无法读取系统安全随机数，请稍后重试或手动填写 Token。")
		return
	end
	api_token:write(section, token)
end

local data_dir = settings:option(Value, "data_dir", translate("数据目录"))
data_dir.default = "/etc/ackwrap"
data_dir.rmempty = false

local logger = settings:option(Flag, "logger", translate("启用日志"))
logger.default = logger.enabled

local network_repair = settings:option(Button, "_network_repair", translate("网络修复"))
network_repair.inputtitle = translate("立即修复")
network_repair.inputstyle = "apply"
network_repair.description = translate("核心异常停止后，恢复 Ackwrap 接管的 DNS、路由和防火墙状态。检测到 sing-box 核心仍在运行时将拒绝修复。")
function network_repair.write()
	local output = trim(sys.exec("/etc/init.d/ackwrap network_repair 2>&1"))
	local marker, message = output:match("^([^\n]+)\n?(.*)$")
	message = trim(message)
	if marker == "OK" then
		m.message = message ~= "" and message or translate("网络修复完成。")
	elseif marker == "ERROR" then
		m.message = message ~= "" and message or translate("网络修复失败，请检查系统日志。")
	else
		m.message = translate("网络修复失败：未收到有效执行结果。")
	end
end

return m
