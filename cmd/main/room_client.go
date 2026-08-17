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

// clientWriteTimeout 限制向客户端写消息的最长时间。
// clientWriteTimeout limits how long a write to a client may take.
const clientWriteTimeout = 5 * time.Second

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
// 参数/Parameters:
//   - closePayload: 可选 close 帧载荷。 Optional close-frame payload.
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
// 参数/Parameters:
//   - clientID: 要关闭的客户端 ID。 Client ID to close.
//   - closePayload: 可选 close 帧载荷。 Optional close-frame payload.
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

// addClient 将客户端加入房间并返回当前客户端数量。
// addClient adds a client to the room and returns the current client count.
// 参数/Parameters:
//   - client: 要加入房间的客户端。 Client to add to the room.
func (r *Room) addClient(client *Client) int {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	r.clients[client.id] = client
	return len(r.clients)
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
	return clients
}

// AddClient 将新的下游 WebSocket 客户端接入房间，并按房间状态启动监听或返回状态。
// AddClient attaches a downstream WebSocket client and starts listening or returns status based on room state.
// 参数/Parameters:
//   - socket: 下游客户端 WebSocket 连接。 Downstream client WebSocket connection.
func (r *Room) AddClient(socket *gws.Conn) {
	clientID := socket.RemoteAddr().String()
	client := NewClient(clientID, socket)

	r.mu.Lock()
	if r.pendingClients > 0 {
		r.pendingClients--
	}
	if r.closed {
		r.mu.Unlock()
		client.close(serviceClosingMessage)
		return
	}
	r.clientsMu.Lock()
	r.clients[clientID] = client
	count := len(r.clients)
	r.clientsMu.Unlock()

	switch {
	case r.douyinLive != nil:
		upstreamReady := r.upstreamReady
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, nil)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		if upstreamReady {
			r.sendToClient(clientID, gws.OpcodeText, r.onlineStatusMessage())
		}
		return
	case r.monitorStopCh != nil:
		statusUnknown := r.statusUnknown
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, nil)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		if statusUnknown {
			r.sendToClient(clientID, gws.OpcodeText, r.statusUnknownMessage())
		} else {
			r.sendToClient(clientID, gws.OpcodeText, r.offlineStatusMessage())
		}
		return
	case r.starting:
		r.mu.Unlock()
		go client.writeLoop(func() {
			r.closeClient(clientID, nil)
		})
		r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)
		return
	default:
		r.starting = true
		r.mu.Unlock()
	}

	go client.writeLoop(func() {
		r.closeClient(clientID, nil)
	})

	r.logger.Info("客户端连接到房间", "client_id", clientID, "room_id", r.id, "client_count", count)

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
		r.closeAllClients(roomInvalidMessage)
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
	r.closeAllClients(liveStartFailedMessage)
	r.removeIfIdle()
}

// RemoveClient 从房间移除并关闭指定客户端。
// RemoveClient removes and closes a client from the room.
// 参数/Parameters:
//   - clientID: 需要移除的客户端 ID。 Client ID to remove.
func (r *Room) RemoveClient(clientID string) {
	r.closeClient(clientID, nil)
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
	if client.enqueue(opcode, payload) {
		return
	}

	r.logger.Warn("客户端消费过慢，关闭连接", "client_id", clientID, "room_id", r.id)
	r.closeClient(clientID, slowClientClosingMessage)
}

// closeAllClients 关闭并移除房间内所有客户端。
// closeAllClients closes and removes every client in the room.
// 参数/Parameters:
//   - closePayload: 可选 close 帧载荷。 Optional close-frame payload.
func (r *Room) closeAllClients(closePayload []byte) {
	clients := r.clearClients()
	for _, client := range clients {
		client.close(closePayload)
	}
}

// Broadcast 向房间内所有客户端广播消息。
// Broadcast sends a message to every client in the room.
// 参数/Parameters:
//   - message: 要广播的消息字节。 Message bytes to broadcast.
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
