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

### 方式三：Docker 运行

直接启动最新版镜像：

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

如果你需要自定义配置，可以挂载自己的配置文件：

```bash
docker run --rm -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

如果你希望容器退出后自动删除，就保留 `--rm`；如果你希望长期后台运行，可以改成：

```bash
docker run -d \
  --name douyinlive \
  --restart unless-stopped \
  -p 1088:1088 \
  -v $(pwd)/config.yaml:/app/config.yaml:ro \
  ghcr.io/jwwsjlm/douyinlive:latest --config /app/config.yaml
```

常用查看命令：

```bash
docker logs -f douyinlive
docker ps
docker stop douyinlive
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

如果你传的是无效标识，或者直播间未开播，服务端会关闭这个连接。

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

## 客户端怎么接

你的客户端只需要连本地 WebSocket 服务即可。

### JavaScript 示例

```javascript
const ws = new WebSocket('ws://127.0.0.1:1088/ws/516466932480');

ws.onopen = () => {
  console.log('已连接');
};

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  console.log('收到消息:', data);

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

---

## 支持的消息类型

目前常见的包括：

- `WebcastChatMessage`：弹幕消息
- `WebcastGiftMessage`：礼物消息
- `WebcastLikeMessage`：点赞消息
- `WebcastMemberMessage`：进场消息
- `WebcastSocialMessage`：关注消息
- `WebcastFansclubMessage`：粉丝团消息
- `WebcastControlMessage`：开播 / 下播控制消息
- `WebcastEmojiChatMessage`：表情弹幕
- `WebcastRoomStatsMessage`：直播间统计
- `WebcastRoomUserSeqMessage`：在线观众序列
- `WebcastRankMessage`：排行相关消息

项目会持续补充更多类型。

---

## 作为 Go 库使用

如果你不想跑本地 WebSocket 服务，也可以直接把它当库引入。

```go
package main

import (
	"fmt"
	"log"

	douyinLive "github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
)

func main() {
	dl, err := douyinLive.NewDouyinLive("516466932480", log.Default(), "")
	if err != nil {
		panic(err)
	}

	dl.Subscribe(func(event *new_douyin.Webcast_Im_Message) {
		fmt.Printf("method=%s payload_len=%d\n", event.Method, len(event.Payload))
	})

	go dl.Start()

	select {}
}
```

第三个参数是 Cookie：

- 不需要时传 `""`
- 需要登录态时传完整 Cookie 字符串

---

## 常见问题

### 1）程序启动了，但连接直播间失败
先检查：

- 直播间是否真的开播
- `/ws/后面的标识` 是否正确
- 是否需要补 Cookie

### 2）没有配置文件能不能跑
可以。

没有 `config.yaml` 时，会使用默认配置。

### 3）端口被占用了怎么办
程序会从你指定端口开始往后找可用端口。

例如你设置了 `1088`，如果被占用，程序可能实际监听到 `1089`、`1090` 等端口。

以程序日志输出为准。

### 4）为什么客户端发 `ping` 会收到 `pong`
这是本地 WebSocket 服务做的简单心跳响应，用来方便前端检测连接是否还活着。

### 5）这个项目稳不稳
这个项目最近做过一轮重连、关闭流程、保活方面的优化，但抖音侧接口和风控本身会变化，所以稳定性仍然会受：

- 直播间类型
- Cookie 状态
- 风控策略
- 抖音页面 / 参数变化

影响。

如果你遇到特定房间更容易掉线，欢迎提 issue。

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
- [saermar/DouyinLiveWebFetcher](https://github.com/saermar/DouyinLiveWebFetcher)
- [douyin_proto](https://github.com/Remember-the-past/douyin_proto)

感谢原作者们的公开分享。

---

## 许可证

[MIT](./LICENSE)

---

## 支持

如果这个项目对你有帮助，欢迎点个 Star。