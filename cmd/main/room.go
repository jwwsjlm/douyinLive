package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jwwsjlm/douyinLive/v2"
	"github.com/lxzan/gws"
)

var (
	pongMessage              = []byte("pong")
	serviceClosingMessage    = []byte(`{"type":"system","event":"service_status","code":"SERVICE_SHUTTING_DOWN","message":"服务正在关闭，当前连接将断开","suggestion":"等待服务重新启动后再连接"}`)
	roomInvalidMessage       = []byte(`{"type":"system","event":"live_status","code":"ROOM_NOT_FOUND","valid":false,"live":false,"status":"not_found","status_text":"直播间不存在或房间号无效","message":"直播间不存在或房间号无效，已关闭连接","suggestion":"请检查直播间ID是否输入正确；如果是短号或主页号，请确认网页可以正常打开该账号或直播间"}`)
	liveStartFailedMessage   = []byte(`{"type":"system","event":"live_status","code":"ROOM_CHECK_FAILED","valid":false,"live":false,"status":"error","status_text":"直播间状态检查失败","message":"直播间状态检查失败，请稍后重试","suggestion":"请稍后重新连接；如果多次失败，请开启 debug 日志并检查 Cookie 是否过期"}`)
	slowClientClosingMessage = []byte(`{"type":"system","event":"client_status","code":"CLIENT_TOO_SLOW","message":"客户端接收消息太慢，服务端已关闭连接","suggestion":"请检查客户端消费逻辑，避免长时间阻塞消息读取"}`)

	errRoomInactive      = errors.New("房间已关闭或无客户端")
	errRoomManagerClosed = errors.New("RoomManager 已关闭")
)

// roomCloseTimeout bounds how long Close waits for room-owned background tasks.
// roomCloseTimeout 限制 Close 等待房间后台任务退出的最长时间。
const roomCloseTimeout = 5 * time.Second

const (
	defaultRoomPollInterval   = 15 * time.Second
	defaultRoomNotifyInterval = 30 * time.Second
)

// Room 表示一个直播间及其下游客户端、上游抖音监听和离线监控状态。
// Room represents one live room with downstream clients, upstream Douyin listener, and offline monitor state.
type Room struct {
	// Lock order: acquire mu before clientsMu when both are needed. Never
	// acquire mu while holding clientsMu in a new code path.
	// 锁顺序：同时需要两把锁时先获取 mu，再获取 clientsMu；新增代码禁止反向加锁。
	id                   string
	logger               *appLogger
	clients              map[string]*Client
	connIDs              map[*gws.Conn]string
	clientsMu            sync.RWMutex
	douyinLive           *douyinLive.DouyinLive
	probeLive            *douyinLive.DouyinLive
	probeFailures        int
	mu                   sync.Mutex
	onClose              func()
	unknown              bool
	cookie               string
	signProvider         string
	tikHubKey            string
	pollInterval         time.Duration
	notifyInterval       time.Duration
	liveName             string
	title                string
	avatarThumb          string
	accountOnly          bool
	knownValid           bool
	statusUnknown        bool
	pendingClients       int
	starting             bool
	closed               bool
	upstreamReady        bool
	monitorStopCh        chan struct{}
	monitorDoneCh        chan struct{}
	monitorStopRequested bool
	lifecycleCtx         context.Context
	lifecycleCancel      context.CancelFunc
	tasks                sync.WaitGroup
	activeTasks          int
	closeDone            chan struct{}
	closeDoneOnce        sync.Once
}

// roomSnapshot is a read-only view exposed by the HTTP API.
// roomSnapshot 是 HTTP API 使用的只读房间快照。
type roomSnapshot struct {
	LiveID        string
	RoomID        string
	Status        string
	IsLive        *bool
	HasRoom       *bool
	AccountOnly   *bool
	Title         string
	ClientCount   int
	UpstreamReady bool
	StatusUnknown bool
}

// snapshot returns a consistent room state without exposing internal locks.
// snapshot 返回一致的房间状态，不向 HTTP 层暴露内部锁。
func (r *Room) snapshot() roomSnapshot {
	r.mu.Lock()
	d := r.douyinLive
	probe := r.probeLive
	status := "unknown"
	var isLive *bool
	var hasRoom *bool
	var accountOnly *bool
	if r.upstreamReady {
		status = "online"
		value := true
		isLive = &value
		hasRoom = &value
		accountOnlyValue := false
		accountOnly = &accountOnlyValue
	} else if r.accountOnly {
		status = "account_no_room"
		live := false
		has := false
		accountOnlyValue := true
		isLive, hasRoom, accountOnly = &live, &has, &accountOnlyValue
	} else if r.statusUnknown {
		status = "unknown"
	} else if r.monitorStopCh != nil || r.knownValid {
		status = "offline"
		value := false
		isLive = &value
		has := true
		hasRoom = &has
		accountOnlyValue := false
		accountOnly = &accountOnlyValue
	}
	title := r.title
	knownValid := r.knownValid
	upstreamReady := r.upstreamReady
	statusUnknown := r.statusUnknown
	r.mu.Unlock()
	if d == nil {
		d = probe
	}
	roomID := ""
	if d != nil {
		roomID = d.GetRoomID()
		if title == "" {
			title = d.GetTitle()
		}
	}
	if !knownValid && !upstreamReady && !statusUnknown {
		status = "unknown"
		isLive = nil
	}
	return roomSnapshot{LiveID: r.id, RoomID: roomID, Status: status, IsLive: isLive, HasRoom: hasRoom, AccountOnly: accountOnly, Title: title, ClientCount: r.clientCount(), UpstreamReady: upstreamReady, StatusUnknown: statusUnknown}
}

// setStatusUnknown 记录监控阶段当前是否只能得到“状态未知”。
// setStatusUnknown records whether the monitor can currently report only an indeterminate status.
func (r *Room) setStatusUnknown(unknown bool) bool {
	r.mu.Lock()
	changed := r.statusUnknown != unknown
	r.statusUnknown = unknown
	r.mu.Unlock()
	return changed
}

// isStatusUnknown 返回监控阶段当前是否处于“状态未知”。
// isStatusUnknown reports whether the monitor is currently in an indeterminate state.
func (r *Room) isStatusUnknown() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusUnknown
}

// reserveClient 为已通过 HTTP 校验但尚未完成 WebSocket 升级的客户端预留房间。
// reserveClient reserves the room for a client whose WebSocket upgrade hasn't completed yet.
func (r *Room) reserveClient() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	r.pendingClients++
	return true
}

// releaseClientReservation 释放升级失败或取消的客户端预留。
// releaseClientReservation releases a reservation after a failed or canceled WebSocket upgrade.
func (r *Room) releaseClientReservation() {
	r.mu.Lock()
	if r.pendingClients > 0 {
		r.pendingClients--
	}
	r.mu.Unlock()
	r.removeIfIdle()
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
	if pollInterval <= 0 {
		pollInterval = defaultRoomPollInterval
	}
	if notifyInterval <= 0 {
		notifyInterval = defaultRoomNotifyInterval
	}
	lifecycleCtx, lifecycleCancel := context.WithCancel(context.Background())
	return &Room{
		id:              id,
		logger:          logger,
		clients:         make(map[string]*Client),
		connIDs:         make(map[*gws.Conn]string),
		onClose:         onClose,
		unknown:         unknown,
		cookie:          cookie,
		signProvider:    normalizedProvider,
		tikHubKey:       strings.TrimSpace(tikHubKey),
		pollInterval:    pollInterval,
		notifyInterval:  notifyInterval,
		lifecycleCtx:    lifecycleCtx,
		lifecycleCancel: lifecycleCancel,
		closeDone:       make(chan struct{}),
	}
}

// startTask starts a room-owned background task unless the room is already closed.
// startTask 在房间关闭前启动一个由房间管理的后台任务；关闭后拒绝新任务。
func (r *Room) startTask(task func()) bool {
	if r == nil || task == nil {
		return false
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return false
	}
	r.tasks.Add(1)
	r.activeTasks++
	r.mu.Unlock()
	go func() {
		defer func() {
			r.tasks.Done()
			r.mu.Lock()
			if r.activeTasks > 0 {
				r.activeTasks--
			}
			r.mu.Unlock()
			r.removeIfIdle()
		}()
		task()
	}()
	return true
}

func newClientID() string {
	return uuid.NewString()
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
	probe := r.probeLive
	r.douyinLive = nil
	r.probeLive = nil
	r.probeFailures = 0
	r.mu.Unlock()

	if d != nil {
		d.Close()
	}
	if probe != nil && probe != d {
		probe.Dispose()
	}
}

// removeIfIdle 在房间无客户端且无后台任务时从管理器移除房间。
// removeIfIdle removes the room from the manager when it has no clients or background work.
func (r *Room) removeIfIdle() {
	r.mu.Lock()
	r.clientsMu.RLock()
	clientCount := len(r.clients)
	r.clientsMu.RUnlock()
	idle := !r.closed && clientCount == 0 && r.pendingClients == 0 && r.douyinLive == nil && r.probeLive == nil && r.monitorStopCh == nil && !r.starting && r.activeTasks == 0
	if idle {
		r.closed = true
		if r.lifecycleCancel != nil {
			r.lifecycleCancel()
		}
	}
	r.mu.Unlock()

	if idle && r.onClose != nil {
		r.onClose()
	}
	if idle {
		r.finishClose()
	}
}

// finishClose signals that all synchronous room-close bookkeeping has completed.
// finishClose 标记房间关闭同步清理已经完成。
func (r *Room) finishClose() {
	if r == nil || r.closeDone == nil {
		return
	}
	r.closeDoneOnce.Do(func() { close(r.closeDone) })
}

// Close 关闭房间、停止后台任务并释放上游监听资源。
// Close closes the room, stops background work, and releases upstream listener resources.
func (r *Room) Close() {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		if r.closeDone != nil {
			select {
			case <-r.closeDone:
			case <-time.After(roomCloseTimeout):
				r.logger.Warn("等待房间关闭完成超时", "room_id", r.id, "active_tasks", r.activeTaskCount())
			}
		}
		return
	}
	r.closed = true
	if r.lifecycleCancel != nil {
		r.lifecycleCancel()
	}
	d := r.douyinLive
	probe := r.probeLive
	r.douyinLive = nil
	r.probeLive = nil
	r.probeFailures = 0
	onClose := r.onClose
	r.mu.Unlock()

	r.stopMonitorLoop()

	r.closeAllClients(serviceClientClose)
	r.logger.Info("房间所有客户端连接已关闭", "room_id", r.id)

	if d != nil {
		d.Close()
		r.logger.Info("抖音直播监听已关闭", "room_id", r.id)
	}
	if probe != nil && probe != d {
		probe.Dispose()
		r.logger.Debug("直播状态探测会话已释放", "room_id", r.id)
	}

	if onClose != nil {
		onClose()
	}
	waitDone := make(chan struct{})
	go func() {
		r.tasks.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(roomCloseTimeout):
		r.logger.Warn("等待房间后台任务退出超时", "room_id", r.id, "active_tasks", r.activeTaskCount(), "clients", r.clientCount())
	}
	r.finishClose()
}

func (r *Room) activeTaskCount() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.activeTasks
}
