# CLI 使用指南

[返回项目首页](../README.md)

本文介绍可执行程序启动、直播间参数、命令行选项、签名来源、日志和问题排查。

## 直播间参数怎么传

很多人第一次用会卡在这里。

这个程序启动时**不需要**在命令行传直播间号。

直播间标识是通过 WebSocket 路径传进去的：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

也就是说：

- 程序只负责启动本地服务
- 你连接哪个房间，是由 `/ws/后面的内容` 决定的

## 什么叫“直播间标识”

一般就是你访问下面这个地址时，后面的那段：

```text
https://live.douyin.com/xxxxx
```

这里的 `xxxxx` 就是你应该传给 `/ws/` 的内容。

例如：

- `https://live.douyin.com/516466932480`
  - 则连接：`ws://127.0.0.1:1088/ws/516466932480`

如果你传的是无效标识，服务端会关闭这个连接。

如果直播间暂时未开播：

- 本地 WebSocket 连接会保留
- 服务端会先返回一条“直播间未开播”的状态通知
- 然后按配置的时间间隔持续推送未开播状态
- 一旦检测到开播，就自动切回正常消息流

`live_status` 里的 `live=false` 不是网络错误，也不代表本地服务已经失效。客户端收到这个状态后建议保持连接，等待后续 `live=true` 通知；只有 WebSocket 本身断开时，客户端才需要按自己的策略重连。

---

## CLI 完整示例（推荐先看这里）

`douyinLive` 启动后是一个本地 WebSocket 服务。**直播间标识不是 CLI 启动参数**，而是客户端连接 WebSocket 时写在 URL 里。

### Linux / macOS

```bash
cp config.example.yaml config.yaml
./douyinLive --config ./config.yaml --port 1088 --log-level info
```

然后让你的客户端连接：

```text
ws://127.0.0.1:1088/ws/516466932480
```

### Windows PowerShell

```powershell
Copy-Item .\config.example.yaml .\config.yaml
.\douyinLive.exe --config .\config.yaml --port 1088 --log-level info
```

然后让你的客户端连接：

```text
ws://127.0.0.1:1088/ws/516466932480
```

## 默认启动

如果不需要配置文件，也可以直接启动：

```bash
./douyinLive
```

Windows：

```powershell
.\douyinLive.exe
```

默认行为：

- 读取同目录下的 `config.yaml`（如果存在）
- 如果没有配置文件，就使用默认值
- 默认端口：`1088`
- 默认日志级别：`info`
- 默认使用 `local` 本地签名；需要 TikHub 时在运行时切换为 `tikhub`

## 指定端口

```bash
./douyinLive --port 1088
```

Windows：

```powershell
.\douyinLive.exe --port 1088
```

## 指定配置文件

```bash
./douyinLive --config ./config.yaml
```

Windows：

```powershell
.\douyinLive.exe --config .\config.yaml
```

## 输出未知消息类型（调试用）

```bash
./douyinLive --unknown
```

Windows：

```powershell
.\douyinLive.exe --unknown
```

## 设置日志级别

```bash
./douyinLive --log-level debug
```

Windows：

```powershell
.\douyinLive.exe --log-level debug
```

支持 `debug`、`info`、`warn`、`error`，默认是 `info`。也可以写进配置文件：

```yaml
log:
  level: "debug"
```

日志使用 Go `slog`，并针对终端和 Docker 做了单行可读格式化。时间包含毫秒和时区偏移，级别保持对齐，其他上下文继续使用 `key=value` 结构化字段：

```text
2026-08-17 20:37:59.428 +08:00  INFO   DouyinLive 启动  tag=v2.1.0 commit=abcdef12 sign_provider=local
2026-08-17 20:38:05.672 +08:00  WARN   准备重新连接  room_id=123456 attempt=2
```

这种格式方便人眼阅读，也可以按 `room_id`、`live_id`、`stage`、`step`、`err` 等字段检索，适合长时间挂机时排查连接和重连状态。

## 查看版本和构建来源

```bash
./douyinLive --version
```

Windows：

```powershell
.\douyinLive.exe --version
```

输出会包含：

- `tag`：本次构建对应的 tag，本地手动构建默认为 `dev`
- `commit`：构建时注入的短 commit hash
- `buildDate`：构建时间
- `source`：构建来源，例如 GitHub Actions 或本地构建
- `signProvider`：当前二进制默认签名来源，`local` 或 `tikhub`

## 设置签名来源

程序默认使用 `local`。需要 TikHub 在线签名时，可以通过配置文件、命令行或环境变量切换：

```bash
./douyinLive --sign-provider local
./douyinLive --sign-provider tikhub --tikhub-key YOUR_TIKHUB_KEY
APP_SIGN_PROVIDER=tikhub APP_TIKHUB_KEY=YOUR_TIKHUB_KEY ./douyinLive
```

三种方式任选一种即可，不需要下载单独的 TikHub 版本，也不会和本地签名版本冲突。配置优先级从高到低是：

1. 命令行参数：`--sign-provider`、`--tikhub-key`
2. 环境变量：`APP_SIGN_PROVIDER`、`APP_TIKHUB_KEY`
3. 配置文件：`sign.provider`、`tikhub.key`
4. 程序默认值：`local`

如果多个地方同时配置，以优先级最高的为准。`sign.provider=local` 时：

- WebSocket `signature` 默认使用与 `webmssdk.js` 逐字节校验过的原生 Go 实现；如果上游握手明确失败，会按当前房间的 UA、Cookie 和浏览器画像延迟启用独立 Goja Runtime 兼容重试。
- `/webcast/room/web/enter/` 和 `/webcast/im/fetch/` 使用轻量原生 `AbSign`（SM3 + RC4）计算 `a_bogus`，不会启动大体积 JavaScript 运行时。

为了兼顾稳定性和启动速度，HTTP 请求的 `a_bogus` 与 WebSocket 的 `signature` 分开处理：`a_bogus` 固定使用原生算法，`signature` 根据 `sign.provider` 选择本地原生 Go（必要时 Goja 兼容回退）或 TikHub 在线 API。两者用途不同，不应混用。

只有 `sign.provider=tikhub` 时才会调用 TikHub 在线 API，并且必须提供 `tikhub.key`。

TikHub API Key 可以在 [TikHub 注册页](https://user.tikhub.io/register) 注册账号后，到 [TikHub 用户中心](https://user.tikhub.io/) 创建 API Key / API Token。Key 属于敏感信息，不要提交到仓库。

## 日志等级与 Issue 排查

日志使用 Go `slog` 文本格式，默认输出 `info` 及以上级别。排查连接、签名、心跳或重连问题时，请先临时开启 `debug`：

```bash
./douyinLive --config ./config.yaml --log-level debug
```

Windows：

```powershell
.\douyinLive.exe --config .\config.yaml --log-level debug
```

日志等级含义：

- `debug`：详细排查信息，包括 `web/enter`、`im/fetch`、WebSocket URL 生成、签名输入、cursor/internal_ext、重连上下文等。提交连接类 Issue 时建议开启。
- `info`：正常生命周期信息，例如服务启动、房间开始监听、WebSocket 连接成功、重连成功、正常关闭。
- `warn`：可恢复异常，例如读取 WS 超时、心跳发送失败、一次直播状态兜底检测失败、准备重连。
- `error`：不可自动恢复或最终失败，例如配置加载失败、房间状态刷新失败、连接最终失败。

关键字段说明：

- `stage`：失败发生的大阶段，例如 `startup`、`room_info`、`im_fetch`、`ws`。
- `step`：阶段内的具体步骤，例如 `live_page_state`、`web_enter`、`prefetch`、`signature`、`build_url`、`dial`、`read`、`decode_push_frame`。
- `live_id`：用户传入的直播间标识。
- `room_id`：网页解析到的真实直播间房间 ID。
- `user_unique_id`：网页侧用于 IM/WS 的用户唯一 ID。
- `reason`：重连或读取失败的分类，例如 `timeout`、`closed_network_connection`、`network_or_unknown`。
- `status`、`status_code`、`content_type`、`raw_len`：HTTP 或 WS 响应状态，常用于判断是接口空响应、protobuf 解析失败还是握手失败。

提交 Issue 时建议贴这几段日志：

- 程序启动后的版本行：包含 `tag`、`commit`、`build_date`、`build_source`、`sign_provider`。
- 第一次出现 `stage=room_info` 到 `stage=ws step=dial` 的完整日志。
- 发生断线时，从第一条 `读取 WebSocket 消息失败` 到后续 `检测到需重连`、`重连成功` 或 `连接最终失败` 的日志。
- 如果是签名问题，请贴 `stage=ws step=signature` 和 `stage=ws step=build_url`，但不要贴完整 Cookie、完整 URL、完整 `signature`、完整 `msToken`。

提交前请打码：

- `Cookie`
- `msToken`
- `a_bogus`
- `signature`
- `sessionid` / `sid_guard` / `ttwid`
- 任何账号、手机号、邮箱或私密直播间信息

## CLI 参数速查

```text
--config string      指定配置文件路径，例如 ./config.yaml
--port string        本地 WebSocket 服务端口，默认 1088
--unknown            输出未知 protobuf 消息类型，调试用
--log-level string   日志级别：debug、info、warn、error
--sign-provider      WebSocket 签名来源：local、tikhub
--tikhub-key string  TikHub API Key，仅 sign-provider=tikhub 时需要
--version            输出版本和构建来源
```

---
