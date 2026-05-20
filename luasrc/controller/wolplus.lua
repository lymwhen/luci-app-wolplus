module("luci.controller.wolplus", package.seeall)
local t, a
local x = luci.model.uci.cursor()

function index()
    if not nixio.fs.access("/etc/config/wolplus") then return end
    entry({"admin", "services", "wolplus"}, cbi("wolplus"), _("Wake on LAN"), 95).dependent = true
    entry( {"admin", "services", "wolplus", "awake"}, post("awake") ).leaf = true
    entry( {"admin", "services", "wolplus", "status"}, post("status") ).leaf = true
    entry( {"admin", "services", "wolplus", "shutdown"}, post("shutdown") ).leaf = true
end

function awake(sections)
	lan = x:get("wolplus",sections,"maceth")
	mac = x:get("wolplus",sections,"macaddr")
    local e = {}
    cmd = "/usr/bin/etherwake -D -i " .. lan .. " -b " .. mac .. " 2>&1"
	local p = io.popen(cmd)
	local msg = ""
	if p then
		while true do
			local l = p:read("*l")
			if l then
				if #l > 100 then l = l:sub(1, 100) .. "..." end
				msg = msg .. l
			else
				break
			end
		end
		p:close()
	end
	e["data"] = msg
    luci.http.prepare_content("application/json")
    luci.http.write_json(e)
end

function status(sections)
    local ip = x:get("wolplus", sections, "ipaddr")
    if not ip or ip == "" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({online = false, error = "No IP configured"})
        return
    end
    local cmd = "curl -s -m 2 http://" .. ip .. ":32249/api/v1/status 2>/dev/null"
    local p = io.popen(cmd)
    local result = ""
    if p then
        result = p:read("*a") or ""
        p:close()
    end
    luci.http.prepare_content("application/json")
    if result ~= "" then
        luci.http.write(result)
    else
        luci.http.write_json({online = false})
    end
end

function shutdown(sections)
    local ip = x:get("wolplus", sections, "ipaddr")
    if not ip or ip == "" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({success = false, message = "No IP configured"})
        return
    end
    local cmd = "curl -s -m 5 -X POST http://" .. ip .. ":32249/api/v1/shutdown 2>/dev/null"
    local p = io.popen(cmd)
    local result = ""
    if p then
        result = p:read("*a") or ""
        p:close()
    end
    luci.http.prepare_content("application/json")
    if result ~= "" then
        luci.http.write(result)
    else
        luci.http.write_json({success = false, message = "No response from agent"})
    end
end
