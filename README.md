# douyinLive

一个基于 WebSocket 的抖音直播弹幕抓取工具，可作为独立服务运行，也可以作为 Go 库集成。

> **项目边界说明，请先阅读**
>
> 本项目仅用于研究和记录抖音直播 WebSocket 链接的逆向获取、连接方式及基础数据接收流程。
>
> 本项目不承诺、也不负责保证任何具体业务消息一定能够收到或完整解析。礼物消息缺失、字段无法解析、消息结构变化或个别直播间数据不完整等问题，不作为本项目后续适配目标。

[![GitHub Release](https://img.shields.io/github/v/release/jwwsjlm/douyinLive)](https://github.com/jwwsjlm/douyinLive/releases)
[![License](https://img.shields.io/github/license/jwwsjlm/douyinLive)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26.6-blue)](https://golang.org)

## 功能

- 实时接收弹幕、礼物、点赞、进场、关注等常见直播消息
- 单进程监听多个直播间
- 将消息转发到本地 WebSocket 客户端
- 支持本地签名以及可选 TikHub 在线签名
- 支持断线重连、未开播轮询和基础保活
- 可作为独立服务或 Go 库使用
- 提供只读 HTTP 查询、批量状态、URL 解析和 Prometheus 指标

本项目不负责下载或录制 FLV、M3U8 和直播回放。

## 快速开始

### 下载可执行文件

从 [Releases](https://github.com/jwwsjlm/douyinLive/releases) 下载对应平台的压缩包并解压：

```bash
./douyinLive
```

Windows PowerShell：

```powershell
.\douyinLive.exe
```

程序默认监听本地 `1088` 端口。直播间标识通过 WebSocket 路径传入，而不是启动参数：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

例如：

```text
ws://127.0.0.1:1088/ws/516466932480
```

### 源码编译

```bash
git clone https://github.com/jwwsjlm/douyinLive.git
cd douyinLive
go build -o douyinLive ./cmd/main
./douyinLive
```

查看版本、commit 和构建来源：

```bash
./douyinLive --version
```

### Docker

```bash
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:latest
```

长期运行、配置挂载和 Docker Compose 请阅读 [Docker 部署文档](docs/docker.md)。

## HTTP API 与后端服务

很多项目会直接把 `douyinLive` 作为后端服务使用，因此 v2.2.0 在保留 Go 库用法和原有 WebSocket 行为的基础上，增强了独立服务能力：

- 不建立长期 WebSocket，也可以查询直播状态和直播间信息
- 支持批量状态查询和 `douyin.com` 直播间 URL 解析
- 提供 `/health`、`/metrics` 和 OpenAPI 描述
- 支持自定义 WebSocket 路由，以及 HTTP/WebSocket 共用的可选 API Key
- 查询时可按配置使用房间 Cookie、全局 Cookie，或关闭预存 Cookie

HTTP API 默认只读，不提供发弹幕、点赞、礼物或远程修改配置的接口。完整说明见 [HTTP API 文档](docs/http-api.md)，接口定义见 [OpenAPI 文件](docs/openapi.yaml)。

## 作为 Go 库使用

安装：

```bash
go get github.com/jwwsjlm/douyinLive/v2
```

仅检查直播状态，不建立上游 WebSocket：

```go
ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
defer cancel()

live, err := douyinLive.NewDouyinLiveWithSlog("516466932480", slog.Default(), "")
if err != nil {
    return err
}
defer live.Dispose()

status, err := live.CheckLiveStatus(ctx)
```

监听消息：

```go
subscriptionID := live.SubscribeMessage(func(message *douyinLive.LiveMessage) {
    fmt.Println(message.GetMethod())
})
defer live.Unsubscribe(subscriptionID)

if err := live.Start(); err != nil {
    return err
}
```

完整的状态码、订阅接口、protobuf 解析和生命周期说明请阅读 [Go 库使用文档](docs/library.md)。

## 配置

复制示例配置：

```bash
cp config.example.yaml config.yaml
./douyinLive --config ./config.yaml
```

默认使用 `local` 本地签名。需要 TikHub 时，可通过配置文件、环境变量或命令行切换：

```bash
./douyinLive --sign-provider tikhub --tikhub-key YOUR_TIKHUB_KEY
```

不要把 Cookie、TikHub Key、完整签名 URL 或日志中的敏感字段提交到仓库。详细配置见 [配置文件文档](docs/configuration.md)。

## 文档

| 文档 | 内容 |
| --- | --- |
| [文档导航](docs/README.md) | 全部详细文档入口 |
| [Docker 部署](docs/docker.md) | Docker、Compose、配置挂载和长期运行 |
| [CLI 使用指南](docs/cli.md) | 参数、日志、签名方式和故障排查 |
| [配置文件](docs/configuration.md) | YAML、环境变量、Cookie 和配置优先级 |
| [HTTP API](docs/http-api.md) | 只读查询、批量状态、URL 解析、认证和指标 |
| [OpenAPI](docs/openapi.yaml) | HTTP API 的机器可读接口定义 |
| [作为 Go 库使用](docs/library.md) | 状态检查、订阅、protobuf 和生命周期 |
| [WebSocket 客户端与消息格式](docs/websocket-client.md) | 客户端接入、系统状态和业务消息 |
| [`sign` 包说明](sign/README.md) | `a_bogus` 签名和 CookieManager 的直接调用 |

## 项目结构

```text
douyinLive/
├── cmd/main/                 # 独立服务入口
├── docs/                     # 详细使用文档
├── examples/                 # 调试示例数据
├── internal/webcastsign/     # 内部签名实现
├── jsScript/                 # Goja 兼容回退脚本
├── sign/                     # HTTP 签名与 Cookie 逻辑
├── utils/                    # 工具函数
├── douyin.go                 # 核心库接口
├── live_status_api.go        # 库级直播状态检查
├── config.example.yaml       # 配置示例
└── README.md
```

## 致谢

- [ihmily/DouyinLiveRecorder](https://github.com/ihmily/DouyinLiveRecorder)
- [saermart/DouyinLiveWebFetcher](https://github.com/saermart/DouyinLiveWebFetcher)
- [douyin_proto](https://github.com/Remember-the-past/douyin_proto)

## 许可证

[MIT](LICENSE)
