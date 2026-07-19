# AGENTS.md — Ackwrap 工程规则

## 快速开始

**后端验证：**
```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

**前端验证：**
```bash
cd frontend
npm run build
```

**开发模式：**
- 前端：`cd frontend && npm run dev` (端口 5173，API 自动代理到 :8080)
- 后端：设置 `ACKWRAP_LISTEN_ADDR=127.0.0.1:8080` 后运行 `cd backend && go run ./cmd/server`（远程监听则必须同时设置 `ACKWRAP_API_TOKEN`）

**构建：**
- 前端构建输出到 `backend/internal/webui/dist/`，通过 `go:embed` 打包进后端二进制
- `backend/internal/webui/dist/` 是本地生成目录，除嵌入占位文件外禁止提交其中的 hashed JS/CSS、HTML、图片或其他编译产物
- 发布构建统一从仓库根目录运行 `python build.py`，默认生成 Windows、Linux、OpenWrt amd64 单文件产物；OpenWrt 目标同时生成包含核心、LuCI 和 iStoreOS app-meta 的单一 IPK
- 后端入口：`backend/cmd/server/main.go`

---

## 最重要的原则

每一步都必须满足：
1. 这个功能能不能单独跑通？
2. 这个功能失败时前端能不能知道原因？
3. 这个功能有没有日志？
4. 这个功能有没有最小验证命令？

如果不能，不要进入下一步。

## 审核与提交规则

- 每轮功能审核或自审完成后，必须先修复全部确认问题并执行对应验证；审核无剩余问题时，立即提交本轮全部预期改动，避免下一轮重复审核已经确认的差异。
- 提交前必须检查 `git status`、`git diff`、最近提交和子模块状态，只暂存本轮预期文件；用户或其他代理的无关改动保持未暂存，不得顺带提交、还原或删除。
- 审核未通过、验证失败或仍有明确阻塞时禁止提交；修复后重新审核和验证，再创建新提交。
- 禁止提交 `backend/internal/webui/dist/` 中由 Vite 生成的 hashed JS/CSS 等构建产物；发布包必须通过 `npm run build` 或根目录 `build.py` 在本地重新生成并嵌入。

## devel 与子模块保护规则

- 主仓库 `devel` 是功能开发与分支合并交界，`sing-box-wrap/devel` 同时承接 `sync -> devel -> main`。合并、拉取、rebase、切换分支或更新子模块都可能移动子模块工作树并覆盖未提交文件。
- 执行上述操作前必须先运行 `git -C sing-box-wrap status --short` 和 `git diff --submodule=log -- sing-box-wrap`；子模块非干净状态时禁止直接执行更新。
- 子模块存在功能修改时，必须先在 `sing-box-wrap` 内完成验证、提交并推送，再回主仓库提交新的子模块指针；禁止只提交主仓库映射而不提交对应核心实现。
- 禁止对脏子模块执行 `git submodule update --force`、reset、clean 或会隐式切换子模块提交的脚本。确需更新时先提交，或在获得用户明确同意后建立可恢复的 stash。
- 合并或更新完成后必须再次核对 `git -C sing-box-wrap rev-parse HEAD`、子模块状态和关键新增文件，确认工作树没有被远端 `devel` 覆盖。

## 文档查询规则

遇到不确定的库、框架、SDK、CLI、配置字段或协议字段问题时，必须先通过 Context7 查询对应官方文档，再决定实现或映射方式。尤其是 sing-box / Mihomo / Vue / Vite / Gin / SQLite 等第三方 API 和配置 schema，不允许只凭经验或错误信息直接判断。

## 协议支持验证规则

**遇到 sing-box 配置校验失败 `unknown outbound type: XXX` 时，必须：**

1. **查询 sing-box 源码确认协议是否支持：**
   ```bash
   # 在 sing-box 目录搜索协议定义
   grep -r "TypeXXX\|\"xxx\"" sing-box/constant/
   grep -r "XXXOutboundOptions" sing-box/option/
   ```

2. **区分官方协议与第三方协议：**
   - **官方协议**（`shadowsocks/vmess/vless/trojan/hysteria/tuic/wireguard` 等）：预编译版本支持
   - **第三方协议**（`anytls/shadowtls/naive` 等）：需要自行编译包含，预编译版本**不支持**

3. **第三方协议处理方案：**
   - 在 `config_generator.go` 的 `unsupportedTypes` 中添加该协议
   - 配置生成时自动跳过，不报错
   - 日志记录跳过原因：`节点 %s 使用第三方协议 %s，当前 sing-box 版本不支持`

4. **验证方式：**
   ```bash
   # 检查已安装的 sing-box 是否支持某协议
   sing-box version
   # 查看官方文档确认协议支持情况
   ```

5. **禁止盲目判断：**
   - ❌ 错误：看到 `unknown outbound type` 就认为不支持
   - ✅ 正确：查看 sing-box 源码 `constant/proxy.go` 和 `option/*.go` 确认

## 安全规则

**严禁上传节点敏感信息到聊天远端：**
- 禁止在日志、回复、或任何输出中包含完整的节点连接信息
- 禁止输出包含 `server/port/uuid/password/cipher/flow/reality/private_key` 等连接参数的完整节点数据
- 读取或操作节点数据时，只展示节点名称、协议类型、订阅来源等非敏感元数据
- 调试或日志输出时，敏感字段必须脱敏（如 `uuid: abc***`, `server: 1.2.***`）
- 节点数据库路径、配置文件路径可以提及，但不能读取并输出其中的敏感内容

---

## 技术栈

| 类型 | 选择 |
|------|------|
| 后端语言 | Go |
| HTTP 框架 | Gin |
| 数据库 | SQLite |
| SQLite Driver | modernc.org/sqlite |
| WebSocket | Gorilla WebSocket |
| 定时任务 | github.com/robfig/cron/v3 |
| 前端 | Vue 3 + TypeScript + Vite + Tailwind CSS 4 + DaisyUI |
| 配置格式 | JSON |
| 日志 | internal/logging 包装 log.Printf |
| API 格式 | REST + JSON |
| 实时事件 | WebSocket |

## 暂时不引入

- GORM
- 复杂 DI 框架
- Kafka / Redis
- 多数据库支持
- 复杂权限系统
- 复杂任务队列

---

## 目录结构

```text
backend/cmd/server/
  main.go

backend/internal/api/
  router.go

backend/internal/handler/
  runtime.go
  core.go
  installer.go
  settings.go
  realtime.go
  subscription.go
  node.go
  route_rule.go

backend/internal/service/
  singbox.go
  installer.go
  config.go
  runtime.go
  settings.go
  realtime.go
  subscription.go
  node.go
  route_rule.go
  version.go

backend/internal/store/
  sqlite.go
  migrations.go
  install_state.go
  settings.go
  subscription.go
  node.go
  node_filter.go
  route_rule.go

backend/internal/model/
  api.go
  runtime.go
  installer.go
  settings.go
  realtime.go
  subscription.go
  node.go
  route_rule.go

backend/internal/parser/
  subscription.go
  singbox.go
  clash.go
  uri_list.go
  proxy_uri.go
  transport.go
  vmess.go
  vless.go
  trojan.go
  ss.go
  ssr.go
  socks.go
  http.go
  hysteria.go
  tuic.go
  anytls.go
  wireguard.go
  naive.go
  mieru.go
  snell.go

backend/internal/paths/
  paths.go

backend/internal/logging/
  logging.go

frontend/src/
  pages/
  services/
  components/
```

## 分层职责

| 层 | 职责 |
|----|------|
| handler | 解析请求、调用 service、返回统一 JSON 响应 |
| service | 业务逻辑、编排、日志、状态流转；不直接返回 Gin response |
| store | 数据库读写，SQL 集中在这里 |
| model | 纯数据结构，无业务逻辑 |
| parser | 订阅内容和节点协议解析，不写库 |
| api/router.go | 路由注册 |
| cmd/server/main.go | 依赖组装和启动入口 |

## 源码拆分规则

Go 和 TypeScript 都必须尽量按功能或业务拆分源码，便于后期维护。

规则：
- 后端按 `handler/service/store/model/parser` 分层后，再按业务文件拆分，例如 `subscription.go`、`node.go`、`node_filter.go`
- 前端按页面、服务、组件拆分；页面过大时继续按业务区块拆子组件
- 订阅、节点、设置、配置、运行时等业务不要混在同一个大文件里
- 协议解析器必须按协议拆文件，不能把所有协议堆在一个 parser 文件中
- API client/types 可以按业务增长拆分，避免长期堆在单个巨型文件
- 新增功能时优先放到已有对应业务文件；没有对应业务文件时新增清晰命名的文件
- 拆分是为了降低耦合和便于维护，不为了形式化拆出无意义的一行函数文件
- 公共 helper 只有在 2 个以上业务点复用时才抽到公共文件

## 禁止

- handler 里直接写数据库
- handler 里直接执行 sing-box
- service 里直接返回 Gin response
- parser 里写数据库或调用 HTTP
- 到处散落路径拼接逻辑
- 订阅同步解析出 0 个节点时清空旧节点

---

## 路径管理

所有路径由 `internal/paths.Paths` 提供。

默认路径：
- Windows: `%USERPROFILE%\ackwrap`
- Linux: `/etc/ackwrap`
- macOS: `~/ackwrap`

环境变量覆盖：
- `ACKWRAP_DATA_DIR`
- `ACKWRAP_BINARY_DIR`

配置路径规则：
- 配置目录是 `<data>/config/`
- 规则订阅缓存目录是 `<data>/rules/`
- Geo 数据库目录是 `<data>/geo/`
- `ActiveConfigPath()` 扫描 `.json` 配置
- 优先使用 `config.json`
- 旧版 `<data>/config.json` 需要迁移到配置目录

---

## API 规则

统一前缀：`/api/v1`

错误格式固定，code 用字符串：

```json
{
  "error": {
    "code": "CONFIG_INVALID",
    "message": "配置校验失败",
    "details": {}
  }
}
```

成功响应直接返回资源。

动作类接口返回：

```json
{
  "success": true,
  "message": "service started"
}
```

## 当前 API

```text
Runtime:
  GET  /api/v1/runtime

Installer:
  GET  /api/v1/installer/sing-box
  POST /api/v1/installer/sing-box/install

Config:
  GET  /api/v1/config/status
  POST /api/v1/config/default
  POST /api/v1/config/validate
  POST /api/v1/config/rules/update
  POST /api/v1/config/backup
  POST /api/v1/config/restore

Core:
  POST /api/v1/core/start
  POST /api/v1/core/stop
  POST /api/v1/core/restart
  POST /api/v1/core/reload-config

Settings:
  GET    /api/v1/settings/update
  PUT    /api/v1/settings/update
  GET    /api/v1/settings/node-filters
  POST   /api/v1/settings/node-filters
  PUT    /api/v1/settings/node-filters/:id
  DELETE /api/v1/settings/node-filters/:id

Subscriptions:
  GET    /api/v1/subscriptions
  GET    /api/v1/subscriptions/user-agents
  POST   /api/v1/subscriptions
  PUT    /api/v1/subscriptions/:id
  DELETE /api/v1/subscriptions/:id
  POST   /api/v1/subscriptions/:id/sync
  POST   /api/v1/subscriptions/sync

Nodes:
  GET  /api/v1/nodes/facets
  GET  /api/v1/nodes
  POST /api/v1/nodes/import/preview
  POST /api/v1/nodes/import
  POST /api/v1/nodes/tcping
  POST /api/v1/nodes/add-emoji
  POST /api/v1/nodes/batch-rename
  PUT  /api/v1/nodes/:uid/enabled
  PUT  /api/v1/nodes/:uid/preferred

Route Rules:
  GET    /api/v1/rules
  POST   /api/v1/rules
  GET    /api/v1/rules/subscriptions
  POST   /api/v1/rules/subscriptions
  POST   /api/v1/rules/subscriptions/sync
  GET    /api/v1/rules/subscriptions/:id/content
  POST   /api/v1/rules/subscriptions/:id/sync
  PUT    /api/v1/rules/subscriptions/:id
  DELETE /api/v1/rules/subscriptions/:id
  GET    /api/v1/rules/geo
  POST   /api/v1/rules/geo/sync
  PUT    /api/v1/rules/geo/:id
  POST   /api/v1/rules/geo/:id/sync
  PUT    /api/v1/rules/:id
  DELETE /api/v1/rules/:id
  POST   /api/v1/rules/reorder
  GET    /api/v1/rules/preview

Realtime:
  GET /api/v1/realtime/ws
```

---

## WebSocket 规则

唯一通道：`/api/v1/realtime/ws`

事件格式：

```json
{
  "type": "installer.progress",
  "time": 1710000000000,
  "data": {}
}
```

规则：
- `type` 使用 `模块.事件` 命名
- `time` 是毫秒时间戳
- `data` 是固定结构 payload
- REST 触发动作，过程和最终状态通过 WS 推送

当前事件：
- `runtime.status`
- `installer.status`
- `installer.progress`
- `core.status`
- `core.log`
- `config.status`
- `subscription.sync`

订阅同步失败时，`subscription.sync` 必须带 `error` 字段，前端要能展示失败原因。

---

## SQLite 规则

数据库位置：`<ACKWRAP_DATA_DIR>/ackwrap.db`

当前表：
- `app_settings`
- `install_state`
- `subscriptions`
- `nodes`
- `node_filters`
- `route_rules`
- `route_rule_subscriptions`
- `geo_assets`

迁移规则：
- 所有 DDL 写在 `internal/store/migrations.go`
- 新字段用 `ALTER TABLE ... ADD COLUMN ...`
- 重复列错误通过 `isDuplicateColumnMigration()` 白名单忽略
- 不引入复杂 migration 框架

## 订阅规则

订阅支持：
- Clash YAML `proxies`
- sing-box JSON `outbounds`
- base64 URI list
- plain URI list

同步流程：
1. 使用订阅保存的 `user_agent` 和 `sync_timeout_seconds` 拉取远端内容
2. 解析节点
3. 如果解析结果为 0，返回失败并保留旧节点
4. 应用启用的节点过滤规则
5. 如果过滤后为 0，返回失败并保留旧节点
6. `ReplaceSubscriptionNodes()` 按 UID 全量替换节点
7. 更新订阅流量、到期时间、节点数、最后同步时间
8. WS 推送 `subscription.sync`

定时同步：
- 使用 `robfig/cron/v3`
- 必须使用 `cron.WithSeconds()`，因为当前 cron 表达式是 6 段
- `daily` 根据 `sync_time` 触发
- `weekly` 根据 `sync_time` + `sync_weekday` 触发
- Create 后立即异步同步一次
- URL 变更后立即异步同步一次
- Create/Update/Delete 必须同步维护对应 cron job

## 节点规则

节点必须有稳定 UID。

UID 生成规则：
- 使用核心连接字段白名单生成 SHA-256 短哈希
- 参与字段包括 `type/server/port/uuid/password/cipher/flow/tls/transport/reality` 等连接参数
- 不参与字段包括 `name/tag/raw/raw_json/latency/status/id/created_at/updated_at`
- 节点改名不应导致 UID 改变

同步替换节点时，必须按 UID 继承：
- `enabled`
- `preferred`
- `latency_ms`
- `status`
- 自定义名称 `name_overridden`

节点改名或添加 emoji 后必须设置 `name_overridden=1`，后续订阅同步不能覆盖自定义名称。

节点删除不作为第一选择。订阅节点建议用 `enabled=false` 禁用，避免下次订阅同步又恢复。

手动导入节点规则：
- 手动导入入口在订阅管理页下方，不作为顶层页面
- 后端使用内部订阅源 `manual://local` / `手动导入` 保存手动节点
- 订阅列表前端不展示 `manual://local`
- `SyncAll()` 必须跳过 `manual://local`
- 手动导入支持 URI list、base64 URI list、Clash YAML、sing-box JSON
- 手动导入预览必须走后端解析与过滤规则，不能只靠前端猜格式
- 手动导入是按 UID 追加/更新，不清空旧手动节点
- 手动导入同样应用启用的节点过滤规则

## 节点过滤规则

过滤规则存 `node_filters` 表。

字段：
- `name`
- `target`: `all/name/type/server/raw/raw_json`
- `pattern`: Go 正则表达式
- `enabled`

规则：
- 后端必须校验正则表达式
- 过滤必须发生在订阅解析后、写入数据库前
- 过滤不是前端过滤
- 过滤导致 0 节点时必须同步失败并保留旧节点

## 节点测速规则

当前节点测速按协议分流：TCP 类协议使用 TCPing，UDP/QUIC 类协议复用 sing-box Clash API 真实出站测速。

行为：
- `hysteria/hysteria2/tuic/wireguard` 必须使用 `/proxies/:tag/delay`，不能用 TCPing 或无响应语义的 UDP Dial
- UDP/QUIC 节点必须已载入活动配置；未载入时返回明确错误，不回退 TCPing
- 其他协议使用 `net.Dialer{Timeout: 5s}` 连接 `server:server_port`
- TCPing 必须使用 `net.JoinHostPort()`，兼容 IPv6
- 成功写 `latency_ms` 和 `status=available`
- 失败写 `latency_ms=0` 和 `status=unavailable`

TCPing 不验证：
- 协议握手
- UUID/password
- TLS/Reality 参数
- 代理外网访问
- 下载速度

## 规则管理规则

规则管理对应 sing-box `route.rules` 和 `route.rule_set`，与节点过滤规则不同。

当前手动规则存 `route_rules` 表。

字段：
- `name`
- `enabled`
- `priority`
- `rule_type`: `domain/domain_suffix/domain_keyword/ip_cidr/geoip/geosite/rule_set`
- `values_json`: 多行匹配值 JSON 数组
- `outbound`: `proxy/direct/block`
- `invert`

规则订阅存 `route_rule_subscriptions` 表。

字段：
- `name`
- `enabled`
- `tag`
- `url`
- `format`: `auto/binary/source/clash`，入库后 `auto` 会按 URL 后缀识别成具体格式
- `use_proxy`
- `sync_mode`: `off/daily/weekly`
- `sync_time`
- `sync_weekday`
- `sync_status`
- `sync_error`
- `last_sync_at`
- `cached_path`
- `cached_updated_at`

Geo 数据库存 `geo_assets` 表。

字段：
- `name`
- `type`: `geoip/geosite`
- `url`
- `use_proxy`
- `sync_mode`: `off/daily/weekly`
- `sync_time`
- `sync_weekday`
- `sync_status`
- `sync_error`
- `last_sync_at`
- `local_path`
- `cached_updated_at`

规则：
- 第一版先做 CRUD、启用/停用、排序、JSON 预览
- `GET /rules/preview` 返回已启用规则转成的 sing-box `route.rules`，以及已启用规则订阅转成的 `route.rule_set`
- 规则订阅保存后会异步同步一次，URL/格式/代理设置变化后也会异步同步一次
- 规则订阅支持手动同步单个/全部，并支持 `daily/weekly` 定时同步；必须用 `cron.WithSeconds()`
- 规则订阅内容缓存到 `<data>/rules/`，`/rules/subscriptions/:id/content` 优先返回本地缓存，无缓存时才拉取上游并写缓存
- 规则订阅 URL 为 `.yml/.yaml` 时自动按 Clash rule-provider YAML 处理，预览生成 sing-box `format=source`，`url` 指向后端转换接口 `/rules/subscriptions/:id/content`
- Clash YAML 转换支持 `payload`/`rules`，支持 classical 行如 `DOMAIN-SUFFIX,example.com`，也支持无 `behavior` 的纯域名或纯 CIDR payload
- 预览中的 remote rule_set `url` 统一指向本机后端转换/缓存接口；sing-box 1.14+ 不生成已弃用的 `download_detour`，本机请求使用默认 HTTP transport
- 后端拉取上游规则订阅时按 `use_proxy` 决定是否走本地代理
- Geo 数据库默认包含 `geoip.db` 和 `geosite.db` 两项，下载到 `<data>/geo/`，支持手动同步单个/全部和 `daily/weekly` 定时同步
- 还未确认出站策略生成前，不直接写入正式 sing-box 配置
- 后续接入配置生成时，必须写临时文件并执行 `sing-box check` 后才能覆盖正式配置

---

## sing-box 管理规则

配置生成规则：
- 默认配置必须先写临时文件
- 必须执行 `sing-box check -c config.tmp.json`
- 校验通过后才覆盖正式配置
- 第一版默认配置目标是能启动、能校验、能开本地 mixed 代理
- 默认配置必须在用户规则前生成 Ackwrap/sing-box 进程和全部启用节点服务器的 direct 白名单，启用 `route.find_process`，避免 TUN/全局模式代理回环
- 节点域名使用 `domain`，IPv4/IPv6 使用 `/32`、`/128` 的 `ip_cidr`，不得把节点敏感地址写入日志

runtime 状态机：

```text
not_installed → no_config → stopped → running
```

---

## 日志规则

所有关键动作必须打日志：
- `runtime.check`
- `installer.start`
- `installer.download`
- `installer.extract`
- `config.generate`
- `config.validate`
- `core.start`
- `core.stop`
- `core.restart`
- `settings.update`
- `subscription.create`
- `subscription.update`
- `subscription.delete`
- `subscription.sync`
- `subscription.scheduler`
- `node.list`
- `node.facets`
- `node.tcping`
- `node.enabled`
- `node.preferred`
- `route_rule.list`
- `route_rule.create`
- `route_rule.update`
- `route_rule.delete`
- `route_rule.reorder`
- `route_rule.preview`
- `route_rule_subscription.list`
- `route_rule_subscription.create`
- `route_rule_subscription.update`
- `route_rule_subscription.delete`
- `route_rule_subscription.convert`
- `route_rule_subscription.sync`
- `route_rule_subscription.scheduler`
- `geo.list`
- `geo.update`
- `geo.sync`
- `geo.scheduler`
- `websocket.connect`
- `websocket.broadcast`

---

## 前端规则

- 前端构建输出到 `backend/internal/webui/dist/` 并由后端嵌入
- Go 后端负责静态资源和 SPA fallback
- 视觉风格遵循当前暗色蓝灰主题，不照搬参考项目的橙色选中样式
- 新增或调整 UI 时，优先复用全局组件、全局 CSS class 和设计 token；不要在单个页面里重复硬编码大段颜色、弹窗、表格、按钮样式
- 弹窗、表单控件、表格、Toast、确认框等必须优先使用全局组件或全局样式；禁止在页面内写死 `bg-[#152235]`、`text-white`、`bg-white/[0.04]` 等只适配暗色主题的颜色类，必须使用 `var(--bg-*)`、`var(--text-*)`、`var(--border-*)` 等 token，确保白天/夜间模式一致可读
- 编辑/新增表单优先使用弹窗展示，不在页面中部展开大段内嵌编辑区，避免页面跳动、滚动位置丢失和列表上下文被打断；确有必要内嵌时必须有明确理由
- 通用样式应沉淀到 `frontend/src/styles/global.css`、`tokens.css` 或公共组件中，例如弹窗、数据表格、筛选 chip、分页等
- 前端单个源码文件超过 800 行时必须拆分到同目录业务组件、公共组件或工具文件中，不能继续堆在单个页面文件里
- 节点页协议/订阅筛选统计必须来自后端 `/nodes/facets`，不能用当前过滤后的列表推断
- 失败信息必须展示给用户，尤其是订阅同步失败原因

---

## 文档维护规则

**每次修改业务功能都必须在代码审核通过后同步更新 docs/ 目录下的相关文档。**

文档更新时序：
1. 先完成业务代码和测试
2. 执行功能审核或自审
3. 修复全部确认问题并完成对应验证
4. 审核无剩余问题后，再根据最终实现更新文档

禁止在审核前提前修改功能文档，避免审核修复导致文档与最终实现不一致。纯文档任务不受此时序限制。

### 文档更新对照表

| 修改类型 | 需要更新的文档 |
|---------|---------------|
| 新增功能模块 | `05-功能模块.md`, `08-已完成功能.md`, `04-API接口.md`（如有新接口） |
| 修改现有功能 | `05-功能模块.md`, 对应功能说明章节 |
| 新增 API 接口 | `04-API接口.md` |
| 修改 API 接口 | `04-API接口.md` |
| 新增数据库表/字段 | `03-数据库设计.md` |
| 修改数据库结构 | `03-数据库设计.md` |
| 新增开发规范 | `06-开发规范.md` |
| 新增安全规则 | `07-安全规范.md` |
| 完成待开发功能 | `08-已完成功能.md`, `09-待开发功能.md`, `12-项目里程碑.md` |
| 变更技术架构 | `02-技术架构.md` |
| 变更部署方案 | `10-部署方案.md` |
| 新增测试用例 | `11-测试计划.md` |

### 文档更新要求

1. **代码审核通过并完成问题修复验证后立即更新文档**，不要在审核前提前更新
2. **文档描述必须准确**，与实际实现保持一致
3. **API 接口变更必须更新请求/响应示例**
4. **数据库变更必须更新字段说明和示例**
5. **删除功能时同步删除文档中的相关章节**
6. **重大变更需要在变更日志中记录**

### 检查清单

完成功能开发后，检查：

- [ ] 功能说明文档已更新
- [ ] API 文档已更新（如有接口变更）
- [ ] 数据库文档已更新（如有表结构变更）
- [ ] 已完成功能列表已更新
- [ ] 待开发功能列表已更新（如完成了计划功能）
- [ ] 相关示例代码已更新

---

## Lint & 验证

后端每次模块改动必须跑：

```bash
cd backend
go build ./...
go test ./...
go vet ./...
```

前端改动必须跑：

```bash
cd frontend
npm run build
```

如果只改纯文档，可以不跑编译，但最终回复要说明未运行验证的原因。
