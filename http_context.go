package douyinLive

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jwwsjlm/douyinLive/v2/jsScript"
	"github.com/jwwsjlm/req/v3"
)

var impersonatedUserAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; WOW64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 11.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36",
}

// newHTTPUserAgent 随机选择一个用于 HTTP 伪装的浏览器 UA。
// newHTTPUserAgent randomly selects a browser user agent for HTTP impersonation.
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

// newHTTPClient 创建带浏览器伪装和超时设置的 HTTP 客户端。
// newHTTPClient creates an HTTP client with browser impersonation and timeout settings.
// 参数/Parameters:
//   - userAgent: 请求使用的浏览器 User-Agent。 Browser User-Agent used for requests.
func newHTTPClient(userAgent string) *req.Client {
	return req.C().
		ImpersonateChromeWithOS(req.BrowserOSWindows).
		EnableHTTP3().
		EnableHTTP3FallbackOnError().
		SetUserAgent(userAgent).
		SetTimeout(httpRequestTimeout)
}

// queryEscapeValue 按查询参数规则转义并保持空格为 %20。
// queryEscapeValue escapes a query value while preserving spaces as %20.
// 参数/Parameters:
//   - value: 待转义的查询参数值。 Query value to escape.
func queryEscapeValue(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

func queryEscapeURLSearchParamsValue(value string) string {
	return url.QueryEscape(value)
}

func responseString(resp *req.Response) (string, error) {
	if resp == nil {
		return "", errRoomInfoEmpty
	}
	return resp.ToString()
}

func responseBytes(resp *req.Response) ([]byte, error) {
	if resp == nil {
		return nil, errors.New("empty response")
	}
	return resp.ToBytes()
}

// rebuildHTTPClientAndHeaders 重建 HTTP 客户端并刷新基础请求头。
// rebuildHTTPClientAndHeaders rebuilds the HTTP client and refreshes base headers.
func (dl *DouyinLive) rebuildHTTPClientAndHeaders() {
	dl.client = newHTTPClient(dl.userAgent)
	dl.headers = make(http.Header)
	dl.headers.Set("User-Agent", dl.userAgent)
	dl.refreshSignerUserAgent()
}

// refreshSignerUserAgent 将当前 UA 同步给签名器。
// refreshSignerUserAgent syncs the current user agent to the signer.
func (dl *DouyinLive) refreshSignerUserAgent() {
	if dl.signer != nil {
		dl.signer.UpdateUserAgent(dl.userAgent)
	}
}

// ensureCloseContextLocked 在持锁状态下确保关闭上下文存在。
// ensureCloseContextLocked ensures the close context exists while the lock is held.
func (dl *DouyinLive) ensureCloseContextLocked() {
	if dl.closeCtx != nil && dl.closeCancel != nil {
		return
	}
	dl.closeCtx, dl.closeCancel = context.WithCancel(context.Background())
	if dl.closeSignalClosed {
		dl.closeCancel()
	}
}

// signalClose 广播关闭信号并取消关闭上下文。
// signalClose broadcasts the close signal and cancels the close context.
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

// resetCloseSignal 为新一轮 Start 流程重置关闭信号。
// resetCloseSignal resets the close signal for a new Start cycle.
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

// closeSignal 返回当前关闭信号通道。
// closeSignal returns the current close-signal channel.
func (dl *DouyinLive) closeSignal() <-chan struct{} {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.closeCh == nil {
		dl.closeCh = make(chan struct{})
	}
	return dl.closeCh
}

// waitForReconnectDelay 等待重连延迟，并在关闭信号到来时提前退出。
// waitForReconnectDelay waits for reconnect delay and exits early on close signal.
// 参数/Parameters:
//   - delay: 本次重连前等待的时长。 Duration to wait before the reconnect attempt.
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

// requestContext 创建受关闭信号和请求超时共同控制的上下文。
// requestContext creates a context governed by both close signal and request timeout.
func (dl *DouyinLive) requestContext() (context.Context, context.CancelFunc) {
	dl.mu.Lock()
	dl.ensureCloseContextLocked()
	parent := dl.closeCtx
	dl.mu.Unlock()

	return context.WithTimeout(parent, httpRequestTimeout)
}

// contextWithCloseSignal 将关闭通道转换为可取消上下文。
// contextWithCloseSignal converts a close channel into a cancellable context.
// 参数/Parameters:
//   - closeCh: 关闭信号通道。 Close-signal channel.
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

// prepareRequestContextLocked 在持上下文锁时准备 HTTP 请求头和 Cookie。
// prepareRequestContextLocked prepares HTTP headers and cookies while the context lock is held.
func (dl *DouyinLive) prepareRequestContextLocked() error {
	if dl.shouldFetchTTWID() {
		if err := dl.fetchTTWID(); err != nil {
			return err
		}
	}

	dl.headers.Set("User-Agent", dl.userAgent)
	dl.headers.Set("Origin", "https://live.douyin.com")
	dl.headers.Set("Referer", "https://live.douyin.com/"+dl.liveID)
	dl.setupCookies()
	return nil
}

// prepareWebSocketContextLocked 准备 WebSocket 建连所需的房间信息和签名运行时。
// prepareWebSocketContextLocked prepares room data and signer runtime needed for WebSocket dialing.
func (dl *DouyinLive) prepareWebSocketContextLocked() error {
	if err := dl.prepareRequestContextLocked(); err != nil {
		return err
	}

	if err := dl.fetchLivePageState(); err != nil {
		dl.logger.Debug("从直播间页面预取状态失败，继续请求 web/enter", logFlowArgs("room_info", "live_page_state", "live_id", dl.liveID, "endpoint", "live_page", "fallback", "web_enter", "err", err)...)
	}
	if dl.isKnownOfflineStatus() {
		roomInfo := dl.roomInfoSnapshot()
		if roomInfo.roomID != "" || roomInfo.liveName != "" || roomInfo.title != "" {
			dl.logger.Info("直播页显示当前未开播，暂不建立上游 WebSocket",
				logFlowArgs("room_info", "live_page_offline",
					"live_id", dl.liveID,
					"room_id", roomInfo.roomID,
					"live_name", roomInfo.liveName,
					"title", roomInfo.title,
				)...,
			)
			return nil
		}
	}

	initialIMFetched := false
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID != "" && roomInfo.pushID != "" {
		if err := dl.fetchInitialIMState(); err != nil {
			dl.logger.Debug("预取 IM cursor 失败，继续使用 web/enter 后兜底", logFlowArgs("im_fetch", "prefetch", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "web_enter", "err", err)...)
		} else {
			initialIMFetched = true
		}
	}

	if _, err := dl.fetchRoomEnterData(); err != nil {
		roomInfo := dl.roomInfoSnapshot()
		if !isRoomInfoEmptyError(err) || roomInfo.roomID == "" || roomInfo.pushID == "" {
			return err
		}
		dl.logger.Debug("web/enter 返回空响应，已使用直播间页面状态继续", logFlowArgs("room_info", "web_enter", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "live_page_state", "err", err)...)
	}

	if dl.signer == nil || dl.signer.Name() == SignProviderLocal {
		if err := jsScript.LoadGojaWithCookie(dl.userAgent, dl.getCookieString()); err != nil {
			return fmt.Errorf("加载JavaScript脚本失败: %w", err)
		}

	}
	if !initialIMFetched {
		if err := dl.fetchInitialIMState(); err != nil {
			roomInfo := dl.roomInfoSnapshot()
			dl.logger.Debug("预取 IM cursor 失败，继续使用兜底 WebSocket 参数", logFlowArgs("im_fetch", "prefetch", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "default_ws_params", "err", err)...)
		}
	}

	return nil
}

// PrepareWebSocketContext 按网页流程预取直播页、im/fetch 和签名上下文。
// PrepareWebSocketContext preloads live page, im/fetch, and signing context using the browser flow.
func (dl *DouyinLive) PrepareWebSocketContext() error {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()
	return dl.prepareWebSocketContextLocked()
}

// refreshReconnectContextLocked 按重连策略刷新 UA、HTTP 客户端和房间上下文。
// refreshReconnectContextLocked refreshes user agent, HTTP client, and room context for reconnects.
// 参数/Parameters:
//   - changeUA: 是否更换浏览器 User-Agent。 Whether to rotate the browser User-Agent.
//   - rebuildHTTP: 是否重建 HTTP 客户端。 Whether to rebuild the HTTP client.
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

// getCookieParts 组装当前有效 Cookie 的键值片段。
// getCookieParts builds key-value parts for the currently effective cookies.
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

// getCookieString 返回用于请求头的 Cookie 字符串。
// getCookieString returns the Cookie header string.
func (dl *DouyinLive) getCookieString() string {
	parts := dl.getCookieParts()
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "; ")
}

// cookieValue 按名称读取当前有效 Cookie 值，优先使用用户配置的 Cookie。
// cookieValue reads the effective cookie value by name, preferring user-configured cookies.
// 参数/Parameters:
//   - name: Cookie 名称。 Cookie name.
func (dl *DouyinLive) cookieValue(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || dl.cookieManager == nil {
		return ""
	}

	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		for _, c := range dl.cookieManager.ParseCookies(configCookie) {
			if c.Name == name {
				return c.Value
			}
		}
		return ""
	}

	if name == "ttwid" {
		return dl.ttwid
	}
	return dl.additionalCookies[name]
}

// shouldFetchTTWID 判断是否需要自动请求首页获取 ttwid。
// shouldFetchTTWID reports whether ttwid should be fetched automatically from the homepage.
func (dl *DouyinLive) shouldFetchTTWID() bool {
	return !dl.hasConfiguredCookie()
}

// hasConfiguredCookie 判断用户是否显式配置了 Cookie。
// hasConfiguredCookie reports whether the user explicitly configured a Cookie.
func (dl *DouyinLive) hasConfiguredCookie() bool {
	return dl.cookieManager != nil && strings.TrimSpace(dl.cookieManager.GetDouyinCookie()) != ""
}

// setupCookies 将当前 Cookie 写入请求头。
// setupCookies writes the current cookie string into request headers.
func (dl *DouyinLive) setupCookies() {
	dl.headers.Set("Cookie", dl.getCookieString())
}

// fetchTTWID 请求抖音首页并提取 ttwid 及附加 Cookie。
// fetchTTWID requests the Douyin homepage and extracts ttwid plus extra cookies.
func (dl *DouyinLive) fetchTTWID() error {
	ctx, cancel := dl.requestContext()
	defer cancel()

	resp, err := dl.client.R().
		SetContext(ctx).
		Get("https://live.douyin.com/")
	if err != nil {
		return fmt.Errorf("请求TTWID失败: %w", err)
	}

	// 收集所有 Cookie。
	// Collect all cookies.
	cookies := make(map[string]string)
	for _, c := range resp.Cookies() {
		cookies[c.Name] = c.Value
	}

	// 刷新额外 Cookie，避免旧值残留。
	// Refresh extra cookies to avoid stale values.
	dl.additionalCookies = make(map[string]string)

	// 优先使用 ttwid，如果没有找到则报错。
	// Prefer ttwid and fail when it is missing.
	if ttwid, exists := cookies["ttwid"]; exists {
		dl.ttwid = ttwid
	} else {
		return errors.New("未找到TTWID cookie")
	}

	// 存储其他重要 Cookie。
	// Store the remaining useful cookies.
	for name, value := range cookies {
		if name != "ttwid" {
			dl.additionalCookies[name] = value
		}
	}
	return nil
}

// chromeVersionFromUserAgent 从 User-Agent 中提取 Chrome 完整版本号。
// chromeVersionFromUserAgent extracts the full Chrome version from User-Agent.
// 参数/Parameters:
//   - userAgent: 浏览器 User-Agent 字符串。 Browser User-Agent string.
func chromeVersionFromUserAgent(userAgent string) string {
	const marker = "Chrome/"
	if idx := strings.Index(userAgent, marker); idx >= 0 {
		version := userAgent[idx+len(marker):]
		if end := strings.IndexByte(version, ' '); end >= 0 {
			version = version[:end]
		}
		if version != "" {
			return version
		}
	}
	return browserVersionFromUserAgent(userAgent)
}

func chromeMajorVersionFromUserAgent(userAgent string) string {
	version := chromeVersionFromUserAgent(userAgent)
	if major, _, ok := strings.Cut(version, "."); ok && major != "" {
		return major
	}
	if version != "" {
		return version
	}
	return "133"
}

func browserClientHintHeaders(userAgent string) map[string]string {
	major := chromeMajorVersionFromUserAgent(userAgent)
	return map[string]string{
		"sec-ch-ua":          fmt.Sprintf(`"Not;A=Brand";v="8", "Chromium";v="%s", "Google Chrome";v="%s"`, major, major),
		"sec-ch-ua-mobile":   "?0",
		"sec-ch-ua-platform": `"Windows"`,
	}
}
