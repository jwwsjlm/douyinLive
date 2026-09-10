**Issue #65：采集连接代理支持方案**

状态：已实施，随 `v2.3.0-beta.1` 提供测试。核对日期：2026-09-10。设计基线：本地 `main`，提交 `58b34a3`。以下保留原方案，实际用法见 `configuration.md` 和 `library.md`。

实施调整：复用已安装的 `golang.org/x/net/http/httpproxy` 在实例创建时快照环境变量规则，避免标准库进程级缓存与重建时的环境变化；环境测试无需子进程。另修复了代理握手挂起时实例关闭不能立即取消的问题。依赖升级到当前兼容版本，Go 更新为 1.27.1，req 为 3.61.2。

Issue #65 于 2026-09-08 提出多连接采集时希望支持代理。建议首版实现“全局默认代理 + 按直播间固定代理”，让指定房间的抖音 HTTP 请求和上游 WebSocket 使用同一代理配置。验收以请求路由正确、重连不丢配置为准；issue 对 IP 风控的归因尚未通过日志或对照实验验证。

**已确认的代码现状**

| 位置 | 现有行为 | 对方案的影响 |
| --- | --- | --- |
| `session_profile.go`：`newHTTPClient`、`rebuildHTTPClientAndHeaders` | 每个会话持有独立 req 客户端，启用 HTTP/3；重连时可能重建客户端 | 代理必须保存在会话中，创建和重建均重新应用 |
| `cookie_context.go`、`room_info.go`、`websocket_connection.go` | 获取 ttwid、直播页、web/enter 和 im/fetch 均经过 `dl.client` | 在 HTTP 客户端创建处接入即可覆盖这些请求 |
| `websocket_connection.go`、`reconnect.go` | 首连和重连各自复制 `websocket.DefaultDialer`，最终经过 `dialUpstreamWebSocket` | 在公共拨号入口应用代理，覆盖首次连接、签名回退和重连 |
| `cmd/main/room_session.go`：`acquireProbeLive` | 创建离线探测实例，确认开播后将其提升为直播实例；探测失败可能重建实例 | 从第一次状态探测开始固定房间代理，实例重建也沿用它 |
| `cmd/main/room_manager.go`：`probeFactory` | HTTP API 的独立状态查询另外创建实例 | 必须接入相同的房间代理选择逻辑 |
| `cmd/main/config.go`、`cmd/main/app.go` | 支持严格 YAML 校验、环境变量和 CLI 覆盖；应用复制部分配置防止外部修改 | 代理配置沿用现有加载顺序，房间映射也要复制 |

当前依赖为 `github.com/jwwsjlm/req/v3 v3.61.1` 和 `github.com/gorilla/websocket v1.5.3`。两者默认均使用 `http.ProxyFromEnvironment`，所以当前已有环境变量代理的底层入口，但尚未提供显式的房间代理配置，不能据此认定整条链路都已可靠走代理。

req 的 `SetProxy` 明确只作用于 HTTP/1 和 HTTP/2；项目启用了 HTTP/3，因此代理模式需要关闭 HTTP/3，包括通过 Alt-Svc 升级的路径。Gorilla 当前版本原生支持 HTTP CONNECT 和 SOCKS5，首版使用二者共同支持的协议即可，无需新增依赖。

**一、配置与生效规则**

新增配置示例：

```yaml
proxy:
  # 全局默认代理；空字符串表示沿用环境变量代理行为
  url: ""
  # 键与 cookie.rooms 一致，使用传入服务的直播间标识
  rooms:
    "1001": "http://proxy-a.example:8080"
    "1002": "socks5://user:password@proxy-b.example:1080"
```

| 配置入口 | 用途 |
| --- | --- |
| `proxy.url` | 全局默认代理 |
| `proxy.rooms` | 房间到代理 URL 的固定映射 |
| `APP_PROXY_URL` | 覆盖 YAML 中的全局默认代理 |
| `APP_PROXY_ROOMS` | JSON 字符串对象，整体替换 YAML 的房间映射；空值清空映射 |
| `--proxy-url` | 覆盖全局默认代理 |

配置来源优先级沿用现有规则：CLI > 应用环境变量 > YAML > 默认值。房间路由优先级单独计算：非空房间代理 > 非空全局代理 > 标准环境变量代理 > 直连。`--proxy-url` 只覆盖全局值，不抹掉房间配置；房间值为空时继承全局值。

标准环境变量兼容 `HTTP_PROXY`、`HTTPS_PROXY`、`NO_PROXY` 及其小写形式，继续使用标准库解释。首版不扩展 `ALL_PROXY`，不增加单独的“强制直连”配置。

显式指定代理时使用 `http.ProxyURL`，不再受环境变量或 `NO_PROXY` 覆盖。未显式指定时保留标准库按目标地址选择代理的行为；此兼容模式不承诺不同域名一定使用同一出口。

配置在进程启动时加载，房间映射在管理器生命周期内不可变。变更代理后重启服务；库调用方重新创建实例。程序保证使用固定代理地址，实际出口是否固定还取决于代理服务，应使用提供稳定出口的代理配置。

**二、协议范围与输入校验**

- 支持 `http://host:port`，通过 CONNECT 承载 HTTPS/WSS；支持 URL 中的用户名和密码。
- 支持 `socks5://host:port` 及用户名密码认证，域名交给代理解析。
- 首版不接受 `https://` 代理、`socks4://` 或 `socks5h://` 别名，返回明确的不支持错误。这里的 `https://` 是“到代理服务器使用 TLS”，与“HTTP 代理访问 HTTPS 目标”不同；后者在首版范围内。
- 使用 `net/url` 解析，校验协议、主机、端口范围和控制字符；HTTP 未填端口时按 80 处理，SOCKS5 要求显式填写端口。拒绝非根路径、查询参数、片段和 opaque URL。房间键复用 `ValidateLiveID`，规范化后重复的键应报错。
- 在配置加载、程序化创建应用以及库构造入口进行相应校验，不能只依赖 CLI。错误只包含配置项、房间标识和错误原因，不包含原始代理 URL 或认证信息。

不直接依赖 `req.SetProxyURL` 做输入校验：当前实现解析失败时记录日志并返回客户端，容易让无效配置继续运行。应先校验，再把解析结果交给 `SetProxy(http.ProxyURL(u))`。环境变量解析结果也要经过协议兼容性检查，不支持的协议报错，不静默直连。

**三、库层实现**

新增一个结构体构造入口，保留现有四个公开构造函数的签名和默认行为。接口草案：

```go
type Options struct {
    Cookie       string
    ProxyURL     string
    SignProvider string
    TikHubToken  string
}

func NewDouyinLiveWithOptions(liveID string, logger Logger, options Options) (*DouyinLive, error)
```

`SignProvider` 为空时使用现有本地签名；选择 TikHub 时校验 token。需要 slog 的调用方复用 `NewSlogLogger`，不再增加一组“带代理的构造函数”。旧入口与新入口汇入共享构造逻辑，先校验参数，再创建签名器和网络资源。

代理策略保存到 `sessionProfile`，实例开始请求后不可变。HTTP 客户端和 WS 拨号器共用同一策略，不修改 `http.DefaultTransport`、`websocket.DefaultDialer` 或进程环境变量。

HTTP 改动集中在 `newHTTPClient`：

1. 保留现有浏览器伪装、UA、超时和 TLS 验证。
2. 在创建客户端时设置代理。
3. 显式配置代理，或存在非空标准 HTTP/HTTPS 代理环境变量时，关闭 HTTP/3。即使环境模式下部分请求命中 `NO_PROXY`，该会话也统一关闭 HTTP/3，避免按请求修改共享传输配置。
4. 无显式代理且无上述环境变量时，保留现有 HTTP/3 行为。
5. `rebuildHTTPClientAndHeaders` 使用会话保存的代理策略重建客户端，并继续关闭旧客户端的空闲连接。

WS 改动集中在 `dialUpstreamWebSocket`：复制传入的 dialer，在副本上设置会话代理，再执行原有 `DialContext`。现有首连、签名回退和重连都经过这里，因此无需在每个调用处复制代理逻辑。保留已有超时、关闭信号、心跳和指数退避。

代理连接或认证失败时返回错误，重试仍走同一代理，不自动切换代理或直连。网络代理错误也不应被误认为签名算法错误。

**四、服务层接入**

在 `RoomManagerOptions` 增加默认代理和房间映射，在 `RoomManager` 增加一个简单的 `proxyForRoom(liveID)` 选择函数，复用现有按房间选择 Cookie 的组织方式。`NewApp` 和管理器创建时复制映射，避免调用方修改导致出口漂移。

需要覆盖两个实例创建入口：

1. `GetOrCreateRoom` 在房间发布到共享 map 前确定代理并保存在 `Room`；`acquireProbeLive` 创建实例时传入它。匿名探测重建、开播提升、下播后恢复监控均继承该配置。
2. `RoomManager.probeFactory` 为独立 HTTP API 状态查询使用 `proxyForRoom(liveID)`，防止 API 探测遗漏代理。

首版按房间固定映射且不支持热更新，同一管理器内同一房间的有效代理不会变化，因此继续使用现有 `roomID + Cookie 摘要` 复用键即可。不新增代理参与的缓存键；以后允许单次请求覆盖代理或热更新时，再把有效代理摘要纳入会话和探测复用键。

代理只作用于抖音采集流量。本地 WebSocket 服务、HTTP API 入站不需要改动；TikHub 在线签名使用独立客户端，本期保持它现有的网络策略，不继承房间代理。本地客户端也不增加 `?proxy=` 请求参数。

日志记录 `live_id`、`proxy_source`、`proxy_scheme`、`proxy_host` 和 `has_auth` 即可；不记录用户名、密码、完整 URL。检查错误包装后的文本，避免解析失败或代理返回内容间接泄露凭据。

**五、改动清单**

| 文件 | 计划改动 |
| --- | --- |
| `douyin.go`、必要时 `logging.go` | 新增 Options 构造入口，旧入口复用共享逻辑 |
| `session_profile.go` | 代理解析校验和会话保存；HTTP 创建、重建时应用；关闭代理模式的 HTTP/3 |
| `websocket_connection.go` | 在公共 WS 拨号入口应用代理 |
| `cmd/main/config.go`、`cmd/main/app.go` | YAML、应用环境变量、CLI、校验和配置复制 |
| `cmd/main/room_manager.go`、`cmd/main/room.go`、`cmd/main/room_session.go` | 房间代理选择、保存及两个实例创建入口接入 |
| 现有测试文件，必要时新增 `proxy_test.go` | 配置、实际代理转发、重建和失败行为验证 |
| `config.example.yaml`、`docs/configuration.md`、`docs/library.md`、`docs/cli.md` | 配置、库调用、CLI 示例及协议边界 |

`reconnect.go`、`room_info.go`、`cookie_context.go` 的调用路径通过公共入口覆盖，不必逐点修改。依赖按后续实施要求一并升级。

**六、验收**

使用现有 Go `testing` 和 `httptest`，搭建本地 HTTP CONNECT/SOCKS5 测试代理和 HTTP/WS 上游，不依赖真实抖音账号或外部付费代理。代理记录实际目标和连接次数；仅检查配置字段不算通过。

| 场景 | 通过条件 |
| --- | --- |
| 默认兼容 | 旧构造函数、旧配置可用；清空代理环境后请求直连 |
| 配置优先级 | CLI/环境/YAML 的覆盖、房间优先、空值继承与映射替换符合约定 |
| HTTP 完整路径 | ttwid、直播页、web/enter、im/fetch 和状态查询实际经过指定代理 |
| WS 完整路径 | 首连、签名回退和断线重连实际经过同一代理 |
| 生命周期 | HTTP 客户端重建、匿名探测实例重建、监控转直播后代理仍生效 |
| 并发隔离 | 两个房间分别走代理 A/B，连接和 Cookie 不串用；全局默认 dialer 未被修改 |
| 认证与协议 | HTTP CONNECT 和 SOCKS5 的匿名、用户名密码认证可用；SOCKS5 将目标域名交给代理 |
| 环境兼容 | HTTP_PROXY/HTTPS_PROXY/NO_PROXY 的大小写形式及显式代理覆盖行为符合约定；环境场景使用独立子进程，避免标准库缓存相互干扰 |
| HTTP/3 | 代理模式下，上游返回 Alt-Svc 后后续请求仍经过代理，测试 UDP 端点无 QUIC 流量 |
| 失败处理 | 代理不可达、认证失败或协议不支持时，上游直连计数为零；状态保持 unknown/error，不误报下播 |
| 参数与日志 | 非法 URL、端口、房间键在预期入口被拒绝；日志和返回错误不出现代理认证信息 |
| 关闭与取消 | 慢代理、挂起握手在现有超时/关闭时限内退出，不遗留连接；如现有 dialer 无法满足，需要补最小取消处理后再验收 |

实现后执行：

```sh
go test ./...
go test -race ./...   # 在具备 race 工具链的环境执行
```

实现覆盖库层、服务配置及两个实例入口；代理转发、重连、取消及隔离测试随源码提供。发布说明记录最终验证结果。

**后续范围**

首版不做代理池、自动轮换、健康检查、失败切换、外部代理平台对接或热更新。只有手工维护房间映射已成为实际负担时，再讨论自动分配；若要切换出口，需要明确整个会话及 Cookie 的重建策略。

**核对来源**

- Issue：`https://github.com/jwwsjlm/douyinLive/issues/65`
- 本地代码：上述文件，基线 `58b34a3`。
- req 代理与 HTTP/3 实现：`https://raw.githubusercontent.com/jwwsjlm/req/v3.61.1/transport.go`
- Gorilla 默认拨号策略：`https://raw.githubusercontent.com/gorilla/websocket/v1.5.3/client.go`
- Gorilla HTTP CONNECT：`https://raw.githubusercontent.com/gorilla/websocket/v1.5.3/proxy.go`
- Gorilla SOCKS5 与协议分派：`https://raw.githubusercontent.com/gorilla/websocket/v1.5.3/x_net_proxy.go`
