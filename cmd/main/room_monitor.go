package main

import (
	"errors"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
)

// startMonitorLoop 启动未开播轮询，并在开播后切换到直播监听。
// startMonitorLoop starts offline polling and switches to live listening once the room starts.
func (r *Room) startMonitorLoop() {
	r.mu.Lock()
	if r.closed || r.monitorStopCh != nil || r.douyinLive != nil {
		r.mu.Unlock()
		return
	}
	stopCh := make(chan struct{})
	doneCh := make(chan struct{})
	var finishOnce sync.Once
	finish := func() {
		finishOnce.Do(func() {
			r.mu.Lock()
			if r.monitorStopCh == stopCh {
				r.monitorStopCh = nil
				r.monitorDoneCh = nil
				r.monitorStopRequested = false
			}
			r.mu.Unlock()
			close(doneCh)
		})
	}
	r.monitorStopCh = stopCh
	r.monitorDoneCh = doneCh
	r.monitorStopRequested = false
	pollInterval := r.pollInterval
	notifyInterval := r.notifyInterval
	r.mu.Unlock()

	monitorTask := func() {
		defer finish()
		defer func() {
			r.removeIfIdle()
		}()

		pollTicker := time.NewTicker(pollInterval)
		defer pollTicker.Stop()
		notifyTicker := time.NewTicker(notifyInterval)
		defer notifyTicker.Stop()
		lifecycleCtx := r.lifecycleCtx

		for {
			select {
			case <-lifecycleCtx.Done():
				return
			case <-stopCh:
				return
			case <-notifyTicker.C:
				if r.clientCount() == 0 {
					return
				}
				r.notifyMonitorStatus()
			case <-pollTicker.C:
				if r.clientCount() == 0 {
					return
				}

				r.mu.Lock()
				if r.closed || r.douyinLive != nil {
					r.mu.Unlock()
					return
				}
				if r.starting {
					r.mu.Unlock()
					continue
				}
				r.starting = true
				r.mu.Unlock()

				err := r.startLiveSession()

				r.mu.Lock()
				r.starting = false
				r.mu.Unlock()

				switch {
				case err == nil:
					return
				case errors.Is(err, errRoomInactive):
					return
				case errors.Is(err, douyinLive.ErrRoomNotFound):
					if r.hasKnownValidRoom() {
						r.logger.Warn("已确认过的直播间本次查询暂时不可用，保留客户端并继续轮询", "room_id", r.id, "err", err)
						continue
					}
					r.logger.Warn("轮询发现直播间不存在，关闭客户端连接", "room_id", r.id, "err", err)
					r.closeAllClients(invalidRoomClientClose)
					return
				case errors.Is(err, douyinLive.ErrLiveNotStarted):
					if r.setStatusUnknown(false) {
						r.notifyOfflineStatus()
					}
					r.logger.Debug("房间仍未开播，继续等待", "room_id", r.id)
				case errors.Is(err, douyinLive.ErrLiveStatusUnknown):
					changed := r.setStatusUnknown(true)
					r.logger.Warn("直播状态暂时无法确认，继续轮询", "room_id", r.id, "err", err)
					if changed {
						r.notifyStatusUnknown()
					}
				default:
					r.logger.Warn("检查直播状态失败，将继续轮询", "room_id", r.id, "err", err)
				}
			}
		}
	}
	if !r.startTask(monitorTask) {
		finish()
	}
}

// stopMonitorLoop 停止未开播轮询并等待后台 goroutine 退出。
// stopMonitorLoop stops offline polling and waits for the background goroutine to exit.
func (r *Room) stopMonitorLoop() {
	r.mu.Lock()
	stopCh := r.monitorStopCh
	doneCh := r.monitorDoneCh
	if stopCh != nil && !r.monitorStopRequested {
		close(stopCh)
		r.monitorStopRequested = true
	}
	r.mu.Unlock()

	if doneCh != nil {
		select {
		case <-doneCh:
		case <-time.After(1500 * time.Millisecond):
			r.logger.Warn("等待监控循环退出超时，跳过阻塞等待", "room_id", r.id)
		}
	}
}
