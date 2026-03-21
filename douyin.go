package douyinLive

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"

	"io"
	"math/rand"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tidwall/gjson"

	"github.com/avast/retry-go"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/jsScript"
	"github.com/jwwsjlm/douyinLive/sign"
	"google.golang.org/protobuf/proto"

	"github.com/jwwsjlm/douyinLive/utils"
)

const (
	defaultMaxRetries       = 5
	websocketConnectTimeout = 10 * time.Second
	baseReconnectDelay      = 1500 * time.Millisecond
	maxReconnectJitter      = 1200 * time.Millisecond
	minUAChangeInterval     = 8 * time.Second
	gzipBufferSize          = 1024 * 4
	wsWriteTimeout          = 5 * time.Second
	heartbeatInterval       = 20 * time.Second
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
	roomIDRegex     = regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	pushIDRegex     = regexp.MustCompile(`user_unique_id\\":\\"(\d+)\\"`)
	isLiveRegex     = regexp.MustCompile(`id_str\\":\\"(\d+)\\",\\"status\\":(\d+),\\"status_str\\":\\"(\d+)\\",\\"title\\":\\"(.*?)\\",\\"user_count_str\\":\\"(.*?)\\"`)
	anchorInfoRegex = regexp.MustCompile(`data-anchor-info="([\s\S]*?)" data-room-info="`)

	ErrRiskControlPage = errors.New("检测到页面风控或验证")
)

// DouyinLive 结构体定义
type DouyinLive struct {
	liveID                 string
	roomID                 string
	pushID                 string
	LiveName               string
	ttwid                  string
	userAgent              string
	client                 *req.Client
	conn                   *websocket.Conn
	headers                http.Header
	bufferPool             *sync.Pool
	logger                 logger
	eventHandlers          []EventHandler
	mu                     sync.Mutex
	isLiveClosed           bool
	manualClose            bool
	lastUserAgentChange    time.Time
	consecutiveFailures    int
	lastReconnectReason    string
	lastReconnectErrorTime time.Time
	lastPageContent        string
	additionalCookies      map[string]string   // 新增：存储额外的 Cookie
	cookieManager          *sign.CookieManager // 新增：Cookie 管理器
	heartbeatStopCh        chan struct{}
	heartbeatDoneCh        chan struct{}
	writeMu                sync.Mutex
}

type logger interface {
	Print(v ...interface{})
	Printf(format string, v ...interface{})
	Println(v ...interface{})
}

type EventHandler struct {
	ID      string
	Handler func(*new_douyin.Webcast_Im_Message)
}

// NewDouyinLive 创建一个新的 DouyinLive 实例
// cookie 参数：可选的手动传入 Cookie，用于需要登录态的请求
func NewDouyinLive(liveID string, logger logger, cookie string) (*DouyinLive, error) {
	userAgent := utils.RandomUserAgent()
	dl := &DouyinLive{
		liveID:    liveID,
		userAgent: userAgent,
		client:    req.C().SetUserAgent(userAgent),
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, gzipBufferSize))
			},
		},
		headers:                make(http.Header),
		additionalCookies:      make(map[string]string),
		logger:                 logger,
		lastUserAgentChange:    time.Now(),
		consecutiveFailures:    0,
		lastReconnectReason:    "",
		lastReconnectErrorTime: time.Time{},
		heartbeatStopCh:        make(chan struct{}),
		heartbeatDoneCh:        make(chan struct{}),
	}

	// 初始化 Cookie 管理器
	dl.cookieManager = sign.NewCookieManager()
	// 优先使用手动传入的 Cookie，其次尝试从配置文件读取
	if cookie != "" {
		dl.cookieManager.SetDouyinCookie(cookie)
	} else {
		_ = dl.cookieManager.LoadConfig("config.yaml")
	}

	if dl.IsLive() == false {
		return nil, fmt.Errorf("直播间 %s 未开播", liveID)
	}
	return dl, nil
}

// Close 关闭抖音直播连接，确保资源正确释放
func (dl *DouyinLive) Close() {
	// 原子性地设置直播状态为关闭
	dl.setLiveStatus(false)
	dl.setManualClose(true)
	dl.stopHeartbeatLoop()

	// 获取连接并在锁外关闭，避免长时间持锁
	dl.mu.Lock()
	conn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	// 检查连接是否已经关闭
	if conn == nil {
		dl.logger.Println("连接已关闭或未初始化")
		return
	}

	// 创建一个带超时的通道，用于等待关闭操作完成
	done := make(chan struct{})

	// 异步执行关闭操作
	go func() {
		defer close(done)

		// 发送关闭帧
		msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, "closing connection")

		// 先尝试正常关闭
		if err := conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(2*time.Second)); err != nil {
			dl.logger.Printf("发送关闭消息失败: %v\n", err)
		}

		// 等待一段时间，让对方有机会响应
		time.Sleep(500 * time.Millisecond)

		// 确保连接最终关闭
		if err := conn.Close(); err != nil {
			dl.logger.Printf("关闭连接失败: %v\n", err)
		}
	}()

	// 等待关闭操作完成或超时
	select {
	case <-done:
		dl.logger.Println("连接已成功关闭")
	case <-time.After(3 * time.Second):
		dl.logger.Println("关闭连接超时")
	}
}

func (dl *DouyinLive) rebuildHTTPClientAndHeaders() {
	dl.client = req.C().SetUserAgent(dl.userAgent)
	dl.headers = make(http.Header)
	dl.headers.Set("User-Agent", dl.userAgent)
}

func (dl *DouyinLive) resetReconnectTracking() {
	dl.mu.Lock()
	dl.consecutiveFailures = 0
	dl.lastReconnectReason = ""
	dl.lastReconnectErrorTime = time.Time{}
	dl.mu.Unlock()
}

func (dl *DouyinLive) recordReconnectFailure(reason string) int {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.consecutiveFailures++
	dl.lastReconnectReason = reason
	dl.lastReconnectErrorTime = time.Now()
	return dl.consecutiveFailures
}

func (dl *DouyinLive) getConsecutiveFailures() int {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.consecutiveFailures
}

func (dl *DouyinLive) isRiskControlContent(content string) bool {
	markers := []string{
		"验证后继续访问",
		"请输入验证码",
		"访问过于频繁",
		"网络环境存在风险",
		"风险提示",
	}
	lower := strings.ToLower(content)
	for _, marker := range markers {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return true
		}
	}
	return false
}

// initialize 初始化 DouyinLive 实例
func (dl *DouyinLive) initialize() error {
	if err := dl.fetchTTWID(); err != nil {
		return err
	}

	if err := dl.fetchRoomInfo(); err != nil {
		return err
	}

	if err := jsScript.LoadGoja(dl.userAgent); err != nil {
		return fmt.Errorf("加载JavaScript脚本失败: %w", err)
	}

	dl.headers.Set("User-Agent", dl.userAgent)

	// 设置 Cookie - 优先使用配置文件中的 Cookie，其次使用获取到的 Cookie
	dl.setupCookies()

	return nil
}

// refreshReconnectContext 重连前刷新上下文；可选是否切换 UA 和是否全量重建 HTTP 上下文
func (dl *DouyinLive) refreshReconnectContext(changeUA bool, rebuildHTTP bool) error {
	oldUserAgent := dl.userAgent
	if changeUA {
		now := time.Now()
		if now.Sub(dl.lastUserAgentChange) >= minUAChangeInterval {
			newUserAgent := utils.RandomUserAgent()
			dl.userAgent = newUserAgent
			dl.lastUserAgentChange = now
			dl.logger.Printf("重连前刷新 UA: %s -> %s", oldUserAgent, newUserAgent)
		} else {
			dl.logger.Printf("本次重连跳过 UA 刷新：距离上次切换仅 %s", now.Sub(dl.lastUserAgentChange).Round(time.Millisecond))
		}
	}

	if rebuildHTTP || dl.client == nil || dl.headers == nil {
		dl.rebuildHTTPClientAndHeaders()
	} else {
		dl.client.SetUserAgent(dl.userAgent)
		dl.headers.Set("User-Agent", dl.userAgent)
	}

	if err := dl.initialize(); err != nil {
		return fmt.Errorf("刷新重连上下文失败: %w", err)
	}

	return nil
}

// getCookieParts 获取当前有效的 Cookie 键值对
func (dl *DouyinLive) getCookieParts() []string {
	configCookie := dl.cookieManager.GetDouyinCookie()
	if configCookie != "" {
		return strings.Split(configCookie, "; ")
	}

	parts := []string{fmt.Sprintf("ttwid=%s", dl.ttwid)}
	for name, value := range dl.additionalCookies {
		parts = append(parts, fmt.Sprintf("%s=%s", name, value))
	}
	return parts
}

// getCookies 获取 Cookie 列表（统一方法）
func (dl *DouyinLive) getCookies() []*http.Cookie {
	parts := dl.getCookieParts()
	if len(parts) == 0 {
		return nil
	}
	return dl.cookieManager.ParseCookies(strings.Join(parts, "; "))
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
	resp, err := dl.client.R().Get("https://live.douyin.com/")
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

// fetchRoomInfo 获取房间信息
func (dl *DouyinLive) fetchRoomInfo() error {
	body, err := dl.getPageContent()
	if err != nil {
		return err
	}

	dl.roomID = extractString(roomIDRegex, body, 1)
	dl.pushID = extractString(pushIDRegex, body, 1)
	name := extractString(anchorInfoRegex, body, 1)
	cleanJSON := strings.ReplaceAll(name, `&quot;`, `"`)
	result := gjson.Get(cleanJSON, "nickname")
	dl.LiveName = result.String()
	if dl.roomID == "" || dl.pushID == "" {
		return errors.New("无法提取房间信息")
	}
	return nil
}

// getPageContent 获取直播间页面内容
func (dl *DouyinLive) getPageContent() (string, error) {
	resp, err := dl.client.R().
		SetCookies(dl.getCookies()...).
		Get(fmt.Sprintf("https://live.douyin.com/%s", dl.liveID))

	if err != nil {
		return "", fmt.Errorf("请求直播间页面失败: %w", err)
	}

	content := resp.String()
	dl.lastPageContent = content
	if dl.isRiskControlContent(content) {
		return content, ErrRiskControlPage
	}
	return content, nil
}

// IsLive 检查直播间是否开播
func (dl *DouyinLive) IsLive() bool {
	content, err := dl.getPageContent()
	if err != nil {
		if errors.Is(err, ErrRiskControlPage) {
			dl.logger.Println("检测到页面风控或验证页，暂不判定为正常开播")
		}
		dl.setLiveStatus(false)
		return false
	}

	matches := isLiveRegex.FindStringSubmatch(content)
	if len(matches) < 3 {
		dl.setLiveStatus(false)
		return false
	}

	status := matches[2]
	dl.setLiveStatus(status == "2")
	return dl.isLiveStatus()
}

// setLiveStatus 设置直播间状态（线程安全）
func (dl *DouyinLive) setLiveStatus(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.isLiveClosed = status
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

// Start 启动直播间连接
func (dl *DouyinLive) Start() {
	defer dl.cleanup()

	if !dl.IsLive() {
		dl.logger.Println("直播间未开播、连接失败或触发风控页")
		return
	}

	if err := dl.startWebSocket(); err != nil {
		dl.logger.Printf("WebSocket连接失败: %v\n", err)
		// 尝试重连
		if dl.reconnect(defaultMaxRetries, true, false) {
			dl.processMessages()
		}
		return
	}

	// 使用 context 来控制 goroutine 生命周期
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	dl.processMessages()
}

// connectWebSocket 连接 WebSocket
func (dl *DouyinLive) startWebSocket() error {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = websocketConnectTimeout
	url, err := dl.makeURL()
	if err != nil {
		return fmt.Errorf("构建WebSocket URL失败: %w", err)
	}
	conn, resp, err := dialer.Dial(url, dl.headers)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("连接失败 (状态码: %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("连接失败: %w", err)
	}
	dl.logger.Printf("直播间连接成功(状态码):[%d] 直播间名称:[%s]\n", resp.StatusCode, dl.LiveName)
	dl.mu.Lock()
	dl.conn = conn
	dl.mu.Unlock()
	dl.startHeartbeatLoop()
	return nil
}

// buildWebsocketURL 基于当前上下文构建 WebSocket URL
func (dl *DouyinLive) buildWebsocketURL() string {
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)
	browserInfo := dl.userAgent
	if parts := strings.SplitN(dl.userAgent, "Mozilla", 2); len(parts) == 2 {
		browserInfo = parts[1]
	}
	parsedBrowser := strings.ReplaceAll(browserInfo, " ", "%20")

	// 使用纯算 a_bogus 签名
	//params := fmt.Sprintf("aid=6383&app_name=douyin_web&live_id=1&device_platform=web&language=zh-CN&web_rid=%s", dl.roomID)
	//signature := sign.AbSign(params, dl.userAgent)
	signature := jsScript.ExecuteJS(utils.GetxMSStub(
		utils.NewOrderedMap(dl.roomID, dl.pushID),
	))
	return fmt.Sprintf(wssURLTemplate,
		parsedBrowser,
		dl.roomID,
		dl.pushID,
		fetchTime,
		fetchTime,
		fetchTime,
		dl.pushID,
		dl.roomID,
		signature,
	)
}

// makeURL 初始化上下文后构建 WebSocket URL
func (dl *DouyinLive) makeURL() (string, error) {
	if err := dl.initialize(); err != nil {
		return "", fmt.Errorf("初始化失败: %w", err)
	}
	return dl.buildWebsocketURL(), nil
}

// processMessages 处理消息
func (dl *DouyinLive) processMessages() {
	for dl.isLiveStatus() {
		messageType, data, err := dl.readMessage()
		pushFrame := &new_douyin.Webcast_Im_PushFrame{}
		response := &new_douyin.Webcast_Im_Response{}
		controlMsg := &douyin.ControlMessage{}
		//log.Printf("读取消息类型: %d, 数据长度: %d, err:%v\n", messageType, len(data), err)
		if err != nil {
			dl.logger.Printf("读取消息失败:%v\n", err)
			if !dl.handleReadError(err) {
				break
			}
			continue
		}

		if messageType != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		if err := proto.Unmarshal(data, pushFrame); err != nil {
			dl.logger.Printf("解析PushFrame失败: %v\n", err)
			continue
		}

		if pushFrame.PayloadType == "msg" && utils.HasGzipEncoding(pushFrame.Headers) {
			dl.handleGzipMessage(pushFrame, response, controlMsg)
		}
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
	dl.mu.Unlock()

	go func() {
		defer close(doneCh)

		ticker := time.NewTicker(heartbeatInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if dl.isManualClose() || !dl.isLiveStatus() {
					return
				}
				if err := dl.sendPing(); err != nil {
					dl.logger.Printf("发送保活 ping 失败: %v\n", err)
				}
			case <-stopCh:
				return
			}
		}
	}()
}

func (dl *DouyinLive) stopHeartbeatLoop() {
	dl.mu.Lock()
	stopCh := dl.heartbeatStopCh
	doneCh := dl.heartbeatDoneCh
	dl.heartbeatStopCh = nil
	dl.heartbeatDoneCh = nil
	dl.mu.Unlock()

	if stopCh != nil {
		close(stopCh)
	}
	if doneCh != nil {
		<-doneCh
	}
}

// handleGzipMessage 处理 GZIP 消息
func (dl *DouyinLive) handleGzipMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *douyin.ControlMessage) {
	uncompressed, err := dl.decompressGzip(pushFrame.Payload)
	if err != nil {
		dl.logger.Printf("GZIP解压失败: %v\n", err)
		return
	}
	if err := dl.decodeResponse(uncompressed, pushFrame, response, controlMsg); err != nil {
		dl.logger.Printf("解析Response失败: %v\n", err)
	}
}

func (dl *DouyinLive) handlePlainMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *douyin.ControlMessage) {
	if err := dl.decodeResponse(pushFrame.Payload, pushFrame, response, controlMsg); err != nil {
		dl.logger.Printf("解析Response失败: %v\n", err)
	}
}

func (dl *DouyinLive) decodeResponse(data []byte, pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *douyin.ControlMessage) error {
	*response = new_douyin.Webcast_Im_Response{}
	if err := proto.Unmarshal(data, response); err != nil {
		return err
	}

	if response.NeedAck {
		dl.sendAck(pushFrame.LogID, response.InternalExt)
	}

	for _, msg := range response.Messages {
		dl.handleSingleMessage(msg, controlMsg)
	}
	return nil
}

// decompressGzip 解压 GZIP 数据
func (dl *DouyinLive) decompressGzip(data []byte) ([]byte, error) {
	buf := dl.bufferPool.Get().(*bytes.Buffer)
	defer func() {
		buf.Reset()
		dl.bufferPool.Put(buf)
	}()

	buf.Write(data)
	gz, err := gzip.NewReader(buf)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	result := bytes.NewBuffer(make([]byte, 0, len(data)*2))
	if _, err = io.Copy(result, gz); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
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
		dl.logger.Printf("心跳包序列化失败: %v\n", err)
		return
	}

	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn != nil {
		if err := dl.writeBinaryMessage(data); err != nil {
			dl.logger.Printf("发送心跳包失败: %v\n", err)
		}
	}
}

// handleSingleMessage 处理单条消息
func (dl *DouyinLive) handleSingleMessage(msg *new_douyin.Webcast_Im_Message,
	controlMsg *douyin.ControlMessage) {
	dl.emitEvent(msg)

	if msg.Method == "WebcastControlMessage" {
		if err := proto.Unmarshal(msg.Payload, controlMsg); err != nil {
			dl.logger.Printf("解析控制消息失败: %v\n", err)
			return
		}
		if controlMsg.Status == 3 {
			dl.logger.Printf("[%s]直播间已关闭", dl.LiveName)
			dl.setLiveStatus(false)
		}
	}
}

func (dl *DouyinLive) reconnectPlan(reason string, failureCount int, baseDelay time.Duration, allowUARefresh bool) (delay time.Duration, changeUA bool, rebuildHTTP bool) {
	delay = baseDelay
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
		delay += 2 * time.Second
	}

	switch reason {
	case "try_again_later_1013":
		delay = 5 * time.Second
		changeUA = true
	case "service_restart_1012":
		delay = 3 * time.Second
	case "going_away_1001":
		delay = 2 * time.Second
	case "risk_control_page":
		delay = 6 * time.Second
		changeUA = true
		rebuildHTTP = true
	}

	return delay, changeUA, rebuildHTTP
}

// reconnectDecision 描述重连策略
func (dl *DouyinLive) reconnectDecision(err error) (reason string, shouldRetry bool, delay time.Duration, allowUARefresh bool) {
	if dl.isManualClose() {
		return "manual_close", false, 0, false
	}

	if errors.Is(err, websocket.ErrCloseSent) {
		return "close_sent", false, 0, false
	}

	if errors.Is(err, ErrRiskControlPage) {
		return "risk_control_page", true, 6 * time.Second, true
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
		dl.logger.Println("连接被手动关闭，不进行重连")
		return false
	}

	reason, shouldRetry, baseDelay, allowUARefresh := dl.reconnectDecision(err)
	if !shouldRetry {
		dl.logger.Printf("连接关闭且不重连 [%s]: %v\n", reason, err)
		return false
	}

	failureCount := dl.recordReconnectFailure(reason)
	delay, changeUA, rebuildHTTP := dl.reconnectPlan(reason, failureCount, baseDelay, allowUARefresh)
	jitter := time.Duration(rand.Int63n(int64(maxReconnectJitter)))
	sleepFor := delay + jitter
	dl.logger.Printf("检测到需重连 [%s]，连续失败=%d，将在 %s 后尝试重连（changeUA=%v rebuildHTTP=%v）: %v\n", reason, failureCount, sleepFor, changeUA, rebuildHTTP, err)
	time.Sleep(sleepFor)

	return dl.reconnect(defaultMaxRetries, changeUA, rebuildHTTP)
}

// 优化后的 reconnect 方法
func (dl *DouyinLive) reconnect(attempts int, changeUA bool, rebuildHTTP bool) bool {
	// 如果是手动关闭，不进行重连
	if dl.isManualClose() {
		dl.logger.Println("连接被手动关闭，不进行重连")
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

		if err := dl.refreshReconnectContext(attemptChangeUA, attemptRebuildHTTP); err != nil {
			attemptIndex++
			return err
		}

		url := dl.buildWebsocketURL()
		dialer := *websocket.DefaultDialer
		dialer.HandshakeTimeout = websocketConnectTimeout
		conn, _, err := dialer.Dial(url, dl.headers)
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
		dl.startHeartbeatLoop()
		dl.resetReconnectTracking()
		return nil
	}

	err := retry.Do(
		retryable,
		retry.Attempts(uint(attempts)),
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
			dl.logger.Printf("第%d次重试连接失败，下一次（第%d次）策略：changeUA=%v rebuildHTTP=%v err=%v\n", n+1, nextAttempt, changeUA && nextAttempt > 1, rebuildHTTP || nextAttempt >= 3, err)
		}),
	)
	if err != nil {
		dl.logger.Printf("连接最终失败: %v", err)
		return false
	}

	dl.logger.Println("重连成功")
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
}

// emitEvent 触发事件，遍历处理所有有效处理器
func (dl *DouyinLive) emitEvent(msg *new_douyin.Webcast_Im_Message) {
	dl.mu.Lock()
	handlers := append([]EventHandler(nil), dl.eventHandlers...)
	dl.mu.Unlock()

	for _, handler := range handlers {
		handler.Handler(msg)
	}
}

// Subscribe 订阅事件，生成唯一ID
func (dl *DouyinLive) Subscribe(handler func(*new_douyin.Webcast_Im_Message)) string {
	id := utils.GenerateUniqueID() // 假设这是一个生成唯一ID的函数
	dl.mu.Lock()
	dl.eventHandlers = append(dl.eventHandlers, EventHandler{
		ID:      id,
		Handler: handler,
	})
	dl.mu.Unlock()
	return id
}

// Unsubscribe 取消订阅事件，通过ID查找并移除
func (dl *DouyinLive) Unsubscribe(id string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	for i, h := range dl.eventHandlers {
		if h.ID == id {
			dl.eventHandlers = append(dl.eventHandlers[:i], dl.eventHandlers[i+1:]...)
			break
		}
	}
}

// extractString 辅助函数，从正则匹配中提取字符串
func extractString(re *regexp.Regexp, s string, index int) string {
	if matches := re.FindStringSubmatch(s); len(matches) > index {
		return matches[index]
	}
	return ""
}
