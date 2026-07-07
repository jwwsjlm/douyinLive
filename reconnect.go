package douyinLive

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/codeGROOVE-dev/retry"
	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinLive/v2/utils"
)

// reconnectPlan 根据失败原因和次数计算重连延迟及刷新策略。
// reconnectPlan computes reconnect delay and refresh strategy from failure reason and count.
func (dl *DouyinLive) reconnectPlan(reason string, failureCount int, baseDelay time.Duration, allowUARefresh bool) (delay time.Duration, changeUA bool, rebuildHTTP bool) {
	if dl.hasConfiguredCookie() {
		allowUARefresh = false
	}

	// 指数退避：delay = baseDelay * 2^(failureCount-1)
	// 失败次数越多，等待时间越长，避免频繁重试触发风控
	delay = baseDelay
	if failureCount > 1 {
		expDelay := baseDelay * (1 << (failureCount - 1))
		if expDelay > maxReconnectDelay {
			delay = maxReconnectDelay
		} else {
			delay = expDelay
		}
	}

	changeUA = false
	rebuildHTTP = false

	switch {
	case failureCount <= 1:
		changeUA = false
		rebuildHTTP = false
	case failureCount <= 3:
		changeUA = allowUARefresh
		rebuildHTTP = false
	default:
		changeUA = allowUARefresh
		rebuildHTTP = true
	}

	switch reason {
	case "try_again_later_1013":
		delay = max(delay, 5*time.Second)
		changeUA = true
	case "service_restart_1012":
		delay = max(delay, 3*time.Second)
	case "going_away_1001":
		delay = max(delay, 2*time.Second)
	}

	if !allowUARefresh {
		changeUA = false
	}

	return delay, changeUA, rebuildHTTP
}

// max 是 time.Duration 版本的 max 函数
// max 返回两个 duration 中较大的一个。
// max returns the larger of two durations.
func max(a, b time.Duration) time.Duration {
	if a > b {
		return a
	}
	return b
}

// reconnectDecision 描述重连策略
// reconnectDecision 将读错误转换为重连决策。
// reconnectDecision converts a read error into a reconnect decision.
func (dl *DouyinLive) reconnectDecision(err error) (reason string, shouldRetry bool, delay time.Duration, allowUARefresh bool) {
	if dl.isManualClose() {
		return "manual_close", false, 0, false
	}

	if errors.Is(err, websocket.ErrCloseSent) {
		return "close_sent", false, 0, false
	}

	var closeErr *websocket.CloseError
	if errors.As(err, &closeErr) {
		switch closeErr.Code {
		case websocket.CloseNormalClosure:
			return "normal_close", false, 0, false
		case websocket.CloseAbnormalClosure:
			return "abnormal_close_1006", true, baseReconnectDelay, true
		case websocket.CloseTryAgainLater:
			return "try_again_later_1013", true, 5 * time.Second, true
		case websocket.CloseServiceRestart:
			return "service_restart_1012", true, 3 * time.Second, false
		case websocket.CloseGoingAway:
			return "going_away_1001", true, 2 * time.Second, false
		case websocket.ClosePolicyViolation:
			return "policy_violation_1008", false, 0, false
		case websocket.CloseInvalidFramePayloadData:
			return "invalid_frame_payload_1007", false, 0, false
		default:
			return fmt.Sprintf("close_code_%d", closeErr.Code), true, baseReconnectDelay, true
		}
	}

	return "network_or_unknown", true, baseReconnectDelay, true
}

// handleReadError 判断读错误是否需要重连。
// handleReadError 判断读错误是否需要重连并执行重连流程。
// handleReadError decides whether a read error should reconnect and runs the reconnect flow.
func (dl *DouyinLive) handleReadError(err error) bool {
	// 如果是手动关闭，不进行重连
	if dl.isManualClose() {
		dl.logger.Info("连接被手动关闭，不进行重连", "live_id", dl.liveID)
		return false
	}
	if !dl.isLiveStatus() {
		dl.logger.Info("直播状态已结束，不进行重连", "live_id", dl.liveID)
		return false
	}

	isLive, statusErr := dl.fetchLiveStatusFromAPI()

	if statusErr != nil {
		dl.logger.Warn("WS 读错后 HTTP 兜底检测失败，继续按重连流程处理", "live_id", dl.liveID, "err", statusErr)
	} else if dl.shouldCloseAfterStatusCheck(isLive) {
		dl.logger.Info("WS 读错后 HTTP 兜底确认直播已下播，不再重连", "live_id", dl.liveID, "live_name", dl.GetName())
		return false
	} else if !isLive {
		dl.logger.Warn("WS 读错后 HTTP 兜底检测到一次未开播状态，继续重连等待二次确认", "live_id", dl.liveID, "live_name", dl.GetName())
	}

	reason, shouldRetry, baseDelay, allowUARefresh := dl.reconnectDecision(err)
	if !shouldRetry {
		dl.logger.Info("连接关闭且不重连", "live_id", dl.liveID, "reason", reason, "err", err)
		return false
	}

	failureCount := dl.recordReconnectFailure(reason)
	delay, changeUA, rebuildHTTP := dl.reconnectPlan(reason, failureCount, baseDelay, allowUARefresh)
	jitter := time.Duration(utils.GenerateJitterNanos(maxReconnectJitter))
	sleepFor := delay + jitter
	dl.logger.Warn("检测到需重连，将稍后尝试", "live_id", dl.liveID, "reason", reason, "failures", failureCount, "delay", sleepFor, "change_ua", changeUA, "rebuild_http", rebuildHTTP, "err", err)
	if !dl.waitForReconnectDelay(sleepFor) {
		dl.logger.Info("重连等待被关闭信号打断", "live_id", dl.liveID, "reason", reason)
		return false
	}

	return dl.reconnect(defaultMaxRetries, changeUA, rebuildHTTP)
}

// reconnect 按退避策略重建上游 WebSocket 连接。
// reconnect 按退避策略重建上游 WebSocket 连接。
// reconnect rebuilds the upstream WebSocket connection with backoff.
func (dl *DouyinLive) reconnect(attempts int, changeUA bool, rebuildHTTP bool) bool {
	// 如果是手动关闭，不进行重连
	if dl.isManualClose() {
		dl.logger.Info("连接被手动关闭，不进行重连", "live_id", dl.liveID)
		return false
	}

	dl.mu.Lock()
	oldConn := dl.conn
	dl.conn = nil
	dl.mu.Unlock()

	dl.stopHeartbeatLoop()

	if oldConn != nil {
		msg := websocket.FormatCloseMessage(websocket.CloseGoingAway, "reconnecting")
		_ = oldConn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(3*time.Second))
		_ = oldConn.Close()
	}

	attemptIndex := 0
	retryable := func() error {
		attemptChangeUA := changeUA && attemptIndex > 0
		attemptRebuildHTTP := rebuildHTTP || attemptIndex >= 2

		url, headers, err := dl.reconnectDialContext(attemptChangeUA, attemptRebuildHTTP)
		if err != nil {
			attemptIndex++
			return err
		}

		dialer := *websocket.DefaultDialer
		dialer.HandshakeTimeout = websocketConnectTimeout
		ctx, cancel := dl.requestContext()
		defer cancel()
		conn, _, err := dialer.DialContext(ctx, url, headers)
		if err != nil {
			attemptIndex++
			if websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseInvalidFramePayloadData,
			) {
				return retry.Unrecoverable(err)
			}
			return err
		}

		dl.mu.Lock()
		dl.conn = conn
		dl.mu.Unlock()
		dl.configureWebSocket(conn)
		dl.startHeartbeatLoop()
		dl.resetReconnectTracking()
		return nil
	}

	retryCtx, cancelRetry := contextWithCloseSignal(dl.closeSignal())
	defer cancelRetry()

	err := retry.Do(
		retryable,
		retry.Attempts(uint(attempts)),
		retry.Context(retryCtx),
		retry.DelayType(retry.BackOffDelay),
		retry.MaxJitter(maxReconnectJitter),
		retry.RetryIf(func(err error) bool {
			return !websocket.IsCloseError(err,
				websocket.ClosePolicyViolation,
				websocket.CloseInvalidFramePayloadData,
			)
		}),
		retry.OnRetry(func(n uint, err error) {
			nextAttempt := n + 2
			dl.logger.Warn("重试连接失败", "live_id", dl.liveID, "attempt", n+1, "next_attempt", nextAttempt, "change_ua", changeUA && nextAttempt > 1, "rebuild_http", rebuildHTTP || nextAttempt >= 3, "err", err)
		}),
	)
	if err != nil {
		if dl.isManualClose() || errors.Is(err, context.Canceled) {
			dl.logger.Info("重连已取消", "live_id", dl.liveID)
			return false
		}
		dl.logger.Error("连接最终失败", "live_id", dl.liveID, "err", err)
		return false
	}

	dl.logger.Info("重连成功", "live_id", dl.liveID, "live_name", dl.GetName())
	return true
}
