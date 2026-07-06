package main

import "github.com/lxzan/gws"

// WsHandler 实现 gws.EventInterfaces，并把连接事件转交给 Room。
// WsHandler implements gws.EventInterfaces and delegates connection events to a Room.
type WsHandler struct {
	gws.BuiltinEventHandler
	room *Room // room 关联当前 WebSocket 连接所在的房间。 room links the handler to its room.
}

// NewWsHandler 创建 WebSocket 事件处理器。
// NewWsHandler creates a WebSocket event handler.
func NewWsHandler(room *Room) *WsHandler {
	return &WsHandler{room: room}
}

// OnOpen 在新连接建立时将客户端加入房间。
// OnOpen adds a client to the room when a new connection is established.
func (c *WsHandler) OnOpen(socket *gws.Conn) {
	c.room.AddClient(socket)
}

// OnClose 在连接关闭时将客户端从房间移除。
// OnClose removes a client from the room when the connection closes.
func (c *WsHandler) OnClose(socket *gws.Conn, err error) {
	c.room.RemoveClient(socket.RemoteAddr().String())
}

// OnPing 按 WebSocket 规范使用相同 payload 回复 pong。
// OnPing replies with a pong carrying the same payload as required by WebSocket semantics.
func (c *WsHandler) OnPing(socket *gws.Conn, payload []byte) {
	_ = socket.WritePong(payload)
}

// OnMessage 处理客户端文本消息，目前仅响应 ping。
// OnMessage handles client text messages and currently responds to ping.
func (c *WsHandler) OnMessage(socket *gws.Conn, message *gws.Message) {
	defer message.Close()
	if message.Data.String() == "ping" {
		c.room.sendToClient(socket.RemoteAddr().String(), gws.OpcodeText, pongMessage)
	}
}
