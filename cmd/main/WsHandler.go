package main

import "github.com/lxzan/gws"

// WsHandler 实现 gws.EventInterfaces 接口
type WsHandler struct {
	gws.BuiltinEventHandler
	room *Room // 关联的房间实例
}

// NewWsHandler 创建一个新的 WebSocket 处理器
func NewWsHandler(room *Room) *WsHandler {
	return &WsHandler{room: room}
}

// OnOpen 当一个新连接建立时调用
func (c *WsHandler) OnOpen(socket *gws.Conn) {
	c.room.AddClient(socket)
}

// OnClose 当一个连接关闭时调用
func (c *WsHandler) OnClose(socket *gws.Conn, err error) {
	c.room.RemoveClient(socket.RemoteAddr().String())
}

// OnMessage 当收到消息时调用
func (c *WsHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	// 实现心跳检查
	if message.Data.String() == "ping" {
		_ = socket.WriteString("pong")
	}
}
