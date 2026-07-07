package douyinLive

import (
	"errors"
	"time"
)

const (
	defaultMaxRetries       = 5
	websocketConnectTimeout = 10 * time.Second
	baseReconnectDelay      = 1500 * time.Millisecond
	maxReconnectDelay       = 60 * time.Second
	maxReconnectJitter      = 1200 * time.Millisecond
	minUAChangeInterval     = 8 * time.Second
	gzipBufferSize          = 1024 * 4
	maxGzipPayloadSize      = 32 << 20
	httpRequestTimeout      = 15 * time.Second
	wsWriteTimeout          = 5 * time.Second
	wsReadTimeout           = 70 * time.Second
	heartbeatInterval       = 10 * time.Second
	liveStatusPollInterval  = 30 * time.Second
	controlActionLiveEnd    = 3
	webcastSDKVersion       = "1.0.15"
	websocketPushURL        = "wss://webcast100-ws-web-lf.douyin.com/webcast/im/push/v2/"
)

var (
	ErrLiveNotStarted        = errors.New("直播间未开播")
	ErrRoomNotFound          = errors.New("直播间不存在")
	errRoomInfoEmpty         = errors.New("直播间信息响应为空")
	errLivePageStateNotFound = errors.New("直播页状态不存在")
)
