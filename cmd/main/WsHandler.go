package main

import (
	"github.com/jwwsjlm/douyinLive"
	"github.com/jwwsjlm/douyinLive/generated/new_douyin"
	"github.com/lxzan/gws"
	"log"
)

// OnOpen 链接到服务端
func (c *WsHandler) OnOpen(socket *gws.Conn) {
	clientID := socket.RemoteAddr().String()
	logger.Printf("客户端 %s 连接到房间 %s", clientID, c.RoomID)
	//c.OnClose(socket, nil) // 先关闭连接，避免重复连接
	// 先将客户端添加到房间组

	group, loaded := roomGroups.LoadOrStore(c.RoomID, &RoomGroup{
		connections: gws.NewConcurrentMap[string, *gws.Conn](16, 128),
		douyinLive:  nil, // 延迟初始化
	})
	roomGroup := group.(*RoomGroup)

	// 添加客户端连接
	roomGroup.connections.Store(clientID, socket)

	logger.Printf("房间 %s 当前连接数: %d", c.RoomID, roomGroup.connections.Len())

	// 如果是第一个连接到该房间的客户端，初始化抖音直播实例
	if !loaded {
		// 新创建的房间组，初始化抖音直播实例
		d, err := douyinLive.NewDouyinLive(c.RoomID, logger)
		if err != nil {
			// 发送通知给客户端
			if err := socket.WriteString(`{"type":"system","message":"直播间未开播"}`); err != nil {
				log.Printf("发送未开播消息失败: %v", err)
			}
			logger.Printf("连接到抖音房间 %s 失败: %v", c.RoomID, err)

			// 关闭WebSocket连接
			if err := socket.WriteClose(1000, nil); err != nil {
				logger.Printf("关闭WebSocket连接失败: %v", err)
			}
			return
		}

		// 订阅事件
		d.Subscribe(func(eventData *new_douyin.Webcast_Im_Message) {
			routeEventToRoomClients(c.RoomID, eventData, d.LiveName)
		})

		// 开始处理
		go d.Start()

		// 更新组中的抖音直播实例

		roomGroup.douyinLive = d

	} else {
		// 检查直播间是否已开播

		if roomGroup.douyinLive == nil || !roomGroup.douyinLive.IsLive() {
			// 直播间未开播，通知客户端并关闭连接
			if err := socket.WriteString(`{"type":"system","message":"直播间未开播"}`); err != nil {
				logger.Printf("发送未开播消息失败: %v", err)
			}

			// 关闭WebSocket连接
			if err := socket.WriteClose(1000, []byte(`{"type":"system","message":"直播间未开播"}`)); err != nil {
				logger.Printf("关闭WebSocket连接失败: %v", err)
			}
		}
	}
}

func (c *WsHandler) OnClose(socket *gws.Conn, err error) {
	clientID := socket.RemoteAddr().String()
	logger.Printf("客户端 %s 断开连接，房间: %s", clientID, c.RoomID)

	// 获取房间组
	if group, ok := roomGroups.Load(c.RoomID); ok {
		roomGroup := group.(*RoomGroup)
		roomGroup.connections.Delete(clientID) // 从房间组中删除客户端连接

		logger.Printf("房间 %s 当前连接数: %d", c.RoomID, roomGroup.connections.Len())

		// 如果房间组为空，关闭抖音直播实例并删除房间组
		if roomGroup.connections.Len() == 0 {
			if roomGroup.douyinLive != nil {
				roomGroup.douyinLive.Close() // 停止抖音直播实例
			}
			roomGroups.Delete(c.RoomID)
			logger.Printf("房间 %s 已清空并关闭", c.RoomID)
		}
	}
}

// OnMessage 收到客户端消息
func (c *WsHandler) OnMessage(socket *gws.Conn, message *gws.Message) {

	defer message.Close()
	clientID := socket.RemoteAddr().String()
	msgStr := string(message.Bytes())

	logger.Printf("收到来自客户端 %s (房间: %s) 的消息: %s", clientID, c.RoomID, msgStr)

	// 如果是ping消息，回复pong
	if msgStr == "ping" {
		if err := socket.WriteString("pong"); err != nil {
			logger.Printf("写入消息失败: %v\n", err)
		}
		return
	}

	// 其他消息处理逻辑可以在这里添加
}
