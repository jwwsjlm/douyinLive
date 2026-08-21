# DouyinLive HTTP API

v2.2.0 在现有 WebSocket 端口上提供只读 HTTP API。查询未连接的直播间时，服务会执行一次 HTTP 状态探测，不会创建长期 WebSocket 房间。

## 认证

默认不强制认证，适合本机兼容调用。配置 `api.key` 或环境变量 `APP_API_KEY` 后，所有 `/api/v1/*` 请求必须携带：

```text
Authorization: Bearer <API_KEY>
```

API Key 不支持通过 URL 参数传递，也不会写入日志。

> **公网部署提醒**：服务不强制要求 API Key，是否公网暴露由使用者自行决定。若监听地址可被公网访问，建议配置 `api.key` 或 `APP_API_KEY`，并结合防火墙、反向代理或来源访问控制。未配置 API Key 时，`/api/v1/*` 支持匿名访问。

服务本身不实现按 IP、QPS 或客户端身份的访问限速。公网部署如需限速、黑白名单或连接数策略，请在 Nginx、Caddy、Traefik、网关或防火墙层配置，避免把部署策略固化在程序内。

## 接口

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/api/v1/health` | 服务和构建信息 |
| GET | `/api/v1/capabilities` | 能力和消息类型 |
| GET | `/api/v1/rooms` | 当前已建立/监控中的房间，不触发上游请求 |
| GET | `/api/v1/rooms/{live_id}` | 查询直播间状态、标题和房间标识 |
| GET | `/api/v1/rooms/{live_id}/status` | 只返回状态的快捷查询 |
| POST | `/api/v1/rooms/status:batch` | 批量查询多个直播间状态，最多 50 个 |
| GET | `/api/v1/rooms/resolve?url=...` | 解析 douyin.com 直播间 URL，不访问传入 URL |

运维接口：

| 方法 | 路径 | 说明 |
| --- | --- | --- |
| GET | `/health` | 基础存活检查 |
| GET | `/metrics` | Prometheus 运行指标（配置 `api.key` 后同样需要 Bearer Token） |

状态值：`online`、`offline`、`account_no_room`、`unknown`、`not_found`。

批量接口的汇总字段包含 `online`、`offline`、`account_no_room`、`not_found` 和 `unknown` 五类计数。

`online`、`offline`、`account_no_room` 返回 HTTP 200；明确不存在返回 404；超时、风控页或无法验证返回 503；参数错误返回 400。

所有响应都包含 `request_id`，同时写入响应头 `X-Request-ID`。错误响应不会包含 Cookie、`msToken`、`a_bogus`、WebSocket `signature` 或完整上游 URL。

认证失败返回 `401` 和 `WWW-Authenticate`；方法不支持时返回 `405` 并带 `Allow`。批量请求严格要求单个 JSON 对象，最多 50 个直播间标识，请求体上限为 1 MiB，超过时返回 `413 request_too_large`。

`GET /api/v1/rooms` 的活动房间条目始终包含 `client_count`、`upstream_ready` 和 `status_unknown`，即使对应值为 `0` 或 `false`；一次性查询接口不会返回这些仅属于活动连接的字段。同一 `live_id` 因不同 Cookie 建立多个内部上游会话时，此接口仍按逻辑直播间聚合为一个条目，并汇总客户端数量，不暴露 Cookie 或 Cookie 摘要。

/health 与 /api/v1/health 使用相同的 JSON envelope；前者不要求 API Key，适合 Docker healthcheck。成功的 JSON 查询响应带有 ETag 和 Cache-Control: private, max-age=...，可使用 If-None-Match 获取 304 Not Modified。错误响应统一使用 Cache-Control: no-store；上游超时/不可验证的 503 响应包含 Retry-After: 5。

/metrics 返回 Prometheus 文本格式（不是 JSON envelope），其中 douyinlive_http_requests_total 统计 HTTP API 请求数，douyinlive_http_request_duration_seconds_sum 和 douyinlive_http_request_duration_seconds_count 用于统计 HTTP 请求耗时。

配置 `api.key` 后，`/metrics` 也需要 `Authorization: Bearer <API_KEY>`；只有 `/health` 始终公开，便于 Docker healthcheck。

## 示例

```bash
curl http://127.0.0.1:1088/api/v1/health
curl http://127.0.0.1:1088/api/v1/rooms/516466932480
curl http://127.0.0.1:1088/api/v1/rooms/516466932480/status
curl -X POST -H 'Content-Type: application/json' \
  -d '{"live_ids":["516466932480","123456"]}' \
  http://127.0.0.1:1088/api/v1/rooms/status:batch
curl 'http://127.0.0.1:1088/api/v1/rooms/resolve?url=https%3A%2F%2Flive.douyin.com%2F516466932480'
curl -H 'Authorization: Bearer YOUR_API_KEY' http://127.0.0.1:1088/api/v1/rooms
```

URL 解析默认只允许 `douyin.com` 及其子域名。`api.allowed_domains` 只能填写 `douyin.com` 或其子域名，不能扩展到其他主域名；该字段用于收紧允许的抖音 Host 范围。仅接受 http/https、无 userinfo、无 query/fragment、无显式端口的 URL；解析接口只检查 URL 结构和域名，不会请求用户提交的 URL，避免形成 SSRF。

HTTP API 仅提供直播间查询，不提供独立主播查询、发弹幕、点赞、礼物、配置修改或远程房间控制。返回字段只描述直播间状态、标题和房间标识，不承诺提供独立主播资料。批量接口中的 `live_ids` 必须唯一，重复标识返回 `400 duplicate_live_id`。

## WebSocket 路由与认证

WebSocket 路由默认是 `/ws/{live_id}`，可通过 `websocket.path` 自定义，例如 `/live-stream/{live_id}`。如果配置了 `api.key` 或 `APP_API_KEY`，WebSocket 握手必须携带 `Authorization: Bearer <API_KEY>`；不支持通过 URL 查询参数传递 API Key。原生浏览器 WebSocket 无法自定义该请求头，浏览器场景应通过反向代理或使用支持自定义握手头的客户端。

可通过 `websocket.allowed_origins` 配置浏览器 Origin 白名单。留空时保持兼容，允许所有 Origin；公网部署时建议配置明确的来源。白名单中存在非法 Origin 时服务会拒绝启动，不会静默降级为允许所有来源。

房间查询默认复用服务配置中的 Cookie：如果 `cookie.use_stored: true`（默认值）且 `cookie.rooms` 存在对应直播间 ID，优先使用该房间 Cookie；否则使用 `cookie.douyin` 全局 Cookie；两者都为空时才使用匿名会话并按程序逻辑自动获取基础 Cookie。设置 `cookie.use_stored: false` 后，HTTP 查询会忽略配置文件中的预存 Cookie。HTTP 查询不会接受 URL 参数中的 Cookie，也不会把 Cookie 写入响应或日志。