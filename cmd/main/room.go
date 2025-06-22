package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"sync"

	"github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/lxzan/gws"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// RoomManager 管理所有直播间实例
type RoomManager struct {
	rooms   sync.Map // key: roomID, value: *Room
	logger  *log.Logger
	unknown bool
}

// NewRoomManager 创建一个新的 RoomManager
func NewRoomManager(logger *log.Logger, unknown bool) *RoomManager {
	return &RoomManager{
		logger:  logger,
		unknown: unknown,
	}
}

// GetOrCreateRoom 获取或创建一个新的房间实例
func (rm *RoomManager) GetOrCreateRoom(roomID string) *Room {
	val, _ := rm.rooms.LoadOrStore(roomID, NewRoom(roomID, rm.logger, rm.unknown, func() {
		rm.rooms.Delete(roomID) // 提供一个回调函数，在房间关闭时从管理器中删除自己
		rm.logger.Printf("房间 %s 已从管理器中移除", roomID)
	}))
	return val.(*Room)
}

// CloseAll 关闭所有房间
func (rm *RoomManager) CloseAll() {
	rm.rooms.Range(func(key, value interface{}) bool {
		room := value.(*Room)
		room.Close()
		return true
	})
}

// Room 代表一个直播间
type Room struct {
	id          string
	logger      *log.Logger
	connections *gws.ConcurrentMap[string, *gws.Conn]
	douyinLive  *douyinLive.DouyinLive
	mu          sync.Mutex
	onClose     func() // 当房间关闭时调用的回调
	unknown     bool
}

// NewRoom 创建一个新的房间实例
func NewRoom(id string, logger *log.Logger, unknown bool, onClose func()) *Room {
	return &Room{
		id:          id,
		logger:      logger,
		connections: gws.NewConcurrentMap[string, *gws.Conn](16, 128),
		onClose:     onClose,
		unknown:     unknown,
	}
}

// AddClient 将一个客户端添加到房间
func (r *Room) AddClient(socket *gws.Conn) {
	r.mu.Lock()
	defer r.mu.Unlock()

	clientID := socket.RemoteAddr().String()
	r.connections.Store(clientID, socket)
	r.logger.Printf("客户端 %s 连接到房间 %s, 当前连接数: %d", clientID, r.id, r.connections.Len())

	// 如果这是第一个客户端，则启动抖音直播会话
	if r.douyinLive == nil {
		r.logger.Printf("房间 %s 的第一个客户端连接, 正在启动抖音直播监听...", r.id)
		if err := r.startLiveSession(socket); err != nil {
			r.logger.Printf("启动抖音直播监听失败: %v", err)
			// 从连接中移除该客户端
			r.connections.Delete(clientID)
		}
	} else if !r.douyinLive.IsLive() {
		// 如果已存在实例但未在直播，通知客户端
		_ = socket.WriteClose(1000, []byte(`{"type":"system","message":"直播间未开播"}`))
		r.connections.Delete(clientID) // 移除此无效连接
	}
}

// RemoveClient 从房间移除一个客户端
func (r *Room) RemoveClient(clientID string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.connections.Delete(clientID)
	r.logger.Printf("客户端 %s 断开连接, 房间 %s 剩余连接数: %d", clientID, r.id, r.connections.Len())

	// 如果这是最后一个客户端，则关闭抖音直播会话
	if r.connections.Len() == 0 && r.douyinLive != nil {
		r.logger.Printf("房间 %s 的最后一个客户端已断开, 正在关闭抖音直播监听...", r.id)
		r.Close()
	}
}

// startLiveSession 启动抖音直播监听和事件处理
func (r *Room) startLiveSession(socket *gws.Conn) error {
	d, err := douyinLive.NewDouyinLive(r.id, r.logger)
	if err != nil {
		// 启动失败，通知客户端并关闭连接
		_ = socket.WriteClose(1000, []byte(`{"type":"system","message":"直播间未开播或ID无效"}`))
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

// handleDouyinEvent 处理从抖音接收到的事件
func (r *Room) handleDouyinEvent(eventData *new_douyin.Webcast_Im_Message, liveName string) {
	msg, err := generated.GetMessageInstance(eventData.Method)
	if err != nil {
		if r.unknown {
			r.logger.Printf("未知消息类型: %v, Payload: %s", err, hex.EncodeToString(eventData.Payload))
		}
		return
	}

	if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
		r.logger.Printf("Protobuf 反序列化失败: %v, 方法: %s", err, eventData.Method)
		return
	}

	// 使用 protojson 进行序列化
	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		r.logger.Printf("JSON 序列化失败: %v", err)
		return
	}

	// 为了添加额外字段，我们解析到 map
	var msgMap map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &msgMap); err != nil {
		r.logger.Printf("解析中间 JSON 失败: %v", err)
		return
	}

	msgMap["livename"] = liveName // 添加直播间名称

	finalJSON, err := json.Marshal(msgMap)
	if err != nil {
		r.logger.Printf("最终 JSON 序列化失败: %v", err)
		return
	}

	r.Broadcast(finalJSON)
}

// Broadcast 将消息广播到房间内的所有客户端
func (r *Room) Broadcast(message []byte) {
	r.connections.Range(func(key string, conn *gws.Conn) bool {
		if err := conn.WriteMessage(gws.OpcodeText, message); err != nil {
			r.logger.Printf("发送消息到客户端 %s (房间: %s) 失败: %v", key, r.id, err)
		}
		return true
	})
}

// Close 关闭房间，停止监听并清理资源
func (r *Room) Close() {
	if r.douyinLive != nil {
		r.douyinLive.Close()
		r.douyinLive = nil
	}
	// 调用回调，通知 RoomManager 将自己移除
	if r.onClose != nil {
		r.onClose()
	}
}
