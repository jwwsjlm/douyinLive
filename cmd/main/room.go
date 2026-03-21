package main

import (
	"bytes"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/lxzan/gws"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const clientSendQueueSize = 256

var (
	pongMessage              = []byte("pong")
	serviceClosingMessage    = []byte(`{"type":"system","message":"服务正在关闭"}`)
	liveInvalidMessage       = []byte(`{"type":"system","message":"直播间未开播或ID无效"}`)
	liveNotStartedMessage    = []byte(`{"type":"system","message":"直播间未开播"}`)
	slowClientClosingMessage = []byte(`{"type":"system","message":"客户端消费过慢，连接已关闭"}`)
)

// RoomManager 管理所有直播间实例
type RoomManager struct {
	rooms   map[string]*Room
	roomsMu sync.RWMutex
	logger  *log.Logger
	unknown bool
	cookie  string // 抖音 Cookie
}

// NewRoomManager 创建一个新的 RoomManager
// cookie 参数：可选的抖音 Cookie
func NewRoomManager(logger *log.Logger, unknown bool, cookie string) *RoomManager {
	return &RoomManager{
		rooms:   make(map[string]*Room),
		logger:  logger,
		unknown: unknown,
		cookie:  cookie,
	}
}

// GetOrCreateRoom 获取或创建一个新的房间实例
func (rm *RoomManager) GetOrCreateRoom(roomID string) *Room {
	rm.roomsMu.RLock()
	room, ok := rm.rooms[roomID]
	rm.roomsMu.RUnlock()
	if ok {
		return room
	}

	rm.roomsMu.Lock()
	defer rm.roomsMu.Unlock()
	if room, ok = rm.rooms[roomID]; ok {
		return room
	}

	room = NewRoom(roomID, rm.logger, rm.unknown, rm.cookie, func() {
		rm.roomsMu.Lock()
		delete(rm.rooms, roomID)
		rm.roomsMu.Unlock()
		rm.logger.Printf("房间 %s 已从管理器中移除", roomID)
	})
	rm.rooms[roomID] = room
	return room
}

// CloseAll 关闭所有房间
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

type outboundMessage struct {
	opcode  gws.Opcode
	payload []byte
}

type Client struct {
	id        string
	conn      *gws.Conn
	sendQueue chan outboundMessage
	stopCh    chan struct{}
	closeOnce sync.Once
}

func NewClient(id string, conn *gws.Conn) *Client {
	return &Client{
		id:        id,
		conn:      conn,
		sendQueue: make(chan outboundMessage, clientSendQueueSize),
		stopCh:    make(chan struct{}),
	}
}

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

func (c *Client) writeLoop(room *Room) {
	for {
		select {
		case <-c.stopCh:
			return
		case msg := <-c.sendQueue:
			if err := c.conn.WriteMessage(msg.opcode, msg.payload); err != nil {
				c.close(nil)
				return
			}
		}
	}
}

func (c *Client) close(closePayload []byte) {
	c.closeOnce.Do(func() {
		close(c.stopCh)
		if closePayload != nil {
			_ = c.conn.WriteClose(1000, closePayload)
		}
		if nc := c.conn.NetConn(); nc != nil {
			_ = nc.Close()
		}
	})
}

// Room 代表一个直播间
type Room struct {
	id                   string
	logger               *log.Logger
	clients              map[string]*Client
	clientsMu            sync.RWMutex
	douyinLive           *douyinLive.DouyinLive
	mu                   sync.Mutex
	onClose              func()
	unknown              bool
	cookie               string
	liveNameCacheMu      sync.RWMutex
	liveNameCacheKey     string
	liveNameCachePayload []byte
}

// NewRoom 创建一个新的房间实例
// cookie 参数：可选的抖音 Cookie
func NewRoom(id string, logger *log.Logger, unknown bool, cookie string, onClose func()) *Room {
	return &Room{
		id:      id,
		logger:  logger,
		clients: make(map[string]*Client),
		onClose: onClose,
		unknown: unknown,
		cookie:  cookie,
	}
}

func (r *Room) addClient(client *Client) int {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()
	r.clients[client.id] = client
	return len(r.clients)
}

func (r *Room) getClient(clientID string) (*Client, bool) {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	client, ok := r.clients[clientID]
	return client, ok
}

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

func (r *Room) snapshotClients() []*Client {
	r.clientsMu.RLock()
	defer r.clientsMu.RUnlock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	return clients
}

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

func (r *Room) getLiveNamePayload(liveName string) []byte {
	r.liveNameCacheMu.RLock()
	if r.liveNameCacheKey == liveName {
		payload := r.liveNameCachePayload
		r.liveNameCacheMu.RUnlock()
		return payload
	}
	r.liveNameCacheMu.RUnlock()

	payload := []byte(fmt.Sprintf(`,"livename":%s`, strconv.Quote(liveName)))

	r.liveNameCacheMu.Lock()
	r.liveNameCacheKey = liveName
	r.liveNameCachePayload = payload
	r.liveNameCacheMu.Unlock()

	return payload
}

// AddClient 将一个客户端添加到房间
func (r *Room) AddClient(socket *gws.Conn) {
	clientID := socket.RemoteAddr().String()
	client := NewClient(clientID, socket)
	count := r.addClient(client)
	go client.writeLoop(r)

	r.logger.Printf("客户端 %s 连接到房间 %s, 当前连接数: %d", clientID, r.id, count)

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.douyinLive == nil {
		r.logger.Printf("房间 %s 的第一个客户端连接, 正在启动抖音直播监听...", r.id)
		if err := r.startLiveSession(); err != nil {
			r.logger.Printf("启动抖音直播监听失败: %v", err)
			_, _, _ = r.removeClient(clientID)
			client.close(liveInvalidMessage)
		}
	} else if !r.douyinLive.IsLive() {
		_, _, _ = r.removeClient(clientID)
		client.close(liveNotStartedMessage)
	}
}

// RemoveClient 从房间移除一个客户端
func (r *Room) RemoveClient(clientID string) {
	client, remaining, removed := r.removeClient(clientID)
	if !removed {
		return
	}

	if client != nil {
		client.close(nil)
	}

	r.logger.Printf("客户端 %s 断开连接, 房间 %s 剩余连接数: %d", clientID, r.id, remaining)

	r.mu.Lock()
	shouldClose := remaining == 0 && r.douyinLive != nil
	r.mu.Unlock()

	if shouldClose {
		r.logger.Printf("房间 %s 的最后一个客户端已断开, 正在关闭抖音直播监听...", r.id)
		go r.closeDouyinLive()
	}
}

func (r *Room) sendToClient(clientID string, opcode gws.Opcode, payload []byte) {
	client, ok := r.getClient(clientID)
	if !ok {
		return
	}
	if client.enqueue(opcode, payload) {
		return
	}

	r.logger.Printf("客户端 %s (房间: %s) 消费过慢，关闭连接", clientID, r.id)
	client.close(slowClientClosingMessage)
}

// closeDouyinLive 在后台关闭抖音直播连接
func (r *Room) closeDouyinLive() {
	r.mu.Lock()
	d := r.douyinLive
	r.douyinLive = nil
	r.mu.Unlock()

	if d != nil {
		d.Close()
	}
}

// startLiveSession 启动抖音直播监听和事件处理
func (r *Room) startLiveSession() error {
	d, err := douyinLive.NewDouyinLive(r.id, r.logger, r.cookie)
	if err != nil {
		return err
	}

	d.Subscribe(func(eventData *new_douyin.Webcast_Im_Message) {
		r.handleDouyinEvent(eventData, d.LiveName)
	})

	go d.Start()
	r.douyinLive = d
	r.logger.Printf("房间 %s 的抖音直播监听已成功启动", r.id)
	return nil
}

// handleDouyinEvent 处理从抖音接收到的事件（优化版：减少 JSON 转换）
func (r *Room) handleDouyinEvent(eventData *new_douyin.Webcast_Im_Message, liveName string) {
	msg, err := generated.GetMessageInstance(eventData.Method)
	if err != nil {
		if r.unknown {
			r.logger.Printf("未知消息类型: method=%s payload_len=%d", eventData.Method, len(eventData.Payload))
		}
		return
	}

	if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
		r.logger.Printf("Protobuf 反序列化失败: %v, 方法: %s", err, eventData.Method)
		return
	}

	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		r.logger.Printf("JSON 序列化失败: %v", err)
		return
	}

	lastCloseBrace := bytes.LastIndexByte(jsonBytes, '}')
	if lastCloseBrace == -1 {
		r.logger.Printf("无效的 JSON 格式")
		return
	}

	livenameJSON := r.getLiveNamePayload(liveName)
	finalJSON := make([]byte, 0, len(jsonBytes)+len(livenameJSON))
	finalJSON = append(finalJSON, jsonBytes[:lastCloseBrace]...)
	finalJSON = append(finalJSON, livenameJSON...)
	finalJSON = append(finalJSON, '}')

	r.Broadcast(finalJSON)
}

// Broadcast 将消息广播到房间内的所有客户端
func (r *Room) Broadcast(message []byte) {
	clients := r.snapshotClients()
	for _, client := range clients {
		if client.enqueue(gws.OpcodeText, message) {
			continue
		}
		r.logger.Printf("客户端 %s (房间: %s) 消费过慢，关闭连接", client.id, r.id)
		client.close(slowClientClosingMessage)
		go r.RemoveClient(client.id)
	}
}

// Close 关闭房间，停止监听并清理资源（优雅退出）
func (r *Room) Close() {
	r.mu.Lock()
	d := r.douyinLive
	r.douyinLive = nil
	onClose := r.onClose
	r.mu.Unlock()

	for _, client := range r.clearClients() {
		client.close(serviceClosingMessage)
	}
	r.logger.Printf("房间 %s 的所有客户端连接已关闭", r.id)

	if d != nil {
		d.Close()
		r.logger.Printf("房间 %s 的抖音直播监听已关闭", r.id)
	}

	if onClose != nil {
		onClose()
	}
}
