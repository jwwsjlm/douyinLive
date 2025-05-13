package douyinLive

import (
	"compress/gzip"
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

type EventHandler func(eventData *new_douyin.Webcast_Im_Message)
type DouyinLive struct {
	key           string
	ttwid         string
	roomid        string
	liveid        string
	liveurl       string
	userAgent     string
	c             *req.Client
	eventHandlers []EventHandler
	headers       http.Header
	buffers       *sync.Pool
	gzip          *gzip.Reader
	Conn          *websocket.Conn
	wssurl        string
	pushid        string
	isLiveClosed  bool
}
