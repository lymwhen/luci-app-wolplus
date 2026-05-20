# luci-app-wolplus（网络唤醒++）

[English](README_en.md)

OpenWrt LuCI 插件，支持 **Wake-on-LAN 远程开机**、**在线状态检测** 和 **远程关机**。配套 Windows 后台 Agent，通过 HTTP 响应路由器指令。

## 功能

- 发送 Magic Packet 远程唤醒局域网设备
- 实时在线状态检测（需安装 Windows Agent）
- 远程关机（需安装 Windows Agent）
- Material Design 卡片式 UI，响应式布局
- 设备配置持久化（UCI）

## 架构

```
OpenWrt Router (Luci App)                    Windows PC (Agent)
┌──────────────────────────┐                ┌─────────────────────┐
│  /admin/services/wolplus │                │  wol-agent.exe      │
│  ├─ 唤醒  → etherwake ───┼── Magic Packet ──▶ 网卡               │
│  ├─ 状态  → curl ────────┼── HTTP GET ────▶  :32249/api/v1/status│
│  └─ 关机  → curl ────────┼── HTTP POST ───▶  :32249/api/v1/shutdown│
└──────────────────────────┘                └─────────────────────┘
```

## 截图

```
┌──────────────────────────────────────┐
│  + 添加设备                        ▼  │
│  ┌──────────────────────────────────┐ │
│  │ 💻 安装 Windows Agent     [展开]  │ │
│  │ ──────────────────────────────── │ │
│  │ 名称: [___] MAC: [快速选择 ▼]    │ │
│  │ 网络接口: [▼] IP: [快速选择 ▼]   │ │
│  │                  [取消] [添加]   │ │
│  └──────────────────────────────────┘ │
│                                       │
│  ┌────────────────────┐ ┌────────────┐│
│  │ 🟢 My-Gaming-PC    │ │ ⚪ Server  ││
│  │   192.168.1.100    │ │   未配置IP ││
│  │        ⚡  ⏻  ✕    │ │  ⚡ ⏻ ✕   ││
│  └────────────────────┘ └────────────┘│
└──────────────────────────────────────┘
```

## 依赖

**路由器：**
- `etherwake` — 发送 Magic Packet
- `curl` — 调用 Agent API

**Windows（目标电脑）：**
- [nssm](https://nssm.cc) — 将 Agent 注册为 Windows 服务
- Go 1.21+（仅编译时需要）

## 安装

### 1. 安装 Luci App

```bash
# 手动部署（开发测试用）
scp luasrc/controller/wolplus.lua  root@<ROUTER_IP>:/usr/lib/lua/luci/controller/
scp luasrc/view/wolplus/index.htm  root@<ROUTER_IP>:/usr/lib/lua/luci/view/wolplus/

# 编译翻译
ssh root@<ROUTER_IP> "po2lmo /tmp/wolplus.po /usr/lib/lua/luci/i18n/wolplus.zh-cn.lmo"

# 清除缓存
ssh root@<ROUTER_IP> "rm -rf /tmp/luci-* && /etc/init.d/uhttpd restart"
```

或通过 OpenWrt 构建系统编译为 ipk 包。

### 2. 安装 Windows Agent

在 Luci 页面中找到「添加设备」→「安装 Windows Agent」，下载 `wol-agent.exe`。

```batch
# 以管理员身份运行
nssm install WolAgent "C:\path\to\wol-agent.exe" --port 32249
nssm set WolAgent Start SERVICE_AUTO_START
nssm start WolAgent
```

Agent 默认监听端口 `32249`。

### 3. 添加设备

在 Luci 页面展开「添加设备」，填写设备信息：

| 字段 | 说明 |
|------|------|
| 名称 | 设备显示名称 |
| MAC 地址 | 目标网卡 MAC（Quick Pick 提供 ARP 表建议） |
| 网络接口 | 路由器发出 Magic Packet 的网卡 |
| IP 地址 | 目标电脑 IP（Quick Pick 提供 DHCP 租约建议） |

## Windows Agent API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/status` | 返回在线状态、主机名、系统、运行时间 |
| POST | `/api/v1/shutdown` | 执行 `shutdown /s /t 5`，支持取消 |

```json
// GET /api/v1/status
{"online":true,"hostname":"DESKTOP-ABC","os":"windows","uptime":3600}

// POST /api/v1/shutdown
{"success":true,"action":"shutdown","delay":5,"message":"System will shutdown in 5 seconds"}
```

## 编译 Agent

```powershell
cd wol-agent
go build -ldflags="-s -w" -o wol-agent.exe    # ~6MB
upx --best wol-agent.exe                       # ~1.5MB（可选）
```

## 项目结构

```
luci-app-wolplus/
├── Makefile
├── luasrc/
│   ├── controller/wolplus.lua     # 路由 & API（7 个端点）
│   ├── model/cbi/wolplus.lua      # CBI 模型（向后兼容）
│   └── view/wolplus/
│       ├── index.htm              # 主页面（Material Design 卡片 UI）
│       └── awake.htm              # 按钮模板（CBI 模式遗留）
├── po/zh_Hans/wolplus.po          # 简体中文翻译
├── wol-agent/                     # Windows Agent (Go)
│   ├── main.go
│   ├── go.mod
│   ├── install.bat
│   └── wol-agent.exe
└── root/etc/config/wolplus        # UCI 默认配置
```

## 致谢

基于 [sundaqiang/openwrt-packages](https://github.com/sundaqiang/openwrt-packages) 原版 wolplus 插件扩展开发。
