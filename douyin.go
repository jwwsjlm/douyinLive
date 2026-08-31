package douyinLive

import (
	"bytes"
	"context"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// DouyinLive 管理一个抖音直播间的 HTTP 初始化、WebSocket 连接和消息分发。
// DouyinLive manages HTTP initialization, WebSocket connection, and message dispatch for one Douyin live room.
type DouyinLive struct {
	liveID   string
	roomID   string
	pushID   string
	liveName string
	sessionProfile
	conn                   *websocket.Conn
	bufferPool             *sync.Pool
	logger                 logSink
	events                 *messageBus
	mu                     sync.Mutex
	contextMu              sync.Mutex
	contextPrepared        bool
	isLive                 bool
	manualClose            bool
	lifecycleState         listenerLifecycleState
	consecutiveFailures    int
	heartbeatStopCh        chan struct{}
	heartbeatDoneCh        chan struct{}
	heartbeatEvery         time.Duration
	wsCursor               string
	wsInternalExt          string
	wsPushURL              string
	liveStatusGuard        liveStatusGuard
	liveStatusKnown        bool
	writeMu                sync.Mutex
	title                  string
	avatarThumb            string
	anchorOnlyPageIdentity bool
	roomEnterCacheBody     string
	roomEnterCacheExpires  time.Time
	releaseOnce            sync.Once
	closeCtx               context.Context
	closeCancel            context.CancelFunc
	readyMu                sync.Mutex
	readyCh                chan struct{}
	readyClosed            bool
}

// NewDouyinLive 创建使用本地签名的抖音直播监听实例。
// NewDouyinLive creates a Douyin live listener that uses local signing.
// 参数/Parameters:
//   - liveID: 直播间短号、web_rid 或房间标识。 Live room short ID, web_rid, or room identifier.
//   - logger: 可选日志器；为 nil 时使用默认日志器。 Optional logger; nil uses the default logger.
//   - cookie: 可选抖音 Cookie，用于登录态请求。 Optional Douyin Cookie for authenticated requests.
func NewDouyinLive(liveID string, logger Logger, cookie string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newLocalWebsocketSigner())
}

// NewDouyinLiveWithTikHub 创建使用 TikHub 在线签名的抖音直播监听实例。
// NewDouyinLiveWithTikHub creates a Douyin live listener that uses TikHub online signing.
// 参数/Parameters:
//   - liveID: 直播间短号、web_rid 或房间标识。 Live room short ID, web_rid, or room identifier.
//   - logger: 可选日志器；为 nil 时使用默认日志器。 Optional logger; nil uses the default logger.
//   - cookie: 可选抖音 Cookie，用于登录态请求。 Optional Douyin Cookie for authenticated requests.
//   - tikHubToken: TikHub API Token，用于在线生成 WebSocket 签名。 TikHub API token for online WebSocket signing.
func NewDouyinLiveWithTikHub(liveID string, logger Logger, cookie string, tikHubToken string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newTikHubWebsocketSigner(tikHubToken, ""))
}

// newDouyinLive 初始化 DouyinLive 的共享构造逻辑。
// newDouyinLive initializes the shared construction logic for DouyinLive.
// 参数/Parameters:
//   - liveID: 直播间短号、web_rid 或房间标识。 Live room short ID, web_rid, or room identifier.
//   - baseLogger: 可选日志器；为 nil 时使用默认日志器。 Optional logger; nil uses the default logger.
//   - cookie: 可选抖音 Cookie，用于登录态请求。 Optional Douyin Cookie for authenticated requests.
//   - signer: WebSocket 签名实现。 WebSocket signature provider.
func newDouyinLive(liveID string, baseLogger Logger, cookie string, signer websocketSigner) (*DouyinLive, error) {
	liveID, err := ValidateLiveID(liveID)
	if err != nil {
		if closer, ok := signer.(websocketSignerCloser); ok {
			closer.Close()
		}
		return nil, err
	}
	userAgent := newHTTPUserAgent()
	profile := newSessionProfile(userAgent, signer, cookie)
	closeCtx, closeCancel := context.WithCancel(context.Background())
	dl := &DouyinLive{
		liveID:         liveID,
		liveName:       "",
		sessionProfile: profile,
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, gzipBufferSize))
			},
		},
		events:      newMessageBus(),
		logger:      normalizeLogger(baseLogger),
		closeCtx:    closeCtx,
		closeCancel: closeCancel,
		readyCh:     make(chan struct{}),
	}

	dl.logger.Debug(
		"浏览器会话画像已创建",
		"live_id", dl.liveID,
		"user_agent", dl.userAgent,
		"fingerprint_preset", dl.fingerprint.Preset,
		"fingerprint_id", dl.fingerprint.ID,
		"screen_width", dl.fingerprint.ScreenWidth,
		"screen_height", dl.fingerprint.ScreenHeight,
		"device_memory", dl.fingerprint.DeviceMemory,
		"hardware_concurrency", dl.fingerprint.HardwareConcurrency,
		"sign_provider", dl.signer.Name(),
	)
	if statusLogger, ok := dl.signer.(interface {
		LogStatus(logSink, string)
	}); ok {
		statusLogger.LogStatus(dl.logger, dl.liveID)
	}

	return dl, nil
}
