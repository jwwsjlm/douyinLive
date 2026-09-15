package douyinLive

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
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

// Options configures one listener. ProxyURL is fixed until the listener is disposed.
// Empty ProxyURL follows HTTP_PROXY/HTTPS_PROXY/NO_PROXY; empty SignProvider uses local signing.
type Options struct {
	Cookie       string
	ProxyURL     string
	SignProvider string
	TikHubToken  string
	// ProtocolMode 选择协议画像："web" 为默认稳定画像，"pc" 复现抖音桌面客户端且仍处于测试阶段。
	// 空值使用 DefaultProtocolMode。取值非法时构造失败并返回 ErrProtocolModeInvalid。
	// ProtocolMode selects the protocol profile: "web" is the stable default, while "pc"
	// reproduces the Douyin desktop client and remains experimental. Empty uses DefaultProtocolMode.
	ProtocolMode string
}

// NewDouyinLiveWithOptions creates a listener with optional per-instance proxy settings.
// Use NewSlogLogger to adapt a slog.Logger.
func NewDouyinLiveWithOptions(liveID string, logger Logger, options Options) (*DouyinLive, error) {
	var signer websocketSigner
	switch strings.ToLower(strings.TrimSpace(options.SignProvider)) {
	case "", SignProviderLocal:
		signer = newLocalWebsocketSigner()
	case SignProviderTikHub:
		if strings.TrimSpace(options.TikHubToken) == "" {
			return nil, ErrTikHubTokenEmpty
		}
		signer = newTikHubWebsocketSigner(options.TikHubToken, "")
	default:
		return nil, errors.New("sign provider must be local or tikhub")
	}
	return newDouyinLiveWithProxy(liveID, logger, options.Cookie, signer, options.ProxyURL, options.ProtocolMode)
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
	return newDouyinLiveWithProxy(liveID, baseLogger, cookie, signer, "", "")
}

// newDouyinLiveWithProxy 构造监听实例并解析协议画像。
// newDouyinLiveWithProxy builds a listener and resolves its protocol profile.
// 参数/Parameters:
//   - protocolMode: 协议画像名称；空值取 DefaultProtocolMode。 Protocol mode name; empty uses DefaultProtocolMode.
func newDouyinLiveWithProxy(liveID string, baseLogger Logger, cookie string, signer websocketSigner, proxyURL string, protocolMode string) (*DouyinLive, error) {
	protocolModeResolved, err := resolveProtocolMode(protocolMode)
	if err != nil {
		if closer, ok := signer.(websocketSignerCloser); ok {
			closer.Close()
		}
		return nil, err
	}
	liveID, err = ValidateLiveID(liveID)
	var proxy proxyPolicy
	if err == nil {
		proxy, err = newProxyPolicy(proxyURL)
	}
	if err != nil {
		if closer, ok := signer.(websocketSignerCloser); ok {
			closer.Close()
		}
		return nil, err
	}
	userAgent := selectUserAgent(protocolModeResolved.userAgents(), "")
	profile := newSessionProfile(protocolModeResolved, userAgent, signer, cookie, proxy)
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
		"protocol_mode", string(dl.protocol),
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
	source := "environment"
	if strings.TrimSpace(proxyURL) != "" {
		source = "explicit"
	} else if !proxy.disableHTTP3 {
		source = "direct"
	}
	request, _ := http.NewRequest(http.MethodGet, "https://live.douyin.com/", nil)
	if u, err := proxy.resolve(request); err == nil && u != nil {
		dl.logger.Info("采集代理已配置", "live_id", liveID, "proxy_source", source,
			"proxy_scheme", u.Scheme, "proxy_host", u.Host, "has_auth", u.User != nil)
	}

	return dl, nil
}

// ProtocolMode 返回本实例使用的协议画像名称（"pc" 或 "web"）。
// ProtocolMode returns this listener's protocol profile name ("pc" or "web").
func (dl *DouyinLive) ProtocolMode() string {
	if dl == nil {
		return ""
	}
	return string(dl.protocol)
}

// UserAgent 返回本实例当前使用的 User-Agent。
// UserAgent returns the User-Agent currently used by this listener.
func (dl *DouyinLive) UserAgent() string {
	if dl == nil {
		return ""
	}
	return dl.userAgent
}
