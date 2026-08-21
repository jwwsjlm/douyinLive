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
	wsReadMaxPayloadSize    = 16 << 20
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
	// ErrLiveNotStarted indicates that a verified room is currently offline.
	// ErrLiveNotStarted 表示已确认存在的直播间当前未开播。
	ErrLiveNotStarted = errors.New("直播间未开播")
	// ErrRoomNotFound indicates that the requested room identity was definitively not found.
	// ErrRoomNotFound 表示请求的直播间标识已被明确确认不存在。
	ErrRoomNotFound = errors.New("直播间不存在")
	// ErrLiveStatusUnknown indicates that the upstream response could not verify a live status.
	// ErrLiveStatusUnknown 表示上游响应暂时无法验证直播状态。
	ErrLiveStatusUnknown = errors.New("直播状态暂时无法确认")
	// ErrDouyinLiveAlreadyStarted indicates that Start is already running for this instance.
	// ErrDouyinLiveAlreadyStarted 表示当前实例已经在执行 Start，不能并发重复启动。
	ErrDouyinLiveAlreadyStarted = errors.New("DouyinLive 已经启动")
	// ErrDouyinLiveClosed indicates that the instance has been closed and cannot be restarted.
	// ErrDouyinLiveClosed 表示实例已经关闭，不能再次启动。
	ErrDouyinLiveClosed      = errors.New("DouyinLive 已关闭")
	errRoomInfoEmpty         = errors.New("直播间信息响应为空")
	errLivePageStateNotFound = errors.New("直播页状态不存在")
)
