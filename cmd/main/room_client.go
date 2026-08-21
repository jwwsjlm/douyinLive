package main

import (
	"errors"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
	"github.com/lxzan/gws"
)

// clientSendQueueSize 限制单个客户端的待发送消息队列长度。
// clientSendQueueSize limits the pending outbound queue size for each client.
const clientSendQueueSize = 256

// maxClientQueuedBytes bounds retained outbound payloads per downstream client.
// maxClientQueuedBytes 限制每个下游客户端待发送消息占用的总字节数。
const maxClientQueuedBytes = 16 << 20

// maxClientMessageBytes prevents one oversized upstream message from
// monopolizing memory for every connected downstream client.
// maxClientMessageBytes 防止单条异常上游消息占满所有下游客户端内存。
const maxClientMessageBytes = 8 << 20

// clientWriteTimeout 限制向客户端写消息的最长时间。
// clientWriteTimeout limits how long a write to a client may take.
const clientWriteTimeout = 5 * time.Second

// clientControlWriteTimeout bounds close and pong control-frame writes.
// clientControlWriteTimeout 限制 close 和 pong 控制帧的写入耗时。
const clientControlWriteTimeout = time.Second

type enqueueResult uint8

const (
	enqueueAccepted enqueueResult = iota
	enqueueClientClosed
	enqueueQueueFull
	enqueueQueueBytesExceeded
	enqueueMessageTooLarge
)

// outboundMessage 表示写入客户端队列的一条待发送消息。
// outboundMessage represents one pending message in a client's outbound queue.
type outboundMessage struct {
	opcode  gws.Opcode
	payload []byte
}

// clientCloseDirective describes the final application message and the short
// protocol close reason sent when a downstream client is disconnected.
// clientCloseDirective 描述断开下游客户端前发送的最终业务消息，以及符合协议长度限制的简短关闭原因。
type clientCloseDirective struct {
	message []byte
	code    uint16
	reason  string
}

var (
	normalClientClose          = clientCloseDirective{code: 1000, reason: "normal_close"}
	serviceClientClose         = clientCloseDirective{message: serviceClosingMessage, code: 1001, reason: "service_shutdown"}
	invalidRoomClientClose     = clientCloseDirective{message: roomInvalidMessage, code: 1008, reason: "room_not_found"}
	liveStartFailedClientClose = clientCloseDirective{message: liveStartFailedMessage, code: 1011, reason: "upstream_start_failed"}
	slowClientClose            = clientCloseDirective{message: slowClientClosingMessage, code: 1008, reason: "client_too_slow"}
)

// Client 表示一个下游 WebSocket 客户端连接。
// Client represents one downstream WebSocket client connection.
type Client struct {
	id          string
	conn        *gws.Conn
	sendQueue   chan outboundMessage
	stopCh      chan struct{}
	closeOnce   sync.Once
	stateMu     sync.RWMutex
	writeMu     sync.Mutex
	queuedBytes int
}

// NewClient 创建客户端连接包装器。
// NewClient creates a client connection wrapper.
// 参数/Parameters:
//   - id: 客户端连接唯一 ID。 Unique client connection ID.
//   - conn: 底层 WebSocket 连接。 Underlying WebSocket connection.
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
// 参数/Parameters:
//   - opcode: 要发送的 WebSocket 帧类型。 WebSocket frame opcode to send.
//   - payload: 要发送的消息载荷。 Message payload to send.
func (c *Client) enqueue(opcode gws.Opcode, payload []byte) bool {
	return c.enqueueWithResult(opcode, payload) == enqueueAccepted
}

func (c *Client) enqueueWithResult(opcode gws.Opcode, payload []byte) enqueueResult {
	if len(payload) > maxClientMessageBytes {
		return enqueueMessageTooLarge
	}
	c.stateMu.Lock()
	defer c.stateMu.Unlock()
	select {
	case <-c.stopCh:
		return enqueueClientClosed
	default:
	}
	if c.queuedBytes+len(payload) > maxClientQueuedBytes {
		return enqueueQueueBytesExceeded
	}

	select {
	case c.sendQueue <- outboundMessage{opcode: opcode, payload: payload}:
		c.queuedBytes += len(payload)
		return enqueueAccepted
	default:
		return enqueueQueueFull
	}
}

func (c *Client) releaseQueuedBytes(size int) {
	if size <= 0 {
		return
	}
	c.stateMu.Lock()
	c.queuedBytes -= size
	if c.queuedBytes < 0 {
		c.queuedBytes = 0
	}
	c.stateMu.Unlock()
}

// writeLoop 串行消费发送队列并写入客户端连接。
// writeLoop serially drains the outbound queue and writes to the client connection.
// 参数/Parameters:
//   - onWriteError: 写入失败时调用的清理回调。 Cleanup callback invoked on write failure.
func (c *Client) writeLoop(onWriteError func()) {
	for {
		select {
		case <-c.stopCh:
			return
		case msg, ok := <-c.sendQueue:
			if !ok {
				return
			}
			err := c.writeMessage(msg.opcode, msg.payload)
			c.releaseQueuedBytes(len(msg.payload))
			if err != nil {
				c.close(normalClientClose)
				if onWriteError != nil {
					onWriteError()
				}
				return
			}
		}
	}
}

// close 幂等停止发送循环，发送可选的最终业务消息，再以简短 reason 关闭连接。
// close idempotently stops the send loop, sends an optional final application message, and closes with a short reason.
// 参数/Parameters:
//   - directive: 最终消息、关闭码和简短关闭原因。 Final message, close code, and short close reason.
func (c *Client) close(directive clientCloseDirective) {
	c.closeOnce.Do(func() {
		c.stateMu.Lock()
		close(c.stopCh)
		c.stateMu.Unlock()
		if c.conn == nil {
			return
		}
		_ = c.writeFinalAndClose(directive)
		if nc := c.conn.NetConn(); nc != nil {
			_ = nc.Close()
		}
	})
}

func (c *Client) writeFinalAndClose(directive clientCloseDirective) error {
	if c == nil || c.conn == nil {
		return errors.New("client connection is nil")
	}
	nc := c.conn.NetConn()
	if nc != nil {
		// Interrupt an already-blocked data write before waiting for writeMu.
		// 在等待 writeMu 前先打断可能阻塞的数据写入，避免关闭流程被慢客户端拖住。
		_ = nc.SetWriteDeadline(time.Now().Add(clientControlWriteTimeout))
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if nc != nil {
		_ = nc.SetWriteDeadline(time.Now().Add(clientControlWriteTimeout))
	}
	if len(directive.message) > 0 {
		if err := c.conn.WriteMessage(gws.OpcodeText, directive.message); err != nil {
			_ = c.conn.WriteClose(directive.code, []byte(directive.reason))
			return err
		}
	}
	return c.conn.WriteClose(directive.code, []byte(directive.reason))
}

func (c *Client) writeMessage(opcode gws.Opcode, payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("client connection is nil")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	nc := c.conn.NetConn()
	if nc != nil {
		_ = nc.SetWriteDeadline(time.Now().Add(clientWriteTimeout))
		defer nc.SetWriteDeadline(time.Time{})
	}
	return c.conn.WriteMessage(opcode, payload)
}

func (c *Client) writePong(payload []byte) error {
	if c == nil || c.conn == nil {
		return errors.New("client connection is nil")
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	nc := c.conn.NetConn()
	if nc != nil {
		_ = nc.SetWriteDeadline(time.Now().Add(clientControlWriteTimeout))
		defer nc.SetWriteDeadline(time.Time{})
	}
	return c.conn.WritePong(payload)
}

// closeClient 从房间移除并按指定关闭策略关闭客户端。
// closeClient removes a client from the room and closes it with the specified directive.
// 参数/Parameters:
//   - clientID: 要关闭的客户端 ID。 Client ID to close.
//   - directive: 最终业务消息和 WebSocket 关闭信息。 Final application message and WebSocket close information.
func (r *Room) closeClient(clientID string, directive clientCloseDirective) {
	client, _, removed := r.removeClient(clientID)
	if !removed {
		return
	}

	if client != nil {
		client.close(directive)
	}

	remaining := r.clientCount()
	r.logger.Info("客户端断开连接", "client_id", clientID, "room_id", r.id, "remaining_clients", remaining)

	if remaining == 0 {
		r.logger.Info("最后一个客户端已断开，正在关闭后台监听", "room_id", r.id)
		if !r.startTask(r.closeBackgroundWorkers) {
			r.closeBackgroundWorkers()
		}
	}
}

func (r *Room) clientIDForSocket(socket *gws.Conn) (string, bool) {
	if socket == nil {
		return "", false
	}
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	id, ok := r.connIDs[socket]
	return id, ok
}

// getClient 按 ID 获取客户端。
// getClient returns a client by ID.
// 参数/Parameters:
//   - clientID: 要查找的客户端 ID。 Client ID to look up.
func (r *Room) getClient(clientID string) (*Client, bool) {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	client, ok := r.clients[clientID]
	return client, ok
}

// removeClient 从房间移除客户端并返回剩余数量。
// removeClient removes a client from the room and returns the remaining count.
// 参数/Parameters:
//   - clientID: 要移除的客户端 ID。 Client ID to remove.
func (r *Room) removeClient(clientID string) (*Client, int, bool) {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	client, ok := r.clients[clientID]
	if !ok {
		return nil, len(r.clients), false
	}
	delete(r.clients, clientID)
	if client != nil && client.conn != nil {
		delete(r.connIDs, client.conn)
	}
	return client, len(r.clients), true
}

// clientCount 返回当前客户端数量。
// clientCount returns the current number of clients.
func (r *Room) clientCount() int {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	return len(r.clients)
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
	r.connIDs = make(map[*gws.Conn]string)
	return clients
}

// AddClient 将新的下游 WebSocket 客户端接入房间，并按房间状态启动监听或返回状态。
// AddClient attaches a downstream WebSocket client and starts listening or returns status based on room state.
// 参数/Parameters:
//   - socket: 下游客户端 WebSocket 连接。 Downstream client WebSocket connection.
func (r *Room) AddClient(socket *gws.Conn) string {
	clientID := newClientID()
	client := NewClient(clientID, socket)

	r.mu.Lock()
	if r.pendingClients > 0 {
		r.pendingClients--
	}
	if r.closed {
		r.mu.Unlock()
		client.close(serviceClientClose)
		return clientID
	}
	r.clientsMu.Lock()
	r.clients[clientID] = client
	if socket != nil {
		r.connIDs[socket] = clientID
	}
	count := len(r.clients)
	r.clientsMu.Unlock()

	switch {
	case r.douyinLive != nil:
		upstreamReady := r.upstreamReady
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, normalClientClose)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		if upstreamReady {
			r.sendToClient(clientID, gws.OpcodeText, r.onlineStatusMessage())
		}
		return clientID
	case r.monitorStopCh != nil:
		statusUnknown := r.statusUnknown
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, normalClientClose)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		if statusUnknown {
			r.sendToClient(clientID, gws.OpcodeText, r.statusUnknownMessage())
		} else {
			r.sendToClient(clientID, gws.OpcodeText, r.offlineStatusMessage())
		}
		return clientID
	case r.starting:
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, normalClientClose)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		return clientID
	default:
		r.starting = true
		r.mu.Unlock()
	}

	go client.writeLoop(func() {
		r.closeClient(clientID, normalClientClose)
	})

	r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)

	// Do not hold the WebSocket upgrade/read-loop path while talking to Douyin.
	// 上游 HTTP 初始化可能耗时较长，必须异步执行，避免阻塞客户端握手。
	if !r.startTask(r.initializeLiveSession) {
		r.closeClient(clientID, serviceClientClose)
	}
	return clientID
}

func (r *Room) initializeLiveSession() {
	r.logger.Info("第一个客户端连接，正在检查直播状态", "room_id", r.id)
	err := r.startLiveSession()

	r.mu.Lock()
	r.starting = false
	r.mu.Unlock()

	if err == nil {
		r.logger.Info("直播连接初始化已提交，等待上游 WebSocket 握手", "room_id", r.id)
		return
	}
	if errors.Is(err, errRoomInactive) {
		r.removeIfIdle()
		return
	}
	if errors.Is(err, douyinLive.ErrRoomNotFound) {
		r.logger.Warn("直播间不存在，关闭客户端连接", "room_id", r.id, "err", err)
		r.closeAllClients(invalidRoomClientClose)
		r.removeIfIdle()
		return
	}
	if errors.Is(err, douyinLive.ErrLiveNotStarted) {
		if r.clientCount() == 0 {
			r.removeIfIdle()
			return
		}
		r.logger.Info("当前未开播，进入后台轮询监控", "room_id", r.id)
		r.setStatusUnknown(false)
		r.notifyOfflineStatus()
		r.startMonitorLoop()
		return
	}
	if errors.Is(err, douyinLive.ErrLiveStatusUnknown) {
		if r.clientCount() == 0 {
			r.removeIfIdle()
			return
		}
		r.logger.Warn("暂时无法确认直播状态，保留客户端并进入轮询", "room_id", r.id, "err", err)
		r.setStatusUnknown(true)
		r.notifyStatusUnknown()
		r.startMonitorLoop()
		return
	}

	r.logger.Error("启动抖音直播监听失败", "room_id", r.id, "err", err)
	r.closeAllClients(liveStartFailedClientClose)
	r.removeIfIdle()
}

func (r *Room) writePong(socket *gws.Conn, payload []byte) bool {
	id, ok := r.clientIDForSocket(socket)
	if !ok {
		return false
	}
	client, ok := r.getClient(id)
	if !ok {
		return false
	}
	return client.writePong(payload) == nil
}

// RemoveClient 从房间移除并关闭指定客户端。
// RemoveClient removes and closes a client from the room.
// 参数/Parameters:
//   - clientID: 需要移除的客户端 ID。 Client ID to remove.
func (r *Room) RemoveClient(clientID string) {
	r.closeClient(clientID, normalClientClose)
}

// sendToClient 向指定客户端发送消息，队列满时关闭慢客户端。
// sendToClient sends a message to one client and closes slow clients when their queue is full.
// 参数/Parameters:
//   - clientID: 目标客户端 ID。 Target client ID.
//   - opcode: 要发送的 WebSocket 帧类型。 WebSocket frame opcode to send.
//   - payload: 要发送的消息载荷。 Message payload to send.
func (r *Room) sendToClient(clientID string, opcode gws.Opcode, payload []byte) {
	client, ok := r.getClient(clientID)
	if !ok {
		return
	}
	switch client.enqueueWithResult(opcode, payload) {
	case enqueueAccepted, enqueueClientClosed:
		return
	case enqueueMessageTooLarge:
		r.logger.Warn("跳过超过下游消息大小限制的消息", "client_id", clientID, "room_id", r.id, "payload_len", len(payload), "max_payload_len", maxClientMessageBytes)
		return
	}

	r.logger.Warn("客户端消费过慢，关闭连接", "client_id", clientID, "room_id", r.id)
	r.closeClient(clientID, slowClientClose)
}

// closeAllClients 按指定关闭策略关闭并移除房间内所有客户端。
// closeAllClients closes and removes every client in the room using the specified directive.
// 参数/Parameters:
//   - directive: 最终业务消息和 WebSocket 关闭信息。 Final application message and WebSocket close information.
func (r *Room) closeAllClients(directive clientCloseDirective) {
	clients := r.clearClients()
	for _, client := range clients {
		client.close(directive)
	}
}

// Broadcast 向房间内所有客户端广播消息。
// Broadcast sends a message to every client in the room.
// 参数/Parameters:
//   - message: 要广播的消息字节。 Message bytes to broadcast.
func (r *Room) Broadcast(message []byte) {
	clients := r.snapshotClients()
	for _, client := range clients {
		switch client.enqueueWithResult(gws.OpcodeText, message) {
		case enqueueAccepted, enqueueClientClosed:
			continue
		case enqueueMessageTooLarge:
			r.logger.Warn("跳过超过下游消息大小限制的广播消息", "room_id", r.id, "payload_len", len(message), "max_payload_len", maxClientMessageBytes)
			return
		}
		r.logger.Warn("客户端消费过慢，关闭连接", "client_id", client.id, "room_id", r.id)
		r.closeClient(client.id, slowClientClose)
	}
}
