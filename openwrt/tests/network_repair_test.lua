local options, sections, commands, response, running
package.preload["luci.sys"] = function()
	return {
		call = function(value)
			assert(value == "pidof /usr/bin/ackwrap >/dev/null 2>&1")
			return running and 0 or 1
		end,
		exec = function(value) commands[#commands + 1] = value; return response end,
	}
end
package.preload["luci.dispatcher"] = function()
	return { build_url = function() return "/test" end }
end
translate = function(value) return value end
SimpleSection, TypedSection, Flag, Value, Button = {}, {}, {}, {}, {}
Map = function()
	return {
		section = function()
			local section = {
				option = function(_, _, name)
					local option = {}
					options[name] = option
					return option
				end,
			}
			sections[#sections + 1] = section
			return section
		end,
	}
end
local function load_page()
	options, sections, commands = {}, {}, {}
	response, running = "", false
	return dofile("openwrt/luci/usr/lib/lua/luci/model/cbi/ackwrap.lua")
end

for _, result in ipairs({
	{"OK\n强制网络修复完成\n", "强制网络修复完成"},
	{"ERROR\nfw4 reload failed\n", "fw4 reload failed"},
	{"", "未收到有效执行结果"},
}) do
	local page = load_page()
	local button = assert(options._network_repair)
	assert(button.inputtitle == "立即修复")
	response = result[1]
	button.write()
	assert(commands[1] == "/etc/init.d/ackwrap network_repair --force 2>&1")
	assert(page.message:find(result[2], 1, true))
	-- CBI reparses children after applying. Never repair twice or restart here.
	button.write()
	page.on_after_apply()
	assert(#commands == 1)
end

local page = load_page()
-- Map.parse commits UCI before on_after_apply when apply_on_parse is false.
assert(page.apply_on_parse == false)
assert(#commands == 0) -- Loading/saving alone does not start the service.
response = "OK\n设置已应用，Ackwrap 已启动。\n"
page.on_after_apply()
assert(commands[1] == "/etc/init.d/ackwrap apply_settings 2>&1")
assert(page.message:find("已启动", 1, true))
running = true
page.on_init()
assert(sections[1].running == true)
running = false
page.on_init()
assert(sections[1].running == false)

page = load_page()
response = "ERROR\n无法启动 Ackwrap\n"
page.on_after_apply()
assert(page.message == "无法启动 Ackwrap")
print("LuCI repair/apply results, reparse guard and fresh service status passed")
