local i = require "luci.sys"
local t, e
t = Map("wolplus", translate("Wake on LAN +"), "Wake-on-LAN remotely starts local computers over Ethernet. " .. [[<a href="https://github.com/sundaqiang/openwrt-packages" target="_blank">Powered by sundaqiang</a>]])
t.template = "wolplus/index"
e = t:section(TypedSection, "macclient", translate("Machines"))
e.template = "cbi/tblsection"
e.anonymous = true
e.addremove = true
---- add device section
a = e:option(Value, "name", "Name")
a.optional = false
---- mac address
nolimit_mac = e:option(Value, "macaddr", "MAC Address")
nolimit_mac.rmempty = false
i.net.mac_hints(function(e, t) nolimit_mac:value(e, "%s (%s)" % {e, t}) end)
----- network interface
nolimit_eth = e:option(Value, "maceth", translate("Network Interface"))
nolimit_eth.rmempty = false
for t, e in ipairs(i.net.devices()) do if e ~= "lo" then nolimit_eth:value(e) end end
----- ip address
nolimit_ip = e:option(Value, "ipaddr", translate("IP Address"))
nolimit_ip.rmempty = true
nolimit_ip.datatype = "ipaddr"
-- ip hints from dhcp leases
local function ip_hints(cb)
    local f = io.open("/tmp/dhcp.leases")
    if f then
        local seen = {}
        for line in f:lines() do
            local _, mac, ip = line:match("^(%S+)%s+(%S+)%s+(%S+)")
            if mac and ip and not seen[ip] then
                seen[ip] = true
                cb(mac:upper() .. " (" .. ip .. ")", ip)
            end
        end
        f:close()
    end
end
ip_hints(function(label, ip) nolimit_ip:value(label, ip) end)
----- wake device
btn = e:option(Button, "_awake", translate("Operate"))
btn.inputtitle = translate("Awake")
btn.inputstyle = "apply"
btn.disabled = false
btn.template = "wolplus/awake"
function gen_uuid(format)
    local uuid = i.exec("echo -n $(cat /proc/sys/kernel/random/uuid)")
    if format == nil then
        uuid = string.gsub(uuid, "-", "")
    end
    return uuid
end
function e.create(e, t)
    local uuid = gen_uuid()
    t = uuid
    TypedSection.create(e, t)
end

return t
