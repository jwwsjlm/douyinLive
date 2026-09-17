# 配置文件

[返回项目首页](../README.md)

本文说明 `config.yaml`、环境变量、代理、Cookie、签名方式和配置优先级。

你可以创建一个 `config.yaml` 放在程序同目录下。

示例：

```yaml
port: "1088"
websocket:
  path: "/ws"
  allowed_origins: []
unknown: false
protocol:
  mode: web
log:
  level: "info"
sign:
  provider: ""
tikhub:
  key: ""
api:
  key: ""
  allowed_domains:
    - "douyin.com"
monitor:
  poll_interval: "15s"
  notify_interval: "30s"
proxy:
  url: ""
  rooms: {}
cookie:
  use_stored: true
  douyin: ""
  rooms:
    # "516466932480": "ttwid=...; sessionid=..."
```

项目里也自带了一个示例文件：

- `config.example.yaml`

## 配置项说明

### `port`
本地 WebSocket 服务端口。

默认值：

```yaml
port: "1088"
```

### `unknown`
是否打印未知消息类型。

默认值：

```yaml
unknown: false
```

### `protocol.mode`

选择连接上游时使用的协议画像：

- `web`：浏览器 Web 端画像，默认值，兼容性优先。
- `pc`：抖音桌面客户端画像，当前为测试功能，暂不保证稳定性。

```yaml
protocol:
  mode: web
```

也可以使用环境变量 `APP_PROTOCOL` 或命令行参数 `--protocol pc|web` 覆盖。

### `websocket.path`

本地 WebSocket 路由前缀，默认是 `/ws`。例如设置为 `/live-stream` 后，客户端使用 `/live-stream/{live_id}` 连接。不能与 `/health`、`/metrics` 或 `/api/*` 等保留 HTTP 路由冲突；配置值是字面路径，不允许 ServeMux 通配符花括号或百分号编码路径。

```yaml
websocket:
  path: "/ws"
```

### `websocket.allowed_origins`

可选的浏览器 `Origin` 白名单。留空时保持向后兼容并允许所有来源；配置后只允许精确匹配的 `http` 或 `https` Origin。

```yaml
websocket:
  allowed_origins:
    - "https://client.example.com"
```

### `log.level`

日志级别。默认输出 `info` 及以上级别，排查连接、心跳、重连问题时可以临时调整为 `debug`。

默认值：

```yaml
log:
  level: "info"
```

### `sign.provider`

WebSocket 签名来源。可选值：

- `local`：使用内置原生 Go 签名，WebSocket 签名失败时自动使用独立 Goja Runtime 兼容回退，默认推荐。
- `tikhub`：使用 TikHub 在线 API 生成签名，需要配置 `tikhub.key`。

默认值：

```yaml
sign:
  provider: ""
```

留空表示使用当前二进制默认值，也就是 `local`。如果你想强制指定，也可以写成 `local` 或 `tikhub`。

### `tikhub.key`

TikHub API Key，仅当 `sign.provider` 为 `tikhub` 时需要。

如果选择了 `tikhub` 但没有提供 Key，程序会在启动阶段直接报错，不会等到连接直播间后才失败。日志不会输出完整 API Key。

获取方式：

1. 打开 [TikHub 注册页](https://user.tikhub.io/register) 注册账号
2. 登录 [TikHub 用户中心](https://user.tikhub.io/)
3. 创建 API Key / API Token
4. 把 Key 保存到本地 `config.yaml`

配置写法：

```yaml
sign:
  provider: "tikhub"
tikhub:
  key: "YOUR_TIKHUB_KEY"
```

也可以通过环境变量传入，适合 Docker、systemd、CI 等不想把 Key 写进配置文件的场景：

```bash
APP_SIGN_PROVIDER=tikhub APP_TIKHUB_KEY=YOUR_TIKHUB_KEY ./douyinLive
```

### `api.key`

只读 HTTP API 和 WebSocket 握手共用的可选 Bearer Token。留空时不强制认证；配置后 `/api/v1/*`、`/metrics` 和 WebSocket 握手必须携带 `Authorization: Bearer <key>`。`/health` 始终无需认证，便于容器健康检查。

```yaml
api:
  key: "YOUR_API_KEY"
```

也可以使用环境变量 `APP_API_KEY`。API Key 不支持放在 URL 查询参数中。

### `api.allowed_domains`

URL 解析接口允许的域名。程序只接受 `douyin.com` 主域名或其子域名，避免把该接口变成通用 URL 获取器。默认值：

```yaml
api:
  allowed_domains:
    - "douyin.com"
```

### `monitor.poll_interval`
未开播时，服务端检查“是否已经开播”的时间间隔。

默认值：

```yaml
monitor:
  poll_interval: "15s"
```

### `monitor.notify_interval`
未开播时，服务端向本地 WebSocket 客户端重复推送状态通知的时间间隔。

默认值：

```yaml
monitor:
  notify_interval: "30s"
```

客户端会收到类似：

```json
{"type":"system","event":"live_status","live":false,"room_id":"516466932480","message":"直播间未开播","retry_interval_seconds":30}
```

### `proxy.url` / `proxy.rooms`

为抖音采集设置全局默认代理，或按直播间固定代理：

```yaml
proxy:
  url: "http://127.0.0.1:7890"
  rooms:
    "516466932480": "socks5://user:password@127.0.0.1:1080"
    "123456789": "http://another-proxy.example:8080"
```

支持 HTTP CONNECT 和 SOCKS5，可在 URL 中填写用户名和密码；特殊字符需要 URL 编码。HTTP 代理默认端口为 80，SOCKS5 必须填写端口。`http://` 代理可以承载 HTTPS/WSS，暂不支持到代理服务器本身使用 TLS 的 `https://` 代理，也不接受 `socks5h://` 别名。

路由优先级：非空 `proxy.rooms[房间ID]` > 非空 `proxy.url` > `HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`（兼容小写形式）> 直连。显式配置代理时，`NO_PROXY` 不会绕过它。房间值为空时继承默认值；房间 ID 与 `cookie.rooms` 使用相同格式。

代理覆盖获取 Cookie、直播页、房间接口、IM 初始化、状态查询、未开播监控和上游 WebSocket 首连/重连。代理配置存在时关闭采集 HTTP/3，保留 HTTP/1.1、HTTP/2 和 TLS 验证；代理失败不自动改为直连。环境变量在采集实例创建时读取，更换配置后重启服务；实际出口是否固定还取决于代理服务。

TikHub 签名客户端仍使用其原有网络策略，不继承房间代理。此配置不改变本地 HTTP/WebSocket 服务，也不提供代理池、自动换 IP 或请求级代理参数。

### `cookie.douyin`
抖音默认 Cookie，可选。

没有单独配置某个直播间的 Cookie 时，会优先回退到这里。再往后才是自动获取的逻辑。

```yaml
cookie:
  douyin: "ttwid=...; sessionid=..."
```

### `cookie.use_stored`

是否使用 `cookie.douyin` 和 `cookie.rooms` 中的预存 Cookie，默认是 `true`。设置为 `false` 后，HTTP 查询和 WebSocket 房间都会忽略预存 Cookie，临时传入的连接 Cookie 仍然优先。程序仍会自动获取 `ttwid` 等匿名访问 Cookie；日志中的 `has_cookie=true` 不代表已有登录态。

```yaml
cookie:
  use_stored: true
```

### `cookie.rooms`
按直播间 ID 单独配置 Cookie，可选。

如果你要同时监听多个直播间，而且它们对应不同账号、不同登录态，就可以在这里分别配置。没有配置到的直播间，会自动回退使用 `cookie.douyin`。

```yaml
cookie:
  use_stored: true
  douyin: "默认 Cookie"
  rooms:
    "516466932480": "直播间 516466932480 专用 Cookie"
    "123456789": "直播间 123456789 专用 Cookie"
    "888888888": "直播间 888888888 专用 Cookie"
```

一个更完整的例子：

```yaml
port: "1088"
unknown: false
log:
  level: "info"
sign:
  provider: ""
tikhub:
  key: ""
monitor:
  poll_interval: "15s"
  notify_interval: "30s"
cookie:
  douyin: "默认 Cookie"
  rooms:
    "516466932480": "room A 的 Cookie"
    "123456789": "room B 的 Cookie"
```

Cookie 优先级：

```text
WebSocket 临时 Cookie > 直播间 Cookie(cookie.rooms) > 默认 Cookie(cookie.douyin) > 自动获取
```

WebSocket 临时 Cookie 仅建议临时调试使用：

```text
ws://127.0.0.1:1088/ws/直播间ID?cookie_b64=BASE64URL_COOKIE
```

也支持直接传 URL 编码后的 Cookie：

```text
ws://127.0.0.1:1088/ws/直播间ID?cookie=URL_ENCODED_COOKIE
```

## 环境变量映射

独立服务使用 `APP_` 前缀的环境变量覆盖配置项。常用配置如下：

| 配置项 | 环境变量 |
| --- | --- |
| `port` | `APP_PORT` |
| `unknown` | `APP_UNKNOWN` |
| `log.level` | `APP_LOG_LEVEL` |
| `sign.provider` | `APP_SIGN_PROVIDER` |
| `tikhub.key` | `APP_TIKHUB_KEY` |
| `api.key` | `APP_API_KEY` |
| `api.allowed_domains` | `APP_API_ALLOWED_DOMAINS` |
| `websocket.path` | `APP_WEBSOCKET_PATH` |
| `websocket.allowed_origins` | `APP_WEBSOCKET_ALLOWED_ORIGINS` |
| `cookie.use_stored` | `APP_COOKIE_USE_STORED` |
| `cookie.douyin` | `APP_COOKIE_DOUYIN` |
| `cookie.rooms` | `APP_COOKIE_ROOMS` |
| `proxy.url` | `APP_PROXY_URL` |
| `proxy.rooms` | `APP_PROXY_ROOMS` |
| `monitor.poll_interval` | `APP_MONITOR_POLL_INTERVAL` |
| `monitor.notify_interval` | `APP_MONITOR_NOTIFY_INTERVAL` |

列表环境变量 `APP_API_ALLOWED_DOMAINS` 和 `APP_WEBSOCKET_ALLOWED_ORIGINS` 支持逗号分隔、空白分隔或 JSON 字符串数组。例如：`APP_API_ALLOWED_DOMAINS=live.douyin.com,www.douyin.com`。

`APP_COOKIE_ROOMS` 使用 JSON 字符串对象，并完整覆盖配置文件中的 `cookie.rooms`，例如：`APP_COOKIE_ROOMS={"AbC123":"room-cookie"}`。房间号大小写会原样保留。

`APP_PROXY_ROOMS` 同样使用 JSON 字符串对象，例如 `APP_PROXY_ROOMS={"AbC123":"http://127.0.0.1:7890"}`，整体替换房间代理映射，空值清空映射。`APP_PROXY_URL` 覆盖全局默认代理，`--proxy-url` 的优先级更高；它们不覆盖已配置的房间代理。

命令行参数优先级高于环境变量，环境变量高于配置文件，配置文件高于程序默认值。Cookie 也可以通过 WebSocket URL 的 `cookie_b64` 或 `cookie` 参数临时覆盖，但不建议把 Cookie 长期放在 URL、Shell 历史或进程列表中。

## 什么时候需要 Cookie

不是所有场景都必须填 Cookie。

你可以先不填，直接跑。

如果出现下面这些情况，再考虑补 Cookie：

- 某些直播间拿不到消息
- 请求被限制
- 页面返回结果异常
- 需要更稳定的登录态

如果持续出现“直播页状态不存在”且 `web/enter` 返回空响应，通常是匿名请求遇到了验证页或访问限制；请配置有效登录 Cookie，或更换出口 IP/代理后重试。

## Cookie 怎么拿

1. 浏览器打开：`https://live.douyin.com`
2. 登录抖音
3. 按 `F12`
4. 打开 `Network`
5. 随便点一个请求
6. 复制请求头里的 `Cookie`

然后填到：

```yaml
cookie:
  douyin: "你的完整 Cookie"
```

---
