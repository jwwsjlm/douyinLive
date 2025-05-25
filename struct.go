package douyinLive

import (
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
)

const (
	WebcastChatMessage        = "WebcastChatMessage"
	WebcastGiftMessage        = "WebcastGiftMessage"
	WebcastLikeMessage        = "WebcastLikeMessage"
	WebcastMemberMessage      = "WebcastMemberMessage"
	WebcastSocialMessage      = "WebcastSocialMessage"
	WebcastRoomUserSeqMessage = "WebcastRoomUserSeqMessage"
	WebcastFansclubMessage    = "WebcastFansclubMessage"
	WebcastControlMessage     = "WebcastControlMessage"
	WebcastEmojiChatMessage   = "WebcastEmojiChatMessage"
	WebcastRoomStatsMessage   = "WebcastRoomStatsMessage"
	WebcastRoomMessage        = "WebcastRoomMessage"
	WebcastRoomRankMessage    = "WebcastRoomRankMessage"

	Default = "Default"
)

type DouyinLive struct {
	mu            sync.RWMutex
	liveID        string
	roomID        string
	pushID        string
	wssURL        string
	userAgent     string
	ttwid         string
	client        *req.Client
	conn          *websocket.Conn
	eventHandlers []EventHandler
	headers       http.Header
	bufferPool    *sync.Pool
	isLiveClosed  bool
	LiveName      string
}

// EventHandler 修改 EventHandler 类型，添加唯一ID
type EventHandler struct {
	ID      string
	Handler func(*new_douyin.Webcast_Im_Message)
}
