# douyinLive

一个基于 WebSocket 的抖音直播弹幕抓取工具。

它做的事很简单：

1. 连接抖音直播间消息流
2. 解析弹幕 / 礼物 / 点赞 / 进场等消息
3. 再通过你本地启动的 WebSocket 服务把消息转发给你的客户端

适合两种用法：

- 直接当成一个本地 WebSocket 服务用
- 当成 Go 库集成到你自己的项目里

[![GitHub Release](https://img.shields.io/github/v/release/jwwsjlm/douyinLive)](https://github.com/jwwsjlm/douyinLive/releases)
[![License](https://img.shields.io/github/license/jwwsjlm/douyinLive)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.25.7-blue)](https://golang.org)

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

如果你要排查内存、goroutine 或 CPU 问题，可以开启内置 `pprof`：

```bash
./douyinLive --pprof
```

或指定端口：

```bash
./douyinLive --pprof --pprof-port 6060
```

程序启动后可访问：

```text
http://127.0.0.1:6060/debug/pprof/
```

常用命令：

```bash
go tool pprof http://127.0.0.1:6060/debug/pprof/heap
go tool pprof http://127.0.0.1:6060/debug/pprof/allocs
go tool pprof http://127.0.0.1:6060/debug/pprof/goroutine
```

也可以在 `config.yaml` 中配置：

```yaml
pprof:
  enabled: true
  port: "6060"
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
  -v $(pwd)/data:/app \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
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
      - ./data:/app
    command: ["--config", "/app/config.yaml"]
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

---

## 运行方式

### 默认启动

```bash
./douyinLive
```

默认行为：

- 读取同目录下的 `config.yaml`（如果存在）
- 如果没有配置文件，就使用默认值
- 默认端口：`1088`

### 指定端口

```bash
./douyinLive --port 1088
```

### 指定配置文件

```bash
./douyinLive --config ./config.yaml
```

### 输出未知消息类型（调试用）

```bash
./douyinLive --unknown
```

---

## 配置文件

你可以创建一个 `config.yaml` 放在程序同目录下。

示例：

```yaml
port: "1088"
unknown: false
monitor:
  poll_interval: "15s"
  notify_interval: "30s"
cookie:
  douyin: ""
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
抖音 Cookie，可选。

默认不填。

```yaml
cookie:
  douyin: "ttwid=...; sessionid=..."
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

### 最简使用示例

```go
package main

import (
	"log"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
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

	// 订阅事件，所有抖音消息都会通过这个回调推送过来
	dl.Subscribe(func(msg *new_douyin.Webcast_Im_Message) {
		// 根据 msg.Method 判断消息类型，然后反序列化处理
		log.Printf("收到消息 method=%s payload_len=%d\n", msg.Method, len(msg.Payload))
		// 你可以在这里根据不同消息类型做相应处理
	})

	// 启动监听，会阻塞直到连接关闭
	dl.Start()
}
```

### 处理具体消息类型示例

```go
package main

import (
	"log"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/v2/generated/douyin"
	"google.golang.org/protobuf/proto"
)

func main() {
	roomID := "516466932480"
	dl, err := douyinlive.NewDouyinLive(roomID, log.Default(), "")
	if err != nil {
		log.Fatal(err)
	}

	dl.Subscribe(func(msg *new_douyin.Webcast_Im_Message) {
		switch msg.Method {
		case "WebcastChatMessage":
			chat := &douyin.WebcastChatMessage{}
			if err := proto.Unmarshal(msg.Payload, chat); err != nil {
				log.Println(err)
				return
			}
			// chat.Content 就是弹幕内容
			// chat.User 就是发送用户信息
			if chat.GetContent() != "" && chat.GetUser() != nil {
				log.Printf("弹幕 [%s]: %s\n", chat.User.GetNickname(), chat.GetContent())
			}

		case "WebcastGiftMessage":
			gift := &douyin.WebcastGiftMessage{}
			if err := proto.Unmarshal(msg.Payload, gift); err != nil {
				log.Println(err)
				return
			}
			log.Printf("礼物: %s 赠送了 %s x%d\n",
				gift.GetUser().GetNickname(),
				gift.GetGift().GetName(),
				gift.GetGiftCount(),
			)

		case "WebcastLikeMessage":
			like := &douyin.WebcastLikeMessage{}
			if err := proto.Unmarshal(msg.Payload, like); err != nil {
				log.Println(err)
				return
			}
			log.Printf("%s 点赞了直播间\n", like.GetUser().GetNickname())
		}
	})

	dl.Start()
}
```

更多消息类型可以参考 `generated/douyin` 包下的 protobuf 生成代码。

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

// 可选：给本地服务发 ping，服务会回 pong
setInterval(() => {
  if (ws.readyState === WebSocket.OPEN) {
    ws.send('ping');
  }
}, 30000);
```

### 服务端返回什么格式

服务端会把解析后的 protobuf 消息转成 JSON 文本发给你。

不同消息类型字段不完全一样，但都会包含对应消息内容。

另外会额外补一个字段：

- `livename`：直播间名称

如果直播间还没开播，则会返回系统状态消息，例如：

```json
{"type":"system","event":"live_status","live":false,"room_id":"516466932480","message":"直播间未开播","retry_interval_seconds":30}
```

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
