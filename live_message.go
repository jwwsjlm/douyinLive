package douyinLive

import (
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/utils"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// LiveMessage 封装抖音原始直播消息以及房间元数据。
// LiveMessage wraps a raw Douyin webcast message with room metadata.
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

// GetMethod 返回抖音直播消息的方法名。
// GetMethod returns the Douyin webcast method name.
func (m *LiveMessage) GetMethod() string {
	if m == nil || m.Raw == nil {
		return ""
	}
	return m.Raw.Method
}

// GetPayload 返回消息的原始 protobuf 载荷。
// GetPayload returns the raw protobuf payload for the message.
func (m *LiveMessage) GetPayload() []byte {
	if m == nil || m.Raw == nil {
		return nil
	}
	return m.Raw.Payload
}

// LiveMessageHandler 处理标准化后的直播消息。
// LiveMessageHandler consumes normalized live messages.
type LiveMessageHandler func(*LiveMessage)

// messageSubscriber 保存原始或标准化消息订阅者。
// messageSubscriber stores either a raw or normalized message subscriber.
type messageSubscriber struct {
	id      string
	handler LiveMessageHandler
	legacy  func(*new_douyin.Webcast_Im_Message, proto.Message)
	methods map[string]struct{}
}

// messageBus 管理原始和标准化直播消息的订阅与分发。
// messageBus manages subscription and dispatch for raw and normalized live messages.
type messageBus struct {
	mu          sync.RWMutex
	subscribers []messageSubscriber
}

// newMessageBus 创建一个空的消息总线。
// newMessageBus creates an empty message bus.
func newMessageBus() *messageBus {
	return &messageBus{}
}

// eventBus 返回实例级消息总线，必要时延迟初始化。
// eventBus returns the instance message bus and lazily initializes it when needed.
func (dl *DouyinLive) eventBus() *messageBus {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if dl.events == nil {
		dl.events = newMessageBus()
	}
	return dl.events
}

// SubscribeMessage 订阅所有标准化直播消息。
// SubscribeMessage subscribes to all normalized live messages.
// 参数/Parameters:
//   - handler: 接收标准化直播消息的回调。 Callback that receives normalized live messages.
func (dl *DouyinLive) SubscribeMessage(handler LiveMessageHandler) string {
	return dl.eventBus().subscribe(handler)
}

// SubscribeMethod 订阅单个抖音直播消息方法，例如 WebcastChatMessage。
// SubscribeMethod subscribes to one Douyin webcast method, for example WebcastChatMessage.
// 参数/Parameters:
//   - method: 要订阅的抖音消息方法名。 Douyin message method to subscribe to.
//   - handler: 接收匹配消息的回调。 Callback that receives matching messages.
func (dl *DouyinLive) SubscribeMethod(method string, handler LiveMessageHandler) string {
	return dl.eventBus().subscribe(handler, method)
}

// SubscribeMethods 订阅一组抖音直播消息方法。
// SubscribeMethods subscribes to a set of Douyin webcast methods.
// 参数/Parameters:
//   - methods: 要订阅的抖音消息方法名列表。 Douyin message methods to subscribe to.
//   - handler: 接收匹配消息的回调。 Callback that receives matching messages.
func (dl *DouyinLive) SubscribeMethods(methods []string, handler LiveMessageHandler) string {
	if len(methods) == 0 {
		return ""
	}
	return dl.eventBus().subscribe(handler, methods...)
}

// subscribe 注册一个标准化消息订阅者并返回订阅 ID。
// subscribe registers a normalized-message subscriber and returns its subscription ID.
// 参数/Parameters:
//   - handler: 订阅回调。 Subscription callback.
//   - methods: 可选方法过滤列表；为空表示接收全部消息。 Optional method filters; empty means all messages.
func (b *messageBus) subscribe(handler LiveMessageHandler, methods ...string) string {
	if handler == nil {
		return ""
	}

	methodSet := make(map[string]struct{}, len(methods))
	for _, method := range methods {
		if method != "" {
			methodSet[method] = struct{}{}
		}
	}
	if methods != nil && len(methodSet) == 0 {
		return ""
	}
	if len(methodSet) == 0 {
		methodSet = nil
	}

	return b.add(messageSubscriber{
		handler: handler,
		methods: methodSet,
	})
}

func (b *messageBus) subscribeLegacy(handler func(*new_douyin.Webcast_Im_Message, proto.Message)) string {
	if handler == nil {
		return ""
	}
	return b.add(messageSubscriber{legacy: handler})
}

func (b *messageBus) add(subscriber messageSubscriber) string {
	subscriber.id = utils.GenerateUniqueID()
	b.mu.Lock()
	b.subscribers = append(b.subscribers, subscriber)
	b.mu.Unlock()
	return subscriber.id
}

// unsubscribe 按订阅 ID 移除消息订阅者。
// unsubscribe removes a message subscriber by subscription ID.
// 参数/Parameters:
//   - id: 要取消的订阅 ID。 Subscription ID to remove.
func (b *messageBus) unsubscribe(id string) {
	if id == "" {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	for i, subscriber := range b.subscribers {
		if subscriber.id == id {
			b.subscribers = append(b.subscribers[:i], b.subscribers[i+1:]...)
			return
		}
	}
}

// hasSubscriber 判断订阅 ID 当前是否仍然有效。
// hasSubscriber reports whether a subscription ID is still active.
// 参数/Parameters:
//   - id: 要检查的订阅 ID。 Subscription ID to check.
func (b *messageBus) hasSubscriber(id string) bool {
	if id == "" {
		return false
	}

	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, subscriber := range b.subscribers {
		if subscriber.id == id {
			return true
		}
	}
	return false
}

// publishWithLoggerUntil 先分发旧版原始消息，再分发标准化消息，并在停止条件触发时中止。
// publishWithLoggerUntil dispatches legacy raw handlers before normalized handlers and stops when requested.
// 参数/Parameters:
//   - logger: 用于记录订阅回调 panic 的日志器。 Logger used to record subscriber callback panics.
//   - message: 要分发的标准化直播消息。 Normalized live message to dispatch.
//   - legacyParsed: 传给旧版回调的原始解析结果。 Original parsed payload passed to legacy callbacks.
//   - stop: 可选停止条件；返回 true 时中止分发。 Optional stop condition; true aborts dispatch.
func (b *messageBus) publishWithLoggerUntil(logger logSink, message *LiveMessage, legacyParsed proto.Message, stop func() bool) {
	if message == nil {
		return
	}

	b.mu.RLock()
	subscribers := append([]messageSubscriber(nil), b.subscribers...)
	b.mu.RUnlock()

	for _, legacy := range []bool{true, false} {
		for _, subscriber := range subscribers {
			if stop != nil && stop() {
				return
			}
			if !b.hasSubscriber(subscriber.id) || (subscriber.legacy != nil) != legacy {
				continue
			}
			if !legacy && !subscriber.accepts(message.GetMethod()) {
				continue
			}
			subscriber.publish(logger, message, legacyParsed)
		}
	}
}

func (s messageSubscriber) publish(logger logSink, message *LiveMessage, legacyParsed proto.Message) {
	defer func() {
		if recovered := recover(); recovered != nil && logger != nil {
			logger.Error("消息订阅处理器发生 panic", "method", message.GetMethod(), "legacy", s.legacy != nil, "panic", recovered)
		}
	}()
	if s.legacy != nil {
		s.legacy(message.Raw, legacyParsed)
		return
	}
	s.handler(message)
}

// accepts 判断订阅者是否接收指定方法名的消息。
// accepts reports whether the subscriber accepts messages for the given method.
// 参数/Parameters:
//   - method: 待判断的抖音消息方法名。 Douyin message method to test.
func (s messageSubscriber) accepts(method string) bool {
	if len(s.methods) == 0 {
		return true
	}
	_, ok := s.methods[method]
	return ok
}
