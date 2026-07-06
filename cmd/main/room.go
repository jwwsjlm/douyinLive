package main

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinlive-proto/generated"
	"github.com/lxzan/gws"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// clientSendQueueSize 限制单个客户端的待发送消息队列长度。
// clientSendQueueSize limits the pending outbound queue size for each client.
const clientSendQueueSize = 256

// clientWriteTimeout 限制向客户端写消息的最长时间。
// clientWriteTimeout limits how long a write to a client may take.
const clientWriteTimeout = 5 * time.Second

var (
	pongMessage              = []byte("pong")
	serviceClosingMessage    = []byte(`{"type":"system","message":"服务正在关闭"}`)
	liveInvalidMessage       = []byte(`{"type":"system","message":"直播间未开播或ID无效"}`)
	slowClientClosingMessage = []byte(`{"type":"system","message":"客户端消费过慢，连接已关闭"}`)

	errRoomInactive = errors.New("房间已关闭或无客户端")
)

// RoomManager 管理所有直播间实例
// RoomManager 管理所有直播间实例及其复用键。
// RoomManager manages all room instances and their reuse keys.
type RoomManager struct {
	rooms          map[string]*Room
	roomsMu        sync.RWMutex
	logger         *appLogger
	unknown        bool
	cookie         string            // 抖音默认 Cookie
	roomCookies    map[string]string // 按直播间 ID 配置的 Cookie
	signProvider   string
	tikHubKey      string
	pollInterval   time.Duration
	notifyInterval time.Duration
}

// NewRoomManager 创建一个新的 RoomManager
// cookie 参数：可选的抖音默认 Cookie
// NewRoomManager 创建直播间管理器。
// NewRoomManager creates a room manager.
func NewRoomManager(logger *appLogger, unknown bool, cookie string, roomCookies map[string]string, signProvider string, tikHubKey string, pollInterval time.Duration, notifyInterval time.Duration) *RoomManager {
	if logger == nil {
		logger = newAppLogger(nil)
	}
	normalizedProvider, err := normalizeSignProvider(signProvider)
	if err != nil {
		normalizedProvider = signProviderLocal
	}
	return &RoomManager{
		rooms:          make(map[string]*Room),
		logger:         logger,
		unknown:        unknown,
		cookie:         cookie,
		roomCookies:    roomCookies,
		signProvider:   normalizedProvider,
		tikHubKey:      strings.TrimSpace(tikHubKey),
		pollInterval:   pollInterval,
		notifyInterval: notifyInterval,
	}
}

// cookieForRoom 按连接覆盖、房间配置、默认配置的优先级选择 Cookie。
// cookieForRoom chooses a cookie by connection override, room config, then default config.
func (rm *RoomManager) cookieForRoom(roomID string, override string) string {
	if cookie := strings.TrimSpace(override); cookie != "" {
		return cookie
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
func roomManagerKey(roomID string, cookie string) string {
	cookie = strings.TrimSpace(cookie)
	if cookie == "" {
		return roomID
	}

	sum := sha256.Sum256([]byte(cookie))
	return roomID + "#" + hex.EncodeToString(sum[:8])
}

// GetOrCreateRoom 获取或创建一个新的房间实例
// GetOrCreateRoom 获取现有房间或按当前 Cookie 上下文创建新房间。
// GetOrCreateRoom returns an existing room or creates one for the current cookie context.
func (rm *RoomManager) GetOrCreateRoom(roomID string, cookieOverride string) *Room {
	cookie := rm.cookieForRoom(roomID, cookieOverride)
	key := roomManagerKey(roomID, cookie)

	rm.roomsMu.RLock()
	room, ok := rm.rooms[key]
	rm.roomsMu.RUnlock()
	if ok && !room.isClosed() {
		return room
	}

	rm.roomsMu.Lock()
	defer rm.roomsMu.Unlock()
	if room, ok = rm.rooms[key]; ok && !room.isClosed() {
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

// CloseAll 关闭所有房间
// CloseAll 关闭管理器中的所有房间。
// CloseAll closes every room managed by this manager.
func (rm *RoomManager) CloseAll() {
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

// outboundMessage 表示写入客户端队列的一条待发送消息。
// outboundMessage represents one pending message in a client's outbound queue.
type outboundMessage struct {
	opcode  gws.Opcode
	payload []byte
}

// Client 表示一个下游 WebSocket 客户端连接。
// Client represents one downstream WebSocket client connection.
type Client struct {
	id        string
	conn      *gws.Conn
	sendQueue chan outboundMessage
	stopCh    chan struct{}
	closeOnce sync.Once
}

// NewClient 创建客户端连接包装器。
// NewClient creates a client connection wrapper.
func NewClient(id string, conn *gws.Conn) *Client {
	return &Client{
		id:        id,
		conn:      conn,
		sendQueue: make(chan outboundMessage, clientSendQueueSize),
		stopCh:    make(chan struct{}),
	}
}

// enqueue 将消息放入客户端发送队列，队列满时返回 false。
// enqueue queues a message for the client and returns false when the queue is full.
func (c *Client) enqueue(opcode gws.Opcode, payload []byte) bool {
	select {
	case <-c.stopCh:
		return false
	default:
	}

	select {
	case c.sendQueue <- outboundMessage{opcode: opcode, payload: payload}:
		return true
	default:
		return false
	}
}

// writeLoop 串行消费发送队列并写入客户端连接。
// writeLoop serially drains the outbound queue and writes to the client connection.
func (c *Client) writeLoop(onWriteError func()) {
	for {
		select {
		case <-c.stopCh:
			return
		case msg, ok := <-c.sendQueue:
			if !ok {
				return
			}
			nc := c.conn.NetConn()
			if nc != nil {
				_ = nc.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
			}
			if err := c.conn.WriteMessage(msg.opcode, msg.payload); err != nil {
				c.close(nil)
				if onWriteError != nil {
					onWriteError()
				}
				return
			}
			if nc != nil {
				_ = nc.SetWriteDeadline(time.Time{})
			}
		}
	}
}

// close 幂等关闭客户端连接和发送循环。
// close idempotently closes the client connection and send loop.
func (c *Client) close(closePayload []byte) {
	c.closeOnce.Do(func() {
		close(c.stopCh)
		if c.conn == nil {
			return
		}
		if closePayload != nil {
			_ = c.conn.WriteClose(1000, closePayload)
		}
		if nc := c.conn.NetConn(); nc != nil {
			_ = nc.Close()
		}
	})
}

// closeClient 从房间移除并关闭指定客户端。
// closeClient removes and closes a client from the room.
func (r *Room) closeClient(clientID string, closePayload []byte) {
	client, _, removed := r.removeClient(clientID)
	if !removed {
		return
	}

	if client != nil {
		client.close(closePayload)
	}

	remaining := r.clientCount()
	r.logger.Info("客户端断开连接", "client_id", clientID, "room_id", r.id, "remaining_clients", remaining)

	if remaining == 0 {
		r.logger.Info("最后一个客户端已断开，正在关闭后台监听", "room_id", r.id)
		go r.closeBackgroundWorkers()
	}
}

// Room 代表一个直播间
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
	starting       bool
	closed         bool
	monitorStopCh  chan struct{}
	monitorDoneCh  chan struct{}
}

// NewRoom 创建一个新的房间实例
// cookie 参数：可选的抖音 Cookie
// NewRoom 创建直播间实例。
// NewRoom creates a room instance.
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

// addClient 将客户端加入房间并返回当前客户端数量。
// addClient adds a client to the room and returns the current client count.
func (r *Room) addClient(client *Client) int {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	r.clients[client.id] = client
	return len(r.clients)
}

// getClient 按 ID 获取客户端。
// getClient returns a client by ID.
func (r *Room) getClient(clientID string) (*Client, bool) {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	client, ok := r.clients[clientID]
	return client, ok
}

// removeClient 从房间移除客户端并返回剩余数量。
// removeClient removes a client from the room and returns the remaining count.
func (r *Room) removeClient(clientID string) (*Client, int, bool) {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	client, ok := r.clients[clientID]
	if !ok {
		return nil, len(r.clients), false
	}
	delete(r.clients, clientID)
	return client, len(r.clients), true
}

// clientCount 返回当前客户端数量。
// clientCount returns the current number of clients.
func (r *Room) clientCount() int {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	return len(r.clients)
}

// isClosed 判断房间是否已关闭。
// isClosed reports whether the room has been closed.
func (r *Room) isClosed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closed
}

// snapshotClients 获取客户端快照，避免广播时长时间持锁。
// snapshotClients takes a client snapshot to avoid holding locks while broadcasting.
func (r *Room) snapshotClients() []*Client {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

// clearClients 清空客户端表并返回被移除的客户端。
// clearClients clears the client map and returns the removed clients.
func (r *Room) clearClients() []*Client {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clients = make(map[string]*Client)
	return clients
}

// appendJSONStringField 向已有 JSON 对象字节流追加一个字符串字段。
// appendJSONStringField appends one string field to an existing JSON object buffer.
func appendJSONStringField(dst []byte, key, value string) []byte {
	dst = append(dst, ',')
	dst = append(dst, '"')
	dst = append(dst, key...)
	dst = append(dst, '"', ':')
	dst = strconv.AppendQuote(dst, value)
	return dst
}

// buildEventJSON 将解析后的 protobuf JSON 补充直播间元数据。
// buildEventJSON enriches parsed protobuf JSON with live room metadata.
func (r *Room) buildEventJSON(jsonBytes []byte, method, liveName, title, avatarThumb string) ([]byte, error) {
	if len(jsonBytes) == 0 || jsonBytes[len(jsonBytes)-1] != '}' {
		return nil, fmt.Errorf("无效的事件 JSON")
	}

	extra := 64 + len(method) + len(liveName) + len(title) + len(avatarThumb)
	result := make([]byte, 0, len(jsonBytes)+extra)
	result = append(result, jsonBytes[:len(jsonBytes)-1]...)
	result = appendJSONStringField(result, "method", method)
	result = appendJSONStringField(result, "livename", liveName)
	result = appendJSONStringField(result, "title", title)
	result = appendJSONStringField(result, "avatarThumb", avatarThumb)

	result = append(result, '}')
	return result, nil
}

// offlineStatusMessage 构造未开播状态通知。
// offlineStatusMessage builds the offline status notification.
func (r *Room) offlineStatusMessage() []byte {
	return []byte(fmt.Sprintf(`{"type":"system","event":"live_status","live":false,"room_id":%s,"message":"直播间未开播","retry_interval_seconds":%d}`,
		strconv.Quote(r.id), int(r.notifyInterval/time.Second)))
}

// offlineEndedStatusMessage 构造已下播状态通知。
// offlineEndedStatusMessage builds the ended-offline status notification.
func (r *Room) offlineEndedStatusMessage() []byte {
	return []byte(fmt.Sprintf(`{"type":"system","event":"live_status","live":false,"room_id":%s,"message":"直播间已下播","ended":true,"retry_interval_seconds":%d}`,
		strconv.Quote(r.id), int(r.notifyInterval/time.Second)))
}

// onlineStatusMessage 构造已开播状态通知。
// onlineStatusMessage builds the online status notification.
func (r *Room) onlineStatusMessage() []byte {
	return []byte(fmt.Sprintf(`{"type":"system","event":"live_status","live":true,"room_id":%s,"message":"直播间已开播"}`,
		strconv.Quote(r.id)))
}

// notifyOfflineStatus 广播未开播状态通知。
// notifyOfflineStatus broadcasts the offline status notification.
func (r *Room) notifyOfflineStatus() {
	r.Broadcast(r.offlineStatusMessage())
}

// notifyOfflineEndedStatus 广播已下播状态通知。
// notifyOfflineEndedStatus broadcasts the ended-offline status notification.
func (r *Room) notifyOfflineEndedStatus() {
	r.Broadcast(r.offlineEndedStatusMessage())
}

// notifyOnlineStatus 广播已开播状态通知。
// notifyOnlineStatus broadcasts the online status notification.
func (r *Room) notifyOnlineStatus() {
	r.Broadcast(r.onlineStatusMessage())
}

// AddClient 将一个客户端添加到房间
// AddClient 将新的下游 WebSocket 客户端接入房间，并按房间状态启动监听或返回状态。
// AddClient attaches a downstream WebSocket client and starts listening or returns status based on room state.
func (r *Room) AddClient(socket *gws.Conn) {
	clientID := socket.RemoteAddr().String()
	client := NewClient(clientID, socket)
	count := r.addClient(client)
	go client.writeLoop(func() {
		r.closeClient(clientID, nil)
	})

	r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)

	r.mu.Lock()
	switch {
	case r.closed:
		r.mu.Unlock()
		r.closeClient(clientID, serviceClosingMessage)
		return
	case r.douyinLive != nil:
		r.mu.Unlock()
		r.sendToClient(clientID, gws.OpcodeText, r.onlineStatusMessage())
		return
	case r.monitorStopCh != nil:
		r.mu.Unlock()
		r.sendToClient(clientID, gws.OpcodeText, r.offlineStatusMessage())
		return
	case r.starting:
		r.mu.Unlock()
		return
	default:
		r.starting = true
		r.mu.Unlock()
	}

	r.logger.Info("第一个客户端连接，正在检查直播状态", "room_id", r.id)
	err := r.startLiveSession()

	r.mu.Lock()
	r.starting = false
	r.mu.Unlock()

	if err == nil {
		r.logger.Info("直播连接已成功启动", "room_id", r.id)
		return
	}
	if errors.Is(err, errRoomInactive) {
		r.removeIfIdle()
		return
	}
	if errors.Is(err, douyinLive.ErrLiveNotStarted) {
		if r.clientCount() == 0 {
			r.removeIfIdle()
			return
		}
		r.logger.Info("当前未开播，进入后台轮询监控", "room_id", r.id)
		r.notifyOfflineStatus()
		r.startMonitorLoop()

		return
	}

	r.logger.Error("启动抖音直播监听失败", "room_id", r.id, "err", err)
	r.closeAllClients(liveInvalidMessage)
	r.removeIfIdle()
}

// RemoveClient 从房间移除一个客户端
// RemoveClient 从房间移除并关闭指定客户端。
// RemoveClient removes and closes a client from the room.
func (r *Room) RemoveClient(clientID string) {
	r.closeClient(clientID, nil)
}

// sendToClient 向指定客户端发送消息，队列满时关闭慢客户端。
// sendToClient sends a message to one client and closes slow clients when their queue is full.
func (r *Room) sendToClient(clientID string, opcode gws.Opcode, payload []byte) {
	client, ok := r.getClient(clientID)
	if !ok {
		return
	}
	if client.enqueue(opcode, payload) {
		return
	}

	r.logger.Warn("客户端消费过慢，关闭连接", "client_id", clientID, "room_id", r.id)
	r.closeClient(clientID, slowClientClosingMessage)
}

// closeBackgroundWorkers 停止房间后台监控和上游直播监听。
// closeBackgroundWorkers stops room background monitoring and upstream live listening.
func (r *Room) closeBackgroundWorkers() {
	r.stopMonitorLoop()
	r.closeDouyinLive()
	r.removeIfIdle()
}

// closeAllClients 关闭并移除房间内所有客户端。
// closeAllClients closes and removes every client in the room.
func (r *Room) closeAllClients(closePayload []byte) {
	clients := r.clearClients()
	for _, client := range clients {
		client.close(closePayload)
	}
}

// closeDouyinLive 在后台关闭抖音直播连接
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

// startMonitorLoop 启动未开播轮询，并在开播后切换到直播监听。
// startMonitorLoop starts offline polling and switches to live listening once the room starts.
func (r *Room) startMonitorLoop() {
	r.mu.Lock()
	if r.closed || r.monitorStopCh != nil || r.douyinLive != nil {
		r.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	r.monitorStopCh = stopCh
	r.monitorDoneCh = doneCh
	pollInterval := r.pollInterval
	notifyInterval := r.notifyInterval
	r.mu.Unlock()

	go func() {
		defer close(doneCh)
		defer func() {
			r.mu.Lock()
			if r.monitorStopCh == stopCh {
				r.monitorStopCh = nil
				r.monitorDoneCh = nil
			}
			r.mu.Unlock()
			r.removeIfIdle()
		}()

		pollTicker := time.NewTicker(pollInterval)
		defer pollTicker.Stop()
		notifyTicker := time.NewTicker(notifyInterval)
		defer notifyTicker.Stop()

		for {
			select {
			case <-stopCh:
				return
			case <-notifyTicker.C:
				if r.clientCount() == 0 {
					return
				}
				r.notifyOfflineStatus()
			case <-pollTicker.C:
				if r.clientCount() == 0 {
					return
				}

				r.mu.Lock()
				if r.closed || r.douyinLive != nil {
					r.mu.Unlock()
					return
				}
				if r.starting {
					r.mu.Unlock()
					continue
				}
				r.starting = true
				r.mu.Unlock()

				err := r.startLiveSession()

				r.mu.Lock()
				r.starting = false
				r.mu.Unlock()

				switch {
				case err == nil:
					return
				case errors.Is(err, errRoomInactive):
					return
				case errors.Is(err, douyinLive.ErrLiveNotStarted):
					r.logger.Debug("房间仍未开播，继续等待", "room_id", r.id)
				default:
					r.logger.Warn("检查直播状态失败，将继续轮询", "room_id", r.id, "err", err)
				}
			}
		}
	}()
}

// stopMonitorLoop 停止未开播轮询并等待后台 goroutine 退出。
// stopMonitorLoop stops offline polling and waits for the background goroutine to exit.
func (r *Room) stopMonitorLoop() {
	r.mu.Lock()
	stopCh := r.monitorStopCh
	doneCh := r.monitorDoneCh
	r.monitorStopCh = nil
	r.monitorDoneCh = nil
	r.mu.Unlock()

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
			r.logger.Warn("等待监控循环退出超时，跳过阻塞等待", "room_id", r.id)
		}
	}
}

// startLiveSession 启动抖音直播监听和事件处理。
// 该方法负责创建 DouyinLive、显式判定开播状态，并在确认开播后启动后台 WS 会话。
// startLiveSession 创建 DouyinLive、确认开播状态并启动上游监听。
// startLiveSession creates DouyinLive, verifies live status, and starts upstream listening.
func (r *Room) startLiveSession() error {
	var (
		d   *douyinLive.DouyinLive
		err error
	)
	switch r.signProvider {
	case signProviderTikHub:
		d, err = douyinLive.NewDouyinLiveWithSlogAndTikHub(r.id, r.logger.base, r.cookie, r.tikHubKey)
	default:
		d, err = douyinLive.NewDouyinLiveWithSlog(r.id, r.logger.base, r.cookie)
	}
	if err != nil {
		return err
	}

	isLive, err := d.IsLive()
	if err != nil {
		d.Dispose()
		return fmt.Errorf("检查直播间 %s 状态失败: %w", r.id, err)
	}
	if !isLive {
		d.Dispose()
		return fmt.Errorf("直播间 %s 未开播: %w", r.id, douyinLive.ErrLiveNotStarted)
	}

	r.mu.Lock()
	r.douyinLive = d
	r.mu.Unlock()

	d.SubscribeMessage(func(message *douyinLive.LiveMessage) {
		r.handleDouyinEvent(message)
	})

	if r.clientCount() == 0 {
		r.disposePendingLive(d)
		return errRoomInactive
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		r.disposePendingLive(d)
		return errRoomInactive
	}
	r.mu.Unlock()

	r.notifyOnlineStatus()
	go r.runLiveSession(d)
	r.logger.Info("抖音直播监听已成功启动", "room_id", r.id)
	return nil
}

// disposePendingLive 释放尚未被房间正式接管的 DouyinLive 实例。
// disposePendingLive disposes a DouyinLive instance that the room has not fully adopted.
func (r *Room) disposePendingLive(d *douyinLive.DouyinLive) {
	r.mu.Lock()
	if r.douyinLive == d {
		r.douyinLive = nil
	}
	r.mu.Unlock()

	d.Dispose()
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

// runLiveSession 运行上游直播监听，并在结束后按需切回未开播监控。
// runLiveSession runs upstream live listening and switches back to offline monitoring when needed.
func (r *Room) runLiveSession(d *douyinLive.DouyinLive) {
	if err := d.Start(); err != nil {
		r.logger.Warn("直播监听运行结束", "room_id", r.id, "err", err)
	}

	r.mu.Lock()
	if r.douyinLive == d {
		r.douyinLive = nil
	}
	closed := r.closed
	monitorRunning := r.monitorStopCh != nil
	r.mu.Unlock()

	if closed || r.clientCount() == 0 {
		return
	}

	r.notifyOfflineEndedStatus()
	if !monitorRunning {
		r.logger.Info("直播连接已结束，切回未开播监控模式", "room_id", r.id)
		r.startMonitorLoop()
	}
}

// handleDouyinEvent 处理从抖音接收到的事件
// handleDouyinEvent 将抖音消息解析为 JSON 并广播给房间客户端。
// handleDouyinEvent converts a Douyin message to JSON and broadcasts it to room clients.
func (r *Room) handleDouyinEvent(event *douyinLive.LiveMessage) {
	if r.clientCount() == 0 {
		return
	}
	if event == nil || event.Raw == nil {
		return
	}

	eventData := event.Raw
	msg := event.Parsed
	var err error
	if msg == nil {
		msg, err = generated.GetMessageInstance(eventData.Method)
		if err != nil {
			if r.unknown {
				r.logger.Debug("未知消息类型", "room_id", r.id, "method", eventData.Method, "payload_len", len(eventData.Payload))
			}
			return
		}
		defer generated.PutMessageInstance(eventData.Method, msg)

		if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
			r.logger.Warn("Protobuf 反序列化失败", "room_id", r.id, "method", eventData.Method, "err", err)
			return
		}
	}

	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		r.logger.Warn("JSON 序列化失败", "room_id", r.id, "method", eventData.Method, "err", err)
		return
	}

	finalJSON, err := r.buildEventJSON(jsonBytes, eventData.Method, event.LiveName, event.Title, event.AvatarThumb)
	if err != nil {
		r.logger.Warn("事件 JSON 组装失败", "room_id", r.id, "method", eventData.Method, "err", err)
		return
	}

	r.Broadcast(finalJSON)
}

// Broadcast 将消息广播到房间内的所有客户端
// Broadcast 向房间内所有客户端广播消息。
// Broadcast sends a message to every client in the room.
func (r *Room) Broadcast(message []byte) {
	clients := r.snapshotClients()
	for _, client := range clients {
		if client.enqueue(gws.OpcodeText, message) {
			continue
		}
		r.logger.Warn("客户端消费过慢，关闭连接", "client_id", client.id, "room_id", r.id)
		r.closeClient(client.id, slowClientClosingMessage)
	}
}

// Close 关闭房间，停止监听并清理资源（优雅退出）
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
