package main

import (
	"strings"
	"sync"
	"time"

	"crypto/sha256"
	"encoding/hex"
)

// RoomManager 管理所有直播间实例及其复用键。
// RoomManager manages all room instances and their reuse keys.
type RoomManager struct {
	rooms          map[string]*Room
	roomsMu        sync.RWMutex
	logger         *appLogger
	unknown        bool
	cookie         string            // 抖音默认 Cookie。 Default Douyin Cookie.
	roomCookies    map[string]string // 按直播间 ID 配置的 Cookie。 Per-room Cookie overrides keyed by room ID.
	signProvider   string
	tikHubKey      string
	pollInterval   time.Duration
	notifyInterval time.Duration
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
// 参数/Parameters:
//   - roomID: 当前直播间 ID。 Current live room ID.
//   - override: 本次连接传入的 Cookie 覆盖值。 Cookie override provided by the current connection.
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
