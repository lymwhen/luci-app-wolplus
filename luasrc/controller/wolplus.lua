module("luci.controller.wolplus", package.seeall)
local x = luci.model.uci.cursor()

function index()
    if not nixio.fs.access("/etc/config/wolplus") then return end
    entry({"admin", "services", "wolplus"}, template("wolplus/index"), _("Wake on LAN"), 95).dependent = true
    entry({"admin", "services", "wolplus", "awake"}, post("awake")).leaf = true
    entry({"admin", "services", "wolplus", "status"}, post("status")).leaf = true
    entry({"admin", "services", "wolplus", "status_all"}, post("status_all")).leaf = true
    entry({"admin", "services", "wolplus", "shutdown"}, post("shutdown")).leaf = true
    entry({"admin", "services", "wolplus", "add"}, post("add")).leaf = true
    entry({"admin", "services", "wolplus", "delete"}, post("delete")).leaf = true
end

function awake(sections)
    local lan = x:get("wolplus", sections, "maceth")
    local mac = x:get("wolplus", sections, "macaddr")
    local cmd = "/usr/bin/etherwake -D -i " .. lan .. " -b " .. mac .. " 2>&1"
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
    luci.http.prepare_content("application/json")
    luci.http.write_json({data = msg})
end

function status_all()
    local result = {}
    x:foreach("wolplus", "macclient", function(s)
        local ip = s.ipaddr
        if ip and ip ~= "" then
            local cmd = "curl -s -m 2 http://" .. ip .. ":32249/api/v1/status 2>/dev/null"
            local p = io.popen(cmd)
            local resp = ""
            if p then
                resp = p:read("*a") or ""
                p:close()
            end
            local online = false
            if resp ~= "" and resp:find('"online":true', 1, true) then
                online = true
            end
            result[#result + 1] = {section = s['.name'], online = online}
        end
    end)
    luci.http.prepare_content("application/json")
    luci.http.write_json(result)
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

function add()
    local name  = luci.http.formvalue("name")
    local mac   = luci.http.formvalue("macaddr")
    local eth   = luci.http.formvalue("maceth")
    local ip    = luci.http.formvalue("ipaddr")

    if not name or name == "" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({success = false, message = "Name is required"})
        return
    end
    if not mac or mac == "" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({success = false, message = "MAC address is required"})
        return
    end
    if not eth or eth == "" then
        luci.http.prepare_content("application/json")
        luci.http.write_json({success = false, message = "Network interface is required"})
        return
    end

    local section = x:add("wolplus", "macclient")
    x:set("wolplus", section, "name", name)
    x:set("wolplus", section, "macaddr", mac:upper())
    x:set("wolplus", section, "maceth", eth)
    if ip and ip ~= "" then
        x:set("wolplus", section, "ipaddr", ip)
    end
    x:save("wolplus")
    x:commit("wolplus")

    luci.http.prepare_content("application/json")
    luci.http.write_json({success = true, section = section, name = name, ipaddr = ip})
end

function delete(sections)
    x:delete("wolplus", sections)
    x:save("wolplus")
    x:commit("wolplus")
    luci.http.prepare_content("application/json")
    luci.http.write_json({success = true})
end
