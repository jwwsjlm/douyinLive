package douyinLive

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/avast/retry-go"
	"github.com/gorilla/websocket"
	"github.com/imroc/req/v3"
	"google.golang.org/protobuf/proto"

	"github.com/jwwsjlm/douyinLive/generated/douyin"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/jwwsjlm/douyinLive/jsScript"
	"github.com/jwwsjlm/douyinLive/utils"
)

const (
	defaultMaxRetries       = 5
	websocketConnectTimeout = 10 * time.Second
	gzipBufferSize          = 1024 * 4
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
	roomIDRegex  = regexp.MustCompile(`roomId\\":\\"(\d+)\\"`)
	pushIDRegex  = regexp.MustCompile(`user_unique_id\\":\\"(\d+)\\"`)
	isLiveRegex  = regexp.MustCompile(`id_str\\":\\"(\d+)\\",\\"status\\":(\d+),\\"status_str\\":\\"(\d+)\\",\\"title\\":\\"(.*?)\\",\\"user_count_str\\":\\"(.*?)\\"`)
	emptyStrings = [][]string{{"", "", "", "", ""}}
)

// NewDouyinLive 创建一个新的 DouyinLive 实例
func NewDouyinLive(liveID string) (*DouyinLive, error) {
	log.SetOutput(os.Stdout)
	dl := &DouyinLive{
		liveID:     liveID,
		userAgent:  utils.RandomUserAgent(),
		client:     req.C().SetUserAgent(utils.RandomUserAgent()),
		bufferPool: &sync.Pool{New: func() interface{} { return bytes.NewBuffer(make([]byte, 0, gzipBufferSize)) }},
		headers:    make(http.Header),
	}

	if err := dl.initialize(); err != nil {
		return nil, fmt.Errorf("初始化失败: %w", err)
	}
	return dl, nil
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
	dl.headers.Set("Cookie", fmt.Sprintf("ttwid=%s", dl.ttwid))
	return nil
}

// fetchTTWID 获取 TTWID
func (dl *DouyinLive) fetchTTWID() error {
	resp, err := dl.client.R().Get("https://live.douyin.com/")
	if err != nil {
		return fmt.Errorf("请求TTWID失败: %w", err)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "ttwid" {
			dl.ttwid = c.Value
			return nil
		}
	}
	return errors.New("未找到TTWID cookie")
}

// fetchRoomInfo 获取房间信息
func (dl *DouyinLive) fetchRoomInfo() error {
	body, err := dl.getPageContent()
	if err != nil {
		return err
	}

	dl.roomID = extractString(roomIDRegex, body, 1)
	dl.pushID = extractString(pushIDRegex, body, 1)

	if dl.roomID == "" || dl.pushID == "" {
		return errors.New("无法提取房间信息")
	}
	return nil
}

// getPageContent 获取直播间页面内容
func (dl *DouyinLive) getPageContent() (string, error) {
	cookies := []*http.Cookie{
		{Name: "ttwid", Value: "ttwid=" + dl.ttwid},
		{Name: "__ac_nonce", Value: "0123407cc00a9e438deb4"},
	}

	resp, err := dl.client.R().
		SetCookies(cookies...).
		Get(fmt.Sprintf("https://live.douyin.com/%s", dl.liveID))

	if err != nil {
		return "", fmt.Errorf("请求直播间页面失败: %w", err)
	}
	return resp.String(), nil
}

// IsLive 检查直播间是否开播
func (dl *DouyinLive) IsLive() bool {
	content, err := dl.getPageContent()
	if err != nil {
		dl.setLiveStatus(false)
		return false
	}

	matches := isLiveRegex.FindStringSubmatch(content)
	if len(matches) < 3 {
		return false
	}

	status := matches[2]
	dl.setLiveStatus(status == "2")
	return dl.isLiveClosed
}

// setLiveStatus 设置直播间状态
func (dl *DouyinLive) setLiveStatus(status bool) {
	dl.isLiveClosed = status
}

// Start 启动直播间连接
func (dl *DouyinLive) Start() {
	defer dl.cleanup()

	if !dl.IsLive() {
		log.Println("直播间未开播或连接失败")
		return
	}

	if err := dl.connectWebSocket(); err != nil {
		log.Printf("WebSocket连接失败: %v\n", err)
		return
	}

	dl.processMessages()
}

// connectWebSocket 连接 WebSocket
func (dl *DouyinLive) connectWebSocket() error {
	dialer := websocket.DefaultDialer
	dialer.HandshakeTimeout = websocketConnectTimeout

	conn, resp, err := dialer.Dial(dl.constructWSSURL(), dl.headers)
	if err != nil {
		return fmt.Errorf("连接失败 (状态码: %d): %w", resp.StatusCode, err)
	}
	log.Printf("直播间连接成功%d\n", resp.StatusCode)
	dl.conn = conn
	return nil
}

// constructWSSURL 构建 WebSocket URL
func (dl *DouyinLive) constructWSSURL() string {
	fetchTime := time.Now().UnixNano() / int64(time.Millisecond)
	browserInfo := strings.SplitN(dl.userAgent, "Mozilla", 2)[1]
	parsedBrowser := strings.ReplaceAll(browserInfo, " ", "%20")

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

// processMessages 处理消息
func (dl *DouyinLive) processMessages() {
	var (
		pushFrame = &new_douyin.Webcast_Im_PushFrame{}
		response  = &new_douyin.Webcast_Im_Response{}

		controlMsg = &douyin.ControlMessage{}
	)

	for dl.isLiveClosed {
		messageType, data, err := dl.conn.ReadMessage()
		//log.Printf("读取消息类型: %d, 数据长度: %d, err:%v\n", messageType, len(data), err)
		if err != nil {
			log.Printf("读取消息失败:%v\n", err)
			if !dl.handleReadError(err) {
				break
			}
			continue
		}

		if messageType != websocket.BinaryMessage || len(data) == 0 {
			continue
		}

		if err := proto.Unmarshal(data, pushFrame); err != nil {
			log.Printf("解析PushFrame失败: %v\n", err)
			continue
		}

		if pushFrame.PayloadType == "msg" && utils.HasGzipEncoding(pushFrame.Headers) {
			dl.handleGzipMessage(pushFrame, response, controlMsg)
		}
	}
}

// readMessage 读取消息
func (dl *DouyinLive) readMessage() (int, []byte, error) {
	if dl.conn == nil {
		return 0, nil, errors.New("连接已关闭")
	}
	return dl.conn.ReadMessage()
}

// handleGzipMessage 处理 GZIP 消息
func (dl *DouyinLive) handleGzipMessage(pushFrame *new_douyin.Webcast_Im_PushFrame, response *new_douyin.Webcast_Im_Response, controlMsg *douyin.ControlMessage) {

	uncompressed, err := dl.decompressGzip(pushFrame.Payload)
	if err != nil {
		log.Printf("GZIP解压失败: %v\n", err)
		return
	}

	if err := proto.Unmarshal(uncompressed, response); err != nil {
		log.Printf("解析Response失败: %v\n", err)
		return
	}

	if response.NeedAck {
		dl.sendAck(pushFrame.LogID, response.InternalExt)
	}

	for _, msg := range response.Messages {
		dl.handleSingleMessage(msg, controlMsg)
	}
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
		log.Printf("心跳包序列化失败: %v\n", err)
		return
	}

	if dl.conn != nil {
		err := dl.conn.WriteMessage(websocket.BinaryMessage, data)
		if err != nil {
			log.Printf("发送心跳包失败: %v\n", err)
		}
	}
}

// handleSingleMessage 处理单条消息
func (dl *DouyinLive) handleSingleMessage(msg *new_douyin.Webcast_Im_Message,
	controlMsg *douyin.ControlMessage) {
	dl.emitEvent(msg)

	if msg.Method == "WebcastControlMessage" {
		if err := proto.Unmarshal(msg.Payload, controlMsg); err != nil {
			log.Printf("解析控制消息失败: %v\n", err)
			return
		}
		if controlMsg.Status == 3 {
			log.Println("直播间已关闭")
			dl.setLiveStatus(false)
		}
	}
}

// 修改 handleReadError 方法，使用库自带方法判断错误
func (dl *DouyinLive) handleReadError(err error) bool {
	// 使用 websocket.IsUnexpectedCloseError 判断特定关闭码
	if !websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure) {
		log.Printf("正常关闭: %v\n", err)
		return false // 不需要重连
	}
	log.Printf("检测到非正常关闭，尝试重连...错误代码:%v\n", err)
	// 处理非正常关闭错误
	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		log.Printf("WebSocket关闭错误: code=%d, reason=%s\n", closeErr.Code, closeErr.Text)

		// 针对特定错误码处理
		switch closeErr.Code {
		case websocket.CloseAbnormalClosure: // 1006 异常关闭
			log.Println("检测到异常关闭，尝试重连...")
			return dl.reconnect(defaultMaxRetries)
		case websocket.CloseTryAgainLater: // 1013 临时不可用
			log.Println("服务端要求稍后重试...")
			time.Sleep(5 * time.Second)
			return dl.reconnect(defaultMaxRetries)
		}
	}

	// 处理其他网络错误
	log.Printf("网络错误: %v\n", err)
	return dl.reconnect(defaultMaxRetries)
}

// 优化后的 reconnect 方法
func (dl *DouyinLive) reconnect(attempts int) bool {
	if dl.conn != nil {
		// 使用标准方法发送关闭帧
		msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "reconnecting")
		_ = dl.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(3*time.Second))
		dl.conn.Close()
		dl.conn = nil
	}

	retryable := func() error {
		conn, _, err := websocket.DefaultDialer.Dial(dl.constructWSSURL(), dl.headers)
		if err != nil {
			// 处理不可恢复错误
			if websocket.IsCloseError(err,
				websocket.CloseAbnormalClosure,         // 1006 异常关闭
				websocket.CloseTryAgainLater,           // 1013 临时不可用
				websocket.CloseServiceRestart,          // 1012 服务重启
				websocket.CloseGoingAway,               // 1001 端点离开
				websocket.CloseNoStatusReceived,        // 1005 无状态码
				websocket.ClosePolicyViolation,         // 1008 策略违规
				websocket.CloseInvalidFramePayloadData, // 1007 无效数据
			) {
				return retry.Unrecoverable(err)
			}
			return err
		}
		dl.conn = conn
		return nil
	}

	err := retry.Do(
		retryable,
		retry.Attempts(uint(attempts)),
		retry.DelayType(retry.BackOffDelay),
		retry.RetryIf(func(err error) bool {
			// 过滤不可重试的错误
			return !websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseInvalidFramePayloadData,
			)
		}),
		retry.OnRetry(func(n uint, err error) {
			log.Printf("第%d次重试连接: %v\n", n+1, err)
		}),
	)

	return err == nil
}

// 使用库方法判断意外关闭
func isUnexpectedClose(err error) bool {
	return websocket.IsUnexpectedCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived,
	)
}

// cleanup 清理资源
func (dl *DouyinLive) cleanup() {
	if dl.conn != nil {
		dl.conn.Close()
	}
}

// emitEvent 触发事件，遍历处理所有有效处理器
func (dl *DouyinLive) emitEvent(msg *new_douyin.Webcast_Im_Message) {
	for _, handler := range dl.eventHandlers {
		handler.Handler(msg)
	}
}

// Subscribe 订阅事件，生成唯一ID
func (dl *DouyinLive) Subscribe(handler func(*new_douyin.Webcast_Im_Message)) string {
	id := utils.GenerateUniqueID() // 假设这是一个生成唯一ID的函数
	dl.eventHandlers = append(dl.eventHandlers, EventHandler{
		ID:      id,
		Handler: handler,
	})
	return id
}

// Unsubscribe 取消订阅事件，通过ID查找并移除
func (dl *DouyinLive) Unsubscribe(id string) {
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
