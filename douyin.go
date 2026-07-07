package douyinLive

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/dgraph-io/ristretto/v2"
	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinLive/v2/sign"
	"github.com/jwwsjlm/req/v3"
)

// DouyinLive 结构体定义
// DouyinLive 管理一个抖音直播间的 HTTP 初始化、WebSocket 连接和消息分发。
// DouyinLive manages HTTP initialization, WebSocket connection, and message dispatch for one Douyin live room.
type DouyinLive struct {
	liveID              string
	roomID              string
	pushID              string
	liveName            string
	ttwid               string
	msToken             string
	userAgent           string
	signer              websocketSigner
	client              *req.Client
	conn                *websocket.Conn
	headers             http.Header
	bufferPool          *sync.Pool
	logger              logSink
	events              *messageBus
	eventHandlers       []eventHandler
	mu                  sync.Mutex
	contextMu           sync.Mutex
	isLiveClosed        bool
	manualClose         bool
	lastUserAgentChange time.Time
	consecutiveFailures int
	additionalCookies   map[string]string
	cookieManager       *sign.CookieManager
	heartbeatStopCh     chan struct{}
	heartbeatDoneCh     chan struct{}
	heartbeatEvery      time.Duration
	wsCursor            string
	wsInternalExt       string
	wsPushURL           string
	liveStatusGuard     liveStatusGuard
	writeMu             sync.Mutex
	title               string
	avatarThumb         string
	ristretto           *ristretto.Cache[string, string]
	releaseOnce         sync.Once
	closeCh             chan struct{}
	closeSignalClosed   bool
	closeCtx            context.Context
	closeCancel         context.CancelFunc
}

// NewDouyinLive 创建一个新的 DouyinLive 实例
// cookie 参数：可选的手动传入 Cookie，用于需要登录态的请求
// NewDouyinLive 创建使用本地签名的抖音直播监听实例。
// NewDouyinLive creates a Douyin live listener that uses local signing.
func NewDouyinLive(liveID string, logger logger, cookie string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newLocalWebsocketSigner())
}

// NewDouyinLiveWithTikHub 创建使用 TikHub 在线签名的抖音直播监听实例。
// NewDouyinLiveWithTikHub creates a Douyin live listener that uses TikHub online signing.
func NewDouyinLiveWithTikHub(liveID string, logger logger, cookie string, tikHubToken string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newTikHubWebsocketSigner(tikHubToken, ""))
}

// newDouyinLive 初始化 DouyinLive 的共享构造逻辑。
// newDouyinLive initializes the shared construction logic for DouyinLive.
func newDouyinLive(liveID string, baseLogger logger, cookie string, signer websocketSigner) (*DouyinLive, error) {
	userAgent := newHTTPUserAgent()
	if signer == nil {
		signer = newLocalWebsocketSigner()
	}
	signer.UpdateUserAgent(userAgent)
	cache, err := ristretto.NewCache(&ristretto.Config[string, string]{
		NumCounters: 500,
		MaxCost:     500,
		Metrics:     false,
		BufferItems: 64,
	})
	if err != nil {
		return nil, fmt.Errorf("初始化缓存失败: %w", err)
	}
	closeCtx, closeCancel := context.WithCancel(context.Background())
	dl := &DouyinLive{
		liveID:    liveID,
		liveName:  liveID,
		userAgent: userAgent,
		signer:    signer,
		client:    newHTTPClient(userAgent),
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, gzipBufferSize))
			},
		},
		events:              newMessageBus(),
		ristretto:           cache,
		headers:             make(http.Header),
		additionalCookies:   make(map[string]string),
		logger:              normalizeLogger(baseLogger),
		lastUserAgentChange: time.Now(),
		closeCh:             make(chan struct{}),
		closeCtx:            closeCtx,
		closeCancel:         closeCancel,
	}

	dl.cookieManager = sign.NewCookieManager()
	if cookie != "" {
		dl.cookieManager.SetDouyinCookie(cookie)
	}
	if statusLogger, ok := signer.(interface {
		LogStatus(logSink, string)
	}); ok {
		statusLogger.LogStatus(dl.logger, dl.liveID)
	}

	return dl, nil
}
