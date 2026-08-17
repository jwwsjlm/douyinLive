# 作为 Go 库使用

[返回项目首页](../README.md)

本文介绍 `CheckLiveStatus`、消息订阅、protobuf 类型、生命周期和资源释放。

项目可以直接嵌入其他 Go 程序，不需要启动本地 HTTP 服务。库模式下建议由调用方负责传入 `context.Context`、订阅消息并在退出时调用 `Close` 或 `Dispose`。

## 检查直播间是否开播

`CheckLiveStatus` 只执行直播页和 `web/enter` 状态检查，不会建立 WebSocket：

```go
package main

import (
    "context"
    "errors"
    "fmt"
    "log/slog"
    "time"

    douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func main() {
    if err := check(); err != nil {
        panic(err)
    }
}

func check() error {
    live, err := douyinLive.NewDouyinLiveWithSlog("516466932480", slog.Default(), "")
    if err != nil {
        return err
    }
    defer live.Dispose()

    ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
    defer cancel()

    status, err := live.CheckLiveStatus(ctx)
    if errors.Is(err, douyinLive.ErrLiveStatusUnknown) {
        fmt.Printf("暂时无法确认状态: %+v\n", status)
        return nil
    }
    if errors.Is(err, douyinLive.ErrRoomNotFound) {
        fmt.Printf("直播间不存在: %+v\n", status)
        return nil
    }
    if err != nil {
        return err
    }
    fmt.Printf("状态=%s 开播=%v 房间=%s\n", status.Code, *status.Live, status.RoomID)
    return nil
}
```

状态码含义：

- `online`：已确认正在直播；
- `offline`：已确认账号有房间但当前未开播；
- `account_no_room`：已确认账号存在，但当前没有房间；
- `not_found`：已明确判定直播间标识不存在，同时返回 `ErrRoomNotFound`；
- `unknown`：上游超时、风控页、接口异常或缺少可验证数据，同时返回 `ErrLiveStatusUnknown`。

`unknown` 不等于未开播，调用方应根据 error 进行重试，不要直接当作离线处理。

## 监听消息

```go
func run() error {
    live, err := douyinLive.NewDouyinLiveWithSlog("516466932480", slog.Default(), "")
    if err != nil {
        return err
    }
    defer live.Dispose()

    subscriptionID := live.SubscribeMessage(func(message *douyinLive.LiveMessage) {
        fmt.Println(message.GetMethod(), len(message.GetPayload()))
    })
    defer live.Unsubscribe(subscriptionID)

    return live.Start()
}
```

库模式注意事项：

- `Start()` 会阻塞当前 goroutine，建议由调用方放入独立 goroutine；
- 使用 `Close()` 停止当前监听，使用 `Dispose()` 释放 HTTP Client、缓存和签名 Runtime；
- `Close()` 和 `Dispose()` 可以重复调用；
- 不要访问 `DouyinLive` 的内部字段，房间名称、标题和头像通过 `GetName`、`GetTitle`、`GetAvatarThumb` 获取；
- `SubscribeMessage` 会自动按消息方法分发，若只关心一种消息可使用 `SubscribeMethod`。

你也可以直接把 `douyinLive` 作为 Go 库集成到你自己的项目中。

## 安装

```bash
go get github.com/jwwsjlm/douyinLive/v2
```

## Protobuf 类型来源

当前 protobuf 定义和生成代码已经从主仓库拆到单独仓库维护：

```text
github.com/jwwsjlm/douyinlive-proto
```

如果你只是把本项目当成本地 WebSocket 服务使用，不需要额外处理。服务端会继续把解析后的消息转成 JSON 发给客户端。

如果你把本项目当 Go 库使用，并且需要自己解析 `LiveMessage.GetPayload()`，请使用新的 protobuf import 路径：

```go
import (
	"github.com/jwwsjlm/douyinlive-proto/generated"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
)
```

旧版本里如果使用过下面这种路径：

```go
import "github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
```

需要改成：

```go
import "github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
```

`new_douyin` 的 protobuf schema 没有因为拆仓库改变，抖音下发的二进制 payload 解析方式也不变；变化的是 Go 代码的 import 路径。升级后建议执行：

```bash
go get github.com/jwwsjlm/douyinLive/v2@latest
go get github.com/jwwsjlm/douyinlive-proto@latest
go mod tidy
```

## 订阅接口怎么选

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

## 生命周期和关闭方式

`Start()` 会阻塞当前 goroutine，直到直播连接结束、主动 `Close()` 或发生不可恢复错误。如果你的程序需要自己控制停止时机，建议把 `Start()` 放到 goroutine 里运行，然后在退出时调用 `Close()`。

`Close()` 表示主动停止当前实例。调用后不要再对同一个 `DouyinLive` 实例重新 `Start()`；如果要重新连接同一个直播间，重新创建一个新的 `DouyinLive` 实例即可。

`Dispose()` 适合“创建了实例但不再进入 `Start()`”的场景，比如只调用 `IsLive()` 做状态检查后就结束。已经正常进入 `Start()` 的实例，退出时内部会自动清理连接和缓存，通常只需要 `Close()`。

推荐的停止流程：

1. 业务层先标记自己的 `stopped` 状态，避免 handler 继续处理耗时任务
2. 调用 `Unsubscribe(id)` 取消订阅
3. 调用 `Close()` 停止直播连接
4. 等待 `Start()` 所在 goroutine 返回

`Unsubscribe()` 会阻止后续还没开始执行的回调继续触发；如果某个 handler 已经正在运行，Go 无法从外部强行中断它，所以 handler 里不要做长时间阻塞操作。确实需要耗时处理时，建议在 handler 内检查业务层的停止标记，或者把任务投递到你自己的队列里异步处理。

## 最简使用示例

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

## 处理具体消息类型示例

```go
package main

import (
	"log"

	douyinlive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
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

更多消息类型可以参考 [`github.com/jwwsjlm/douyinlive-proto/generated/new_douyin`](https://github.com/jwwsjlm/douyinlive-proto/tree/main/generated/new_douyin) 包下的 protobuf 生成代码。

旧的 `Subscribe(func(raw, parsed))` 接口仍然保留，方便已有代码兼容；新代码建议优先使用 `SubscribeMessage` / `SubscribeMethod` / `SubscribeMethods`。

## 可主动停止的库模式示例

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
