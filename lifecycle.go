package douyinLive

import (
	"context"
	"errors"
	"fmt"

	"github.com/gorilla/websocket"
)

type listenerLifecycleState uint8

const (
	listenerLifecycleNew listenerLifecycleState = iota
	listenerLifecycleRunning
	listenerLifecycleClosing
	listenerLifecycleClosed
)

func (dl *DouyinLive) beginStart() error {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	switch dl.lifecycleState {
	case listenerLifecycleNew:
		if dl.manualClose || dl.closeSignalClosed {
			dl.lifecycleState = listenerLifecycleClosed
			return fmt.Errorf("%w: %w", ErrDouyinLiveClosed, context.Canceled)
		}
		dl.lifecycleState = listenerLifecycleRunning
		dl.manualClose = false
		return nil
	case listenerLifecycleRunning, listenerLifecycleClosing:
		return ErrDouyinLiveAlreadyStarted
	default:
		return fmt.Errorf("%w: %w", ErrDouyinLiveClosed, context.Canceled)
	}
}

func (dl *DouyinLive) beginClose() {
	dl.mu.Lock()
	dl.manualClose = true
	switch dl.lifecycleState {
	case listenerLifecycleNew:
		dl.lifecycleState = listenerLifecycleClosed
	case listenerLifecycleRunning:
		dl.lifecycleState = listenerLifecycleClosing
	}
	dl.mu.Unlock()
}

func (dl *DouyinLive) finishLifecycle() {
	dl.mu.Lock()
	dl.lifecycleState = listenerLifecycleClosed
	dl.mu.Unlock()
}

func (dl *DouyinLive) ensureUsable() error {
	if dl == nil {
		return ErrDouyinLiveClosed
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if dl.lifecycleState == listenerLifecycleClosing || dl.lifecycleState == listenerLifecycleClosed {
		return ErrDouyinLiveClosed
	}
	return nil
}

// Close permanently stops this listener and releases its current connection.
// Close 永久停止当前监听实例并释放连接；关闭后的实例不能再次 Start。
func (dl *DouyinLive) Close() {
	if dl == nil {
		return
	}
	dl.beginClose()
	dl.setLiveStatus(false)
	dl.signalClose()
	dl.stopHeartbeatLoop()
	dl.closeCurrentConnection(websocket.CloseNormalClosure, "closing connection")
}

// Dispose permanently closes the instance and releases all session resources.
// Dispose 永久关闭实例并释放全部会话资源，可安全重复调用。
func (dl *DouyinLive) Dispose() {
	if dl == nil {
		return
	}
	dl.Close()
	dl.contextMu.Lock()
	defer dl.contextMu.Unlock()
	dl.releaseResources()
}

// releaseResources 幂等释放缓存、HTTP 空闲连接和会话级签名 Runtime。
// releaseResources idempotently releases cache, idle HTTP connections, and the session signer runtime.
func (dl *DouyinLive) releaseResources() {
	dl.releaseOnce.Do(func() {
		dl.sessionProfile.close()
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

// isManualClose 返回当前是否处于主动关闭流程。
// isManualClose reports whether the listener is in a manual close flow.
func (dl *DouyinLive) isManualClose() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.manualClose
}

// Start starts this single-use listener and blocks while processing WebSocket messages until it ends.
// Start 启动一次性监听实例并阻塞处理 WebSocket 消息；同一实例不能并发或重复 Start。
func (dl *DouyinLive) Start() error {
	if dl == nil {
		return ErrDouyinLiveClosed
	}
	if err := dl.beginStart(); err != nil {
		return err
	}
	dl.resetCloseSignal()
	defer dl.cleanup()
	dl.logger.Info("开始连接抖音直播间", logFlowArgs("startup", "start_room", "live_id", dl.liveID)...)
	if dl.isKnownOfflineStatus() {
		return ErrLiveNotStarted
	}
	if err := dl.startWebSocket(); err != nil {
		if errors.Is(err, ErrLiveNotStarted) {
			return ErrLiveNotStarted
		}
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
	defer dl.finishLifecycle()
	dl.stopHeartbeatLoop()
	dl.contextMu.Lock()
	dl.contextPrepared = false
	dl.contextMu.Unlock()

	dl.mu.Lock()
	conn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	if conn != nil {
		_ = conn.Close()
	}
	dl.releaseResources()
	dl.logger.Info("抖音直播连接资源已释放", "live_id", dl.liveID)
}
