package douyinLive

import (
	"time"

	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// emitEvent 向旧版原始订阅者和新版标准化订阅者分发事件。
// emitEvent dispatches events to legacy raw subscribers and normalized message subscribers.
// 参数/Parameters:
//   - msg: 抖音原始 protobuf 消息。 Raw Douyin protobuf message.
//   - parsed: 可选的解析后 protobuf 消息。 Optional parsed protobuf payload.
func (dl *DouyinLive) emitEvent(msg *new_douyin.Webcast_Im_Message, parsed proto.Message) {
	if msg == nil {
		return
	}

	parsedSnapshot := parsed
	if parsedSnapshot != nil {
		parsedSnapshot = proto.Clone(parsedSnapshot)
	}

	roomInfo := dl.roomInfoSnapshot()
	dl.eventBus().publishWithLoggerUntil(dl.logger, &LiveMessage{
		LiveID:      roomInfo.liveID,
		RoomID:      roomInfo.roomID,
		LiveName:    roomInfo.liveName,
		Title:       roomInfo.title,
		AvatarThumb: roomInfo.avatarThumb,
		Raw:         msg,
		Parsed:      parsedSnapshot,
		ReceivedAt:  time.Now(),
	}, parsed, func() bool {
		return dl.isManualClose()
	})
}

// Subscribe 订阅原始抖音消息和可选解析结果。
// Subscribe subscribes to raw Douyin messages and their optional parsed result.
// 参数/Parameters:
//   - handler: 接收原始消息和可选解析结果的回调。 Callback receiving the raw message and optional parsed payload.
func (dl *DouyinLive) Subscribe(handler func(*new_douyin.Webcast_Im_Message, proto.Message)) string {
	return dl.eventBus().subscribeLegacy(handler)
}

// Unsubscribe 通过订阅 ID 取消原始消息或标准化消息订阅。
// Unsubscribe cancels a raw-message or normalized-message subscription by ID.
// 参数/Parameters:
//   - id: Subscribe 或标准化订阅方法返回的订阅 ID。 Subscription ID returned by Subscribe or normalized subscription APIs.
func (dl *DouyinLive) Unsubscribe(id string) {
	dl.eventBus().unsubscribe(id)
}
