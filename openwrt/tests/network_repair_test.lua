local options, command, response = {}, nil, ""
package.preload["luci.sys"] = function()
	return {
		call = function() return 1 end,
		exec = function(value) command = value; return response end,
	}
end
package.preload["luci.dispatcher"] = function()
	return { build_url = function() return "/test" end }
end
translate = function(value) return value end
SimpleSection, TypedSection, Flag, Value, Button = {}, {}, {}, {}, {}
local page = {
	section = function()
		return {
			option = function(_, _, name)
				local option = {}
				options[name] = option
				return option
			end,
		}
	end,
}
Map = function() return page end
dofile("openwrt/luci/usr/lib/lua/luci/model/cbi/ackwrap.lua")
local button = assert(options._network_repair)
assert(button.inputtitle == "立即修复")
response = "OK\n强制网络修复完成\n"
button.write()
assert(command == "/etc/init.d/ackwrap network_repair --force 2>&1")
assert(page.message == "强制网络修复完成")
response = "ERROR\nfw4 reload failed\n"
button.write()
assert(page.message == "fw4 reload failed")
response = "unexpected response"
button.write()
assert(page.message:find("未收到有效执行结果", 1, true))
print("LuCI repair button: force command, success and failure passed")
