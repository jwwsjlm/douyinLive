package douyinlive

import (
	"compress/gzip"
	"douyinlive/generated/douyin"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"net/http"
	"sync"
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

type EventHandler func(eventData *douyin.Message)
type DouyinLive struct {
	wssurl        string
	headers       http.Header
	c             *req.Client
	ttwid         string
	roomid        string
	liveid        string
	liveurl       string
	userAgent     string
	pushid        string
	Conn          *websocket.Conn
	eventHandlers []EventHandler
	gzip          *gzip.Reader
	isLiveClosed  bool
	buffers       *sync.Pool
}
