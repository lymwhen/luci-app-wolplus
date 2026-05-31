# luci-app-wolplus — 项目文档

OpenWrt LuCI 远程开机插件 + Windows 后台 Agent，同时为 [EasyTier](../../../projectsAlpha/EasyTier/) 移动端 App 提供 WOL API 服务。

## 目录

1. [项目定位](#项目定位)
2. [架构总览](#架构总览)
3. [Luci App 设计](#luci-app-设计)
4. [Windows Agent 设计](#windows-agent-设计)
5. [EasyTier 集成](#easytier-集成)
6. [设计决策记录](#设计决策记录)
7. [测试与验证](#测试与验证)

---

## 项目定位

### 两个使用场景

| 场景 | 用户 | 访问方式 |
|------|------|---------|
| **OpenWrt 路由器 Web 管理** | 路由器管理员 | 浏览器访问 `/cgi-bin/luci/admin/services/wolplus` |
| **EasyTier 移动端 App** | EasyTier 用户 | App 内通过 TOML 配置设备，通过路由器 CGI 和 Agent HTTP 操作 |

### 功能矩阵

| 功能 | Luci Web | EasyTier App |
|------|---------|-------------|
| 唤醒 (Magic Packet) | ✓ 按钮 | ✓ 通过路由器 CGI |
| 在线状态 | ✓ 30s 轮询 | ✓ 30s 轮询 + 手动刷新 |
| 远程关机 | ✓ 按钮 | ✓ 直接调 Agent |
| 设备管理 (增删改) | ✓ 完整 | ✓ TOML 配置 |
| 排序 | ✓ 上移/下移 | ✗ 按状态排序 |

---

## 架构总览

```
┌──────────────────────────────────────────────────────────────────┐
│                        OpenWrt Router                            │
│                                                                  │
│  ┌─────────────────────┐    ┌──────────────────────────────────┐ │
│  │ Luci Web (浏览器)    │    │ EasyTier App (Android)          │ │
│  │ /admin/services/    │    │ /cgi-bin/wolplus-api?action=   │ │
│  │   wolplus           │    │   wake&mac=...&iface=...        │ │
│  └────────┬────────────┘    └───────────────┬──────────────────┘ │
│           │                                 │                    │
│  ┌────────▼─────────────────────────────────▼──────────────────┐ │
│  │              controller/wolplus.lua                         │ │
│  │  ├─ awake(section)    → etherwake                           │ │
│  │  ├─ status(section)   → curl agent                         │ │
│  │  ├─ shutdown(section) → curl agent                         │ │
│  │  ├─ add/delete        → UCI                                │ │
│  │  └─ move_up/down      → UCI order                          │ │
│  └─────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  依赖: etherwake (Magic Packet), curl (HTTP to Agent)            │
└──────────────────────────────────────────────────────────────────┘
         │ Magic Packet (L2)            │ HTTP (L3)
         ▼                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                       Windows PC                                 │
│                                                                  │
│  wol-agent.exe (Go, 端口 32249, nssm 服务化)                     │
│  ├─ GET  /api/v1/status   → {"online":true,"hostname":"..."}    │
│  ├─ POST /api/v1/shutdown → shutdown /s /t 5                    │
│  └─ POST /api/v1/reboot   → shutdown /r /t 5 (预留)             │
└──────────────────────────────────────────────────────────────────┘
```

### 数据流

| 操作 | 触发方 | 路径 | 目标 |
|------|--------|------|------|
| 唤醒 | Luci Web / EasyTier | 路由器 → etherwake → Magic Packet (L2) | 目标 PC 网卡 |
| 状态 | Luci Web | 路由器 curl → Agent HTTP | Agent |
| 状态 | EasyTier | App httpGet → Agent HTTP (直连) | Agent |
| 关机 | Luci Web | 路由器 curl → Agent HTTP | Agent |
| 关机 | EasyTier | App httpPost → Agent HTTP (直连) | Agent |

---

## Luci App 设计

### 技术栈

- **后端**：Lua (Luci controller)，通过 UCI cursor 读写 `/etc/config/wolplus`
- **前端**：Luci `template()` 模式，内联 CSS + JS，零外部依赖
- **图标**：内联 SVG (Feather Icons 风格，`currentColor`)
- **通信**：XMLHttpRequest (XHR shim，因模板模式不加载 `cbi.js`)

### 页面布局

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
│  │ ●  My PC                     ⚡ ⏻ ⋮ │  │   │  ← 设备卡片
│  │    192.168.1.100                   │  │   │     (宽屏2列)
│  └────────────────────────────────────┘ ─┘   │
│  ┌────────────────────────────────────┐      │
│  │ ●  Office Laptop            ⚡ ⏻ ⋮ │      │
│  │    192.168.1.101                   │      │
│  └────────────────────────────────────┘      │
└──────────────────────────────────────────────┘
```

⋮ 按钮展开菜单：上移 / 下移 / 删除。

### UI 设计原则

- **Material Design** 风格：卡片阴影、8px 圆角、状态过渡动画
- **响应式**：默认单列，≥700px 双列网格；≤520px 表单单列 + 按钮换行
- **折叠表单**：默认隐藏，点击标题展开/收起，与卡片列表分离
- **表单内嵌 Agent 说明**：表单顶部折叠区，展开后展示下载按钮 + nssm 安装命令
- **低频操作收合**：上移/下移/删除收入 ⋮ 菜单，点击展开下拉面板
- **彩色状态点**：🟢 在线 / ⚪ 离线 / 🟠 开机中闪烁 / 🔴 关机中闪烁
- **彩色图标按钮**：唤醒绿色(#388e3c)、关机红色(#c62828)、菜单灰色(#9e9e9e)
- **暗色模式**：CSS 自定义属性 + `@media (prefers-color-scheme: dark)` 自动适配
- **移动端后台恢复**：`visibilitychange` + `pageshow` + `focus` 三重事件监听
- **Luci 主题色**：下载按钮等交互元素使用 `var(--primary)` 自动跟随主题
- **无外部依赖**：所有 CSS 内联，图标使用内联 SVG

### 图标系统

五枚 Feather Icons 风格内联 SVG，`viewBox="0 0 24 24"`，`currentColor` 自动匹配按钮颜色：

| 图标 | 用途 | SVG |
|------|------|-----|
| 唤醒 | 闪电形状 | `<polygon points="13 2 3 14 12 14 11 22 21 10 12 10 13 2"/>` |
| 关机 | 电源符号 | 圆弧路径 + 竖线 |
| 删除 | X 形交叉线 | 两条交叉 line |
| Windows 徽标 | Agent 安装入口 | 四格 `<rect>` |
| 菜单 | 展开更多操作 | 三点纵排 `<circle>` x3 |

### 设备排序机制

每个设备有 `order` 整数字段，存储在 UCI 配置中。

- **添加**：自动分配 `max(existing_orders) + 1`
- **上移**：与 order 值相邻的上一设备交换 order
- **下移**：与 order 值相邻的下一设备交换 order
- **前端**：`swapCards()` 直接交换 DOM 节点，立即生效无需刷新
- **迁移**：首次加载时检测旧设备（order=0），按当前列表顺序分配 1, 2, 3...

### 状态检测与轮询

**轮询策略**：

| 场景 | 间隔 | 行为 |
|------|------|------|
| 页面空闲 | 30s | 全量轮询所有设备 |
| 点击唤醒后 | 5s | 快速轮询该设备，最多 60s |
| 点击关机后 | 5s | 快速轮询该设备，最多 60s |
| 等待期间 | — | 状态保持闪烁，不降级 |

**pollOne 保护规则**：

| 状态 | agent 在线 | agent 离线 |
|------|-----------|-----------|
| `g_waking` | 变绿 + clearWaking | **忽略**（保持橙闪） |
| `g_shutting` | **忽略**（保持红闪） | 变灰 + clearShutting |
| 空闲 | 变绿 | 变灰 |

### Agent 下载入口

Agent 安装入口位于「添加设备」表单顶部，默认折叠。

- **下载链接**：GitHub raw URL (`raw.githubusercontent.com/.../wol-agent.exe`)，不内置在 ipk 中
- **安装说明**：nssm 命令以浅色代码块展示
- **设计理由**：「先装 Agent 再加设备」的心智模型，且在表单顶部自然可见

### 暗色模式

通过 CSS 自定义属性实现，13 个变量覆盖所有组件：

```css
.md-app {
    --md-bg-card: #fff;           /* → dark: #1e1e1e */
    --md-bg-card-alt: #fafafa;    /* → dark: #252525 */
    --md-bg-input: #fff;          /* → dark: #2a2a2a */
    --md-text-primary: #212121;   /* → dark: #e0e0e0 */
    --md-text-secondary: #757575; /* → dark: #9e9e9e */
    --md-border: #e0e0e0;         /* → dark: #424242 */
    /* ... 等 */
}
@media (prefers-color-scheme: dark) {
    .md-app { /* 覆盖为暗色值 */ }
}
```

语义色（唤醒绿 #388e3c、关机红 #c62828、状态圆点色）保留不变，双模式均适用。

### UCI 配置格式 (`/etc/config/wolplus`)

```
config macclient '<uuid>'
    option name     'My PC'
    option macaddr  'AA:BB:CC:DD:EE:FF'
    option maceth   'br-lan'
    option ipaddr   '192.168.1.100'
    option order    '1'
```

---

## Windows Agent 设计

### 技术选型

- **语言**：Go 1.21+
- **依赖**：仅标准库 (`net/http`, `encoding/json`, `os/exec`)
- **二进制大小**：~6MB (UPX 压缩 ~1.5MB)
- **内存占用**：~3-5MB
- **部署**：nssm 注册为 Windows Service，开机自启

### API 规范

```
GET  /api/v1/status
  → 200 {"online":true,"hostname":"DESKTOP-ABC","os":"windows","uptime":3600}

POST /api/v1/shutdown
  → 200 {"success":true,"action":"shutdown","delay":5,"message":"System will shutdown in 5 seconds"}
  → 执行 shutdown /s /t 5（5 秒延迟，用户可取消）
```

### 构建与安装

```powershell
cd wol-agent
go build -ldflags="-s -w" -o wol-agent.exe
# 管理员运行 install.bat
```

`install.bat` 自动完成 nssm 服务注册、开机自启、立即启动。

---

## EasyTier 集成

### 集成概览

EasyTier Android App 通过两种方式与 luci-app-wolplus 交互：

| 操作 | 协议 | 路由 |
|------|------|------|
| **唤醒** | HTTP GET → 路由器 CGI | `http://<router_ip>/cgi-bin/wolplus-api?action=wake&mac=...&iface=...` |
| **状态** | HTTP GET → Agent 直连 | `http://<pc_ip>:<agent_port>/api/v1/status` (通过 SOCKS5 代理) |
| **关机** | HTTP POST → Agent 直连 | `http://<pc_ip>:<agent_port>/api/v1/shutdown` (通过 SOCKS5 代理) |

### CGI 端点 (`/cgi-bin/wolplus-api`)

独立 Lua CGI 脚本 (`root/www/cgi-bin/wolplus-api`)，**不经 Luci 鉴权**，由 uhttpd 直接执行。假设 EasyTier 网络可信。

支持的 action：

| action | 参数 | 功能 |
|--------|------|------|
| `wake` | `mac`, `iface` | 执行 etherwake 发送 Magic Packet |
| `status` | `ip`, `port` | 代理查询 Agent 状态 |
| `shutdown` | `ip`, `port` | 代理发送 Agent 关机指令 |

> 该 CGI 脚本部署在 OpenWrt 的 `/www/cgi-bin/wolplus-api`，通过 `root/` 目录随 ipk 安装。

### EasyTier 连接模式

| 模式 | 检测方式 | 代理 |
|------|---------|------|
| **ET 模式** (通过 EasyTier 隧道) | HTTP GET 路由器 CGI | SOCKS5 (`easytier-core socks5`) |
| **LAN 模式** (同局域网) | TCP ping 路由器 80 端口 | 直连 |

### 设备配置格式 (EasyTier TOML)

```toml
[[device]]
name = "3070"
mac = "2C:F0:5D:CE:4D:23"
ip = "192.168.2.2"
interface = "br-lan"
router_ip = "192.168.2.1"
agent_port = 32249
```

### EasyTier 轮询策略

- **全量轮询**：30 秒间隔，并行请求所有设备的 Agent status 端点
- **唤醒后追踪**：5 秒间隔 × 12 次（60 秒），检测到在线后发 Snackbar 通知
- **关机后追踪**：5 秒间隔 × 12 次（60 秒），检测到离线后发 Snackbar 通知
- **路由器离线保护**：路由器不可达时所有设备显示"路由器离线"，不执行操作

### EasyTier App 架构要点

- 基于 Tauri v2 (Rust + Vue 3)
- WOL 请求通过 `reqwest` + SOCKS5 代理（`easytier-core` 提供）
- VPN 排除自身流量（`disallowedApplications`），避免 HTTP 请求被自己路由
- 设备配置持久化于 `localStorage.wolDevicesToml`

---

## 设计决策记录

### 为什么用 Template 而非 CBI

CBI 的 `tblsection` 表格渲染无法满足 Material Design 卡片布局需求。

- **优点**：完全控制 HTML/CSS/JS，支持卡片布局、折叠表单、自定义交互
- **代价**：失去 CBI 自动表单处理、需手动实现 XHR 封装、需显式引入 header/footer
- **数据层**：直接通过 UCI cursor 读写配置，add/delete 通过 controller 端点实现

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
- 默认折叠，不影响表单简洁性
- 使用 GitHub raw URL 而非内置 exe，保持 ipk 轻量（~30KB）
- Agent 更新无需更新 Luci 包，推送 GitHub 即可

### 为什么设备排序用 ⋮ 菜单而非拖拽

- 拖拽在移动端实现复杂（双列网格下路径计算困难）
- ⋮ 菜单最小 UI 侵入，低频操作不外露
- 上下箭头在所有设备上均可点击

---

## 测试与验证

### Agent 测试

```bash
# 在线状态
curl -s -m 2 http://<PC_IP>:32249/api/v1/status

# 远程关机
curl -s -m 5 -X POST http://<PC_IP>:32249/api/v1/shutdown
```

### Luci 接口测试

```bash
# 获取 token 后测试
TOKEN=$(curl -s -d '{"method":"login","params":["root","<PASS>"]}' \
  http://<ROUTER_IP>/cgi-bin/luci/rpc/auth | jq -r '.result')

# 状态检测
curl -s -d "token=$TOKEN" \
  http://<ROUTER_IP>/cgi-bin/luci/admin/services/wolplus/status/<section>
```

### 前端检查

1. 打开 F12 控制台，确认无 `ReferenceError` 或 403 错误
2. 状态圆点是否每 30 秒更新
3. 唤醒后是否进入 5 秒快速轮询（橙点闪烁）
4. 关机后是否有确认弹窗、1 分钟内红点保持闪烁
5. 暗色模式下卡片和表单是否正确适配
6. 移动端切后台再切回是否立即全量检测
