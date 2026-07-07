package douyinLive

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

// buildHeartbeatFrame 构造网页端同款 PushFrame 心跳包。
// buildHeartbeatFrame builds the browser-compatible PushFrame heartbeat.
func buildHeartbeatFrame() ([]byte, error) {
	return proto.Marshal(&new_douyin.Webcast_Im_PushFrame{
		PayloadType: "hb",
	})
}

// applyWebsocketResponseState 保存 im/fetch 或 WS 响应下发的游标、内部扩展和动态推送地址。
// applyWebsocketResponseState stores cursor, internal extension, and dynamic push URL from im/fetch or WS responses.
// 参数/Parameters:
//   - response: 已解码的抖音 IM Response protobuf。 Decoded Douyin IM Response protobuf.
func (dl *DouyinLive) applyWebsocketResponseState(response *new_douyin.Webcast_Im_Response) {
	if response == nil {
		return
	}
	dl.mu.Lock()
	defer dl.mu.Unlock()
	if response.Cursor != "" {
		dl.wsCursor = response.Cursor
	}
	if response.InternalExt != "" {
		dl.wsInternalExt = response.InternalExt
	}
	if pushURL := websocketPushURLFromResponse(response); pushURL != "" {
		dl.wsPushURL = pushURL
	}
	if response.HeartbeatDuration > 0 {
		dl.heartbeatEvery = time.Duration(response.HeartbeatDuration) * time.Second
	}
}

// websocketStateSnapshot 返回当前 WS 拼接所需状态的线程安全快照。
// websocketStateSnapshot returns a thread-safe snapshot of state needed to build the WS URL.
func (dl *DouyinLive) websocketStateSnapshot() (cursor string, internalExt string, pushURL string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.wsCursor, dl.wsInternalExt, dl.wsPushURL
}

// currentHeartbeatInterval 返回当前心跳间隔，并保证不小于默认浏览器节奏。
// currentHeartbeatInterval returns the current heartbeat interval, clamped to the browser default cadence.
func (dl *DouyinLive) currentHeartbeatInterval() time.Duration {
	dl.mu.Lock()
	heartbeatEvery := dl.heartbeatEvery
	dl.mu.Unlock()
	if heartbeatEvery < heartbeatInterval {
		return heartbeatInterval
	}
	return heartbeatEvery
}

// sendHeartbeat 发送抖音网页端使用的应用层 PushFrame 心跳。
// sendHeartbeat sends the application-level PushFrame heartbeat used by Douyin web.
func (dl *DouyinLive) sendHeartbeat() error {
	data, err := buildHeartbeatFrame()
	if err != nil {
		return err
	}
	if err := dl.writeBinaryMessage(data); err != nil {
		return err
	}
	return dl.refreshCurrentReadDeadline()
}

// startHeartbeatLoop 启动 PushFrame 心跳循环。
// startHeartbeatLoop starts the PushFrame heartbeat loop.
func (dl *DouyinLive) startHeartbeatLoop() {
	dl.stopHeartbeatLoop()

	stopCh := make(chan struct{})
	doneCh := make(chan struct{})

	dl.mu.Lock()
	dl.heartbeatStopCh = stopCh
	dl.heartbeatDoneCh = doneCh
	dl.liveStatusGuard.Reset()
	dl.mu.Unlock()

	go func() {
		defer close(doneCh)

		sendHeartbeatOrStop := func() bool {
			if dl.isManualClose() || !dl.isLiveStatus() {
				return false
			}
			if err := dl.sendHeartbeat(); err != nil {
				dl.logger.Warn("发送保活心跳失败", "live_id", dl.liveID, "err", err)
				dl.closeCurrentConnection(websocket.CloseGoingAway, "heartbeat failed")
				return false
			}
			return true
		}

		if !sendHeartbeatOrStop() {
			return
		}

		heartbeatTimer := time.NewTimer(dl.currentHeartbeatInterval())
		defer heartbeatTimer.Stop()

		for {
			select {
			case <-heartbeatTimer.C:
				if !sendHeartbeatOrStop() {
					return
				}
				heartbeatTimer.Reset(dl.currentHeartbeatInterval())
			case <-stopCh:
				return
			}
		}
	}()
}

// stopHeartbeatLoop 停止心跳循环并等待 goroutine 退出。
// stopHeartbeatLoop stops the heartbeat loop and waits for its goroutine to exit.
func (dl *DouyinLive) stopHeartbeatLoop() {
	dl.mu.Lock()
	stopCh := dl.heartbeatStopCh
	doneCh := dl.heartbeatDoneCh
	dl.heartbeatStopCh = nil
	dl.heartbeatDoneCh = nil
	dl.mu.Unlock()

	if stopCh != nil {
		select {
		case <-stopCh:
		default:
			close(stopCh)
		}
	}
	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(1500 * time.Millisecond):
			dl.logger.Warn("等待心跳循环退出超时，跳过阻塞等待", "live_id", dl.liveID)
		}
	}
}

// sendAck 向上游发送 PushFrame ACK。
// sendAck sends a PushFrame ACK to the upstream server.
// 参数/Parameters:
//   - logID: 上游 PushFrame 的日志 ID。 Log ID from the upstream PushFrame.
//   - internalExt: 上游响应要求回传的内部扩展字段。 Internal extension value required by the upstream response.
func (dl *DouyinLive) sendAck(logID uint64, internalExt string) {
	ackFrame := &new_douyin.Webcast_Im_PushFrame{
		LogID:       logID,
		PayloadType: "ack",
		Payload:     []byte(internalExt),
	}

	data, err := proto.Marshal(ackFrame)
	if err != nil {
		dl.logger.Warn("ACK 序列化失败", "live_id", dl.liveID, "err", err)
		return
	}

	dl.mu.Lock()
	conn := dl.conn
	dl.mu.Unlock()

	if conn != nil {
		if err := dl.writeBinaryMessage(data); err != nil {
			dl.logger.Warn("发送 ACK 失败", "live_id", dl.liveID, "log_id", logID, "err", err)
		}
	}
}
