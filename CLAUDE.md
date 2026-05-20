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
│   ├── controller/wolplus.lua        # 路由与 API 端点（7 个 endpoint）
│   ├── model/cbi/wolplus.lua         # CBI 数据模型（向后兼容，模板模式不使用）
│   └── view/wolplus/
│       ├── index.htm                 # 主页面：卡片 UI + CSS + JS（~560行）
│       └── awake.htm                 # 按钮模板（CBI 模式遗留，模板模式不使用）
├── po/zh_Hans/wolplus.po             # 简体中文翻译源文件
├── wol-agent/                        # Windows 后台 Agent（Go）
│   ├── main.go                       # HTTP server，端口 32249
│   ├── go.mod                        # Go module
│   ├── install.bat                   # nssm 服务注册脚本
│   └── wol-agent.exe                 # 编译产物（~6MB, UPX后~1.5MB）
└── root/etc/config/wolplus           # UCI 默认配置模板
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
| POST | `/add` | 添加设备 (UCI write) | formvalue |
| POST | `/delete/<section>` | 删除设备 (UCI delete) | URL segment |

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

## UI 设计系统

### 布局

```
┌──────────────────────────────────────────────┐
│  + 添加设备                            ▼     │  ← 折叠表单卡片
│  ┌──────────────────────────────────────────┐│
│  │ 💻 安装 Windows Agent            [展开] ││  ← 表单内嵌，默认折叠
│  │ ─────────────────────────────────────── ││
│  │ Name: [____]  MAC: [Quick pick ▼]       ││
│  │ Interface: [▼]  IP: [Quick pick ▼]      ││
│  │                     [取消] [添加]        ││
│  └──────────────────────────────────────────┘│
│                                              │
│  ┌────────────────────────────────────┐ ─┐   │
│  │ ●  My PC                     ⚡ ⏻ ✕ │  │   │  ← 设备卡片
│  │    192.168.1.100                   │  │   │     (宽屏2列)
│  └────────────────────────────────────┘ ─┘   │
│  ┌────────────────────────────────────┐      │
│  │ ●  Office Laptop            ⚡ ⏻ ✕ │      │
│  │    192.168.1.101                   │      │
│  └────────────────────────────────────┘      │
└──────────────────────────────────────────────┘
```

### 设计原则

- **Material Design** 风格：卡片阴影、8px 圆角、状态过渡动画
- **无外部依赖**：所有 CSS 内联，图标使用内联 SVG（不依赖字体或 CDN）
- **响应式**：默认单列，≥700px 双列网格；≤520px 表单单列 + 按钮换行
- **折叠表单**：默认隐藏，点击标题展开/收起，与卡片列表分离
- **表单内嵌 Agent 说明**：表单顶部折叠区，展开后展示 Windows Agent 下载按钮 + nssm 安装命令
- **彩色状态点**：🟢 在线 / ⚪ 离线 / 🟠 开机中闪烁 / 🔴 关机中闪烁
- **彩色图标按钮**：唤醒绿色(#388e3c)、关机红色(#c62828)、删除灰色(#9e9e9e)
- **Toast 提示**：底部居中黑底白字，2.5s 自动消失
- **Luci 主题色**：下载按钮等交互元素使用 CSS 变量 `var(--primary)`，自动跟随 Luci 主题

### 图标

四枚 Feather Icons 风格内联 SVG，`viewBox="0 0 24 24"`，使用 `currentColor` 自动匹配按钮颜色：

- **唤醒** — 闪电形状 `<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>`
- **关机** — 电源符号（圆弧+竖线）
- **删除** — X 形交叉线
- **Windows 徽标** — 四格窗口 `<rect>` x4（Agent 安装入口图标）

SVG 内联方式避免了 emoji 跨平台渲染差异和字体依赖问题。

### Agent 下载

Agent 安装入口位于「添加设备」表单顶部，默认折叠。展开后显示：

- **下载按钮**：指向 GitHub raw URL（`https://raw.githubusercontent.com/<user>/<repo>/main/wol-agent/wol-agent.exe`），不内置在 ipk 中以保持包体轻量
- **nssm 命令**：浅色代码块（`#f5f5f5` 背景、`#616161` 灰字）
- 下载按钮无背景无边框，文字颜色由 Luci 主题 CSS 变量 `var(--primary)` 控制

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
g_shutting[section] = true        → 红点闪烁，保护不被 pollOne 覆盖为灰色
setDot(section, state)            → 更新 DOM 圆点样式
clearWaking / clearShutting       → 检测到在线/离线后提前结束等待
```

## 关键设计决策

### 为什么用 Template 而非 CBI

CBI 的 `tblsection` 表格渲染无法满足 Material Design 卡片布局需求。
改用 `template()` 后：

- **优点**：完全控制 HTML/CSS/JS，支持卡片布局、折叠表单、自定义交互
- **代价**：失去 CBI 自动表单处理、需手动实现 XHR 封装、需显式引入 header/footer
- **数据层**：直接通过 UCI cursor 读写 `/etc/config/wolplus`，add/delete 通过 controller 端点实现

### 为什么用内联 SVG 而非 emoji/iconfont

- emoji 跨平台渲染差异大（如 ⏻ 在某些浏览器显示为方框）
- iconfont 需要外部字体文件，OpenWrt 环境不支持
- 内联 SVG 零依赖、矢量无损、`currentColor` 自动匹配按钮状态色

### 为什么 MAC/IP 使用 Quick Pick select + input 组合

- `<datalist>` 在移动端 Safari 完全不显示建议列表
- `<select>` 在移动端有原生下拉 UI，兼容性最好
- 保留 `<input>` 允许手动输入不在列表中的值
- select 选中后自动填入 input，表单提交实际读取 input 值

### 为什么 Agent 下载放在表单顶部 + GitHub 外链

- Agent 是设备功能的前提，放在表单顶部符合「先装 Agent 再加设备」的心智模型
- 默认折叠，不影响表单简洁性，需要时展开
- 使用 GitHub raw URL 而非内置 exe，保持 ipk 轻量（~30KB）
- Agent 更新无需更新 Luci 包，推送 GitHub 即可

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
