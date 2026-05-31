# CLAUDE.md — luci-app-wolplus

OpenWrt 远程开机 LuCI 插件 + Windows 后台 Agent，支持 Wake-on-LAN 唤醒、在线状态检测、远程关机。

## 架构

```
OpenWrt Router (Luci App)                     Windows PC (Go Agent)
┌──────────────────────────────┐              ┌─────────────────────────┐
│  /admin/services/wolplus     │              │  wol-agent.exe           │
│  controller/wolplus.lua      │  HTTP/JSON   │  listen :32249           │
│  ├─ awake     → etherwake    │─────▶────────│  ├─ GET  /api/v1/status  │
│  ├─ status    → curl agent   │◀─────┼───────│  ├─ POST /api/v1/shutdown│
│  ├─ shutdown  → curl agent   │              │  └─ POST /api/v1/reboot  │
│  ├─ add       → UCI write    │              │  (reboot 预留)           │
│  └─ delete    → UCI delete   │              └─────────────────────────┘
│                              │
│  index.htm (template 渲染)    │
│  ├─ 服务端 Lua 读 UCI 配置    │
│  ├─ Material Design 卡片列表  │
│  ├─ 折叠式添加表单            │
│  └─ JS: 30s 轮询 + 60s 等待  │
└──────────────────────────────┘
```

## 项目结构

```
luci-app-wolplus/
├── Makefile                          # OpenWrt 编译描述
├── luasrc/
│   ├── controller/wolplus.lua        # 路由与 API 端点（9 个 endpoint）
│   ├── model/cbi/wolplus.lua         # CBI 数据模型（向后兼容，模板模式不使用）
│   └── view/wolplus/
│       ├── index.htm                 # 主页面：卡片 UI + CSS + JS（~660行）
│       └── awake.htm                 # 按钮模板（CBI 模式遗留，模板模式不使用）
├── po/zh_Hans/wolplus.po             # 简体中文翻译源文件
├── wol-agent/                        # Windows 后台 Agent（Go）
│   ├── main.go                       # HTTP server，端口 32249
│   ├── go.mod                        # Go module
│   ├── install.bat                   # nssm 服务注册脚本
│   └── wol-agent.exe                 # 编译产物（~6MB, UPX后~1.5MB）
├── root/etc/config/wolplus           # UCI 默认配置模板
├── docs/project.md                   # 详细设计文档
└── CLAUDE.md                         # 本文件 — 关键参考
```

## Luci App — Controller API

主页面从 CBI Map 模式改为纯 Template 模式：

```lua
-- controller/wolplus.lua
entry({"admin", "services", "wolplus"}, template("wolplus/index"), ...)
-- 而非 cbi("wolplus")
```

**关键影响**：模板模式不加载 `cbi.js` → `XHR` 类不可用 → 需自行实现。也不带 admin 框架 → 需显式 `<%+header%>` / `<%+footer%>`。

### API 端点一览

| 方法 | 路径 | 功能 | 参数来源 |
|------|------|------|---------|
| POST | `/awake/<section>` | 发送 Magic Packet (etherwake) | URL segment |
| POST | `/status/<section>` | 查询单台在线状态 (curl agent) | URL segment |
| POST | `/status_all` | 批量查询所有在线状态 | — |
| POST | `/shutdown/<section>` | 远程关机 (curl agent) | URL segment |
| POST | `/add` | 添加设备 (UCI write + 自动排序) | formvalue |
| POST | `/delete/<section>` | 删除设备 (UCI delete) | URL segment |
| POST | `/move_up/<section>` | 上移设备 (交换 order) | URL segment |
| POST | `/move_down/<section>` | 下移设备 (交换 order) | URL segment |

**Token 验证**：POST 端点有自动 CSRF 检查。在 `template()` 模式下，token 必须出现在 query string 中（而非 JSON body），因为 Luci 的 token 解析优先检查 URL 参数。

## Windows Agent

### 构建

```powershell
# 需要 Go 1.21+，安装 https://go.dev/dl/
cd wol-agent
go build -ldflags="-s -w" -o wol-agent.exe   # ~6MB
upx --best wol-agent.exe                       # ~1.5MB（可选）
```

### API

```
GET  /api/v1/status    → 200 {"online":true,"hostname":"DESKTOP-XX","os":"windows","uptime":3600}
POST /api/v1/shutdown  → 200 {"success":true,"action":"shutdown","delay":5,"message":"..."}
                        → 执行 shutdown /s /t 5
```

### 安装为 Windows 服务

```batch
# 需要 nssm (https://nssm.cc)
nssm install WolAgent "C:\path\to\wol-agent.exe" --port 32249
nssm set WolAgent Start SERVICE_AUTO_START
nssm start WolAgent
```

或直接运行 `install.bat`（管理员权限）。

## 状态检测与轮询逻辑

### 轮询策略

| 场景 | 间隔 | 行为 |
|------|------|------|
| 页面空闲 | 30s | 全量轮询所有设备 |
| 点击唤醒后 | 5s | 快速轮询该设备，最多 60s（12 次） |
| 点击关机后 | 5s | 快速轮询该设备，最多 60s（12 次） |
| 等待期间 | — | 状态保持闪烁，即使 Agent 无响应也**不降级为灰色** |

### 关键 JS 状态机

```
g_waking[section] = true          → 黄点闪烁，保护不被 pollOne 覆盖为灰色
g_shutting[section] = true        → 红点闪烁，保护不被 pollOne 覆盖为绿色
setDot(section, state)            → 更新 DOM 圆点样式
clearWaking / clearShutting       → 检测到在线/离线后提前结束等待
```

**pollOne 保护规则**：

| 状态 | agent 在线 | agent 离线 |
|------|-----------|-----------|
| `g_waking` | 变绿 + clearWaking | **忽略**（保持橙闪） |
| `g_shutting` | **忽略**（保持红闪） | 变灰 + clearShutting |
| 空闲 | 变绿 | 变灰 |

关机期间即使 agent 短暂仍在线（关机未完成），红点持续闪烁不跳变。

## 常见问题与解决方案

### 1. Template 页面 403 Forbidden

**现象**：所有 POST 端点返回 403。
**原因**：CSRF token 在 JSON body 中未被 Luci dispatcher 识别（template 模式 vs CBI 模式解析差异）。
**解决**：XHR shim 自动将 token 拼接到 URL query string 中：

```javascript
if (url.indexOf('token=') === -1) {
    url += (url.indexOf('?') === -1 ? '?' : '&') + 'token=' + encodeURIComponent(token);
}
```

### 2. XHR is not defined

**现象**：控制台报 `ReferenceError: XHR is not defined`。
**原因**：`template()` 页面不加载 `/luci-static/resources/cbi.js`。
**解决**：在 JS 开头添加 XHR shim：

```javascript
function XHR() {}
XHR.prototype.post = function(url, data, callback) {
    var x = new XMLHttpRequest();
    x.open('POST', url, true);
    if (data && typeof data === 'object') {
        x.setRequestHeader('Content-Type', 'application/json');
        data = JSON.stringify(data);
    }
    x.onload = function() { callback(x); };
    x.send(data || null);
};
```

### 3. OpenWrt 框架（菜单栏/标题）不显示

**现象**：页面只显示自定义内容，左侧菜单和顶部栏消失。
**原因**：`template()` 不带 admin 主题壳。
**解决**：在模板 HTML 最外层加 `<%+header%>` 和 `<%+footer%>`。

### 4. Lua 模板语法错误

**现象**：`Syntax error ... ')' expected (to close '(' at line X) near 'local'`
**原因**：`uci:foreach(...)` 或类似函数调用中，内联 function 的 `end` 之后缺少关闭 `)`。
**检查**：确认 `end)` 而非 `end`（`end` 关闭函数体，`）` 关闭外层函数调用）。

### 5. 翻译不生效

**现象**：修改 `.po` 后重启，页面仍显示原文。
**原因**：Luci 读取的是编译后的 `.lmo` 文件，位于 `/usr/lib/lua/luci/i18n/`。
**解决**：需在路由器上运行 `po2lmo wolplus.po wolplus.zh-cn.lmo`。

### 6. 卡片间距异常

**现象**：卡片间距过大或与 Add Device 卡片间距不一致。
**原因**：CSS Grid `gap` 与 `.md-card` 的 `margin-bottom` 叠加。
**解决**：Grid 容器内的卡片去掉 `margin-bottom`，仅靠 grid `gap` 控制；Grid 外的 `.md-add-card` 单独设置 `margin-bottom`。

### 7. Luci 主题色覆盖链接样式

**现象**：自定义 `<a>` 标签的颜色在 Luci 页面上被覆盖为主题色。
**原因**：Luci 全局 CSS 使用 `var(--primary)` 为链接设置颜色，优先级高于内联样式。
**解决**：Agent 下载按钮不设置 `background` 和 `border`，作为纯文字链接，让 Luci 主题自然控制颜色。按钮仅保留 `padding` + `border-radius` 作为点击区域。

### 8. 暗色模式下按钮/下拉菜单出现意外边框

**现象**：暗色模式下菜单项出现灰色边框。
**原因**：Luci 暗色模式 CSS 有全局规则 `button { border: 1px solid #3c3c3c !important; }`，覆盖了自定义的 `border: none`。
**解决**：自定义按钮必须用 `border: none !important` 反制，只作用于精确的 class 选择器下（如 `.md-dropdown button`）。

## 部署清单

### Luci App 部署

```bash
# 文件 → 路由器
scp luasrc/controller/wolplus.lua  root@<IP>:/usr/lib/lua/luci/controller/
scp luasrc/view/wolplus/index.htm  root@<IP>:/usr/lib/lua/luci/view/wolplus/

# 翻译编译 + 部署（路由器上执行）
scp po/zh_Hans/wolplus.po root@<IP>:/tmp/
ssh root@<IP> "po2lmo /tmp/wolplus.po /usr/lib/lua/luci/i18n/wolplus.zh-cn.lmo"

# 清缓存 + 重启 web
ssh root@<IP> "rm -rf /tmp/luci-* && /etc/init.d/uhttpd restart"
```

### Windows Agent 部署

```powershell
cd wol-agent
go build -ldflags="-s -w" -o wol-agent.exe
# 管理员运行 install.bat（需预先安装 nssm）
```

### 前置依赖

- **路由器**：`etherwake`（发送 Magic Packet）、`curl`（调用 Agent API）
- **Windows**：Go 1.21+（仅编译时需要）、nssm（服务化）

### Commit

- **格式**：`type: description`，type = `feat` / `fix` / `docs` / `chore` / `refactor`
- **禁止**以 `@` 开头，禁止在 commit message 两端使用 `@'...'@` 包裹
- **推送前**检查 `git log --oneline -1` 确认格式正确
