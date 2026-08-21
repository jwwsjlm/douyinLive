package main

import (
	"context"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"crypto/sha256"
	"encoding/hex"
	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

// RoomManager 管理所有直播间实例及其复用键。
// RoomManager manages all room instances and their reuse keys.
type RoomManager struct {
	rooms           map[string]*Room
	roomsMu         sync.RWMutex
	logger          *appLogger
	unknown         bool
	useStoredCookie bool
	cookie          string            // 抖音默认 Cookie。 Default Douyin Cookie.
	roomCookies     map[string]string // 按直播间 ID 配置的 Cookie。 Per-room Cookie overrides keyed by room ID.
	signProvider    string
	tikHubKey       string
	pollInterval    time.Duration
	notifyInterval  time.Duration
	probeMu         sync.Mutex
	probeCalls      map[string]*roomProbeCall
	probeSem        chan struct{}
	probeWG         sync.WaitGroup
	probeCtx        context.Context
	probeCancel     context.CancelFunc
	closeOnce       sync.Once
	closed          atomic.Bool
	metrics         *apiMetrics
	probeFactory    func(liveID, cookie string) (statusProbe, error)
}

type roomProbeCall struct {
	done     chan struct{}
	result   roomProbeResult
	cancel   context.CancelFunc
	waiters  int
	finished bool
}

type roomProbeResult struct {
	status douyinLive.LiveStatus
	err    error
}

type statusProbe interface {
	CheckLiveStatus(context.Context) (douyinLive.LiveStatus, error)
	Dispose()
}

// RoomManagerOptions groups the dependencies and runtime settings used by a RoomManager.
// RoomManagerOptions 将 RoomManager 所需的依赖和运行配置集中到一个参数结构中。
type RoomManagerOptions struct {
	Logger          *appLogger
	Unknown         bool
	Cookie          string
	RoomCookies     map[string]string
	SignProvider    string
	TikHubKey       string
	PollInterval    time.Duration
	NotifyInterval  time.Duration
	UseStoredCookie *bool
}

// NewRoomManager 创建直播间管理器。
// NewRoomManager creates a room manager.
// 参数/Parameters:
//   - logger: 应用日志器。 Application logger.
//   - unknown: 是否保留未知消息类型。 Whether to keep unknown message types.
//   - cookie: 可选抖音默认 Cookie。 Optional default Douyin Cookie.
//   - roomCookies: 按直播间 ID 配置的 Cookie。 Per-room Cookie overrides keyed by room ID.
//   - signProvider: WebSocket 签名来源。 WebSocket signature provider.
//   - tikHubKey: TikHub API Key。 TikHub API key.
//   - pollInterval: 未开播轮询间隔。 Offline-room polling interval.
//   - notifyInterval: 未开播状态通知间隔。 Offline status notification interval.
//   - useStoredCookie: 是否使用配置文件中的预存 Cookie；省略时默认为 true。 Whether to use configured cookies; defaults to true when omitted.
//
// Deprecated: use NewRoomManagerWithOptions for new code.
func NewRoomManager(logger *appLogger, unknown bool, cookie string, roomCookies map[string]string, signProvider string, tikHubKey string, pollInterval time.Duration, notifyInterval time.Duration, useStoredCookie ...bool) *RoomManager {
	useStored := true
	if len(useStoredCookie) > 0 {
		useStored = useStoredCookie[0]
	}
	return NewRoomManagerWithOptions(RoomManagerOptions{
		Logger: logger, Unknown: unknown, Cookie: cookie, RoomCookies: roomCookies,
		SignProvider: signProvider, TikHubKey: tikHubKey, PollInterval: pollInterval,
		NotifyInterval: notifyInterval, UseStoredCookie: boolPtr(useStored),
	})
}

// NewRoomManagerWithOptions creates a room manager from grouped options.
// NewRoomManagerWithOptions 使用结构化配置创建直播间管理器，避免长参数顺序错误。
func NewRoomManagerWithOptions(options RoomManagerOptions) *RoomManager {
	logger := options.Logger
	if logger == nil {
		logger = newAppLogger(nil)
	}
	normalizedProvider, err := normalizeSignProvider(options.SignProvider)
	if err != nil {
		logger.Warn("无效的签名提供商，回退到 local", "configured_provider", strings.TrimSpace(options.SignProvider), "err", err)
		normalizedProvider = signProviderLocal
	}
	roomCookiesCopy := make(map[string]string, len(options.RoomCookies))
	for roomID, value := range options.RoomCookies {
		roomCookiesCopy[roomID] = value
	}
	useStored := true
	if options.UseStoredCookie != nil {
		useStored = *options.UseStoredCookie
	}
	probeCtx, probeCancel := context.WithCancel(context.Background())
	rm := &RoomManager{
		rooms:           make(map[string]*Room),
		logger:          logger,
		unknown:         options.Unknown,
		useStoredCookie: useStored,
		cookie:          options.Cookie,
		roomCookies:     roomCookiesCopy,
		signProvider:    normalizedProvider,
		tikHubKey:       strings.TrimSpace(options.TikHubKey),
		pollInterval:    options.PollInterval,
		notifyInterval:  options.NotifyInterval,
		probeCalls:      make(map[string]*roomProbeCall),
		probeSem:        make(chan struct{}, 8),
		probeCtx:        probeCtx,
		probeCancel:     probeCancel,
	}
	rm.probeFactory = func(liveID, cookie string) (statusProbe, error) {
		if rm.signProvider == signProviderTikHub {
			return douyinLive.NewDouyinLiveWithSlogAndTikHub(liveID, rm.logger.base, cookie, rm.tikHubKey)
		}
		return douyinLive.NewDouyinLiveWithSlog(liveID, rm.logger.base, cookie)
	}
	return rm
}

const (
	roomProbeTimeout         = 20 * time.Second
	probeShutdownWaitTimeout = 5 * time.Second
)

// SnapshotRooms returns active room snapshots without triggering upstream requests.
// SnapshotRooms 返回活动房间快照，不触发上游查询。
func (rm *RoomManager) SnapshotRooms() []roomSnapshot {
	rm.roomsMu.RLock()
	rooms := make([]*Room, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		rooms = append(rooms, room)
	}
	rm.roomsMu.RUnlock()
	byLiveID := make(map[string]roomSnapshot, len(rooms))
	for _, room := range rooms {
		if room != nil && !room.isClosed() {
			snapshot := room.snapshot()
			if current, ok := byLiveID[snapshot.LiveID]; ok {
				byLiveID[snapshot.LiveID] = mergeRoomSnapshots(current, snapshot)
			} else {
				byLiveID[snapshot.LiveID] = snapshot
			}
		}
	}
	result := make([]roomSnapshot, 0, len(byLiveID))
	for _, snapshot := range byLiveID {
		result = append(result, snapshot)
	}
	return result
}

func mergeRoomSnapshots(current, next roomSnapshot) roomSnapshot {
	clients := current.ClientCount + next.ClientCount
	upstreamReady := current.UpstreamReady || next.UpstreamReady
	selected, fallback := current, next
	if roomSnapshotStatusPriority(next.Status) > roomSnapshotStatusPriority(current.Status) {
		selected, fallback = next, current
	}
	selected.ClientCount = clients
	selected.UpstreamReady = upstreamReady
	selected.StatusUnknown = selected.Status == "unknown"
	if selected.RoomID == "" {
		selected.RoomID = fallback.RoomID
	}
	if selected.Title == "" {
		selected.Title = fallback.Title
	}
	return selected
}

func roomSnapshotStatusPriority(status string) int {
	switch status {
	case "online":
		return 4
	case "offline":
		return 3
	case "account_no_room":
		return 2
	default:
		return 1
	}
}

// LookupRoom performs a one-shot HTTP status probe without creating a managed Room.
// LookupRoom 执行一次性 HTTP 查询，不创建长期 Room 或 WebSocket。
func (rm *RoomManager) LookupRoom(ctx context.Context, liveID string) (douyinLive.LiveStatus, error) {
	if rm == nil {
		return douyinLive.LiveStatus{}, errors.New("RoomManager 不能为空")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	var err error
	liveID, err = douyinLive.ValidateLiveID(liveID)
	if err != nil {
		return douyinLive.LiveStatus{}, err
	}
	if rm.closed.Load() {
		return douyinLive.LiveStatus{}, errRoomManagerClosed
	}
	if err := ctx.Err(); err != nil {
		return douyinLive.LiveStatus{}, err
	}
	rm.probeMu.Lock()
	if rm.closed.Load() {
		rm.probeMu.Unlock()
		return douyinLive.LiveStatus{}, errRoomManagerClosed
	}
	if existing := rm.probeCalls[liveID]; existing != nil && !existing.finished && existing.waiters > 0 {
		existing.waiters++
		if rm.metrics != nil {
			rm.metrics.probeMergedWaiters.Add(1)
		}
		rm.probeMu.Unlock()
		return rm.waitForRoomProbe(ctx, existing)
	}
	queryCtx, queryCancel := rm.probeContext(context.Background())
	call := &roomProbeCall{done: make(chan struct{}), cancel: queryCancel, waiters: 1}
	rm.probeCalls[liveID] = call
	rm.probeWG.Add(1)
	rm.probeMu.Unlock()

	// The actual upstream probe must not inherit the first caller's context.
	// Otherwise one disconnected client would cancel the shared probe for every
	// other waiter. Each caller below only cancels its own wait.
	go func() {
		defer rm.probeWG.Done()
		rm.executeRoomProbe(queryCtx, liveID, call)
	}()
	return rm.waitForRoomProbe(ctx, call)
}

func (rm *RoomManager) waitForRoomProbe(ctx context.Context, call *roomProbeCall) (douyinLive.LiveStatus, error) {
	select {
	case <-call.done:
		return call.result.status, call.result.err
	default:
	}
	select {
	case <-call.done:
		return call.result.status, call.result.err
	case <-ctx.Done():
		select {
		case <-call.done:
			return call.result.status, call.result.err
		default:
		}
		rm.probeMu.Lock()
		if !call.finished && call.waiters > 0 {
			call.waiters--
			if call.waiters == 0 && call.cancel != nil {
				call.cancel()
			}
		}
		rm.probeMu.Unlock()
		return douyinLive.LiveStatus{}, ctx.Err()
	}
}

func (rm *RoomManager) executeRoomProbe(queryCtx context.Context, liveID string, call *roomProbeCall) {
	defer call.cancel()
	defer func() {
		rm.probeMu.Lock()
		if rm.probeCalls[liveID] == call {
			delete(rm.probeCalls, liveID)
		}
		call.finished = true
		close(call.done)
		rm.probeMu.Unlock()
	}()

	select {
	case rm.probeSem <- struct{}{}:
	case <-queryCtx.Done():
		call.result.err = queryCtx.Err()
		return
	}
	defer func() { <-rm.probeSem }()
	if rm.metrics != nil {
		rm.metrics.probeUpstreamCalls.Add(1)
	}
	cookie := rm.cookieForRoom(liveID, "")
	if rm.probeFactory == nil {
		call.result.err = errors.New("room probe factory 不能为空")
		return
	}
	d, err := rm.probeFactory(liveID, cookie)
	if err == nil {
		if d == nil {
			call.result.err = errors.New("room probe factory returned nil probe")
			return
		}
		defer d.Dispose()
		call.result.status, call.result.err = d.CheckLiveStatus(queryCtx)
	} else {
		call.result.err = err
	}
}

// probeContext combines the caller cancellation, manager shutdown, and probe timeout.
// probeContext 合并调用方取消、管理器关闭信号和探测超时。
func (rm *RoomManager) probeContext(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	base := rm.probeCtx
	if base == nil {
		base = context.Background()
	}
	merged, mergeCancel := context.WithCancel(parent)
	stopPropagation := context.AfterFunc(base, mergeCancel)
	timed, timeoutCancel := context.WithTimeout(merged, roomProbeTimeout)
	return timed, func() {
		stopPropagation()
		timeoutCancel()
		mergeCancel()
	}
}

// Close stops pending probes and closes all active rooms.
// Close 停止未完成的探测并关闭所有活动房间。
func (rm *RoomManager) Close() {
	if rm == nil {
		return
	}
	rm.closeOnce.Do(func() {
		rm.closed.Store(true)
		if rm.probeCancel != nil {
			rm.probeCancel()
		}
		// Synchronize with LookupRoom's probeWG.Add, which is protected by
		// probeMu, before beginning to wait for outstanding probes.
		// 在等待探测任务前，与 LookupRoom 在 probeMu 下执行的 Add 完成同步。
		rm.probeMu.Lock()
		pendingProbes := len(rm.probeCalls)
		rm.probeMu.Unlock()
		if pendingProbes > 0 {
			rm.logger.Debug("正在等待临时直播间探测退出", "pending_probes", pendingProbes)
		}
		rm.CloseAll()
		rm.waitForProbes()
	})
}

func (rm *RoomManager) waitForProbes() {
	done := make(chan struct{})
	go func() {
		rm.probeWG.Wait()
		close(done)
	}()
	select {
	case <-done:
		return
	case <-time.After(probeShutdownWaitTimeout):
		rm.probeMu.Lock()
		pending := len(rm.probeCalls)
		rm.probeMu.Unlock()
		rm.logger.Warn("等待临时直播间探测退出超时", "pending_probes", pending)
	}
}

// cookieForRoom 按连接覆盖、房间配置、默认配置的优先级选择 Cookie。
// cookieForRoom chooses a cookie by connection override, room config, then default config.
// 参数/Parameters:
//   - roomID: 当前直播间 ID。 Current live room ID.
//   - override: 本次连接传入的 Cookie 覆盖值。 Cookie override provided by the current connection.
func (rm *RoomManager) cookieForRoom(roomID string, override string) string {
	if cookie := strings.TrimSpace(override); cookie != "" {
		return cookie
	}
	if !rm.useStoredCookie {
		return ""
	}

	if rm.roomCookies != nil {
		if cookie := strings.TrimSpace(rm.roomCookies[roomID]); cookie != "" {
			return cookie
		}
	}

	return strings.TrimSpace(rm.cookie)
}

// roomManagerKey 生成房间复用键，避免不同 Cookie 的连接误共享会话。
// roomManagerKey builds a reuse key that prevents sessions with different cookies from mixing.
// 参数/Parameters:
//   - roomID: 当前直播间 ID。 Current live room ID.
//   - cookie: 当前连接实际使用的 Cookie。 Effective cookie used by the current connection.
func roomManagerKey(roomID string, cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return roomID
	}

	sum := sha256.Sum256([]byte(cookie))
	return roomID + "#" + hex.EncodeToString(sum[:8])
}

// GetOrCreateRoom 获取现有房间或按当前 Cookie 上下文创建新房间。
// GetOrCreateRoom returns an existing room or creates one for the current cookie context.
// 参数/Parameters:
//   - roomID: 用户请求的直播间标识。 Live room identifier requested by the user.
//   - cookieOverride: 本次连接传入的 Cookie 覆盖值。 Cookie override supplied by this connection.
func (rm *RoomManager) GetOrCreateRoom(roomID string, cookieOverride string) *Room {
	if rm == nil || rm.closed.Load() {
		return nil
	}
	cookie := rm.cookieForRoom(roomID, cookieOverride)
	key := roomManagerKey(roomID, cookie)

	rm.roomsMu.Lock()
	defer rm.roomsMu.Unlock()
	// Re-check under the map lock so Close cannot race with room creation.
	if rm.closed.Load() {
		return nil
	}
	room, ok := rm.rooms[key]
	if ok && !room.isClosed() {
		return room
	}

	room = NewRoom(roomID, rm.logger, rm.unknown, cookie, rm.signProvider, rm.tikHubKey, rm.pollInterval, rm.notifyInterval, func() {
		rm.roomsMu.Lock()
		if rm.rooms[key] == room {
			delete(rm.rooms, key)
		}
		rm.roomsMu.Unlock()
		rm.logger.Info("房间已从管理器中移除", "room_id", roomID)
	})
	rm.rooms[key] = room
	return room
}

// AcquireRoom 获取房间并为即将完成的 WebSocket 升级预留一个客户端位置。
// AcquireRoom obtains a room and reserves it for the pending WebSocket upgrade.
func (rm *RoomManager) AcquireRoom(roomID string, cookieOverride string) *Room {
	if rm == nil || rm.closed.Load() {
		return nil
	}
	for {
		if rm.closed.Load() {
			return nil
		}
		room := rm.GetOrCreateRoom(roomID, cookieOverride)
		if room == nil {
			return nil
		}
		if room.reserveClient() {
			return room
		}
	}
}

// CloseAll 关闭管理器中的所有房间，但不会把管理器标记为已关闭。
// CloseAll closes current rooms but does not mark the manager closed.
// Use Close when new rooms and probes must be rejected.
func (rm *RoomManager) CloseAll() {
	if rm == nil {
		return
	}
	rm.roomsMu.RLock()
	rooms := make([]*Room, 0, len(rm.rooms))
	for _, room := range rm.rooms {
		rooms = append(rooms, room)
	}
	rm.roomsMu.RUnlock()

	for _, room := range rooms {
		room.Close()
	}
}
