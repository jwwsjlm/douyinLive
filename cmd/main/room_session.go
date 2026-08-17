package main

import (
	"errors"
	"fmt"

	"github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinlive-proto/generated"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// startLiveSession 启动抖音直播监听和事件处理。
// startLiveSession creates DouyinLive, verifies live status, and starts upstream listening.
func (r *Room) startLiveSession() error {
	var (
		d   *douyinLive.DouyinLive
		err error
	)
	switch r.signProvider {
	case signProviderTikHub:
		d, err = douyinLive.NewDouyinLiveWithSlogAndTikHub(r.id, r.logger.base, r.cookie, r.tikHubKey)
	default:
		d, err = douyinLive.NewDouyinLiveWithSlog(r.id, r.logger.base, r.cookie)
	}
	if err != nil {
		return err
	}

	if err := d.PrepareWebSocketContext(); err != nil {
		if d.IsKnownOfflineStatus() {
			r.updateMetadataFromDouyinLive(d)
			r.markKnownValid()
			d.Dispose()
			return douyinLive.ErrLiveNotStarted
		}
		d.Dispose()
		if errors.Is(err, douyinLive.ErrRoomNotFound) {
			return err
		}
		return fmt.Errorf("初始化直播间 %s 连接上下文失败: %w", r.id, err)
	}
	if err := confirmLiveSessionStatus(d); err != nil {
		r.updateMetadataFromDouyinLive(d)
		if errors.Is(err, douyinLive.ErrLiveNotStarted) {
			r.markKnownValid()
		}
		d.Dispose()
		return err
	}
	r.updateMetadataFromDouyinLive(d)
	r.markKnownValid()

	r.mu.Lock()
	r.douyinLive = d
	r.upstreamReady = false
	r.mu.Unlock()

	d.SubscribeMessage(func(message *douyinLive.LiveMessage) {
		r.handleDouyinEvent(message)
	})

	if r.clientCount() == 0 {
		r.disposePendingLive(d)
		return errRoomInactive
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		r.disposePendingLive(d)
		return errRoomInactive
	}
	r.mu.Unlock()

	go r.runLiveSession(d)
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
func (r *Room) runLiveSession(d *douyinLive.DouyinLive) {
	readyCh := d.Ready()
	startErrCh := make(chan error, 1)
	go func() {
		startErrCh <- d.Start()
	}()

	connected := false
	select {
	case <-readyCh:
		connected = r.markUpstreamReady(d)
	case err := <-startErrCh:
		if err != nil {
			r.logger.Warn("直播监听运行结束", "room_id", r.id, "err", err)
		}
	}

	var startErr error
	if connected {
		startErr = <-startErrCh
	} else {
		select {
		case startErr = <-startErrCh:
		default:
		}
	}
	if startErr != nil {
		r.logger.Warn("直播监听运行结束", "room_id", r.id, "err", startErr)
	}

	r.mu.Lock()
	if r.douyinLive == d {
		r.douyinLive = nil
		r.upstreamReady = false
	}
	closed := r.closed
	monitorRunning := r.monitorStopCh != nil
	r.mu.Unlock()

	if closed || r.clientCount() == 0 {
		return
	}

	if connected {
		r.notifyOfflineEndedStatus()
	} else {
		r.notifyOfflineStatus()
	}
	if !monitorRunning {
		r.logger.Info("直播连接已结束，切回未开播监控模式", "room_id", r.id, "connected", connected)
		r.startMonitorLoop()
	}
}

// markUpstreamReady 在上游 WebSocket 握手成功后更新房间状态并通知客户端。
// markUpstreamReady updates room state and notifies clients after the upstream handshake succeeds.
func (r *Room) markUpstreamReady(d *douyinLive.DouyinLive) bool {
	r.mu.Lock()
	if r.closed || r.douyinLive != d {
		r.mu.Unlock()
		return false
	}
	r.upstreamReady = true
	r.mu.Unlock()

	r.logger.Info("上游 WebSocket 已就绪，开始推送直播消息", "room_id", r.id)
	if r.clientCount() > 0 {
		r.notifyOnlineStatus()
	}
	return true
}

// handleDouyinEvent 将抖音消息解析为 JSON 并广播给房间客户端。
// handleDouyinEvent converts a Douyin message to JSON and broadcasts it to room clients.
// 参数/Parameters:
//   - event: 上游抖音直播消息事件。 Upstream Douyin live message event.
func (r *Room) handleDouyinEvent(event *douyinLive.LiveMessage) {
	if r.clientCount() == 0 {
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

	r.Broadcast(finalJSON)
}
