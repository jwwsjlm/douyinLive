package douyinLive

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinLive/v2/utils"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// startWebSocket 建立上游 WebSocket 连接并启动心跳。
// startWebSocket establishes the upstream WebSocket connection and starts heartbeats.
func (dl *DouyinLive) startWebSocket() error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = websocketConnectTimeout
	url, headers, err := dl.websocketDialContext()
	if err != nil {
		return fmt.Errorf("构建WebSocket URL失败: %w", err)
	}

	conn, resp, err := dl.dialWebSocketWithSignerFallback(&dialer, url, headers, 1)
	if err != nil {
		closeWebSocketHandshakeResponse(resp)
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
	dl.logger.Info("WebSocket 连接成功", logFlowArgs("ws", "dial", "live_id", roomInfo.liveID, "room_id", roomInfo.roomID, "live_name", roomInfo.liveName, "title", roomInfo.title, "status_code", statusCode, "host", websocketHostForLog(url), "sign_implementation", websocketSignerImplementationName(dl.signer))...)
	dl.mu.Lock()
	dl.conn = conn
	dl.mu.Unlock()
	dl.configureWebSocket(conn)
	dl.setLiveStatus(true)
	dl.markReady()
	dl.startHeartbeatLoop()
	return nil
}

// dialUpstreamWebSocket 执行一次带独立超时上下文的上游握手。
// dialUpstreamWebSocket performs one upstream handshake with an independent timeout context.
func (dl *DouyinLive) dialUpstreamWebSocket(dialer *websocket.Dialer, url string, headers http.Header, attempt int) (*websocket.Conn, *http.Response, error) {
	ctx, cancel := dl.requestContext()
	defer cancel()
	roomInfo := dl.roomInfoSnapshot()
	dl.logger.Info("开始建立上游 WebSocket",
		logFlowArgs("ws", "dial",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"user_unique_id", roomInfo.pushID,
			"host", websocketHostForLog(url),
			"url_len", len(url),
			"has_cookie", dl.getCookieString() != "",
			"attempt", attempt,
			"sign_implementation", websocketSignerImplementationName(dl.signer),
		)...,
	)
	return dialer.DialContext(ctx, url, headers)
}

// dialWebSocketWithSignerFallback 在收到上游 HTTP 握手响应时允许原生签名切换到 Goja 重试一次。
// dialWebSocketWithSignerFallback allows one native-to-Goja retry when the upstream returns an HTTP handshake response.
func (dl *DouyinLive) dialWebSocketWithSignerFallback(dialer *websocket.Dialer, url string, headers http.Header, attempt int) (*websocket.Conn, *http.Response, error) {
	conn, response, err := dl.dialUpstreamWebSocket(dialer, url, headers, attempt)
	if err == nil || response == nil || websocketSignerImplementationName(dl.signer) != "native" {
		return conn, response, err
	}

	fallback, ok := dl.signer.(websocketSignerFallback)
	if !ok {
		return conn, response, err
	}
	nativeErr := err
	nativeStatusCode := response.StatusCode
	nativeResponseBody := readWebSocketHandshakeResponseBody(response)
	response = nil
	switched, fallbackErr := fallback.ActivateFallback()
	if fallbackErr != nil {
		return nil, nil, fmt.Errorf("原生签名握手失败且 Goja 回退初始化失败: native=%v fallback=%w", nativeErr, fallbackErr)
	}
	if !switched {
		return nil, nil, nativeErr
	}

	roomInfo := dl.roomInfoSnapshot()
	dl.logger.Warn("原生 WebSocket 签名握手失败，切换 Goja 兼容签名重试",
		logFlowArgs("ws", "signature_fallback",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"native_status_code", nativeStatusCode,
			"native_response_body", nativeResponseBody,
			"sign_implementation", websocketSignerImplementationName(dl.signer),
			"err", nativeErr,
		)...,
	)
	url, headers, err = dl.websocketDialContext()
	if err != nil {
		return nil, nil, fmt.Errorf("构建 Goja 回退 WebSocket URL 失败: %w", err)
	}
	return dl.dialUpstreamWebSocket(dialer, url, headers, attempt+1)
}

// closeWebSocketHandshakeResponse 关闭失败握手返回的响应体，避免连接资源滞留。
// closeWebSocketHandshakeResponse closes a failed handshake response body to avoid retaining transport resources.
func closeWebSocketHandshakeResponse(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

// readWebSocketHandshakeResponseBody 读取并关闭失败握手响应体，仅保留有限长度用于定位上游拒绝原因。
// readWebSocketHandshakeResponseBody reads and closes a failed handshake body, retaining only a bounded diagnostic.
func readWebSocketHandshakeResponseBody(response *http.Response) string {
	if response == nil || response.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2048))
	_ = response.Body.Close()
	if err != nil {
		return fmt.Sprintf("<read failed: %v>", err)
	}
	text := strings.TrimSpace(string(body))
	if len(text) > 512 {
		text = text[:512] + "..."
	}
	return text
}

// Ready 返回首次上游 WebSocket 握手成功时关闭的通道。
// Ready returns a channel closed after the first upstream WebSocket handshake succeeds.
func (dl *DouyinLive) Ready() <-chan struct{} {
	dl.readyMu.Lock()
	defer dl.readyMu.Unlock()
	if dl.readyCh == nil {
		dl.readyCh = make(chan struct{})
	}
	return dl.readyCh
}

// markReady 标记上游 WebSocket 已经完成首次握手。
// markReady marks the upstream WebSocket as ready after its first handshake.
func (dl *DouyinLive) markReady() {
	dl.readyMu.Lock()
	defer dl.readyMu.Unlock()
	if dl.readyClosed || dl.readyCh == nil {
		return
	}
	close(dl.readyCh)
	dl.readyClosed = true
}

// configureWebSocket 设置 WebSocket 读取限制和 pong 处理。
// configureWebSocket configures WebSocket read limits and pong handling.
// 参数/Parameters:
//   - conn: 已建立的 WebSocket 连接。 Established WebSocket connection.
func (dl *DouyinLive) configureWebSocket(conn *websocket.Conn) {
	if conn == nil {
		return
	}

	_ = setWebSocketReadDeadline(conn)
	conn.SetPongHandler(func(string) error {
		return setWebSocketReadDeadline(conn)
	})
}

func setWebSocketReadDeadline(conn *websocket.Conn) error {
	if conn == nil {
		return errors.New("连接已关闭")
	}
	return conn.SetReadDeadline(time.Now().Add(wsReadTimeout))
}

func (dl *DouyinLive) refreshCurrentReadDeadline() error {
	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()
	return setWebSocketReadDeadline(conn)
}

// buildWebsocketURL 生成带签名的抖音 WebSocket URL。
// buildWebsocketURL builds the signed Douyin WebSocket URL.
func (dl *DouyinLive) buildWebsocketURL() (string, error) {
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)
	roomInfo := dl.roomInfoSnapshot()

	signer := dl.signer
	if signer == nil {
		signer = newLocalWebsocketSigner()
		dl.signer = signer
		signer.UpdateUserAgent(dl.userAgent)
	}
	if preparer, ok := signer.(websocketSignerPreparer); ok {
		if err := preparer.Prepare(dl.userAgent, dl.getCookieString()); err != nil {
			return "", fmt.Errorf("初始化本地 WebSocket 签名器失败: %w", err)
		}
	}
	ctx, cancel := dl.requestContext()
	defer cancel()
	signatureParams := newWebsocketSignatureParams(roomInfo.roomID, roomInfo.pushID)
	dl.logger.Debug("WebSocket 签名输入",
		logFlowArgs("ws", "signature",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"user_unique_id", roomInfo.pushID,
			"sign_provider", signer.Name(),
			"sign_implementation", websocketSignerImplementationName(signer),
			"websocket_key", signatureParams.Joined(),
			"x_ms_stub", signatureParams.XMSStub(),
		)...,
	)

	signature, err := signer.Sign(ctx, roomInfo.roomID, roomInfo.pushID, dl.userAgent)
	if err != nil {
		return "", err
	}

	cursor, internalExt, pushURL := dl.websocketURLState(fetchTime, roomInfo)
	screenWidth, screenHeight := dl.fingerprint.screenSize()
	params := newWebsocketURLParamsWithScreen(roomInfo, dl.userAgent, cursor, internalExt, signature, screenWidth, screenHeight)
	wsURL := pushURL + "?" + params.QueryString()
	dl.logger.Debug("WebSocket URL 参数已生成",
		logFlowArgs("ws", "build_url",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"user_unique_id", roomInfo.pushID,
			"push_url", pushURL,
			"cursor", cursor,
			"internal_ext", internalExt,
			"heartbeat_duration", 0,
			"signature_len", len(signature),
			"sign_implementation", websocketSignerImplementationName(signer),
			"url_len", len(wsURL),
		)...,
	)
	return wsURL, nil
}

func (dl *DouyinLive) buildInitialIMFetchParams(roomInfo roomInfoSnapshot, msToken string) string {
	screenWidth, screenHeight := dl.fingerprint.screenSize()
	return newInitialIMFetchParamsWithScreen(roomInfo, dl.userAgent, msToken, screenWidth, screenHeight).QueryString()
}

// initialIMFetchMSToken 返回 im/fetch 使用的 msToken，优先使用用户 Cookie 中的值。
// initialIMFetchMSToken returns the msToken for im/fetch, preferring the value from user cookies.
func (dl *DouyinLive) initialIMFetchMSToken() string {
	if msToken := strings.TrimSpace(dl.cookieValue("msToken")); msToken != "" {
		return msToken
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.msToken == "" {
		dl.msToken = utils.GenerateMsToken(172)
	}
	return dl.msToken
}

// fetchInitialIMState 请求 im/fetch 并保存 protobuf 返回的动态 WS 状态。
// fetchInitialIMState requests im/fetch and stores dynamic WS state returned by protobuf.
func (dl *DouyinLive) fetchInitialIMState() error {
	roomInfo := dl.roomInfoSnapshot()
	if roomInfo.roomID == "" || roomInfo.pushID == "" {
		return errors.New("room_id or user_unique_id is empty")
	}
	msTokenSource := "generated"
	if strings.TrimSpace(dl.cookieValue("msToken")) != "" {
		msTokenSource = "cookie"
	}
	msToken := dl.initialIMFetchMSToken()
	params := dl.buildInitialIMFetchParams(roomInfo, msToken)
	signed := signWebcastHTTPURL("https://live.douyin.com/webcast/im/fetch/", params, dl.userAgent)
	initialURL := signed.URL
	dl.logger.Debug("请求 IM 初始状态",
		logFlowArgs("im_fetch", "prefetch",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"user_unique_id", roomInfo.pushID,
			"endpoint", "/webcast/im/fetch/",
			"query_len", len(params),
			"ms_token_source", msTokenSource,
			"sign_provider", signed.Provider,
			"abogus_len", signed.ABogusLength,
			"sign_duration", signed.Duration,
		)...,
	)
	ctx, cancel := dl.requestContext()
	defer cancel()

	headers := map[string]string{
		"Accept":          "*/*",
		"Accept-Encoding": "identity",
		"Content-Type":    "application/x-www-form-urlencoded; charset=UTF-8",
		"Cookie":          dl.getCookieString(),
		"Referer":         "https://live.douyin.com/" + dl.liveID,
		"User-Agent":      dl.userAgent,
	}
	for key, value := range browserClientHintHeaders(dl.userAgent) {
		headers[key] = value
	}

	resp, err := dl.client.R().
		SetContext(ctx).
		SetHeaders(headers).
		Get(initialURL)
	if err != nil {
		return err
	}
	if resp == nil {
		return errors.New("empty im fetch response")
	}

	body, err := responseBytes(resp)
	if err != nil {
		return err
	}
	response := &new_douyin.Webcast_Im_Response{}
	if err := proto.Unmarshal(body, response); err != nil {
		return fmt.Errorf("parse im fetch protobuf failed status=%d content_type=%q raw_len=%d: %w",
			resp.GetStatusCode(),
			resp.GetHeader("Content-Type"),
			len(body),
			err,
		)
	}
	pushURL, pushURLSource := websocketPushURLFromResponseWithSource(response)
	if pushURL == "" {
		return fmt.Errorf("im fetch response missing websocket push server status=%d content_type=%q raw_len=%d",
			resp.GetStatusCode(),
			resp.GetHeader("Content-Type"),
			len(body),
		)
	}
	dl.applyWebsocketResponseState(response)
	dl.logger.Debug("预取 IM 状态成功",
		logFlowArgs("im_fetch", "prefetch",
			"live_id", roomInfo.liveID,
			"room_id", roomInfo.roomID,
			"user_unique_id", roomInfo.pushID,
			"status", resp.GetStatusCode(),
			"content_type", resp.GetHeader("Content-Type"),
			"raw_len", len(body),
			"cursor", response.Cursor,
			"internal_ext", response.InternalExt,
			"heartbeat_duration", response.HeartbeatDuration,
			"push_server_v2", response.PushServerV2,
			"push_server", response.PushServer,
			"proxy_server", response.ProxyServer,
			"push_url_source", pushURLSource,
			"push_url_host", websocketHostForLog(pushURL),
		)...,
	)
	return nil
}

// websocketURLState 返回构造 WS URL 所需的 cursor、internal_ext 和动态 push URL。
// websocketURLState returns cursor, internal_ext, and dynamic push URL needed to build the WS URL.
// 参数/Parameters:
//   - fetchTime: 当前毫秒时间戳，用于缺省 internal_ext。 Current millisecond timestamp used for default internal_ext.
//   - roomInfo: 房间信息快照。 Room metadata snapshot.
func (dl *DouyinLive) websocketURLState(fetchTime int64, roomInfo roomInfoSnapshot) (cursor string, internalExt string, pushURL string) {
	cursor, internalExt, pushURL = dl.websocketStateSnapshot()
	if cursor == "" {
		cursor = defaultCursor
	}
	if internalExt == "" {
		internalExt = defaultInternalExt(roomInfo.roomID, roomInfo.pushID, fetchTime)
	}
	if pushURL == "" {
		pushURL = websocketPushURL
	}
	return cursor, internalExt, pushURL
}

// websocketDialContext 准备首次建连所需的 URL 和请求头。
// websocketDialContext prepares the URL and headers for the initial dial.
func (dl *DouyinLive) websocketDialContext() (string, http.Header, error) {
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()

	if !dl.contextPrepared {
		if err := dl.prepareWebSocketContextLocked(); err != nil {
			return "", nil, fmt.Errorf("初始化失败: %w", err)
		}
	} else {
		dl.logger.Debug("复用已准备的 WebSocket 上下文", logFlowArgs("ws", "prepare", "live_id", dl.liveID, "reused", true)...)
	}
	url, err := dl.buildWebsocketURL()
	if err != nil {
		return "", nil, err
	}
	return url, dl.headers.Clone(), nil
}

// reconnectDialContext 准备重连所需的 URL 和请求头。
// reconnectDialContext prepares the URL and headers for a reconnect dial.
// 参数/Parameters:
//   - changeUA: 是否更换浏览器 User-Agent。 Whether to rotate the browser User-Agent.
//   - rebuildHTTP: 是否重建 HTTP 客户端。 Whether to rebuild the HTTP client.
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

// processMessages 持续读取上游 WebSocket 消息并按编码类型分发解析。
// processMessages continuously reads upstream WebSocket messages and dispatches decoding by encoding type.
func (dl *DouyinLive) processMessages() {
	for dl.isLiveStatus() {
		messageType, data, err := dl.readMessage()
		pushFrame := &new_douyin.Webcast_Im_PushFrame{}
		response := &new_douyin.Webcast_Im_Response{}
		controlMsg := &new_douyin.Webcast_Im_ControlMessage{}
		if err != nil {
			if dl.isManualClose() {
				dl.logger.Debug("WebSocket 读循环收到关闭信号", logFlowArgs("ws", "read", "live_id", dl.liveID, "reason", "manual_close", "err", err)...)
			} else {
				dl.logger.Warn("读取 WebSocket 消息失败", logFlowArgs("ws", "read", "live_id", dl.liveID, "reason", classifyReadError(err), "err", err)...)
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
			dl.logger.Warn("解析 PushFrame 失败", logFlowArgs("ws", "decode_push_frame", "live_id", dl.liveID, "payload_len", len(data), "message_type", messageType, "err", err)...)
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

// readMessage 从当前 WebSocket 连接读取一条消息。
// readMessage reads one message from the current WebSocket connection.
func (dl *DouyinLive) readMessage() (int, []byte, error) {
	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn == nil {
		return 0, nil, errors.New("连接已关闭")
	}
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		return messageType, data, err
	}
	if err := setWebSocketReadDeadline(conn); err != nil {
		return messageType, data, err
	}
	return messageType, data, nil
}

// writeBinaryMessage 串行写入二进制 WebSocket 消息。
// writeBinaryMessage serially writes a binary WebSocket message.
// 参数/Parameters:
//   - data: 要发送的二进制消息体。 Binary message payload to send.
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

// closeCurrentConnection 关闭当前 WebSocket 连接并发送 close 控制帧。
// closeCurrentConnection closes the current WebSocket connection and sends a close control frame.
// 参数/Parameters:
//   - code: WebSocket close 状态码。 WebSocket close status code.
//   - reason: close 控制帧原因文本。 Close-frame reason text.
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
