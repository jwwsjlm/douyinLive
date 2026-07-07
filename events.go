package douyinLive

import (
	"time"

	"github.com/jwwsjlm/douyinLive/v2/utils"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// emitEvent 触发事件，遍历处理所有有效处理器
// emitEvent 向旧版原始订阅者和新版标准化订阅者分发事件。
// emitEvent dispatches events to legacy raw subscribers and normalized message subscribers.
func (dl *DouyinLive) emitEvent(msg *new_douyin.Webcast_Im_Message, parsed proto.Message) {
	if msg == nil {
		return
	}

	dl.mu.Lock()
	handlers := append([]eventHandler(nil), dl.eventHandlers...)
	dl.mu.Unlock()

	parsedSnapshot := parsed
	if parsedSnapshot != nil {
		parsedSnapshot = proto.Clone(parsedSnapshot)
	}

	for _, handler := range handlers {
		if dl.isManualClose() {
			return
		}
		if !dl.hasEventHandler(handler.id) {
			continue
		}
		func(h eventHandler) {
			defer func() {
				if recovered := recover(); recovered != nil && dl.logger != nil {
					dl.logger.Error("旧事件处理器发生 panic", "live_id", dl.liveID, "panic", recovered)
				}
			}()
			h.handler(msg, parsed)
		}(handler)
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
	}, func() bool {
		return dl.isManualClose()
	})
}

// hasEventHandler 判断旧版订阅 ID 是否仍然有效。
// hasEventHandler reports whether a legacy subscription ID is still active.
func (dl *DouyinLive) hasEventHandler(id string) bool {
	if id == "" {
		return false
	}

	dl.mu.Lock()
	defer dl.mu.Unlock()

	for _, h := range dl.eventHandlers {
		if h.id == id {
			return true
		}
	}
	return false
}

// Subscribe 订阅事件，生成唯一ID
// Subscribe 订阅原始抖音消息和可选解析结果。
// Subscribe subscribes to raw Douyin messages and their optional parsed result.
func (dl *DouyinLive) Subscribe(handler func(*new_douyin.Webcast_Im_Message, proto.Message)) string {
	if handler == nil {
		return ""
	}

	id := utils.GenerateUniqueID()
	dl.mu.Lock()
	dl.eventHandlers = append(dl.eventHandlers, eventHandler{
		id:      id,
		handler: handler,
	})
	dl.mu.Unlock()
	return id
}

// Unsubscribe 取消订阅事件，通过ID查找并移除
// Unsubscribe 通过订阅 ID 取消原始消息或标准化消息订阅。
// Unsubscribe cancels a raw-message or normalized-message subscription by ID.
func (dl *DouyinLive) Unsubscribe(id string) {
	dl.eventBus().unsubscribe(id)

	dl.mu.Lock()
	defer dl.mu.Unlock()

	for i, h := range dl.eventHandlers {
		if h.id == id {
			dl.eventHandlers = append(dl.eventHandlers[:i], dl.eventHandlers[i+1:]...)
			break
		}
	}
}
