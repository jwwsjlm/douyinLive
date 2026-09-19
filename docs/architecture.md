# 架构与维护入口

[返回文档导航](README.md)

独立服务只负责配置、HTTP/WebSocket 接入和房间生命周期；抖音协议、HTTP 请求、上游 WebSocket 和重连保留在根包。依赖方向固定为：

```text
cmd/main → internal/server → douyinLive 根包 → sign / internal/webcastsign / jsScript / utils
```

`cmd/main/main.go` 只保留进程入口和 `main.*` 构建信息注入。它把构建信息传给 `internal/server/runner.go`；runner 加载配置、处理信号并启动 `App`。`App` 建立路由后，HTTP 请求进入 HTTP API，WebSocket 请求获取 `Room`；房间再通过根包的 `NewDouyinLiveWithOptions` 创建上游会话。

| 修改内容 | 首先查看 | 关联入口 |
| --- | --- | --- |
| HTTP API、认证、健康检查、指标 | `internal/server/http_api.go`、`metrics.go` | 路由在 `internal/server/app.go`，接口说明见 [HTTP API](http-api.md) |
| 下游 WebSocket 升级、Origin、Cookie 覆盖 | `internal/server/app.go`、`ws_handler.go` | 客户端队列和关闭在 `room_client.go` |
| 服务配置、优先级、Cookie/代理校验 | `internal/server/config.go` | 进程启动在 `runner.go`，配置说明见 [配置文件](configuration.md) |
| 房间复用、状态探测、未开播监控 | `internal/server/room_manager.go`、`room_session.go`、`room_monitor.go` | 房间与下游消息在 `room.go`、`room_status.go` |
| 上游协议画像 | `protocol_profile.go`、`session_profile.go`、`douyin.go` | 服务只把 `protocol.mode` 传入根包，不在服务层复制协议逻辑 |
| 上游 WebSocket 与重连 | `websocket_connection.go`、`reconnect.go`、`http_context.go` | 不要在 `internal/server` 改写重连策略 |
| HTTP/URL 签名与 Cookie 管理 | `sign/` | WebSocket 签名实现位于 `internal/webcastsign/`，兼容回退脚本位于 `jsScript/` |

历史代理方案保留在 [Issue #65 记录](issue-65-proxy-plan.md)；其中的旧路径只用于说明当时的设计基线，当前维护入口以本文为准。
