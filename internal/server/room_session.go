package server

import (
	"errors"
	"fmt"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinlive-proto/generated"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const anonymousProbeRotateFailures = 3

// acquireProbeLive 获取当前房间稳定复用的状态探测会话；首次调用时才创建。
// acquireProbeLive returns the room's stable reusable status-probe session, creating it lazily.
func (r *Room) acquireProbeLive(sessionGeneration uint64) (*douyinLive.DouyinLive, error) {
	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration {
		r.mu.Unlock()
		return nil, errRoomInactive
	}
	if r.probeLive != nil {
		d := r.probeLive
		r.mu.Unlock()
		return d, nil
	}
	r.mu.Unlock()

	d, err := douyinLive.NewDouyinLiveWithOptions(r.id, douyinLive.NewSlogLogger(r.logger.base), douyinLive.Options{
		Cookie: r.cookie, ProxyURL: r.proxyURL,
		SignProvider: r.signProvider, TikHubToken: r.tikHubKey,
		ProtocolMode: r.protocolMode,
	})
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration {
		r.mu.Unlock()
		d.Dispose()
		return nil, errRoomInactive
	}
	if r.probeLive == nil {
		r.probeLive = d
		r.probeFailures = 0
		r.mu.Unlock()
		return d, nil
	}
	existing := r.probeLive
	r.mu.Unlock()
	d.Dispose()
	return existing, nil
}

// resetProbeFailures 在探测得到有效在线或离线状态后清空连续失败次数。
// resetProbeFailures clears consecutive probe failures after a valid online or offline result.
func (r *Room) resetProbeFailures(d *douyinLive.DouyinLive) {
	r.mu.Lock()
	if r.probeLive == d {
		r.probeFailures = 0
	}
	r.mu.Unlock()
}

// recordProbeFailure 记录探测失败；匿名会话连续失败后才轮换画像，避免每次轮询都更换身份。
// recordProbeFailure records a probe failure and rotates anonymous identity only after repeated failures.
func (r *Room) recordProbeFailure(d *douyinLive.DouyinLive) {
	rotate := false
	failures := 0
	r.mu.Lock()
	if r.probeLive == d {
		r.probeFailures++
		failures = r.probeFailures
		if r.cookie == "" && r.probeFailures >= anonymousProbeRotateFailures {
			r.probeLive = nil
			r.probeFailures = 0
			rotate = true
		}
	}
	r.mu.Unlock()

	if rotate {
		r.logger.Info("匿名状态探测连续失败，下一轮将刷新浏览器画像", "room_id", r.id, "failures", failures)
		d.Dispose()
	}
}

// discardProbeLive 丢弃不能继续使用的探测会话。
// discardProbeLive discards a probe session that must not be reused.
func (r *Room) discardProbeLive(d *douyinLive.DouyinLive) {
	if d == nil {
		return
	}
	r.mu.Lock()
	if r.probeLive == d {
		r.probeLive = nil
		r.probeFailures = 0
	}
	r.mu.Unlock()
	d.Dispose()
}

// promoteProbeLive 将已确认在线的探测会话提升为正式上游直播会话。
// promoteProbeLive promotes a confirmed-online probe into the active upstream session.
func (r *Room) promoteProbeLive(d *douyinLive.DouyinLive, sessionGeneration uint64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.probeLive != d {
		return false
	}
	r.probeLive = nil
	r.probeFailures = 0
	r.douyinLive = d
	r.upstreamReady = false
	r.statusUnknown = false
	r.starting = false
	if r.monitor != nil && r.monitor.generation == sessionGeneration {
		r.monitor.stop()
		r.monitor = nil
	}
	return true
}

// startLiveSession 启动抖音直播监听和事件处理。
// startLiveSession creates DouyinLive, verifies live status, and starts upstream listening.
func (r *Room) startLiveSession(sessionGeneration uint64) error {
	d, err := r.acquireProbeLive(sessionGeneration)
	if err != nil {
		return err
	}

	if err := d.PrepareWebSocketContext(); err != nil {
		if d.IsKnownOfflineStatus() {
			if !r.commitProbeMetadataForGeneration(d, sessionGeneration, true) {
				return errRoomInactive
			}
			r.resetProbeFailures(d)
			return douyinLive.ErrLiveNotStarted
		}
		if errors.Is(err, douyinLive.ErrRoomNotFound) {
			r.discardProbeLive(d)
			return err
		}
		r.recordProbeFailure(d)
		return fmt.Errorf("初始化直播间 %s 连接上下文失败: %w", r.id, err)
	}
	if err := confirmLiveSessionStatus(d); err != nil {
		knownValid := errors.Is(err, douyinLive.ErrLiveNotStarted)
		if !r.commitProbeMetadataForGeneration(d, sessionGeneration, knownValid) {
			return errRoomInactive
		}
		if knownValid {
			r.resetProbeFailures(d)
		} else {
			r.recordProbeFailure(d)
		}
		return err
	}
	if !r.commitProbeMetadataForGeneration(d, sessionGeneration, true) {
		return errRoomInactive
	}
	if !r.promoteProbeLive(d, sessionGeneration) {
		r.discardProbeLive(d)
		return errRoomInactive
	}

	d.SubscribeMessage(func(message *douyinLive.LiveMessage) {
		r.handleDouyinEventForSession(d, sessionGeneration, message)
	})

	r.mu.Lock()
	r.clientsMu.RLock()
	clientCount := len(r.clients)
	r.clientsMu.RUnlock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != d || clientCount == 0 {
		r.mu.Unlock()
		r.disposePendingLive(d)
		return errRoomInactive
	}
	r.mu.Unlock()

	if !r.startTaskForGeneration(sessionGeneration, func() { r.runLiveSession(d, sessionGeneration) }) {
		r.disposePendingLive(d)
		return errRoomInactive
	}
	r.logger.Info("抖音直播监听后台任务已启动，等待上游 WebSocket 握手", "room_id", r.id)
	return nil
}

// liveSessionStatusChecker 描述启动在线会话前所需的直播状态检查能力。
// liveSessionStatusChecker describes the live-status checks required before starting an online session.
type liveSessionStatusChecker interface {
	IsKnownOfflineStatus() bool
	IsKnownLiveStatus() bool
	IsLive() (bool, error)
}

// confirmLiveSessionStatus 确保只有明确确认开播的房间才能进入在线会话。
// confirmLiveSessionStatus ensures only explicitly confirmed live rooms enter an online session.
func confirmLiveSessionStatus(checker liveSessionStatusChecker) error {
	if checker.IsKnownOfflineStatus() {
		return douyinLive.ErrLiveNotStarted
	}
	if checker.IsKnownLiveStatus() {
		return nil
	}

	isLive, err := checker.IsLive()
	if err != nil {
		return fmt.Errorf("确认直播状态失败: %w", err)
	}
	if !isLive {
		return douyinLive.ErrLiveNotStarted
	}
	return nil
}

// disposePendingLive 释放尚未被房间正式接管的 DouyinLive 实例。
// disposePendingLive disposes a DouyinLive instance that the room has not fully adopted.
// 参数/Parameters:
//   - d: 待释放的 DouyinLive 实例。 DouyinLive instance to dispose.
func (r *Room) disposePendingLive(d *douyinLive.DouyinLive) {
	r.mu.Lock()
	if r.douyinLive == d {
		r.douyinLive = nil
	}
	r.mu.Unlock()

	d.Dispose()
}

// runLiveSession 运行上游直播监听，并在结束后按需切回未开播监控。
// runLiveSession runs upstream live listening and switches back to offline monitoring when needed.
// 参数/Parameters:
//   - d: 已接管的上游 DouyinLive 实例。 Adopted upstream DouyinLive instance.
func (r *Room) runLiveSession(d *douyinLive.DouyinLive, sessionGeneration uint64) {
	readyCh := d.Ready()
	startErrCh := make(chan error, 1)
	if !r.startTaskForGeneration(sessionGeneration, func() {
		startErrCh <- d.Start()
	}) {
		d.Dispose()
		return
	}

	connected := false
	startFinished := false
	var startErr error
	select {
	case <-readyCh:
		connected = r.markUpstreamReady(d, sessionGeneration)
		if !connected {
			d.Close()
		}
	case startErr = <-startErrCh:
		startFinished = true
	}

	if !startFinished && connected {
		startErr = <-startErrCh
		startFinished = true
	} else if !startFinished {
		select {
		case startErr = <-startErrCh:
			startFinished = true
		case <-time.After(roomCloseTimeout):
			r.logger.Warn("等待已取消的直播监听退出超时", "room_id", r.id)
		}
	}
	if startFinished && startErr != nil {
		r.logger.Warn("直播监听运行结束", "room_id", r.id, "err", startErr)
	}

	monitor, clients, installed, currentGeneration := r.transitionLiveSessionToMonitor(d, sessionGeneration)
	if !currentGeneration {
		return
	}
	if len(clients) == 0 {
		r.closeBackgroundWorkersIfIdle()
		return
	}

	if connected {
		r.broadcastToClients(clients, r.offlineEndedStatusMessage())
	} else {
		r.broadcastToClients(clients, r.offlineStatusMessage())
	}
	if installed {
		r.logger.Info("直播连接已结束，切回未开播监控模式", "room_id", r.id, "connected", connected)
		r.launchMonitorLoop(monitor)
	}
}

// transitionLiveSessionToMonitor atomically retires one live session, installs
// its same-generation monitor, and captures only clients from that generation.
// transitionLiveSessionToMonitor 原子结束在线会话、安装同代次监控并取得该代次客户端快照。
func (r *Room) transitionLiveSessionToMonitor(d *douyinLive.DouyinLive, sessionGeneration uint64) (*roomMonitorLoop, []*Client, bool, bool) {
	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != d {
		r.mu.Unlock()
		return nil, nil, false, false
	}
	r.douyinLive = nil
	r.upstreamReady = false
	r.starting = false
	r.statusUnknown = false
	r.clientsMu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clientsMu.RUnlock()
	if len(clients) == 0 {
		r.mu.Unlock()
		return nil, clients, false, true
	}
	monitor, installed := r.installMonitorLoopLocked(sessionGeneration)
	if monitor == nil {
		r.mu.Unlock()
		return nil, nil, false, false
	}
	r.mu.Unlock()
	return monitor, clients, installed, true
}

// markUpstreamReady 在上游 WebSocket 握手成功后更新房间状态并通知客户端。
// markUpstreamReady updates room state and notifies clients after the upstream handshake succeeds.
func (r *Room) markUpstreamReady(d *douyinLive.DouyinLive, sessionGeneration uint64) bool {
	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != d {
		r.mu.Unlock()
		return false
	}
	r.upstreamReady = true
	r.statusUnknown = false
	r.clientsMu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clientsMu.RUnlock()
	r.mu.Unlock()

	r.logger.Info("上游 WebSocket 已就绪，开始推送直播消息", "room_id", r.id)
	r.broadcastToClients(clients, r.onlineStatusMessage())
	return true
}

// snapshotClientsForLiveSession captures clients only while d and generation
// still identify the active upstream session.
// snapshotClientsForLiveSession 仅在上游实例及代次仍有效时取得客户端快照。
func (r *Room) snapshotClientsForLiveSession(d *douyinLive.DouyinLive, sessionGeneration uint64) ([]*Client, bool) {
	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != d {
		r.mu.Unlock()
		return nil, false
	}
	r.clientsMu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clientsMu.RUnlock()
	r.mu.Unlock()
	return clients, true
}

func (r *Room) handleDouyinEventForSession(d *douyinLive.DouyinLive, sessionGeneration uint64, event *douyinLive.LiveMessage) {
	clients, ok := r.snapshotClientsForLiveSession(d, sessionGeneration)
	if !ok {
		return
	}
	r.handleDouyinEventForClients(clients, event)
}

func (r *Room) handleDouyinEventForClients(clients []*Client, event *douyinLive.LiveMessage) {
	if len(clients) == 0 {
		return
	}
	if event == nil || event.Raw == nil {
		return
	}

	eventData := event.Raw
	msg := event.Parsed
	var err error
	if msg == nil {
		msg, err = generated.GetMessageInstance(eventData.Method)
		if err != nil {
			if r.unknown {
				r.logger.Debug("未知消息类型", "room_id", r.id, "method", eventData.Method, "payload_len", len(eventData.Payload))
			}
			return
		}
		defer generated.PutMessageInstance(eventData.Method, msg)

		if err := proto.Unmarshal(eventData.Payload, msg); err != nil {
			r.logger.Warn("Protobuf 反序列化失败", "room_id", r.id, "method", eventData.Method, "err", err)
			return
		}
	}

	jsonBytes, err := protojson.Marshal(msg)
	if err != nil {
		r.logger.Warn("JSON 序列化失败", "room_id", r.id, "method", eventData.Method, "err", err)
		return
	}

	finalJSON, err := r.buildEventJSON(jsonBytes, eventData.Method, event.LiveName, event.Title, event.AvatarThumb)
	if err != nil {
		r.logger.Warn("事件 JSON 组装失败", "room_id", r.id, "method", eventData.Method, "err", err)
		return
	}

	r.broadcastToClients(clients, finalJSON)
}
