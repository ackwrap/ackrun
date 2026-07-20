module("luci.controller.ackwrap", package.seeall)

function index()
	local fs = require "nixio.fs"
	if not fs.access("/etc/config/ackwrap") then
		return
	end

	entry({"admin", "services", "ackwrap"}, cbi("ackwrap"), _("Ackwrap"), 51).dependent = true
	entry({"admin", "services", "ackwrap", "status"}, call("action_status")).leaf = true
	entry({"admin", "services", "ackwrap", "open"}, call("action_open")).leaf = true
end

local function ackwrap_url(http, uci)
	local host = http.getenv("HTTP_HOST") or http.getenv("SERVER_NAME") or ""
	host = host:gsub(":%d+$", "")
	if host == "_redirect2ssl" or host == "redirect2ssl" or host == "" then
		host = http.getenv("SERVER_ADDR") or "localhost"
	end
	local port = uci:get("ackwrap", "main", "port") or "8080"
	return "http://" .. host .. ":" .. port .. "/"
end

function action_status()
	local sys = require "luci.sys"
	local uci = require "luci.model.uci".cursor()
	local http = require "luci.http"
	http.prepare_content("application/json")
	http.write_json({
		running = (sys.call("pidof ackwrap >/dev/null") == 0),
		url = require("luci.dispatcher").build_url("admin", "services", "ackwrap", "open")
	})
end

function action_open()
	local uci = require "luci.model.uci".cursor()
	local http = require "luci.http"
	local token = uci:get("ackwrap", "main", "api_token") or ""
	if #token < 16 or #token > 128 or not token:match("^[A-Za-z0-9._~%-]+$") then
		http.status(500, "Invalid Ackwrap API Token")
		http.prepare_content("text/plain")
		http.write("Ackwrap API Token is missing or invalid")
		return
	end
	http.header("Set-Cookie", "ackwrap_api_token=" .. token .. "; Path=/api; Max-Age=2592000; HttpOnly; SameSite=Strict")
	http.redirect(ackwrap_url(http, uci))
end
