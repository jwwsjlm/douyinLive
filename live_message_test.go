package douyinLive

import (
	"log"
	"testing"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func TestMessageBusSubscribeAll(t *testing.T) {
	bus := newMessageBus()
	var got []string

	id := bus.subscribe(func(message *LiveMessage) {
		got = append(got, message.GetMethod())
	})
	if id == "" {
		t.Fatalf("subscribe returned empty id")
	}

	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}})
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastGiftMessage}})

	if len(got) != 2 {
		t.Fatalf("all-message subscriber got %d messages, want 2", len(got))
	}
	if got[0] != WebcastChatMessage || got[1] != WebcastGiftMessage {
		t.Fatalf("unexpected methods: %#v", got)
	}
}

func TestMessageBusSubscribeOneMethod(t *testing.T) {
	bus := newMessageBus()
	var got []string

	id := bus.subscribe(func(message *LiveMessage) {
		got = append(got, message.GetMethod())
	}, WebcastChatMessage)
	if id == "" {
		t.Fatalf("subscribe returned empty id")
	}

	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastGiftMessage}})
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}})
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastLikeMessage}})

	if len(got) != 1 || got[0] != WebcastChatMessage {
		t.Fatalf("method subscriber got %#v, want only %s", got, WebcastChatMessage)
	}
}

func TestMessageBusSubscribeMultipleMethods(t *testing.T) {
	bus := newMessageBus()
	var got []string

	id := bus.subscribe(func(message *LiveMessage) {
		got = append(got, message.GetMethod())
	}, WebcastChatMessage, WebcastGiftMessage)
	if id == "" {
		t.Fatalf("subscribe returned empty id")
	}

	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastLikeMessage}})
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastGiftMessage}})
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}})

	if len(got) != 2 {
		t.Fatalf("multi-method subscriber got %d messages, want 2", len(got))
	}
	if got[0] != WebcastGiftMessage || got[1] != WebcastChatMessage {
		t.Fatalf("unexpected methods: %#v", got)
	}
}

func TestMessageBusUnsubscribe(t *testing.T) {
	bus := newMessageBus()
	calls := 0

	id := bus.subscribe(func(*LiveMessage) {
		calls++
	}, WebcastChatMessage)
	bus.unsubscribe(id)
	bus.publish(&LiveMessage{Raw: &new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}})

	if calls != 0 {
		t.Fatalf("unsubscribed handler was called %d times", calls)
	}
}

func TestMessageBusSkipsSubscriberUnsubscribedDuringPublish(t *testing.T) {
	bus := newMessageBus()
	calls := 0
	var secondID string

	bus.subscribe(func(*LiveMessage) {
		bus.unsubscribe(secondID)
	})
	secondID = bus.subscribe(func(*LiveMessage) {
		calls++
	})

	bus.publish(&LiveMessage{})

	if calls != 0 {
		t.Fatalf("subscriber removed during publish was called %d times", calls)
	}
}

func TestMessageBusRecoversSubscriberPanic(t *testing.T) {
	bus := newMessageBus()
	calls := 0

	bus.subscribe(func(*LiveMessage) {
		panic("boom")
	})
	bus.subscribe(func(*LiveMessage) {
		calls++
	})

	bus.publishWithLogger(normalizeLogger(log.Default()), &LiveMessage{
		Raw: &new_douyin.Webcast_Im_Message{Method: WebcastChatMessage},
	})

	if calls != 1 {
		t.Fatalf("healthy subscriber was called %d times, want 1", calls)
	}
}

func TestMessageBusRejectsEmptyMethodFilter(t *testing.T) {
	bus := newMessageBus()
	id := bus.subscribe(func(*LiveMessage) {}, "")
	if id != "" {
		t.Fatalf("subscribe returned %q for an empty method filter, want empty id", id)
	}
}

func TestDouyinLiveSubscribeMethodDispatchesByMessageMethod(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
	}
	var got []*LiveMessage

	id := dl.SubscribeMethod(WebcastChatMessage, func(message *LiveMessage) {
		got = append(got, message)
	})
	if id == "" {
		t.Fatalf("SubscribeMethod returned empty id")
	}

	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastGiftMessage}, nil)
	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastChatMessage, Payload: []byte("chat")}, nil)

	if len(got) != 1 {
		t.Fatalf("method subscriber got %d messages, want 1", len(got))
	}
	if got[0].GetMethod() != WebcastChatMessage {
		t.Fatalf("method subscriber got method %q, want %q", got[0].GetMethod(), WebcastChatMessage)
	}
	if got[0].LiveID != "live-id" || got[0].RoomID != "room-id" || got[0].LiveName != "live-name" {
		t.Fatalf("message metadata was not propagated: %#v", got[0])
	}
	if string(got[0].GetPayload()) != "chat" {
		t.Fatalf("message payload = %q, want chat", got[0].GetPayload())
	}
}

func TestDouyinLiveSubscribeRejectsNilHandlers(t *testing.T) {
	dl := &DouyinLive{}

	if id := dl.Subscribe(nil); id != "" {
		t.Fatalf("Subscribe(nil) returned %q, want empty id", id)
	}
	if id := dl.SubscribeMessage(nil); id != "" {
		t.Fatalf("SubscribeMessage(nil) returned %q, want empty id", id)
	}
	if id := dl.SubscribeMethod(WebcastChatMessage, nil); id != "" {
		t.Fatalf("SubscribeMethod(nil) returned %q, want empty id", id)
	}
}

func TestDouyinLiveSkipsLegacyHandlerUnsubscribedDuringEmit(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
	}
	calls := 0
	var secondID string

	dl.Subscribe(func(*new_douyin.Webcast_Im_Message, proto.Message) {
		dl.Unsubscribe(secondID)
	})
	secondID = dl.Subscribe(func(*new_douyin.Webcast_Im_Message, proto.Message) {
		calls++
	})

	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}, nil)

	if calls != 0 {
		t.Fatalf("legacy handler removed during emit was called %d times", calls)
	}
}

func TestNewDouyinLiveDefaultsNilLogger(t *testing.T) {
	dl, err := NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive returned error: %v", err)
	}
	defer dl.Dispose()

	if dl.logger == nil {
		t.Fatalf("logger was not defaulted")
	}
}

func TestEmitEventClonesParsedMessageForSubscribers(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
		logger:   normalizeLogger(log.Default()),
	}
	parsed := &new_douyin.Webcast_Im_ChatMessage{Content: "hello"}
	var got *LiveMessage

	dl.SubscribeMessage(func(message *LiveMessage) {
		got = message
	})
	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}, parsed)
	parsed.Content = "mutated"

	if got == nil {
		t.Fatalf("subscriber did not receive message")
	}
	chat, ok := got.Parsed.(*new_douyin.Webcast_Im_ChatMessage)
	if !ok {
		t.Fatalf("Parsed type = %T, want *new_douyin.Webcast_Im_ChatMessage", got.Parsed)
	}
	if chat.GetContent() != "hello" {
		t.Fatalf("Parsed content = %q, want cloned value hello", chat.GetContent())
	}
}

func TestDecodeResponseStopsDispatchingAfterClose(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
		logger:   normalizeLogger(log.Default()),
	}
	dl.setLiveStatus(true)

	var got []string
	dl.SubscribeMessage(func(message *LiveMessage) {
		got = append(got, message.GetMethod())
		dl.Close()
	})

	responseBytes, err := proto.Marshal(&new_douyin.Webcast_Im_Response{
		Messages: []*new_douyin.Webcast_Im_Message{
			{Method: WebcastChatMessage},
			{Method: WebcastGiftMessage},
		},
	})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}

	err = dl.decodeResponse(
		responseBytes,
		&new_douyin.Webcast_Im_PushFrame{},
		&new_douyin.Webcast_Im_Response{},
		&new_douyin.Webcast_Im_ControlMessage{},
	)
	if err != nil {
		t.Fatalf("decodeResponse() returned error: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("got %d callbacks after Close, want 1: %#v", len(got), got)
	}
	if got[0] != WebcastChatMessage {
		t.Fatalf("first callback method = %q, want %q", got[0], WebcastChatMessage)
	}
}

func TestEmitEventStopsLegacyHandlersAfterClose(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
		logger:   normalizeLogger(log.Default()),
	}
	dl.setLiveStatus(true)

	var got []string
	dl.Subscribe(func(*new_douyin.Webcast_Im_Message, proto.Message) {
		got = append(got, "first")
		dl.Close()
	})
	dl.Subscribe(func(*new_douyin.Webcast_Im_Message, proto.Message) {
		got = append(got, "second")
	})

	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}, nil)

	if len(got) != 1 || got[0] != "first" {
		t.Fatalf("legacy handlers after Close = %#v, want only first", got)
	}
}

func TestEmitEventStopsMessageSubscribersAfterClose(t *testing.T) {
	dl := &DouyinLive{
		liveID:   "live-id",
		roomID:   "room-id",
		liveName: "live-name",
		logger:   normalizeLogger(log.Default()),
	}
	dl.setLiveStatus(true)

	var got []string
	dl.SubscribeMessage(func(*LiveMessage) {
		got = append(got, "first")
		dl.Close()
	})
	dl.SubscribeMessage(func(*LiveMessage) {
		got = append(got, "second")
	})

	dl.emitEvent(&new_douyin.Webcast_Im_Message{Method: WebcastChatMessage}, nil)

	if len(got) != 1 || got[0] != "first" {
		t.Fatalf("message subscribers after Close = %#v, want only first", got)
	}
}

func TestParseRoomInfoUsesFallbackFields(t *testing.T) {
	body := `{
		"data": {
			"enter_room_id": "fallback-room",
			"data": [{
				"owner_user_id_str": "fallback-owner",
				"owner": {
					"nickname": "fallback-name",
					"avatar_thumb": {
						"url_list": ["owner-avatar"]
					}
				},
				"title": "room-title"
			}]
		}
	}`

	info, err := parseRoomInfo(body)
	if err != nil {
		t.Fatalf("parseRoomInfo returned error: %v", err)
	}
	if info.roomID != "fallback-room" {
		t.Fatalf("roomID = %q, want fallback-room", info.roomID)
	}
	if info.pushID != "fallback-owner" {
		t.Fatalf("pushID = %q, want fallback-owner", info.pushID)
	}
	if info.liveName != "fallback-name" {
		t.Fatalf("liveName = %q, want fallback-name", info.liveName)
	}
	if info.title != "room-title" {
		t.Fatalf("title = %q, want room-title", info.title)
	}
	if info.avatarThumb != "owner-avatar" {
		t.Fatalf("avatarThumb = %q, want owner-avatar", info.avatarThumb)
	}
}

func TestFetchRoomEnterDataUpdatesRoomInfoFromCache(t *testing.T) {
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 1000,
		MaxCost:     1000,
		BufferItems: 64,
	})
	if err != nil {
		t.Fatalf("new cache: %v", err)
	}
	defer cache.Close()

	const body = `{
		"data": {
			"user": {
				"id_str": "user-id",
				"nickname": "cached-name",
				"avatar_thumb": {
					"url_list": ["avatar-0", "avatar-1", "avatar-2"]
				}
			},
			"data": [{
				"id_str": "room-id",
				"title": "cached-title"
			}]
		}
	}`
	if ok := cache.SetWithTTL("live-id", body, 1, time.Minute); !ok {
		t.Fatalf("cache rejected test room info")
	}
	cache.Wait()
	if _, found := cache.Get("live-id"); !found {
		t.Fatalf("test room info was not committed to cache")
	}

	dl := &DouyinLive{
		liveID:    "live-id",
		liveName:  "old-name",
		logger:    normalizeLogger(log.Default()),
		ristretto: cache,
	}

	got, err := dl.fetchRoomEnterData()
	if err != nil {
		t.Fatalf("fetchRoomEnterData returned error: %v", err)
	}
	if got != body {
		t.Fatalf("fetchRoomEnterData body = %q, want cached body", got)
	}

	info := dl.roomInfoSnapshot()
	if info.roomID != "room-id" || info.pushID != "user-id" || info.liveName != "cached-name" {
		t.Fatalf("room info was not updated from cache: %#v", info)
	}
	if info.title != "cached-title" || info.avatarThumb != "avatar-2" {
		t.Fatalf("room metadata was not updated from cache: %#v", info)
	}
}
