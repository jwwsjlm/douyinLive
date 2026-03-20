# 抖音直播弹幕抓取工具

🎬 基于 WebSocket 的抖音直播实时弹幕抓取工具，支持弹幕、礼物、点赞等多种消息类型。

[![GitHub Release](https://img.shields.io/github/v/release/jwwsjlm/douyinLive)](https://github.com/jwwsjlm/douyinLive/releases)
[![License](https://img.shields.io/github/license/jwwsjlm/douyinLive)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-%3E%3D1.21-blue)](https://golang.org)

---

## ✨ 功能特性

- 🚀 **实时监控** - WebSocket 推送，毫秒级延迟
- 📊 **多房间支持** - 单进程监控多个直播间
- 🎁 **完整数据** - 弹幕、礼物、点赞、关注、进场等消息
- 🔧 **灵活配置** - 配置文件 + 命令行参数双支持
- 💡 **友好提示** - 详细的错误提示和解决方法
- 🛠️ **易于集成** - JSON 格式输出，方便二次开发
- 🔑 **Cookie 支持** - 支持手动填入抖音 Cookie，解决需要登录才能获取弹幕的问题

---

## 🚀 快速开始

### 方式一：下载编译好的程序（推荐）

1. 从 [Releases](https://github.com/jwwsjlm/douyinLive/releases) 下载最新版本
2. 在同目录创建 `config.yaml` 配置文件（可选，用于设置默认端口和 Cookie）：
   ```yaml
   # WebSocket 服务端口（默认：1088）
   port: 1088

   # 是否输出未知类型消息（默认：false）
   unknown: false

   # Cookie 配置（可选，需要登录才能获取弹幕时填入）
   # 获取方式：浏览器打开 https://live.douyin.com/ -> F12 -> Network -> 复制任意请求的 Cookie
   cookie:
     # 抖音 Cookie
     douyin: "ttwid=1%7C...;..."
   ```
3. 运行程序：
   ```bash
   ./douyinLive
   ```
4. **连接 WebSocket**（重要！房间号从这里指定）：
   ```
   ws://127.0.0.1:1088/ws/直播间号
   ```

### 方式二：命令行启动

```bash
# 启动服务
./douyinLive --port 1088

# 连接 WebSocket（房间号从 URL 指定）
ws://127.0.0.1:1088/ws/516466932480
```

### 方式三：源码编译

```bash
# 环境要求：Go 1.21+
git clone https://github.com/jwwsjlm/douyinLive.git
cd douyinLive
go build -o douyinLive ./cmd/main
```

### 作为库使用（二次开发）

```go
package main

import (
	"fmt"
	"log"

	douyinLive "github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
)

func main() {
	// 创建实例，第三个参数是可选 Cookie；不需要时传空字符串即可
	dl, err := douyinLive.NewDouyinLive("912218533434", log.Default(), "")
	if err != nil {
		panic(err)
	}

	// 先订阅，再启动抓取
	dl.Subscribe(func(event *new_douyin.Webcast_Im_Message) {
		fmt.Printf("message: %+v\n", event)
	})

	go dl.Start()

	select {}
}
```

---

## 📖 使用说明

### 连接 WebSocket

服务启动后，客户端可以通过 WebSocket 连接到服务：

```
ws://127.0.0.1:1088/ws/直播间号
```

**JavaScript 示例**：
```javascript
const ws = new WebSocket('ws://127.0.0.1:1088/ws/516466932480');

ws.onmessage = (event) => {
    const data = JSON.parse(event.data);
    console.log('收到消息:', data);
    
    // 根据 data.method 区分消息类型
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
    }
};

// 心跳（每 30 秒发送一次 ping）
setInterval(() => {
    ws.send('ping');
}, 30000);

ws.onclose = () => {
    console.log('连接关闭');
};

ws.onerror = (err) => {
    console.error('WebSocket 错误: ', err);
};
```

---

## 📡 消息类型

支持的消息类型（持续更新中）：

| 类型                          | 说明               |
|-----------------------------|------------------|
| `WebcastChatMessage`        | 弹幕消息           |
| `WebcastGiftMessage`        | 礼物消息           |
| `WebcastLikeMessage`        | 点赞消息           |
| `WebcastMemberMessage`      | 进场消息           |
| `WebcastSocialMessage`      | 关注消息           |
| `WebcastFansclubMessage`    | 粉丝团消息         |
| `WebcastControlMessage`     | 开播/下播控制       |
| `WebcastEmojiChatMessage`   | 表情弹幕           |
| `WebcastRoomStatsMessage`   | 直播间统计         |
| `WebcastRoomUserSeqMessage` | 在线观众列表        |
| `WebcastRankMessage`        | 红包排名           |

---

## ⚙️ 配置说明

### 配置文件 (`config.yaml`)

```yaml
# WebSocket 服务端口（默认：1088）
port: 1088

# 是否输出未知类型消息（默认：false）
unknown: false

# Cookie 配置（可选，需要登录才能获取弹幕时填入）
# 获取方式：浏览器打开 https://live.douyin.com/ -> F12 -> Network -> 复制任意请求的 Cookie
cookie:
  douyin: "ttwid=1%7C...;..."
```

### 命令行参数

| 参数        | 说明                 | 默认值        | 是否必须 |
|------------|----------------------|--------------|----------|
| `--port`   | WebSocket 服务端口    | `1088`       | 否       |
| `--unknown`| 是否输出未知类型消息   | `false`      | 否       |
| `--config` | 指定配置文件路径       | `config.yaml`| 否       |

**重要提示：** `room` 参数只在启动时用来验证配置有效性，实际运行时房间号从 WebSocket URL 中获取！

---

## 🛠️ 最近本地优化

最近这一版在 `main` 分支上做了一轮偏稳定性的优化，重点包括：

- 优化房间关闭、客户端断开、重连时的锁范围，减少死锁风险
- 修正 WebSocket 连接失败时的异常处理，避免空指针
- 统一 `User-Agent` 与 Cookie 处理逻辑，减少重复代码
- 调整事件订阅与分发逻辑，增强并发安全
- 优化优雅退出流程，正常关闭时不再把 `http: Server closed` 误判为异常

这些优化不影响原有使用方式，主要提升稳定性和可维护性。

---

## 🎯 项目结构

```
douyinLive/
├── cmd/
│   └── main/
│       ├── main.go          # 程序入口
│       ├── config.go        # 配置读取
│       ├── app.go           # 应用核心
│       ├── room.go          # 房间管理
│       └── WsHandler.go     # WebSocket 处理
├── protobuf/                # Protobuf 定义
├── generated/               # 生成的 Go 代码
├── utils/                   # 工具函数
├── jsScript/                # JavaScript 签名脚本
├── sign/                    # 签名 / Cookie 相关逻辑
├── douyin.go                # 核心库，对外接口
├── README.md                # 说明文档
```

---

## 🙏 致谢

本项目灵感/参考来自：
- [ihmily/DouyinLiveRecorder](https://github.com/ihmily/DouyinLiveRecorder) - 提供了 Cookie 配置方案参考
- [saermar/DouyinLiveWebFetcher](https://github.com/saermar/DouyinLiveWebFetcher) - 最初 Python 版本灵感
- [douyin_proto](https://github.com/Remember-the-past/douyin_proto) - Protobuf 定义参考

感谢以上作者的无私分享！

---

## 🐛 问题反馈

遇到问题？欢迎提 [Issue](https://github.com/jwwsjlm/douyinLive/issues)！

---

## 📄 许可证

[MIT](./LICENSE)

---

## ⭐ 支持这个项目

如果这个项目对你有帮助，请点 **Star** ⭐ 支持一下！感谢！
