package main

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"sync"
	"time"

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
	id                string
	logger            *appLogger
	clients           map[string]*Client
	connIDs           map[*gws.Conn]string
	clientsMu         sync.RWMutex
	douyinLive        *douyinLive.DouyinLive
	probeLive         *douyinLive.DouyinLive
	probeFailures     int
	mu                sync.Mutex
	onClose           func()
	unknown           bool
	cookie            string
	proxyURL          string
	signProvider      string
	protocolMode      string
	tikHubKey         string
	pollInterval      time.Duration
	notifyInterval    time.Duration
	userUniqueID      string
	liveName          string
	title             string
	avatarThumb       string
	accountOnly       bool
	knownValid        bool
	statusUnknown     bool
	pendingClients    int
	starting          bool
	sessionGeneration uint64
	closed            bool
	upstreamReady     bool
	monitor           *roomMonitorLoop
	lifecycleCtx      context.Context
	lifecycleCancel   context.CancelFunc
	tasks             sync.WaitGroup
	activeTasks       int
	closeDone         chan struct{}
	closeDoneOnce     sync.Once
}

// detachedRoomWorkers contains one generation of room-owned upstream resources.
// detachedRoomWorkers 保存一次房间会话代次中已经原子摘除的上游资源。
type detachedRoomWorkers struct {
	douyinLive  *douyinLive.DouyinLive
	probeLive   *douyinLive.DouyinLive
	monitorDone <-chan struct{}
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
	UserUniqueID  string
	LiveName      string
	AvatarThumb   string
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
	} else if r.monitor != nil || r.knownValid {
		status = "offline"
		value := false
		isLive = &value
		has := true
		hasRoom = &has
		accountOnlyValue := false
		accountOnly = &accountOnlyValue
	}
	title := r.title
	userUniqueID := r.userUniqueID
	liveName := r.liveName
	avatarThumb := r.avatarThumb
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
		if userUniqueID == "" {
			userUniqueID = d.GetUserUniqueID()
		}
		if liveName == "" {
			liveName = d.GetName()
		}
		if title == "" {
			title = d.GetTitle()
		}
		if avatarThumb == "" {
			avatarThumb = d.GetAvatarThumb()
		}
	}
	if !knownValid && !upstreamReady && !statusUnknown {
		status = "unknown"
		isLive = nil
	}
	return roomSnapshot{LiveID: r.id, RoomID: roomID, Status: status, IsLive: isLive, HasRoom: hasRoom, AccountOnly: accountOnly, Title: title, UserUniqueID: userUniqueID, LiveName: liveName, AvatarThumb: avatarThumb, ClientCount: r.clientCount(), UpstreamReady: upstreamReady, StatusUnknown: statusUnknown}
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
//   - protocolMode: 可选上游协议画像；省略时使用 defaultProtocolMode。 Optional upstream protocol profile.
func NewRoom(id string, logger *appLogger, unknown bool, cookie string, signProvider string, tikHubKey string, pollInterval time.Duration, notifyInterval time.Duration, onClose func(), protocolMode ...string) *Room {
	if logger == nil {
		logger = newAppLogger(nil)
	}
	normalizedProvider, err := normalizeSignProvider(signProvider)
	if err != nil {
		normalizedProvider = signProviderLocal
	}
	resolvedProtocolMode := defaultProtocolMode
	if len(protocolMode) > 0 {
		if trimmed := strings.TrimSpace(protocolMode[0]); trimmed != "" {
			resolvedProtocolMode = trimmed
		}
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
		protocolMode:    resolvedProtocolMode,
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
	return r.startTaskInternal(task, nil)
}

// startTaskForGeneration starts a task only while the specified room session
// generation is still current.
// startTaskForGeneration 仅在指定房间会话代次仍有效时启动任务。
func (r *Room) startTaskForGeneration(sessionGeneration uint64, task func()) bool {
	return r.startTaskInternal(task, &sessionGeneration)
}

func (r *Room) startTaskInternal(task func(), sessionGeneration *uint64) bool {
	if r == nil || task == nil {
		return false
	}
	r.mu.Lock()
	if r.closed || (sessionGeneration != nil && r.sessionGeneration != *sessionGeneration) {
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
	return rand.Text()
}

// isClosed 判断房间是否已关闭。
// isClosed reports whether the room has been closed.
func (r *Room) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// detachBackgroundWorkersIfIdle atomically retires the current session generation
// only when the room is still clientless. A client that arrives before this
// decision keeps the current generation; a client that arrives afterwards starts
// against the next generation and cannot be affected by the stale cleanup.
// detachBackgroundWorkersIfIdle 仅在房间仍无客户端时原子摘除当前会话代次。
func (r *Room) detachBackgroundWorkersIfIdle() (detachedRoomWorkers, bool) {
	r.mu.Lock()
	r.clientsMu.RLock()
	idle := len(r.clients) == 0 && r.pendingClients == 0
	r.clientsMu.RUnlock()
	if r.closed || !idle {
		r.mu.Unlock()
		return detachedRoomWorkers{}, false
	}

	workers := detachedRoomWorkers{
		douyinLive: r.douyinLive,
		probeLive:  r.probeLive,
	}
	if r.monitor != nil {
		workers.monitorDone = r.monitor.doneCh
		r.monitor.stop()
		r.monitor = nil
	}
	r.douyinLive = nil
	r.probeLive = nil
	r.probeFailures = 0
	r.upstreamReady = false
	r.starting = false
	r.sessionGeneration++
	r.mu.Unlock()
	return workers, true
}

// close releases resources that were already detached from the room state.
// close 释放已经从房间状态中摘除的资源，不会触碰后续代次的新会话。
func (workers detachedRoomWorkers) close(r *Room) {
	if workers.monitorDone != nil {
		select {
		case <-workers.monitorDone:
		case <-time.After(1500 * time.Millisecond):
			if r != nil {
				r.logger.Warn("等待监控循环退出超时，跳过阻塞等待", "room_id", r.id)
			}
		}
	}
	if workers.douyinLive != nil {
		workers.douyinLive.Close()
	}
	if workers.probeLive != nil && workers.probeLive != workers.douyinLive {
		workers.probeLive.Dispose()
	}
}

// closeBackgroundWorkersIfIdle stops the retired generation without touching a
// client or upstream session that may have arrived in the meantime.
// closeBackgroundWorkersIfIdle 仅清理确认无客户端时摘除的旧代次资源。
func (r *Room) closeBackgroundWorkersIfIdle() {
	workers, ok := r.detachBackgroundWorkersIfIdle()
	if !ok {
		return
	}
	workers.close(r)
	r.removeIfIdle()
}

// removeIfIdle 在房间无客户端且无后台任务时从管理器移除房间。
// removeIfIdle removes the room from the manager when it has no clients or background work.
func (r *Room) removeIfIdle() {
	r.mu.Lock()
	r.clientsMu.RLock()
	clientCount := len(r.clients)
	r.clientsMu.RUnlock()
	idle := !r.closed && clientCount == 0 && r.pendingClients == 0 && r.douyinLive == nil && r.probeLive == nil && r.monitor == nil && !r.starting && r.activeTasks == 0
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
