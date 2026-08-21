package douyinLive

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jwwsjlm/req/v3"
	"net/url"
)

const slowWebSocketPreparationStep = 2 * time.Second

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
	return dl.requestContextWithParent(context.Background())
}

// requestContextWithParent combines the caller context with the listener close context and request timeout.
// requestContextWithParent 将调用方上下文、监听器关闭上下文和请求超时合并起来。
func (dl *DouyinLive) requestContextWithParent(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	dl.mu.Lock()
	dl.ensureCloseContextLocked()
	closeCtx := dl.closeCtx
	dl.mu.Unlock()

	merged, mergeCancel := context.WithCancel(parent)
	stopClosePropagation := context.AfterFunc(closeCtx, mergeCancel)
	timed, timeoutCancel := context.WithTimeout(merged, httpRequestTimeout)
	return timed, func() {
		stopClosePropagation()
		timeoutCancel()
		mergeCancel()
	}
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
func (dl *DouyinLive) prepareRequestContextLocked(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if dl.shouldFetchTTWID() {
		if err := dl.fetchTTWID(ctx); err != nil {
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

func (dl *DouyinLive) prepareWebSocketContextLocked() (err error) {
	if err := dl.ensureUsable(); err != nil {
		return err
	}
	startedAt := time.Now()
	dl.contextPrepared = false
	defer func() {
		dl.contextPrepared = err == nil
		dl.logger.Debug("WebSocket 上下文准备完成",
			"live_id", dl.liveID,
			"success", err == nil,
			"reused", false,
			"duration", time.Since(startedAt).Round(time.Millisecond),
			"err", err,
		)
	}()

	stepStartedAt := time.Now()
	ctx, cancel := dl.requestContext()
	defer cancel()
	if err := dl.prepareRequestContextLocked(ctx); err != nil {
		dl.logWebSocketPreparationStep("request_context", stepStartedAt, err)
		return err
	}
	dl.logWebSocketPreparationStep("request_context", stepStartedAt, nil)

	stepStartedAt = time.Now()
	if err := dl.fetchLivePageState(); err != nil {
		dl.logger.Debug("从直播间页面预取状态失败，继续请求 web/enter", logFlowArgs("room_info", "live_page_state", "live_id", dl.liveID, "endpoint", "live_page", "fallback", "web_enter", "err", err)...)
		dl.logWebSocketPreparationStep("live_page", stepStartedAt, err)
	} else {
		dl.logWebSocketPreparationStep("live_page", stepStartedAt, nil)
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
			return ErrLiveNotStarted
		}
	}

	initialIMFetched := false
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID != "" && roomInfo.pushID != "" {
		stepStartedAt = time.Now()
		if err := dl.fetchInitialIMState(); err != nil {
			dl.logger.Debug("预取 IM cursor 失败，继续使用 web/enter 后兜底", logFlowArgs("im_fetch", "prefetch", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "web_enter", "err", err)...)
			dl.logWebSocketPreparationStep("initial_im_fetch", stepStartedAt, err)
		} else {
			initialIMFetched = true
			dl.logWebSocketPreparationStep("initial_im_fetch", stepStartedAt, nil)
		}
	}

	stepStartedAt = time.Now()
	if _, err := dl.fetchRoomEnterData(); err != nil {
		dl.logWebSocketPreparationStep("room_enter", stepStartedAt, err)
		roomInfo := dl.roomInfoSnapshot()
		if !isRoomInfoEmptyError(err) || roomInfo.roomID == "" || roomInfo.pushID == "" {
			return err
		}
		dl.logger.Debug("web/enter 返回空响应，已使用直播间页面状态继续", logFlowArgs("room_info", "web_enter", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "live_page_state", "err", err)...)
	} else {
		dl.logWebSocketPreparationStep("room_enter", stepStartedAt, nil)
	}
	if dl.isKnownOfflineStatus() {
		return ErrLiveNotStarted
	}
	if !dl.IsKnownLiveStatus() {
		return fmt.Errorf("%w: live_id=%s", ErrLiveStatusUnknown, dl.liveID)
	}

	if preparer, ok := dl.signer.(websocketSignerPreparer); ok {
		stepStartedAt = time.Now()
		if err := preparer.Prepare(dl.userAgent, dl.getCookieString()); err != nil {
			dl.logWebSocketPreparationStep("websocket_signer_runtime", stepStartedAt, err)
			return fmt.Errorf("加载JavaScript脚本失败: %w", err)
		}
		dl.logWebSocketPreparationStep("websocket_signer_runtime", stepStartedAt, nil)
	}
	if !initialIMFetched {
		stepStartedAt = time.Now()
		if err := dl.fetchInitialIMState(); err != nil {
			roomInfo := dl.roomInfoSnapshot()
			dl.logger.Debug("预取 IM cursor 失败，继续使用兜底 WebSocket 参数", logFlowArgs("im_fetch", "prefetch", "live_id", dl.liveID, "room_id", roomInfo.roomID, "user_unique_id", roomInfo.pushID, "fallback", "default_ws_params", "err", err)...)
			dl.logWebSocketPreparationStep("initial_im_fetch", stepStartedAt, err)
		} else {
			dl.logWebSocketPreparationStep("initial_im_fetch", stepStartedAt, nil)
		}
	}

	return nil
}

// logWebSocketPreparationStep 记录连接准备步骤的耗时；慢步骤会在 info 日志级别下以 WARN 输出。
// logWebSocketPreparationStep records preparation timing and emits slow steps as warnings at the info log level.
func (dl *DouyinLive) logWebSocketPreparationStep(step string, startedAt time.Time, err error) {
	duration := time.Since(startedAt).Round(time.Millisecond)
	args := logFlowArgs("ws", "prepare", "live_id", dl.liveID, "step", step, "duration", duration)
	if err != nil {
		args = append(args, "err", err)
	}
	if duration >= slowWebSocketPreparationStep {
		dl.logger.Warn("WebSocket 上下文准备步骤耗时较长", args...)
		return
	}
	dl.logger.Debug("WebSocket 上下文准备步骤完成", args...)
}

// PrepareWebSocketContext 按网页流程预取直播页、im/fetch 和签名上下文。
// PrepareWebSocketContext preloads live page, im/fetch, and signing context using the browser flow.
func (dl *DouyinLive) PrepareWebSocketContext() error {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()
	if err := dl.ensureUsable(); err != nil {
		return err
	}
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
			newUserAgent := newHTTPUserAgentExcept(oldUserAgent)
			dl.userAgent = newUserAgent
			dl.lastUserAgentChange = now
			dl.fingerprint = newBrowserFingerprint()
			dl.refreshSignerFingerprint()
			dl.logger.Info("重连前刷新浏览器画像", "live_id", dl.liveID, "old_user_agent", oldUserAgent, "new_user_agent", newUserAgent, "fingerprint_preset", dl.fingerprint.Preset, "fingerprint_id", dl.fingerprint.ID, "screen_width", dl.fingerprint.ScreenWidth, "screen_height", dl.fingerprint.ScreenHeight)
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
