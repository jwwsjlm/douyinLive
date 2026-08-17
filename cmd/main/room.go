package main

import (
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
)

var (
	pongMessage              = []byte("pong")
	serviceClosingMessage    = []byte(`{"type":"system","event":"service_status","code":"SERVICE_SHUTTING_DOWN","message":"服务正在关闭，当前连接将断开","suggestion":"等待服务重新启动后再连接"}`)
	roomInvalidMessage       = []byte(`{"type":"system","event":"live_status","code":"ROOM_NOT_FOUND","valid":false,"live":false,"status":"not_found","status_text":"直播间不存在或房间号无效","message":"直播间不存在或房间号无效，已关闭连接","suggestion":"请检查直播间ID是否输入正确；如果是短号或主页号，请确认网页可以正常打开该账号或直播间"}`)
	liveStartFailedMessage   = []byte(`{"type":"system","event":"live_status","code":"ROOM_CHECK_FAILED","valid":false,"live":false,"status":"error","status_text":"直播间状态检查失败","message":"直播间状态检查失败，请稍后重试","suggestion":"请稍后重新连接；如果多次失败，请开启 debug 日志并检查 Cookie 是否过期"}`)
	slowClientClosingMessage = []byte(`{"type":"system","event":"client_status","code":"CLIENT_TOO_SLOW","message":"客户端接收消息太慢，服务端已关闭连接","suggestion":"请检查客户端消费逻辑，避免长时间阻塞消息读取"}`)

	errRoomInactive = errors.New("房间已关闭或无客户端")
)

// Room 表示一个直播间及其下游客户端、上游抖音监听和离线监控状态。
// Room represents one live room with downstream clients, upstream Douyin listener, and offline monitor state.
type Room struct {
	id             string
	logger         *appLogger
	clients        map[string]*Client
	clientsMu      sync.RWMutex
	douyinLive     *douyinLive.DouyinLive
	mu             sync.Mutex
	onClose        func()
	unknown        bool
	cookie         string
	signProvider   string
	tikHubKey      string
	pollInterval   time.Duration
	notifyInterval time.Duration
	liveName       string
	title          string
	avatarThumb    string
	accountOnly    bool
	knownValid     bool
	starting       bool
	closed         bool
	upstreamReady  bool
	monitorStopCh  chan struct{}
	monitorDoneCh  chan struct{}
}

// NewRoom 创建直播间实例。
// NewRoom creates a room instance.
// 参数/Parameters:
//   - id: 用户请求的直播间标识。 Live room identifier requested by the user.
//   - logger: 应用日志器。 Application logger.
//   - unknown: 是否保留未知消息类型。 Whether to keep unknown message types.
//   - cookie: 当前房间使用的抖音 Cookie。 Douyin Cookie used by this room.
//   - signProvider: WebSocket 签名来源。 WebSocket signature provider.
//   - tikHubKey: TikHub API Key。 TikHub API key.
//   - pollInterval: 未开播轮询间隔。 Offline-room polling interval.
//   - notifyInterval: 未开播状态通知间隔。 Offline status notification interval.
//   - onClose: 房间关闭后的回调。 Callback invoked after the room closes.
func NewRoom(id string, logger *appLogger, unknown bool, cookie string, signProvider string, tikHubKey string, pollInterval time.Duration, notifyInterval time.Duration, onClose func()) *Room {
	if logger == nil {
		logger = newAppLogger(nil)
	}
	normalizedProvider, err := normalizeSignProvider(signProvider)
	if err != nil {
		normalizedProvider = signProviderLocal
	}
	return &Room{
		id:             id,
		logger:         logger,
		clients:        make(map[string]*Client),
		onClose:        onClose,
		unknown:        unknown,
		cookie:         cookie,
		signProvider:   normalizedProvider,
		tikHubKey:      strings.TrimSpace(tikHubKey),
		pollInterval:   pollInterval,
		notifyInterval: notifyInterval,
	}
}

// isClosed 判断房间是否已关闭。
// isClosed reports whether the room has been closed.
func (r *Room) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// markKnownValid 记录该房间曾经被抖音页面或接口明确识别为有效房间。
// markKnownValid records that Douyin previously confirmed this room identity.
func (r *Room) markKnownValid() {
	r.mu.Lock()
	r.knownValid = true
	r.mu.Unlock()
}

// hasKnownValidRoom 返回该连接周期内是否曾确认过有效房间身份。
// hasKnownValidRoom reports whether this room identity was confirmed during the current lifecycle.
func (r *Room) hasKnownValidRoom() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.knownValid
}

// closeBackgroundWorkers 停止房间后台监控和上游直播监听。
// closeBackgroundWorkers stops room background monitoring and upstream live listening.
func (r *Room) closeBackgroundWorkers() {
	r.stopMonitorLoop()
	r.closeDouyinLive()
	r.removeIfIdle()
}

// closeDouyinLive 关闭当前上游抖音直播连接。
// closeDouyinLive closes the current upstream Douyin live connection.
func (r *Room) closeDouyinLive() {
	r.mu.Lock()
	d := r.douyinLive
	r.douyinLive = nil
	r.mu.Unlock()

	if d != nil {
		d.Close()
	}
}

// removeIfIdle 在房间无客户端且无后台任务时从管理器移除房间。
// removeIfIdle removes the room from the manager when it has no clients or background work.
func (r *Room) removeIfIdle() {
	if r.clientCount() != 0 {
		return
	}

	r.mu.Lock()
	idle := !r.closed && r.douyinLive == nil && r.monitorStopCh == nil && !r.starting
	if idle {
		r.closed = true
	}
	r.mu.Unlock()

	if idle && r.onClose != nil {
		r.onClose()
	}
}

// Close 关闭房间、停止后台任务并释放上游监听资源。
// Close closes the room, stops background work, and releases upstream listener resources.
func (r *Room) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	d := r.douyinLive
	r.douyinLive = nil
	onClose := r.onClose
	r.mu.Unlock()

	r.stopMonitorLoop()

	r.closeAllClients(serviceClosingMessage)
	r.logger.Info("房间所有客户端连接已关闭", "room_id", r.id)

	if d != nil {
		d.Close()
		r.logger.Info("抖音直播监听已关闭", "room_id", r.id)
	}

	if onClose != nil {
		onClose()
	}
}
