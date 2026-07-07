package douyinLive

import (
	"time"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

func buildHeartbeatFrame() ([]byte, error) {
	return proto.Marshal(&new_douyin.Webcast_Im_PushFrame{
		PayloadType: "hb",
	})
}

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

func (dl *DouyinLive) websocketStateSnapshot() (cursor string, internalExt string, pushURL string) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.wsCursor, dl.wsInternalExt, dl.wsPushURL
}

func (dl *DouyinLive) currentHeartbeatInterval() time.Duration {
	dl.mu.Lock()
	heartbeatEvery := dl.heartbeatEvery
	dl.mu.Unlock()
	if heartbeatEvery < heartbeatInterval {
		return heartbeatInterval
	}
	return heartbeatEvery
}

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

// startHeartbeatLoop starts the PushFrame heartbeat and fallback HTTP status-check loop.
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

		heartbeatTimer := time.NewTimer(dl.currentHeartbeatInterval())
		defer heartbeatTimer.Stop()
		statusTicker := time.NewTicker(liveStatusPollInterval)
		defer statusTicker.Stop()

		for {
			select {
			case <-heartbeatTimer.C:
				if dl.isManualClose() || !dl.isLiveStatus() {
					return
				}
				if err := dl.sendHeartbeat(); err != nil {
					dl.logger.Warn("发送保活心跳失败", "live_id", dl.liveID, "err", err)
					dl.closeCurrentConnection(websocket.CloseGoingAway, "heartbeat failed")
					return
				}
				heartbeatTimer.Reset(dl.currentHeartbeatInterval())
			case <-statusTicker.C:
				if dl.isManualClose() || !dl.isLiveStatus() {
					return
				}
				isLive, err := dl.fetchLiveStatusFromAPI()
				if err != nil {
					dl.logger.Warn("HTTP 兜底检测直播状态失败", "live_id", dl.liveID, "err", err)
					continue
				}
				if dl.shouldCloseAfterStatusCheck(isLive) {
					dl.logger.Info("HTTP 兜底检测到直播已下播，关闭当前 WS 连接", "live_id", dl.liveID, "live_name", dl.GetName())
					dl.closeCurrentConnection(websocket.CloseNormalClosure, "live ended by api")
					return
				}
				if !isLive {
					dl.logger.Warn("HTTP 兜底检测到一次未开播状态，等待二次确认", "live_id", dl.liveID, "live_name", dl.GetName())
				}
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

// sendAck 发送 ACK 消息
// sendAck 向上游发送 PushFrame ACK。
// sendAck sends a PushFrame ACK to the upstream server.
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
