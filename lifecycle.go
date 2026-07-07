package douyinLive

import (
	"context"

	"github.com/gorilla/websocket"
)

// Close 主动关闭直播监听并释放当前连接。
// Close actively stops live listening and releases the current connection.
func (dl *DouyinLive) Close() {
	dl.setManualClose(true)
	dl.setLiveStatus(false)
	dl.signalClose()
	dl.stopHeartbeatLoop()
	dl.closeCurrentConnection(websocket.CloseNormalClosure, "closing connection")
}

// Dispose 释放尚未进入 Start 流程的实例资源。
// Dispose releases resources for instances that will not enter Start.
func (dl *DouyinLive) Dispose() {
	dl.Close()
	dl.releaseCache()
}

// releaseCache 幂等释放房间信息缓存。
// releaseCache idempotently releases the room-info cache.
func (dl *DouyinLive) releaseCache() {
	dl.releaseOnce.Do(func() {
		if dl.ristretto != nil {
			dl.ristretto.Close()
		}
	})
}

// resetReconnectTracking 清空连续重连失败计数。
// resetReconnectTracking clears consecutive reconnect failure tracking.
func (dl *DouyinLive) resetReconnectTracking() {
	dl.mu.Lock()
	dl.consecutiveFailures = 0
	dl.mu.Unlock()
}

// recordReconnectFailure 记录一次重连失败并返回连续失败次数。
// recordReconnectFailure records one reconnect failure and returns the consecutive count.
// 参数/Parameters:
//   - reason: 归类后的重连失败原因。 Classified reconnect failure reason.
func (dl *DouyinLive) recordReconnectFailure(reason string) int {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.consecutiveFailures++
	return dl.consecutiveFailures
}

// setManualClose 标记连接是否由调用方主动关闭。
// setManualClose marks whether the connection is being closed by the caller.
// 参数/Parameters:
//   - status: true 表示主动关闭流程。 true means the caller is closing the listener.
func (dl *DouyinLive) setManualClose(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.manualClose = status
}

// isManualClose 返回当前是否处于主动关闭流程。
// isManualClose reports whether the listener is in a manual close flow.
func (dl *DouyinLive) isManualClose() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.manualClose
}

// Start 启动直播监听并阻塞处理 WebSocket 消息直到结束。
// Start starts live listening and blocks while processing WebSocket messages until it ends.
func (dl *DouyinLive) Start() error {
	if dl.isManualClose() {
		return context.Canceled
	}
	dl.resetCloseSignal()
	dl.setManualClose(false)
	defer dl.cleanup()
	dl.logger.Info("开始连接抖音直播间", logFlowArgs("startup", "start_room", "live_id", dl.liveID)...)

	dl.logger.Info("开始连接抖音直播间", "live_id", dl.liveID)
	if err := dl.startWebSocket(); err != nil {
		dl.logger.Warn("WebSocket 连接失败，准备重连", "live_id", dl.liveID, "err", err)
		if dl.reconnect(defaultMaxRetries, true, false) {
			dl.processMessages()
			return nil
		}
		return err
	}

	dl.processMessages()
	return nil
}

// cleanup 释放当前连接、心跳和缓存资源。
// cleanup releases the current connection, heartbeat loop, and cache resources.
func (dl *DouyinLive) cleanup() {
	dl.stopHeartbeatLoop()

	dl.mu.Lock()
	conn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	dl.releaseCache()
	dl.logger.Info("抖音直播连接资源已释放", "live_id", dl.liveID)
}
