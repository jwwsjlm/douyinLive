package main

import (
	"github.com/jwwsjlm/douyinLive"
	"github.com/lxzan/gws"
)

// WsHandler 实现 gws 的 Event 接口，添加房间ID信息
type WsHandler struct {
	gws.BuiltinEventHandler
	RoomID   string                                // 存储当前连接的房间ID
	sessions *gws.ConcurrentMap[string, *gws.Conn] // 使用内置的ConcurrentMap存储连接, 可以减少锁冲突
	//roomGroups sync.Map                              // 存储房间分组，键为房间ID，值为连接集合和抖音直播实例
}

// RoomGroup 房间组结构
type RoomGroup struct {
	connections *gws.ConcurrentMap[string, *gws.Conn] // 客户端ID -> 连接对象
	douyinLive  *douyinLive.DouyinLive
}
