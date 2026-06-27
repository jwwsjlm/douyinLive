package douyinLive

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/codeGROOVE-dev/retry"
	"github.com/dgraph-io/ristretto/v2"

	"github.com/tidwall/gjson"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinLive/v2/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/v2/jsScript"
	"github.com/jwwsjlm/douyinLive/v2/sign"
	"github.com/jwwsjlm/douyinLive/v2/utils"
	"github.com/jwwsjlm/req/v3"
	"google.golang.org/protobuf/proto"
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
	heartbeatInterval       = 20 * time.Second
	liveStatusPollInterval  = 30 * time.Second
	controlActionLiveEnd    = 3
	wssURLTemplate          = "wss://webcast5-ws-web-lf.douyin.com/webcast/im/push/v2/" +
		"?app_name=douyin_web&version_code=180800&webcast_sdk_version=1.0.14-beta.0" +
		"&update_version_code=1.0.14-beta.0&compress=gzip&device_platform=web" +
		"&cookie_enabled=true&screen_width=1920&screen_height=1080&browser_language=zh-CN" +
		"&browser_platform=Win32&browser_name=Mozilla&browser_version=%s&browser_online=true" +
		"&tz_name=Asia/Shanghai&cursor=d-1_u-1_fh-7383731312643626035_t-1719159695790_r-1" +
		"&internal_ext=internal_src:dim|wss_push_room_id:%s|wss_push_did:%s|first_req_ms:%d" +
		"|fetch_time:%d|seq:1|wss_info:0-%d-0-0|wrds_v:7382620942951772256&host=https://live.douyin.com" +
		"&aid=6383&live_id=1&did_rule=3&endpoint=live_pc&support_wrds=1&user_unique_id=%s" +
		"&im_path=/webcast/im/fetch/&identity=audience&need_persist_msg_count=15" +
		"&insert_task_id=&live_reason=&room_id=%s&heartbeatDuration=0&signature=%s"
)

var (
	ErrLiveNotStarted = errors.New("直播间未开播")
)

var impersonatedUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36",
}

// DouyinLive 结构体定义
type DouyinLive struct {
	liveID              string
	roomID              string
	pushID              string
	liveName            string
	ttwid               string
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

type liveStatusGuard struct {
	offlineConfirmations int
}

func (g *liveStatusGuard) Record(isLive bool) bool {
	if isLive {
		g.offlineConfirmations = 0
		return false
	}
	g.offlineConfirmations++
	return g.offlineConfirmations >= 2
}

func (g *liveStatusGuard) Reset() {
	g.offlineConfirmations = 0
}

type roomInfoSnapshot struct {
	liveID      string
	roomID      string
	pushID      string
	liveName    string
	title       string
	avatarThumb string
}

// NewDouyinLive 创建一个新的 DouyinLive 实例
// cookie 参数：可选的手动传入 Cookie，用于需要登录态的请求
func NewDouyinLive(liveID string, logger logger, cookie string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newLocalWebsocketSigner())
}

func NewDouyinLiveWithTikHub(liveID string, logger logger, cookie string, tikHubToken string) (*DouyinLive, error) {
	return newDouyinLive(liveID, logger, cookie, newTikHubWebsocketSigner(tikHubToken, ""))
}

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

func newHTTPUserAgent() string {
	if len(impersonatedUserAgents) == 0 {
		return ""
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(impersonatedUserAgents))))
	if err != nil {
		return impersonatedUserAgents[0]
	}
	return impersonatedUserAgents[n.Int64()]
}

func newHTTPClient(userAgent string) *req.Client {
	return req.C().
		ImpersonateChromeWithOS(req.BrowserOSWindows).
		EnableHTTP3().
		EnableHTTP3FallbackOnError().
		SetUserAgent(userAgent).
		SetTimeout(httpRequestTimeout)
}

func (dl *DouyinLive) GetName() string {
	return dl.roomInfoSnapshot().liveName
}
func (dl *DouyinLive) GetTitle() string {
	return dl.roomInfoSnapshot().title
}
func (dl *DouyinLive) GetAvatarThumb() string {
	return dl.roomInfoSnapshot().avatarThumb
}

func (dl *DouyinLive) roomInfoSnapshot() roomInfoSnapshot {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	return roomInfoSnapshot{
		liveID:      dl.liveID,
		roomID:      dl.roomID,
		pushID:      dl.pushID,
		liveName:    dl.liveName,
		title:       dl.title,
		avatarThumb: dl.avatarThumb,
	}
}

func (dl *DouyinLive) updateRoomInfo(roomID, pushID, liveName, title, avatarThumb string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	dl.roomID = roomID
	dl.pushID = pushID
	dl.liveName = liveName
	dl.title = title
	dl.avatarThumb = avatarThumb
}

func parseRoomInfo(body string) (roomInfoSnapshot, error) {
	roomID := firstNonEmptyGJSON(body,
		"data.data.0.id_str",
		"data.data.0.id",
		"data.room.id_str",
		"data.room.id",
		"data.enter_room_id",
	)
	pushID := firstNonEmptyGJSON(body,
		"data.user.id_str",
		"data.user.id",
		"data.data.0.owner_user_id_str",
		"data.data.0.owner.id_str",
		"data.data.0.owner.id",
		"data.room.owner_user_id_str",
		"data.room.owner.id_str",
		"data.room.owner.id",
	)
	liveName := firstNonEmptyGJSON(body,
		"data.user.nickname",
		"data.data.0.owner.nickname",
		"data.room.owner.nickname",
	)
	avatarThumb := firstNonEmptyGJSON(body,
		"data.user.avatar_thumb.url_list.2",
		"data.user.avatar_thumb.url_list.0",
		"data.data.0.owner.avatar_thumb.url_list.2",
		"data.data.0.owner.avatar_thumb.url_list.0",
		"data.room.owner.avatar_thumb.url_list.2",
		"data.room.owner.avatar_thumb.url_list.0",
	)

	if roomID == "" || pushID == "" {
		return roomInfoSnapshot{}, errors.New("无法提取房间信息")
	}

	return roomInfoSnapshot{
		roomID:      roomID,
		pushID:      pushID,
		liveName:    liveName,
		title:       firstNonEmptyGJSON(body, "data.data.0.title", "data.room.title"),
		avatarThumb: avatarThumb,
	}, nil
}

func firstNonEmptyGJSON(body string, paths ...string) string {
	for _, path := range paths {
		value := gjson.Get(body, path).String()
		if value != "" {
			return value
		}
	}
	return ""
}

func queryEscapeValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// Close 关闭抖音直播连接，确保资源正确释放
func (dl *DouyinLive) Close() {
	dl.setManualClose(true)
	dl.setLiveStatus(false)
	dl.signalClose()
	dl.stopHeartbeatLoop()
	dl.closeCurrentConnection(websocket.CloseNormalClosure, "closing connection")
}

// Dispose releases resources for instances that won't enter Start().
func (dl *DouyinLive) Dispose() {
	dl.Close()
	dl.releaseCache()
}

func (dl *DouyinLive) releaseCache() {
	dl.releaseOnce.Do(func() {
		if dl.ristretto != nil {
			dl.ristretto.Close()
		}
	})
}

func (dl *DouyinLive) rebuildHTTPClientAndHeaders() {
	dl.client = newHTTPClient(dl.userAgent)
	dl.headers = make(http.Header)
	dl.headers.Set("User-Agent", dl.userAgent)
	dl.refreshSignerUserAgent()
}

func (dl *DouyinLive) refreshSignerUserAgent() {
	if dl.signer != nil {
		dl.signer.UpdateUserAgent(dl.userAgent)
	}
}

func (dl *DouyinLive) resetReconnectTracking() {
	dl.mu.Lock()
	dl.consecutiveFailures = 0
	dl.mu.Unlock()
}

func (dl *DouyinLive) recordReconnectFailure(reason string) int {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.consecutiveFailures++
	return dl.consecutiveFailures
}

func (dl *DouyinLive) ensureCloseContextLocked() {
	if dl.closeCtx != nil && dl.closeCancel != nil {
		return
	}
	dl.closeCtx, dl.closeCancel = context.WithCancel(context.Background())
	if dl.closeSignalClosed {
		dl.closeCancel()
	}
}

func (dl *DouyinLive) signalClose() {
	dl.mu.Lock()
	if dl.closeCh == nil {
		dl.closeCh = make(chan struct{})
	}
	dl.ensureCloseContextLocked()
	if !dl.closeSignalClosed {
		close(dl.closeCh)
		dl.closeSignalClosed = true
		dl.closeCancel()
	}
	dl.mu.Unlock()
}

func (dl *DouyinLive) resetCloseSignal() {
	dl.mu.Lock()
	if dl.closeCh == nil || dl.closeSignalClosed {
		dl.closeCh = make(chan struct{})
		dl.closeSignalClosed = false
	}
	if dl.closeCtx == nil || dl.closeCtx.Err() != nil {
		dl.closeCtx, dl.closeCancel = context.WithCancel(context.Background())
	}
	dl.mu.Unlock()
}

func (dl *DouyinLive) closeSignal() <-chan struct{} {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.closeCh == nil {
		dl.closeCh = make(chan struct{})
	}
	return dl.closeCh
}

func (dl *DouyinLive) waitForReconnectDelay(delay time.Duration) bool {
	if delay <= 0 {
		select {
		case <-dl.closeSignal():
			return false
		default:
			return true
		}
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return true
	case <-dl.closeSignal():
		return false
	}
}

func (dl *DouyinLive) requestContext() (context.Context, context.CancelFunc) {
	dl.mu.Lock()
	dl.ensureCloseContextLocked()
	parent := dl.closeCtx
	dl.mu.Unlock()

	return context.WithTimeout(parent, httpRequestTimeout)
}

func contextWithCloseSignal(closeCh <-chan struct{}) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		select {
		case <-closeCh:
			cancel()
		case <-ctx.Done():
		}
	}()
	return ctx, cancel
}

func (dl *DouyinLive) prepareRequestContextLocked() error {
	if err := dl.fetchTTWID(); err != nil {
		return err
	}

	dl.headers.Set("User-Agent", dl.userAgent)
	dl.headers.Set("Origin", "https://live.douyin.com")
	dl.headers.Set("Referer", "https://live.douyin.com/"+dl.liveID)
	dl.setupCookies()
	return nil
}

func (dl *DouyinLive) prepareWebSocketContextLocked() error {
	if err := dl.prepareRequestContextLocked(); err != nil {
		return err
	}

	if _, err := dl.fetchRoomEnterData(); err != nil {
		return err
	}

	if dl.signer == nil || dl.signer.Name() == SignProviderLocal {
		if err := jsScript.LoadGoja(dl.userAgent); err != nil {
			return fmt.Errorf("加载JavaScript脚本失败: %w", err)
		}

	}

	return nil
}

func (dl *DouyinLive) refreshReconnectContextLocked(changeUA bool, rebuildHTTP bool) error {
	oldUserAgent := dl.userAgent
	if changeUA {
		now := time.Now()
		if now.Sub(dl.lastUserAgentChange) >= minUAChangeInterval {
			newUserAgent := newHTTPUserAgent()
			dl.userAgent = newUserAgent
			dl.lastUserAgentChange = now
			dl.logger.Info("重连前刷新 UA", "live_id", dl.liveID, "old_user_agent", oldUserAgent, "new_user_agent", newUserAgent)
		} else {
			dl.logger.Debug("本次重连跳过 UA 刷新", "live_id", dl.liveID, "elapsed", now.Sub(dl.lastUserAgentChange).Round(time.Millisecond))
		}
	}

	if rebuildHTTP || dl.client == nil || dl.headers == nil {
		dl.rebuildHTTPClientAndHeaders()
	} else {
		dl.client.SetUserAgent(dl.userAgent)
		dl.headers.Set("User-Agent", dl.userAgent)
		dl.refreshSignerUserAgent()
	}

	if err := dl.prepareWebSocketContextLocked(); err != nil {
		return fmt.Errorf("刷新重连上下文失败: %w", err)
	}

	return nil
}

// getCookieParts 获取当前有效的 Cookie 键值对
func (dl *DouyinLive) getCookieParts() []string {
	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		cookies := dl.cookieManager.ParseCookies(configCookie)
		parts := make([]string, 0, len(cookies))
		for _, c := range cookies {
			parts = append(parts, fmt.Sprintf("%s=%s", c.Name, c.Value))
		}
		return parts
	}

	parts := []string{fmt.Sprintf("ttwid=%s", dl.ttwid)}
	for name, value := range dl.additionalCookies {
		parts = append(parts, fmt.Sprintf("%s=%s", name, value))
	}
	return parts
}

// getCookieString 获取 Cookie 字符串（用于 headers）
func (dl *DouyinLive) getCookieString() string {
	parts := dl.getCookieParts()
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

// setupCookies 设置 Cookie，优先使用配置文件中的 Cookie
func (dl *DouyinLive) setupCookies() {
	dl.headers.Set("Cookie", dl.getCookieString())
}

// fetchTTWID 获取 TTWID 和其他 Cookie
func (dl *DouyinLive) fetchTTWID() error {
	ctx, cancel := dl.requestContext()
	defer cancel()

	resp, err := dl.client.R().
		SetContext(ctx).
		Get("https://live.douyin.com/")
	if err != nil {
		return fmt.Errorf("请求TTWID失败: %w", err)
	}

	// 收集所有 Cookie
	cookies := make(map[string]string)
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}

	// 刷新额外 Cookie，避免旧值残留
	dl.additionalCookies = make(map[string]string)

	// 优先使用 ttwid，如果没有找到则报错
	if ttwid, exists := cookies["ttwid"]; exists {
		dl.ttwid = ttwid
	} else {
		return errors.New("未找到TTWID cookie")
	}

	// 存储其他重要 Cookie
	for name, value := range cookies {
		if name != "ttwid" {
			dl.additionalCookies[name] = value
		}
	}
	return nil
}

// fetchRoomEnterData 获取直播间接口数据（对齐 DouyinLiveRecorder 的 web/enter 逻辑）
func (dl *DouyinLive) fetchRoomEnterData() (string, error) {
	V, found := dl.ristretto.Get(dl.liveID)
	if found {
		dl.logger.Debug("从缓存获取直播间信息", "live_id", dl.liveID)
		roomInfo, err := parseRoomInfo(V)
		if err != nil {
			return "", err
		}
		dl.updateRoomInfo(
			roomInfo.roomID,
			roomInfo.pushID,
			roomInfo.liveName,
			roomInfo.title,
			roomInfo.avatarThumb,
		)
		return V, nil
	}

	return dl.refreshRoomEnterData()
}

func (dl *DouyinLive) refreshRoomEnterData() (string, error) {
	var body string

	dl.logger.Debug("开始请求直播间信息", "live_id", dl.liveID)
	err := retry.Do(
		func() error {
			// 核心请求逻辑
			reqBody, err := dl.doRequest()
			if err != nil {
				return err
			}
			body = reqBody
			return nil
		},
		retry.Attempts(3),          // 最多重试 3 次
		retry.Delay(1*time.Second), // 每次重试延迟1秒
		retry.RetryIf(func(err error) bool {
			// 只对【直播间信息响应为空】进行重试
			if err == nil {
				return false // 没有错误，不需要重试
			}
			return err.Error() == "直播间信息响应为空"
		}),
	)

	if err != nil {
		dl.logger.Error("请求直播间信息失败，重试结束", "live_id", dl.liveID, "err", err)
		return "", err
	}

	roomInfo, err := parseRoomInfo(body)
	if err != nil {
		dl.logRoomInfoResponseSummary(body)
		return "", err
	}

	dl.updateRoomInfo(
		roomInfo.roomID,
		roomInfo.pushID,
		roomInfo.liveName,
		roomInfo.title,
		roomInfo.avatarThumb,
	)
	dl.ristretto.SetWithTTL(dl.liveID, body, 1, 5*time.Second) // 将结果缓存到 Ristretto，成本为 1

	return body, nil
}

// 把真正的请求抽成独立函数，代码更干净
func (dl *DouyinLive) doRequest() (string, error) {
	params := fmt.Sprintf(
		"aid=6383&app_name=douyin_web&live_id=1&device_platform=web"+
			"&language=zh-CN&browser_language=zh-CN"+
			"&browser_platform=Win32&browser_name=Chrome&browser_version=116.0.0.0&web_rid=%s&msToken=",
		dl.liveID,
	)
	// 参考代码 https://github.com/ihmily/DouyinLiveRecorder
	headers := map[string]string{
		"Accept-Encoding": "identity",
		"Cookie":          "ttwid=1%7C2iDIYVmjzMcpZ20fcaFde0VghXAA3NaNXE_SLR68IyE%7C1761045455%7Cab35197d5cfb21df6cbb2fa7ef1c9262206b062c315b9d04da746d0b37dfbc7d",
		"Referer":         "https://live.douyin.com/" + dl.liveID,
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/116.0.5845.97 Safari/537.36 Core/1.116.567.400 QQBrowser/19.7.6764.400",
	}
	aBogus := sign.AbSign(params, headers["User-Agent"])
	url := fmt.Sprintf("https://live.douyin.com/webcast/room/web/enter/?%s&a_bogus=%s", params, aBogus)
	ctx, cancel := dl.requestContext()
	defer cancel()

	resp, err := dl.client.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get(url)
	if err != nil {
		body := ""
		if resp != nil {
			body = resp.String()
		}
		return body, fmt.Errorf("请求直播间信息失败: %w", err)
	}
	if resp == nil {
		return "", errors.New("直播间信息响应为空")
	}

	body := resp.String()

	if body == "" {
		return "", errors.New("直播间信息响应为空")
	}

	if statusCode := gjson.Get(body, "status_code").Int(); statusCode != 0 {
		return "", fmt.Errorf("直播间信息接口返回异常 status_code=%d", statusCode)
	}

	return body, nil
}

func (dl *DouyinLive) logRoomInfoResponseSummary(body string) {
	if dl.logger == nil {
		return
	}
	dl.logger.Warn("直播间信息响应无法提取房间参数",
		"live_id", dl.liveID,
		"body_len", len(body),
		"status_code", gjson.Get(body, "status_code").Int(),
		"message", firstNonEmptyGJSON(body, "message", "prompts", "extra.log_pb.impr_id"),
		"has_data_data_0", gjson.Get(body, "data.data.0").Exists(),
		"has_enter_room_id", gjson.Get(body, "data.enter_room_id").Exists(),
		"has_user", gjson.Get(body, "data.user").Exists(),
	)
}

// refreshLiveStatusFromAPI 通过房间接口刷新当前直播状态。
func (dl *DouyinLive) refreshLiveStatusFromAPI() (bool, error) {
	isLive, err := dl.fetchLiveStatusFromAPI()
	if err != nil {
		return false, err
	}
	dl.setLiveStatus(isLive)
	return isLive, nil
}

func (dl *DouyinLive) fetchLiveStatusFromAPI() (bool, error) {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()

	if err := dl.prepareRequestContextLocked(); err != nil {
		return false, err
	}

	body, err := dl.refreshRoomEnterData()
	if err != nil {
		return false, err
	}

	status := gjson.Get(body, "data.data.0.status").Int()
	return status == 2, nil
}

// IsLive 检查直播间是否开播，并返回判活过程中的错误。
func (dl *DouyinLive) IsLive() (bool, error) {
	isLive, err := dl.refreshLiveStatusFromAPI()
	if err != nil {
		dl.setLiveStatus(false)
		return false, err
	}
	return isLive, nil
}

// setLiveStatus 设置直播间状态（线程安全）
func (dl *DouyinLive) setLiveStatus(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.isLiveClosed = status
	if status {
		dl.liveStatusGuard.Reset()
	}
}

// isLiveStatus 获取直播间状态（线程安全）
func (dl *DouyinLive) isLiveStatus() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.isLiveClosed
}

// setManualClose 设置是否为手动关闭（线程安全）
func (dl *DouyinLive) setManualClose(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.manualClose = status
}

// isManualClose 获取是否为手动关闭（线程安全）
func (dl *DouyinLive) isManualClose() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.manualClose
}

// Start 启动直播间连接。
// 方法内部会先刷新直播状态，确保作为库直接调用时也能进入消息处理循环。
func (dl *DouyinLive) Start() error {
	if dl.isManualClose() {
		return context.Canceled
	}
	dl.resetCloseSignal()
	dl.setManualClose(false)
	defer dl.cleanup()

	dl.logger.Info("开始连接抖音直播间", "live_id", dl.liveID)
	isLive, err := dl.refreshLiveStatusFromAPI()
	if err != nil {
		dl.setLiveStatus(false)
		dl.logger.Error("刷新直播状态失败", "live_id", dl.liveID, "err", err)
		return err
	}
	if !isLive {
		dl.logger.Info("直播间未开播", "live_id", dl.liveID)
		return ErrLiveNotStarted
	}

	if err := dl.startWebSocket(); err != nil {
		dl.logger.Warn("WebSocket 连接失败，准备重连", "live_id", dl.liveID, "err", err)
		if dl.reconnect(defaultMaxRetries, true, false) {
			dl.processMessages()
			return nil
		}
		return err
	}

	dl.processMessages()
	return nil
}

// startWebSocket 基于当前上下文建立 WebSocket 连接。
func (dl *DouyinLive) startWebSocket() error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = websocketConnectTimeout
	url, headers, err := dl.websocketDialContext()
	if err != nil {
		return fmt.Errorf("构建WebSocket URL失败: %w", err)
	}
	ctx, cancel := dl.requestContext()
	defer cancel()

	conn, resp, err := dialer.DialContext(ctx, url, headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("连接失败 (状态码: %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("连接失败: %w", err)
	}
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}
	roomInfo := dl.roomInfoSnapshot()
	dl.logger.Info("WebSocket 连接成功", "live_id", roomInfo.liveID, "room_id", roomInfo.roomID, "live_name", roomInfo.liveName, "status_code", statusCode)
	dl.mu.Lock()
	dl.conn = conn
	dl.mu.Unlock()
	dl.configureWebSocket(conn)
	dl.startHeartbeatLoop()
	return nil
}

func (dl *DouyinLive) configureWebSocket(conn *websocket.Conn) {
	if conn == nil {
		return
	}

	_ = conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
	})
}

// buildWebsocketURL 基于当前上下文构建 WebSocket URL
func (dl *DouyinLive) buildWebsocketURL() (string, error) {
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)
	roomInfo := dl.roomInfoSnapshot()
	browserInfo := dl.userAgent
	if parts := strings.SplitN(dl.userAgent, "Mozilla/", 2); len(parts) == 2 {
		browserInfo = parts[1]
	}
	parsedBrowser := queryEscapeValue(browserInfo)

	signer := dl.signer
	if signer == nil {
		signer = newLocalWebsocketSigner()
		dl.signer = signer
		signer.UpdateUserAgent(dl.userAgent)
	}
	ctx, cancel := dl.requestContext()
	defer cancel()
	signature, err := signer.Sign(ctx, roomInfo.roomID, roomInfo.pushID, dl.userAgent)
	if err != nil {
		return "", err
	}

	encodedSignature := queryEscapeValue(signature)
	return fmt.Sprintf(wssURLTemplate,
		parsedBrowser,
		roomInfo.roomID,
		roomInfo.pushID,
		fetchTime,
		fetchTime,
		fetchTime,
		roomInfo.pushID,
		roomInfo.roomID,
		encodedSignature,
	), nil
}

func (dl *DouyinLive) websocketDialContext() (string, http.Header, error) {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()

	if err := dl.prepareWebSocketContextLocked(); err != nil {
		return "", nil, fmt.Errorf("初始化失败: %w", err)
	}
	url, err := dl.buildWebsocketURL()
	if err != nil {
		return "", nil, err
	}
	return url, dl.headers.Clone(), nil
}

func (dl *DouyinLive) reconnectDialContext(changeUA bool, rebuildHTTP bool) (string, http.Header, error) {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()

	if err := dl.refreshReconnectContextLocked(changeUA, rebuildHTTP); err != nil {
		return "", nil, err
	}
	url, err := dl.buildWebsocketURL()
	if err != nil {
		return "", nil, err
	}
	return url, dl.headers.Clone(), nil
}

// processMessages 处理消息
func (dl *DouyinLive) processMessages() {
	for dl.isLiveStatus() {
		messageType, data, err := dl.readMessage()
		pushFrame := &new_douyin.Webcast_Im_PushFrame{}
		response := &new_douyin.Webcast_Im_Response{}
		controlMsg := &new_douyin.Webcast_Im_ControlMessage{}
		if err != nil {
			if dl.isManualClose() {
				dl.logger.Debug("WebSocket 读循环收到关闭信号", "live_id", dl.liveID, "err", err)
			} else {
				dl.logger.Warn("读取 WebSocket 消息失败", "live_id", dl.liveID, "err", err)
			}
			if !dl.handleReadError(err) {
				break
			}
			continue
		}

		if messageType != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		if err := proto.Unmarshal(data, pushFrame); err != nil {
			dl.logger.Warn("解析 PushFrame 失败", "live_id", dl.liveID, "payload_len", len(data), "err", err)
			continue
		}

		if pushFrame.PayloadType != "msg" {
			continue
		}

		if utils.HasGzipEncoding(pushFrame.Headers) {
			dl.handleGzipMessage(pushFrame, response, controlMsg)
			continue
		}
		dl.handlePlainMessage(pushFrame, response, controlMsg)
	}
}

// readMessage 读取消息
func (dl *DouyinLive) readMessage() (int, []byte, error) {
	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn == nil {
		return 0, nil, errors.New("连接已关闭")
	}
	return conn.ReadMessage()
}

func (dl *DouyinLive) writeBinaryMessage(data []byte) error {
	dl.writeMu.Lock()
	defer dl.writeMu.Unlock()

	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn == nil {
		return errors.New("连接已关闭")
	}

	if err := conn.SetWriteDeadline(time.Now().Add(wsWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func (dl *DouyinLive) sendPing() error {
	dl.writeMu.Lock()
	defer dl.writeMu.Unlock()

	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn == nil {
		return errors.New("连接已关闭")
	}

	deadline := time.Now().Add(wsWriteTimeout)
	if err := conn.SetWriteDeadline(deadline); err != nil {
		return err
	}
	return conn.WriteControl(websocket.PingMessage, []byte("ping"), deadline)
}

func (dl *DouyinLive) startHeartbeatLoop() {
	dl.stopHeartbeatLoop()

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	dl.mu.Lock()
	dl.heartbeatStopCh = stopCh
	dl.heartbeatDoneCh = doneCh
	dl.liveStatusGuard.Reset()
	dl.mu.Unlock()

	go func() {
		defer close(doneCh)

		heartbeatTicker := time.NewTicker(heartbeatInterval)
		defer heartbeatTicker.Stop()
		statusTicker := time.NewTicker(liveStatusPollInterval)
		defer statusTicker.Stop()

		for {
			select {
			case <-heartbeatTicker.C:
				if dl.isManualClose() || !dl.isLiveStatus() {
					return
				}
				if err := dl.sendPing(); err != nil {
					dl.logger.Warn("发送保活 ping 失败", "live_id", dl.liveID, "err", err)
					dl.closeCurrentConnection(websocket.CloseGoingAway, "ping failed")
					return
				}
			case <-statusTicker.C:
				if dl.isManualClose() || !dl.isLiveStatus() {
					return
				}
				isLive, err := dl.fetchLiveStatusFromAPI()
				if err != nil {
					dl.logger.Warn("HTTP 兜底检测直播状态失败", "live_id", dl.liveID, "err", err)
					continue
				}
				if dl.shouldCloseAfterStatusCheck(isLive) {
					dl.logger.Info("HTTP 兜底检测到直播已下播，关闭当前 WS 连接", "live_id", dl.liveID, "live_name", dl.GetName())
					dl.closeCurrentConnection(websocket.CloseNormalClosure, "live ended by api")
					return
				}
				if !isLive {
					dl.logger.Warn("HTTP 兜底检测到一次未开播状态，等待二次确认", "live_id", dl.liveID, "live_name", dl.GetName())
				}
			case <-stopCh:
				return
			}
		}
	}()
}

func (dl *DouyinLive) shouldCloseAfterStatusCheck(isLive bool) bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	shouldClose := dl.liveStatusGuard.Record(isLive)
	if shouldClose {
		dl.isLiveClosed = false
	} else if isLive {
		dl.isLiveClosed = true
	}
	return shouldClose
}

func (dl *DouyinLive) stopHeartbeatLoop() {
	dl.mu.Lock()
	stopCh := dl.heartbeatStopCh
	doneCh := dl.heartbeatDoneCh
	dl.heartbeatStopCh = nil
	dl.heartbeatDoneCh = nil
	dl.mu.Unlock()

	if stopCh != nil {
		select {
		case <-stopCh:
		default:
			close(stopCh)
		}
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(1500 * time.Millisecond):
			dl.logger.Warn("等待心跳循环退出超时，跳过阻塞等待", "live_id", dl.liveID)
		}
	}
}

func (dl *DouyinLive) closeCurrentConnection(code int, reason string) {
	dl.mu.Lock()
	conn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	if conn == nil {
		return
	}

	msg := websocket.FormatCloseMessage(code, reason)
	if err := conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(2*time.Second)); err != nil {
		dl.logger.Debug("发送 WebSocket 关闭消息失败", "live_id", dl.liveID, "reason", reason, "err", err)
	}
	if err := conn.Close(); err != nil {
		dl.logger.Warn("关闭 WebSocket 连接失败", "live_id", dl.liveID, "reason", reason, "err", err)
	}
}

// handleGzipMessage 处理 GZIP 消息
func (dl *DouyinLive) handleGzipMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if err := dl.decodeGzipResponse(pushFrame.Payload, pushFrame, response, controlMsg); err != nil {
		dl.logger.Warn("解析 GZIP Response 失败", "live_id", dl.liveID, "payload_len", len(pushFrame.Payload), "err", err)
	}
}

func (dl *DouyinLive) handlePlainMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if err := dl.decodeResponse(pushFrame.Payload, pushFrame, response, controlMsg); err != nil {
		dl.logger.Warn("解析 Response 失败", "live_id", dl.liveID, "payload_len", len(pushFrame.Payload), "err", err)
	}
}

func (dl *DouyinLive) decodeResponse(data []byte, pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) error {
	*response = new_douyin.Webcast_Im_Response{}
	if err := proto.Unmarshal(data, response); err != nil {
		return err
	}

	if response.NeedAck {
		dl.sendAck(pushFrame.LogID, response.InternalExt)
	}

	for _, msg := range response.Messages {
		if dl.isManualClose() || !dl.isLiveStatus() {
			break
		}
		dl.handleSingleMessage(msg, controlMsg)
	}
	return nil
}

func (dl *DouyinLive) decodeGzipResponse(data []byte, pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *new_douyin.Webcast_Im_ControlMessage) error {
	buf := dl.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer func() {
		if buf.Cap() > maxGzipPayloadSize {
			return
		}
		buf.Reset()
		dl.bufferPool.Put(buf)
	}()

	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer gz.Close()

	if _, err = buf.ReadFrom(io.LimitReader(gz, maxGzipPayloadSize+1)); err != nil {
		return err
	}
	if buf.Len() > maxGzipPayloadSize {
		return fmt.Errorf("gzip payload too large: %d bytes", buf.Len())
	}

	return dl.decodeResponse(buf.Bytes(), pushFrame, response, controlMsg)
}

// sendAck 发送 ACK 消息
func (dl *DouyinLive) sendAck(logID uint64, internalExt string) {
	ackFrame := &new_douyin.Webcast_Im_PushFrame{
		LogID:       logID,
		PayloadType: "ack",
		Payload:     []byte(internalExt),
	}

	data, err := proto.Marshal(ackFrame)
	if err != nil {
		dl.logger.Warn("ACK 序列化失败", "live_id", dl.liveID, "err", err)
		return
	}

	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn != nil {
		if err := dl.writeBinaryMessage(data); err != nil {
			dl.logger.Warn("发送 ACK 失败", "live_id", dl.liveID, "log_id", logID, "err", err)
		}
	}
}

// handleSingleMessage 处理单条消息
func (dl *DouyinLive) handleSingleMessage(msg *new_douyin.Webcast_Im_Message,
	controlMsg *new_douyin.Webcast_Im_ControlMessage) {
	if dl.isManualClose() || !dl.isLiveStatus() {
		return
	}

	if msg.Method == "WebcastControlMessage" {
		if err := proto.Unmarshal(msg.Payload, controlMsg); err != nil {
			dl.logger.Warn("解析控制消息失败", "live_id", dl.liveID, "payload_len", len(msg.Payload), "err", err)
			return
		}
		dl.emitEvent(msg, controlMsg)
		if controlMsg.GetAction() == controlActionLiveEnd {
			dl.logger.Info("收到直播结束控制消息", "live_id", dl.liveID, "live_name", dl.GetName(), "action", controlMsg.GetAction())
			dl.setLiveStatus(false)
		}
		return
	}

	dl.emitEvent(msg, nil)
}

func (dl *DouyinLive) reconnectPlan(reason string, failureCount int, baseDelay time.Duration, allowUARefresh bool) (delay time.Duration, changeUA bool, rebuildHTTP bool) {
	// 指数退避：delay = baseDelay * 2^(failureCount-1)
	// 失败次数越多，等待时间越长，避免频繁重试触发风控
	delay = baseDelay
	if failureCount > 1 {
		expDelay := baseDelay * (1 << (failureCount - 1))
		if expDelay > maxReconnectDelay {
			delay = maxReconnectDelay
		} else {
			delay = expDelay
		}
	}

	changeUA = false
	rebuildHTTP = false

	switch {
	case failureCount <= 1:
		changeUA = false
		rebuildHTTP = false
	case failureCount <= 3:
		changeUA = allowUARefresh
		rebuildHTTP = false
	default:
		changeUA = allowUARefresh
		rebuildHTTP = true
	}

	switch reason {
	case "try_again_later_1013":
		delay = max(delay, 5*time.Second)
		changeUA = true
	case "service_restart_1012":
		delay = max(delay, 3*time.Second)
	case "going_away_1001":
		delay = max(delay, 2*time.Second)
	}

	return delay, changeUA, rebuildHTTP
}

// max 是 time.Duration 版本的 max 函数
func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// reconnectDecision 描述重连策略
func (dl *DouyinLive) reconnectDecision(err error) (reason string, shouldRetry bool, delay time.Duration, allowUARefresh bool) {
	if dl.isManualClose() {
		return "manual_close", false, 0, false
	}

	if errors.Is(err, websocket.ErrCloseSent) {
		return "close_sent", false, 0, false
	}

	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNormalClosure:
			return "normal_close", false, 0, false
		case websocket.CloseAbnormalClosure:
			return "abnormal_close_1006", true, baseReconnectDelay, true
		case websocket.CloseTryAgainLater:
			return "try_again_later_1013", true, 5 * time.Second, true
		case websocket.CloseServiceRestart:
			return "service_restart_1012", true, 3 * time.Second, false
		case websocket.CloseGoingAway:
			return "going_away_1001", true, 2 * time.Second, false
		case websocket.ClosePolicyViolation:
			return "policy_violation_1008", false, 0, false
		case websocket.CloseInvalidFramePayloadData:
			return "invalid_frame_payload_1007", false, 0, false
		default:
			return fmt.Sprintf("close_code_%d", closeErr.Code), true, baseReconnectDelay, true
		}
	}

	return "network_or_unknown", true, baseReconnectDelay, true
}

// 修改 handleReadError 方法，使用库自带方法判断错误
func (dl *DouyinLive) handleReadError(err error) bool {
	// 如果是手动关闭，不进行重连
	if dl.isManualClose() {
		dl.logger.Info("连接被手动关闭，不进行重连", "live_id", dl.liveID)
		return false
	}
	if !dl.isLiveStatus() {
		dl.logger.Info("直播状态已结束，不进行重连", "live_id", dl.liveID)
		return false
	}

	isLive, statusErr := dl.fetchLiveStatusFromAPI()

	if statusErr != nil {
		dl.logger.Warn("WS 读错后 HTTP 兜底检测失败，继续按重连流程处理", "live_id", dl.liveID, "err", statusErr)
	} else if dl.shouldCloseAfterStatusCheck(isLive) {
		dl.logger.Info("WS 读错后 HTTP 兜底确认直播已下播，不再重连", "live_id", dl.liveID, "live_name", dl.GetName())
		return false
	} else if !isLive {
		dl.logger.Warn("WS 读错后 HTTP 兜底检测到一次未开播状态，继续重连等待二次确认", "live_id", dl.liveID, "live_name", dl.GetName())
	}

	reason, shouldRetry, baseDelay, allowUARefresh := dl.reconnectDecision(err)
	if !shouldRetry {
		dl.logger.Info("连接关闭且不重连", "live_id", dl.liveID, "reason", reason, "err", err)
		return false
	}

	failureCount := dl.recordReconnectFailure(reason)
	delay, changeUA, rebuildHTTP := dl.reconnectPlan(reason, failureCount, baseDelay, allowUARefresh)
	jitter := time.Duration(utils.GenerateJitterNanos(maxReconnectJitter))
	sleepFor := delay + jitter
	dl.logger.Warn("检测到需重连，将稍后尝试", "live_id", dl.liveID, "reason", reason, "failures", failureCount, "delay", sleepFor, "change_ua", changeUA, "rebuild_http", rebuildHTTP, "err", err)
	if !dl.waitForReconnectDelay(sleepFor) {
		dl.logger.Info("重连等待被关闭信号打断", "live_id", dl.liveID, "reason", reason)
		return false
	}

	return dl.reconnect(defaultMaxRetries, changeUA, rebuildHTTP)
}

// 优化后的 reconnect 方法
func (dl *DouyinLive) reconnect(attempts int, changeUA bool, rebuildHTTP bool) bool {
	// 如果是手动关闭，不进行重连
	if dl.isManualClose() {
		dl.logger.Info("连接被手动关闭，不进行重连", "live_id", dl.liveID)
		return false
	}

	dl.mu.Lock()
	oldConn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	dl.stopHeartbeatLoop()

	if oldConn != nil {
		msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "reconnecting")
		_ = oldConn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(3*time.Second))
		_ = oldConn.Close()
	}

	attemptIndex := 0
	retryable := func() error {
		attemptChangeUA := changeUA && attemptIndex > 0
		attemptRebuildHTTP := rebuildHTTP || attemptIndex >= 2

		url, headers, err := dl.reconnectDialContext(attemptChangeUA, attemptRebuildHTTP)
		if err != nil {
			attemptIndex++
			return err
		}

		dialer := *websocket.DefaultDialer
		dialer.HandshakeTimeout = websocketConnectTimeout
		ctx, cancel := dl.requestContext()
		defer cancel()
		conn, _, err := dialer.DialContext(ctx, url, headers)
		if err != nil {
			attemptIndex++
			if websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseInvalidFramePayloadData,
			) {
				return retry.Unrecoverable(err)
			}
			return err
		}

		dl.mu.Lock()
		dl.conn = conn
		dl.mu.Unlock()
		dl.configureWebSocket(conn)
		dl.startHeartbeatLoop()
		dl.resetReconnectTracking()
		return nil
	}

	retryCtx, cancelRetry := contextWithCloseSignal(dl.closeSignal())
	defer cancelRetry()

	err := retry.Do(
		retryable,
		retry.Attempts(uint(attempts)),
		retry.Context(retryCtx),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(maxReconnectJitter),
		retry.RetryIf(func(err error) bool {
			return !websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseInvalidFramePayloadData,
			)
		}),
		retry.OnRetry(func(n uint, err error) {
			nextAttempt := n + 2
			dl.logger.Warn("重试连接失败", "live_id", dl.liveID, "attempt", n+1, "next_attempt", nextAttempt, "change_ua", changeUA && nextAttempt > 1, "rebuild_http", rebuildHTTP || nextAttempt >= 3, "err", err)
		}),
	)
	if err != nil {
		if dl.isManualClose() || errors.Is(err, context.Canceled) {
			dl.logger.Info("重连已取消", "live_id", dl.liveID)
			return false
		}
		dl.logger.Error("连接最终失败", "live_id", dl.liveID, "err", err)
		return false
	}

	dl.logger.Info("重连成功", "live_id", dl.liveID, "live_name", dl.GetName())
	return true
}

// cleanup 清理资源
func (dl *DouyinLive) cleanup() {
	dl.stopHeartbeatLoop()

	dl.mu.Lock()
	conn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	dl.releaseCache()
	dl.logger.Info("抖音直播连接资源已释放", "live_id", dl.liveID)
}

// emitEvent 触发事件，遍历处理所有有效处理器
func (dl *DouyinLive) emitEvent(msg *new_douyin.Webcast_Im_Message, parsed proto.Message) {
	if msg == nil {
		return
	}

	dl.mu.Lock()
	handlers := append([]eventHandler(nil), dl.eventHandlers...)
	dl.mu.Unlock()

	parsedSnapshot := parsed
	if parsedSnapshot != nil {
		parsedSnapshot = proto.Clone(parsedSnapshot)
	}

	for _, handler := range handlers {
		if dl.isManualClose() {
			return
		}
		if !dl.hasEventHandler(handler.id) {
			continue
		}
		func(h eventHandler) {
			defer func() {
				if recovered := recover(); recovered != nil && dl.logger != nil {
					dl.logger.Error("旧事件处理器发生 panic", "live_id", dl.liveID, "panic", recovered)
				}
			}()
			h.handler(msg, parsed)
		}(handler)
	}

	roomInfo := dl.roomInfoSnapshot()
	dl.eventBus().publishWithLoggerUntil(dl.logger, &LiveMessage{
		LiveID:      roomInfo.liveID,
		RoomID:      roomInfo.roomID,
		LiveName:    roomInfo.liveName,
		Title:       roomInfo.title,
		AvatarThumb: roomInfo.avatarThumb,
		Raw:         msg,
		Parsed:      parsedSnapshot,
		ReceivedAt:  time.Now(),
	}, func() bool {
		return dl.isManualClose()
	})
}

func (dl *DouyinLive) hasEventHandler(id string) bool {
	if id == "" {
		return false
	}

	dl.mu.Lock()
	defer dl.mu.Unlock()

	for _, h := range dl.eventHandlers {
		if h.id == id {
			return true
		}
	}
	return false
}

// Subscribe 订阅事件，生成唯一ID
func (dl *DouyinLive) Subscribe(handler func(*new_douyin.Webcast_Im_Message, proto.Message)) string {
	if handler == nil {
		return ""
	}

	id := utils.GenerateUniqueID()
	dl.mu.Lock()
	dl.eventHandlers = append(dl.eventHandlers, eventHandler{
		id:      id,
		handler: handler,
	})
	dl.mu.Unlock()
	return id
}

// Unsubscribe 取消订阅事件，通过ID查找并移除
func (dl *DouyinLive) Unsubscribe(id string) {
	dl.eventBus().unsubscribe(id)

	dl.mu.Lock()
	defer dl.mu.Unlock()

	for i, h := range dl.eventHandlers {
		if h.id == id {
			dl.eventHandlers = append(dl.eventHandlers[:i], dl.eventHandlers[i+1:]...)
			break
		}
	}
}
