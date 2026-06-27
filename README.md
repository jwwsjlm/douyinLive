# douyinLive

一个基于 WebSocket 的抖音直播弹幕抓取工具。

> **项目边界说明，请先阅读**
>
> 本项目仅用于研究和记录抖音直播 WebSocket 链接的逆向获取、连接方式及基础数据接收流程。
>
> 本项目不承诺、也不负责保证任何具体业务消息一定能够收到或完整解析。包括但不限于：礼物消息收不到、某类消息缺失、字段无法解析、消息结构变化、个别直播间数据不完整等问题，均不在本项目维护范围内。
>
> 请不要提交“没有礼物消息”“某类消息解析不了”“为什么收不到某条消息”等相关 Issue。此类问题不会作为 Bug 处理，也不作为本项目后续适配目标。

它做的事很简单：

1. 连接抖音直播间消息流
2. 解析弹幕 / 礼物 / 点赞 / 进场等消息
3. 再通过你本地启动的 WebSocket 服务把消息转发给你的客户端

适合两种用法：

- 直接当成一个本地 WebSocket 服务用
- 当成 Go 库集成到你自己的项目里

[![GitHub Release](https://img.shields.io/github/v/release/jwwsjlm/douyinLive)](https://github.com/jwwsjlm/douyinLive/releases)
[![License](https://img.shields.io/github/license/jwwsjlm/douyinLive)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.26.3-blue)](https://golang.org)

## 功能

- 实时接收直播间消息
- 支持单进程监听多个直播间
- 支持弹幕、礼物、点赞、进场、关注等常见消息
- 支持可选 Cookie，适配部分需要登录态的场景
- 可作为独立服务运行，也可作为 Go 库使用
- 内置断线重连和基础保活逻辑

## 它不是做什么的

这个项目主要是**直播间消息抓取 / 转发**，不是录播工具。

它不负责：

- 下载 flv / m3u8 视频流
- 录制直播画面
- 保存回放

如果你要的是录播，应该看录制类项目；如果你要的是实时弹幕、礼物、互动消息，这个项目更合适。

---

## 快速开始

### 方式一：直接下载可执行文件

1. 打开 [Releases](https://github.com/jwwsjlm/douyinLive/releases)
2. 下载对应平台的程序
3. 运行程序

发布包名称会带上版本号和构建 commit，格式类似：

```text
douyinLive-v2.0.3-abcdef123456-linux-amd64.tar.gz
douyinLive-v2.0.3-abcdef123456-windows-amd64.zip
```

当前只发布一个主版本：

- 默认使用本地 JS 计算 WebSocket 签名，普通用户不需要额外配置。
- 如果要使用 TikHub 在线 API 生成 WebSocket 签名，通过 `sign.provider`、`APP_SIGN_PROVIDER` 或 `--sign-provider` 在运行时切换，不需要下载单独版本。

压缩包里的可执行文件名仍然固定为 `douyinLive`，所以脚本和 Docker 启动命令不需要因为 hash 变化而每次修改。

也就是说，发布包文件名用于区分版本和构建来源；解压后的程序名保持固定：

- Linux / macOS：`douyinLive`
- Windows：`douyinLive.exe`

```bash
./douyinLive
```

程序启动后会在本地启动一个 WebSocket 服务，默认端口是 `1088`。

然后你的客户端连接：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

例如：

```text
ws://127.0.0.1:1088/ws/516466932480
```

### 方式二：源码编译

```bash
git clone https://github.com/jwwsjlm/douyinLive.git
cd douyinLive
go build -o douyinLive ./cmd/main
./douyinLive
```

查看当前二进制的构建信息：

```bash
./douyinLive --version
```

输出示例：

```text
tag=v2.0.3 commit=abcdef123456 buildDate=2026-05-24T00:00:00Z source=github-actions/release#123.1 signProvider=local
```

### 方式三：Docker 运行

#### 1. 直接启动最新版镜像

```bash
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:latest
```

程序启动后，对外提供的 WebSocket 地址仍然是：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

如果你需要固定版本，也可以直接拉指定 tag：

```bash
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:v2.0.3
```

测试版不会覆盖 `latest`。如果你要验证某个 beta 版本，请使用完整测试版 tag：

```bash
docker pull ghcr.io/jwwsjlm/douyinlive:v2.0.18-beta.1
docker run --rm -p 1088:1088 ghcr.io/jwwsjlm/douyinlive:v2.0.18-beta.1
```

Docker 镜像也支持查看构建信息：

```bash
docker run --rm ghcr.io/jwwsjlm/douyinlive:v2.0.3 --version
```

如果要使用 TikHub 在线签名，仍然使用同一个镜像，只需要在配置文件、环境变量或命令行里指定签名来源并提供 TikHub API Key：

```bash
docker run --rm -p 1088:1088 \
  -e APP_SIGN_PROVIDER=tikhub \
  -e APP_TIKHUB_KEY=YOUR_TIKHUB_KEY \
  ghcr.io/jwwsjlm/douyinlive:v2.0.3
```

#### 2. 通过 Docker 挂载 `config.yaml`

如果你希望加载自定义配置，先在宿主机准备一个 `config.yaml`，再把它挂载到容器中的 `/app/config.yaml`，并通过 `--config` 显式传入：

```bash
docker run --rm -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

说明：

- `-v $(pwd)/config.yaml:/app/config.yaml:ro`：把宿主机当前目录下的 `config.yaml` 挂载到容器内
- `:ro`：只读挂载，避免容器误改宿主机配置
- `--config /app/config.yaml`：显式指定程序读取这个配置文件

#### 3. 持久化运行（推荐长期使用）

如果你希望容器长期后台运行，不要使用 `--rm`，建议改成：

```bash
docker run -d \
  --name douyinlive \
  --restart unless-stopped \
  -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

这样即使容器被删除或重建，宿主机上的 `config.yaml` 仍然保留，达到配置持久化的效果。

#### 4. 挂载整个目录（适合后续扩展）

如果你后续不只想挂一个配置文件，也可以直接挂整个目录：

```bash
mkdir -p ./data
cp config.example.yaml ./data/config.yaml

docker run -d \
  --name douyinlive \
  --restart unless-stopped \
  -p 1088:1088 \
  -v $(pwd)/data:/app/data \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/data/config.yaml
```

这种方式更适合统一管理容器运行时使用到的文件。

#### 5. 使用 Docker Compose（推荐）

项目已自带两个 compose 示例文件：

- `compose.yaml`：挂载单个 `config.yaml`
- `compose.data.yaml`：挂载整个 `data` 目录

##### 方案 A：使用 `compose.yaml`

先准备配置文件：

```bash
cp config.example.yaml config.yaml
```

然后直接启动：

```bash
docker compose up -d
docker compose logs -f
docker compose down
```

`compose.yaml` 内容如下：

```yaml
services:
  douyinlive:
    image: ghcr.io/jwwsjlm/douyinlive:latest
    container_name: douyinlive
    restart: unless-stopped
    ports:
      - "1088:1088"
    volumes:
      - ./config.yaml:/app/config.yaml:ro
    command: ["--config", "/app/config.yaml"]
```

##### 方案 B：使用 `compose.data.yaml`

如果你想把配置统一收纳到目录里，先执行：

```bash
mkdir -p data
cp config.example.yaml data/config.yaml
```

然后用下面命令启动：

```bash
docker compose -f compose.data.yaml up -d
docker compose -f compose.data.yaml logs -f
docker compose -f compose.data.yaml down
```

`compose.data.yaml` 内容如下：

```yaml
services:
  douyinlive:
    image: ghcr.io/jwwsjlm/douyinlive:latest
    container_name: douyinlive
    restart: unless-stopped
    ports:
      - "1088:1088"
    volumes:
      - ./data:/app/data
    command: ["--config", "/app/data/config.yaml"]
```

此时你只需要保证宿主机存在：

```text
./data/config.yaml
```

#### 6. 常用查看命令

```bash
docker logs -f douyinlive
docker ps
docker stop douyinlive
docker rm -f douyinlive
```

---

## 最重要的一点：房间参数怎么传

很多人第一次用会卡在这里。

这个程序启动时**不需要**在命令行传直播间号。

直播间标识是通过 WebSocket 路径传进去的：

```text
ws://127.0.0.1:1088/ws/直播间标识
```

也就是说：

- 程序只负责启动本地服务
- 你连接哪个房间，是由 `/ws/后面的内容` 决定的

### 什么叫“直播间标识”

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

## 运行方式

### CLI 完整示例（推荐先看这里）

`douyinLive` 启动后是一个本地 WebSocket 服务。**直播间标识不是 CLI 启动参数**，而是客户端连接 WebSocket 时写在 URL 里。

#### Linux / macOS

```bash
cp config.example.yaml config.yaml
./douyinLive --config ./config.yaml --port 1088 --log-level info
```

然后让你的客户端连接：

```text
ws://127.0.0.1:1088/ws/516466932480
```

#### Windows PowerShell

```powershell
Copy-Item .\config.example.yaml .\config.yaml
.\douyinLive.exe --config .\config.yaml --port 1088 --log-level info
```

然后让你的客户端连接：

```text
ws://127.0.0.1:1088/ws/516466932480
```

### 默认启动

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

### 指定端口

```bash
./douyinLive --port 1088
```

Windows：

```powershell
.\douyinLive.exe --port 1088
```

### 指定配置文件

```bash
./douyinLive --config ./config.yaml
```

Windows：

```powershell
.\douyinLive.exe --config .\config.yaml
```

### 输出未知消息类型（调试用）

```bash
./douyinLive --unknown
```

Windows：

```powershell
.\douyinLive.exe --unknown
```

### 设置日志级别

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

日志使用 Go `slog` 文本格式，会带上 `level`、`time` 以及 `room_id`、`live_id`、`err` 等字段，方便长时间挂机时排查连接和重连状态。

### 查看版本和构建来源

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

### 设置签名来源

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

如果多个地方同时配置，以优先级最高的为准。`sign.provider=local` 时会使用内置本地 JS 签名，`tikhub.key` 即使存在也不会被使用；只有 `sign.provider=tikhub` 时才会调用 TikHub 在线 API，并且必须提供 `tikhub.key`。

TikHub API Key 可以在 [TikHub 注册页](https://user.tikhub.io/register) 注册账号后，到 [TikHub 用户中心](https://user.tikhub.io/) 创建 API Key / API Token。Key 属于敏感信息，不要提交到仓库。

### CLI 参数速查

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

## 配置文件

你可以创建一个 `config.yaml` 放在程序同目录下。

示例：

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
  douyin: ""
  rooms:
    # "516466932480": "ttwid=...; sessionid=..."
```

项目里也自带了一个示例文件：

- `config.example.yaml`

### 配置项说明

#### `port`
本地 WebSocket 服务端口。

默认值：

```yaml
port: "1088"
```

#### `unknown`
是否打印未知消息类型。

默认值：

```yaml
unknown: false
```

#### `log.level`

日志级别。默认输出 `info` 及以上级别，排查连接、心跳、重连问题时可以临时调整为 `debug`。

默认值：

```yaml
log:
  level: "info"
```

#### `sign.provider`

WebSocket 签名来源。可选值：

- `local`：使用内置本地 JS 签名，默认推荐。
- `tikhub`：使用 TikHub 在线 API 生成签名，需要配置 `tikhub.key`。

默认值：

```yaml
sign:
  provider: ""
```

留空表示使用当前二进制默认值，也就是 `local`。如果你想强制指定，也可以写成 `local` 或 `tikhub`。

#### `tikhub.key`

TikHub API Key，仅当 `sign.provider` 为 `tikhub` 时需要。

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

#### `monitor.poll_interval`
未开播时，服务端检查“是否已经开播”的时间间隔。

默认值：

```yaml
monitor:
  poll_interval: "15s"
```

#### `monitor.notify_interval`
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

#### `cookie.douyin`
抖音默认 Cookie，可选。

没有单独配置某个直播间的 Cookie 时，会优先回退到这里。再往后才是自动获取的逻辑。

```yaml
cookie:
  douyin: "ttwid=...; sessionid=..."
```

#### `cookie.rooms`
按直播间 ID 单独配置 Cookie，可选。

如果你要同时监听多个直播间，而且它们对应不同账号、不同登录态，就可以在这里分别配置。没有配置到的直播间，会自动回退使用 `cookie.douyin`。

```yaml
cookie:
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

### 什么时候需要 Cookie

不是所有场景都必须填 Cookie。

你可以先不填，直接跑。

如果出现下面这些情况，再考虑补 Cookie：

- 某些直播间拿不到消息
- 请求被限制
- 页面返回结果异常
- 需要更稳定的登录态

### Cookie 怎么拿

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

## 作为 Go 库集成使用

你也可以直接把 `douyinLive` 作为 Go 库集成到你自己的项目中。

### 安装

```bash
go get github.com/jwwsjlm/douyinLive/v2
```

### 订阅接口怎么选

新版本推荐使用 `LiveMessage` 相关订阅接口：

- `SubscribeMessage(handler)`：订阅所有抖音消息
- `SubscribeMethod(method, handler)`：只订阅一个消息类型
- `SubscribeMethods(methods, handler)`：订阅多个消息类型

消息类型由抖音 WebSocket 下发的 `method` 字段决定，例如 `WebcastChatMessage`、`WebcastGiftMessage`、`WebcastLikeMessage`。也就是说，订阅分发不是靠结构体类型猜测，而是先看 `method` 字符串，再把匹配到的消息交给对应 handler。

`LiveMessage` 会同时带上原始消息、已解析消息和直播间元信息：

```go
type LiveMessage struct {
	LiveID      string
	RoomID      string
	LiveName    string
	Title       string
	AvatarThumb string
	Raw         *new_douyin.Webcast_Im_Message
	Parsed      proto.Message
	ReceivedAt  time.Time
}
```

常用方法：

- `msg.GetMethod()`：获取消息类型
- `msg.GetPayload()`：获取 protobuf 原始 payload

如果你的项目使用 `log/slog`，可以直接用 `NewDouyinLiveWithSlog` 创建实例，日志会保留结构化级别和字段：

```go
dl, err := douyinlive.NewDouyinLiveWithSlog(roomID, slog.Default(), cookie)
```

如果你想在库模式下使用 TikHub 在线签名，可以改用 TikHub 构造器：

```go
dl, err := douyinlive.NewDouyinLiveWithTikHub(roomID, log.Default(), cookie, tikHubKey)
```

对应的 slog 构造器是：

```go
dl, err := douyinlive.NewDouyinLiveWithSlogAndTikHub(roomID, slog.Default(), cookie, tikHubKey)
```

### 生命周期和关闭方式

`Start()` 会阻塞当前 goroutine，直到直播连接结束、主动 `Close()` 或发生不可恢复错误。如果你的程序需要自己控制停止时机，建议把 `Start()` 放到 goroutine 里运行，然后在退出时调用 `Close()`。

`Close()` 表示主动停止当前实例。调用后不要再对同一个 `DouyinLive` 实例重新 `Start()`；如果要重新连接同一个直播间，重新创建一个新的 `DouyinLive` 实例即可。

`Dispose()` 适合“创建了实例但不再进入 `Start()`”的场景，比如只调用 `IsLive()` 做状态检查后就结束。已经正常进入 `Start()` 的实例，退出时内部会自动清理连接和缓存，通常只需要 `Close()`。

推荐的停止流程：

1. 业务层先标记自己的 `stopped` 状态，避免 handler 继续处理耗时任务
2. 调用 `Unsubscribe(id)` 取消订阅
3. 调用 `Close()` 停止直播连接
4. 等待 `Start()` 所在 goroutine 返回

`Unsubscribe()` 会阻止后续还没开始执行的回调继续触发；如果某个 handler 已经正在运行，Go 无法从外部强行中断它，所以 handler 里不要做长时间阻塞操作。确实需要耗时处理时，建议在 handler 内检查业务层的停止标记，或者把任务投递到你自己的队列里异步处理。

### 最简使用示例

```go
package main

import (
	"log"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
)

func main() {
	// 直播间ID，从 https://live.douyin.com/xxxx 获取
	roomID := "516466932480"
	// 可选 Cookie，如果需要登录态可以传入，留空表示不使用
	cookie := ""

	// 创建实例
	dl, err := douyinlive.NewDouyinLive(roomID, log.Default(), cookie)
	if err != nil {
		log.Fatalf("创建失败: %v", err)
		return
	}

	// 订阅所有抖音消息
	dl.SubscribeMessage(func(msg *douyinlive.LiveMessage) {
		log.Printf("收到消息 method=%s payload_len=%d live=%s\n",
			msg.GetMethod(),
			len(msg.GetPayload()),
			msg.LiveName,
		)
	})

	// 启动监听，会阻塞直到连接关闭
	if err := dl.Start(); err != nil {
		log.Printf("监听结束: %v", err)
	}
}
```

### 处理具体消息类型示例

```go
package main

import (
	"log"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func main() {
	roomID := "516466932480"
	dl, err := douyinlive.NewDouyinLive(roomID, log.Default(), "")
	if err != nil {
		log.Fatal(err)
	}

	dl.SubscribeMethod(douyinlive.WebcastChatMessage, func(msg *douyinlive.LiveMessage) {
		chat := &new_douyin.Webcast_Im_ChatMessage{}
		if err := proto.Unmarshal(msg.GetPayload(), chat); err != nil {
			log.Println(err)
			return
		}
		if chat.GetContent() != "" && chat.GetUser() != nil {
			log.Printf("弹幕 [%s]: %s\n", chat.GetUser().GetNickname(), chat.GetContent())
		}
	})

	dl.SubscribeMethods([]string{
		douyinlive.WebcastGiftMessage,
		douyinlive.WebcastLikeMessage,
	}, func(msg *douyinlive.LiveMessage) {
		switch msg.GetMethod() {
		case douyinlive.WebcastGiftMessage:
			gift := &new_douyin.Webcast_Im_GiftMessage{}
			if err := proto.Unmarshal(msg.GetPayload(), gift); err != nil {
				log.Println(err)
				return
			}
			if gift.GetUser() != nil && gift.GetGift() != nil {
				log.Printf("礼物: %s 赠送了 %s x%d\n",
					gift.GetUser().GetNickname(),
					gift.GetGift().GetName(),
					gift.GetCount(),
				)
			}

		case douyinlive.WebcastLikeMessage:
			like := &new_douyin.Webcast_Im_LikeMessage{}
			if err := proto.Unmarshal(msg.GetPayload(), like); err != nil {
				log.Println(err)
				return
			}
			if like.GetUser() != nil {
				log.Printf("%s 点赞了直播间\n", like.GetUser().GetNickname())
			}
		}
	})

	if err := dl.Start(); err != nil {
		log.Printf("监听结束: %v", err)
	}
}
```

更多消息类型可以参考 `generated/new_douyin` 包下的 protobuf 生成代码。

旧的 `Subscribe(func(raw, parsed))` 接口仍然保留，方便已有代码兼容；新代码建议优先使用 `SubscribeMessage` / `SubscribeMethod` / `SubscribeMethods`。

### 可主动停止的库模式示例

如果你的程序要在收到信号、用户退出或业务结束时主动停止监听，可以按下面这种方式组织：

```go
package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
)

func main() {
	dl, err := douyinlive.NewDouyinLive("516466932480", log.Default(), "")
	if err != nil {
		log.Fatal(err)
	}

	var stopped atomic.Bool
	subID := dl.SubscribeMessage(func(msg *douyinlive.LiveMessage) {
		if stopped.Load() {
			return
		}
		log.Printf("收到消息 method=%s\n", msg.GetMethod())
	})

	done := make(chan error, 1)
	go func() {
		done <- dl.Start()
	}()

	time.Sleep(30 * time.Second)
	stopped.Store(true)
	dl.Unsubscribe(subID)
	dl.Close()

	if err := <-done; err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("监听异常退出: %v", err)
	}
}
```

这里的关键点是：`Close()` 用来结束当前实例，`Unsubscribe()` 用来取消后续回调，`done` 用来等待 `Start()` 真正退出。不要在 `Close()` 后复用同一个实例重新 `Start()`。

---

## 客户端怎么接（独立服务模式）

如果你直接运行独立服务，你的客户端只需要连本地 WebSocket 服务即可。

### JavaScript 示例

```javascript
const ws = new WebSocket('ws://127.0.0.1:1088/ws/516466932480');

ws.onopen = () => {
  console.log('已连接');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('收到消息:', data);

  if (data.event === 'live_status') {
    if (data.live) {
      console.log('状态通知: 直播间已开播');
    } else if (data.ended) {
      console.log(`状态通知: ${data.message}，后续会继续按 ${data.retry_interval_seconds} 秒轮询`);
    } else {
      console.log(`状态通知: ${data.message}，${data.retry_interval_seconds} 秒后重试`);
    }
    return;
  }

  switch (data.method) {
    case 'WebcastChatMessage':
      console.log(`弹幕: ${data.user.nickname} - ${data.content}`);
      break;
    case 'WebcastGiftMessage':
      console.log(`礼物: ${data.user.nickname} 赠送了 ${data.gift.name}`);
      break;
    case 'WebcastLikeMessage':
      console.log(`${data.user.nickname} 点赞了直播间`);
      break;
    default:
      break;
  }
};

ws.onclose = () => {
  console.log('连接关闭');
};

ws.onerror = (err) => {
  console.error('WebSocket 错误:', err);
};

// 可选：给本地服务发文本 ping，服务会回文本 pong
setInterval(() => {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send('ping');
  }
}, 30000);
```

浏览器端不能直接发送 WebSocket ping 控制帧，所以示例里使用文本 `"ping"`。如果你的客户端库支持 WebSocket ping frame，也可以发送标准 ping 控制帧，服务端会按规范用相同 payload 回复 pong。

### 服务端返回什么格式

服务端会把解析后的 protobuf 消息转成 JSON 文本发给你。

不同消息类型字段不完全一样，但都会包含对应消息内容。

另外会额外补一个字段：

- `livename`：直播间名称
- `method`：抖音消息类型，例如 `WebcastChatMessage`
- `title`：直播间标题
- `avatarThumb`：主播头像缩略图地址

如果直播间还没开播，则会返回系统状态消息，例如：

```json
{"type":"system","event":"live_status","live":false,"room_id":"516466932480","message":"直播间未开播","retry_interval_seconds":30}
```

这条消息表示服务端正在后台监控开播状态，客户端不需要把它当成 fatal error，也不要因为 `live=false` 立刻断开连接。保持当前 WebSocket 连接即可。

检测到开播时，也会先返回一条状态消息：

```json
{"type":"system","event":"live_status","live":true,"room_id":"516466932480","message":"直播间已开播"}
```

如果直播过程中下播，也会先返回一条状态消息：

```json
{"type":"system","event":"live_status","live":false,"room_id":"516466932480","message":"直播间已下播","ended":true,"retry_interval_seconds":30}
```

---

## 项目结构

```text
douyinLive/
├── cmd/main/                 # 可执行程序入口
│   ├── main.go               # 主程序
│   ├── app.go                # HTTP / WebSocket 服务
│   ├── room.go               # 房间与客户端管理
│   ├── config.go             # 配置读取
│   └── WsHandler.go          # WebSocket 事件处理
├── douyin.go                 # 核心抓取逻辑，对外库接口
├── sign/                     # 签名与 Cookie 相关逻辑
├── jsScript/                 # 签名脚本
├── protobuf/                 # protobuf 定义
├── generated/                # 生成后的 protobuf 代码
├── utils/                    # 工具函数
├── config.example.yaml       # 配置示例
└── README.md
```

---

## 适合谁用

如果你需要：

- 获取抖音直播间实时弹幕
- 做自己的弹幕大屏
- 做直播互动统计
- 做礼物 / 点赞 / 关注监听
- 把抖音消息接进自己的系统

这个项目就比较合适。

---

## 致谢

本项目参考过这些项目和资料：

- [ihmily/DouyinLiveRecorder](https://github.com/ihmily/DouyinLiveRecorder)
- [saermart/DouyinLiveWebFetcher](https://github.com/saermart/DouyinLiveWebFetcher)
- [douyin_proto](https://github.com/Remember-the-past/douyin_proto)

感谢原作者们的公开分享。

---

## 许可证

[MIT](./LICENSE)

---

## 支持

如果这个项目对你有帮助，欢迎点个 Star。
