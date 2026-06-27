package douyinLive

import (
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/v2/utils"
	"google.golang.org/protobuf/proto"
)

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

// GetMethod returns the Douyin webcast method name.
func (m *LiveMessage) GetMethod() string {
	if m == nil || m.Raw == nil {
		return ""
	}
	return m.Raw.Method
}

// GetPayload returns the raw protobuf payload for the message.
func (m *LiveMessage) GetPayload() []byte {
	if m == nil || m.Raw == nil {
		return nil
	}
	return m.Raw.Payload
}

// LiveMessageHandler consumes normalized live messages.
type LiveMessageHandler func(*LiveMessage)

type eventHandler struct {
	id      string
	handler func(*new_douyin.Webcast_Im_Message, proto.Message)
}

type messageSubscriber struct {
	id      string
	handler LiveMessageHandler
	methods map[string]struct{}
}

type messageBus struct {
	mu          sync.RWMutex
	subscribers []messageSubscriber
}

func newMessageBus() *messageBus {
	return &messageBus{}
}

func (dl *DouyinLive) eventBus() *messageBus {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if dl.events == nil {
		dl.events = newMessageBus()
	}
	return dl.events
}

// SubscribeMessage subscribes to normalized live messages.
func (dl *DouyinLive) SubscribeMessage(handler LiveMessageHandler) string {
	return dl.eventBus().subscribe(handler)
}

// SubscribeMethod subscribes to one Douyin webcast method, for example WebcastChatMessage.
func (dl *DouyinLive) SubscribeMethod(method string, handler LiveMessageHandler) string {
	return dl.eventBus().subscribe(handler, method)
}

// SubscribeMethods subscribes to a set of Douyin webcast methods.
func (dl *DouyinLive) SubscribeMethods(methods []string, handler LiveMessageHandler) string {
	if len(methods) == 0 {
		return ""
	}
	return dl.eventBus().subscribe(handler, methods...)
}

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

	subscriber := messageSubscriber{
		id:      utils.GenerateUniqueID(),
		handler: handler,
		methods: methodSet,
	}

	b.mu.Lock()
	b.subscribers = append(b.subscribers, subscriber)
	b.mu.Unlock()

	return subscriber.id
}

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

func (b *messageBus) publish(message *LiveMessage) {
	b.publishWithLogger(nil, message)
}

func (b *messageBus) publishWithLogger(logger logSink, message *LiveMessage) {
	b.publishWithLoggerUntil(logger, message, nil)
}

func (b *messageBus) publishWithLoggerUntil(logger logSink, message *LiveMessage, stop func() bool) {
	if message == nil {
		return
	}

	b.mu.RLock()
	subscribers := append([]messageSubscriber(nil), b.subscribers...)
	b.mu.RUnlock()

	for _, subscriber := range subscribers {
		if stop != nil && stop() {
			return
		}
		if !b.hasSubscriber(subscriber.id) {
			continue
		}
		if !subscriber.accepts(message.GetMethod()) {
			continue
		}
		func(s messageSubscriber) {
			defer func() {
				if recovered := recover(); recovered != nil && logger != nil {
					logger.Error("消息订阅处理器发生 panic", "method", message.GetMethod(), "panic", recovered)
				}
			}()
			s.handler(message)
		}(subscriber)
	}
}

func (s messageSubscriber) accepts(method string) bool {
	if len(s.methods) == 0 {
		return true
	}
	_, ok := s.methods[method]
	return ok
}
