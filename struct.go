package douyinlive

import (
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/jwwsjlm/douyinLive/generated/douyin"
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
	c             *req.Client
	ttwid         string
	roomid        string
	liveid        string
	liveurl       string
	useragent     string
	Conn          *websocket.Conn
	eventHandlers []EventHandler
}
