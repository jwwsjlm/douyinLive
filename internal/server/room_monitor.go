package server

import (
	"errors"
	"sync"
	"time"

	"github.com/jwwsjlm/douyinLive/v2"
)

// roomMonitorLoop binds one offline monitor to the room-session generation
// that created it. A retired loop can therefore neither commit state nor
// notify clients belonging to a replacement generation.
// roomMonitorLoop 将一次离线监控绑定到创建它的房间会话代次。
type roomMonitorLoop struct {
	generation     uint64
	stopCh         chan struct{}
	doneCh         chan struct{}
	pollInterval   time.Duration
	notifyInterval time.Duration
	stopOnce       sync.Once
	finishOnce     sync.Once
}

func newRoomMonitorLoop(generation uint64, pollInterval, notifyInterval time.Duration) *roomMonitorLoop {
	return &roomMonitorLoop{
		generation:     generation,
		stopCh:         make(chan struct{}),
		doneCh:         make(chan struct{}),
		pollInterval:   pollInterval,
		notifyInterval: notifyInterval,
	}
}

func (loop *roomMonitorLoop) stop() {
	if loop == nil {
		return
	}
	loop.stopOnce.Do(func() { close(loop.stopCh) })
}

func (loop *roomMonitorLoop) finish(r *Room) {
	if loop == nil {
		return
	}
	loop.finishOnce.Do(func() {
		r.mu.Lock()
		if r.monitor == loop {
			r.monitor = nil
		}
		r.mu.Unlock()
		close(loop.doneCh)
	})
}

// installMonitorLoopLocked installs a monitor for generation. The caller must
// hold r.mu. The returned boolean reports whether a new loop was installed.
// installMonitorLoopLocked 为指定代次安装监控；调用方必须持有 r.mu。
func (r *Room) installMonitorLoopLocked(sessionGeneration uint64) (*roomMonitorLoop, bool) {
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != nil {
		return nil, false
	}
	if r.monitor != nil {
		if r.monitor.generation != sessionGeneration {
			return nil, false
		}
		return r.monitor, false
	}
	loop := newRoomMonitorLoop(sessionGeneration, r.pollInterval, r.notifyInterval)
	r.monitor = loop
	return loop, true
}

// transitionToMonitorForGeneration atomically commits an offline/unknown
// startup result, installs its monitor, and captures only clients belonging to
// that generation.
// transitionToMonitorForGeneration 原子提交离线或未知状态、安装监控并取得该代次客户端快照。
func (r *Room) transitionToMonitorForGeneration(sessionGeneration uint64, unknown bool) (*roomMonitorLoop, []*Client, bool, bool) {
	r.mu.Lock()
	if r.closed || r.sessionGeneration != sessionGeneration || r.douyinLive != nil {
		r.mu.Unlock()
		return nil, nil, false, false
	}
	r.starting = false
	r.statusUnknown = unknown
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
	loop, installed := r.installMonitorLoopLocked(sessionGeneration)
	if loop == nil {
		r.mu.Unlock()
		return nil, nil, false, false
	}
	r.mu.Unlock()
	return loop, clients, installed, true
}

// launchMonitorLoop starts an already installed generation-bound monitor.
// launchMonitorLoop 启动已经安装且绑定代次的监控循环。
func (r *Room) launchMonitorLoop(loop *roomMonitorLoop) bool {
	if loop == nil {
		return false
	}
	if !r.startTaskForGeneration(loop.generation, func() { r.runMonitorLoop(loop) }) {
		loop.finish(r)
		return false
	}
	return true
}

func (r *Room) snapshotClientsForMonitor(loop *roomMonitorLoop) ([]*Client, bool, bool) {
	r.mu.Lock()
	if loop == nil || r.closed || r.sessionGeneration != loop.generation || r.monitor != loop {
		r.mu.Unlock()
		return nil, false, false
	}
	unknown := r.statusUnknown
	r.clientsMu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clientsMu.RUnlock()
	r.mu.Unlock()
	return clients, unknown, true
}

func (r *Room) beginMonitorProbe(loop *roomMonitorLoop) (bool, bool) {
	r.mu.Lock()
	if loop == nil || r.closed || r.sessionGeneration != loop.generation || r.monitor != loop || r.douyinLive != nil {
		r.mu.Unlock()
		return false, false
	}
	r.clientsMu.RLock()
	hasClients := len(r.clients) > 0
	r.clientsMu.RUnlock()
	if !hasClients {
		r.mu.Unlock()
		return false, false
	}
	if r.starting {
		r.mu.Unlock()
		return false, true
	}
	r.starting = true
	r.mu.Unlock()
	return true, true
}

func (r *Room) completeMonitorProbe(loop *roomMonitorLoop) (bool, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if loop == nil || r.closed || r.sessionGeneration != loop.generation || r.monitor != loop {
		return false, false
	}
	r.starting = false
	return r.knownValid, true
}

func (r *Room) commitMonitorStatus(loop *roomMonitorLoop, unknown bool) ([]*Client, bool, bool) {
	r.mu.Lock()
	if loop == nil || r.closed || r.sessionGeneration != loop.generation || r.monitor != loop {
		r.mu.Unlock()
		return nil, false, false
	}
	r.starting = false
	changed := r.statusUnknown != unknown
	r.statusUnknown = unknown
	r.clientsMu.RLock()
	clients := make([]*Client, 0, len(r.clients))
	for _, client := range r.clients {
		clients = append(clients, client)
	}
	r.clientsMu.RUnlock()
	r.mu.Unlock()
	return clients, changed, true
}

// runMonitorLoop polls an offline room and switches to live listening once it
// starts. Every state change and notification is conditional on loop identity.
// runMonitorLoop 轮询未开播房间；所有状态提交和通知都校验监控实例及代次。
func (r *Room) runMonitorLoop(loop *roomMonitorLoop) {
	defer loop.finish(r)

	pollTicker := time.NewTicker(loop.pollInterval)
	defer pollTicker.Stop()
	notifyTicker := time.NewTicker(loop.notifyInterval)
	defer notifyTicker.Stop()
	lifecycleCtx := r.lifecycleCtx

	for {
		select {
		case <-lifecycleCtx.Done():
			return
		case <-loop.stopCh:
			return
		case <-notifyTicker.C:
			clients, unknown, ok := r.snapshotClientsForMonitor(loop)
			if !ok || len(clients) == 0 {
				return
			}
			if unknown {
				r.broadcastToClients(clients, r.statusUnknownMessage())
			} else {
				r.broadcastToClients(clients, r.offlineStatusMessage())
			}
		case <-pollTicker.C:
			start, keepRunning := r.beginMonitorProbe(loop)
			if !keepRunning {
				return
			}
			if !start {
				continue
			}

			err := r.startLiveSession(loop.generation)
			switch {
			case err == nil:
				return
			case errors.Is(err, errRoomInactive):
				r.completeMonitorProbe(loop)
				return
			case errors.Is(err, douyinLive.ErrRoomNotFound):
				knownValid, current := r.completeMonitorProbe(loop)
				if !current {
					return
				}
				if knownValid {
					r.logger.Warn("已确认过的直播间本次查询暂时不可用，保留客户端并继续轮询", "room_id", r.id, "err", err)
					continue
				}
				r.logger.Warn("轮询发现直播间不存在，关闭客户端连接", "room_id", r.id, "err", err)
				r.closeClientsForFailedGeneration(loop.generation, invalidRoomClientClose)
				return
			case errors.Is(err, douyinLive.ErrLiveNotStarted):
				clients, changed, current := r.commitMonitorStatus(loop, false)
				if !current {
					return
				}
				if changed {
					r.broadcastToClients(clients, r.offlineStatusMessage())
				}
				r.logger.Debug("房间仍未开播，继续等待", "room_id", r.id)
			case errors.Is(err, douyinLive.ErrLiveStatusUnknown):
				clients, changed, current := r.commitMonitorStatus(loop, true)
				if !current {
					return
				}
				r.logger.Warn("直播状态暂时无法确认，继续轮询", "room_id", r.id, "err", err)
				if changed {
					r.broadcastToClients(clients, r.statusUnknownMessage())
				}
			default:
				if _, current := r.completeMonitorProbe(loop); !current {
					return
				}
				r.logger.Warn("检查直播状态失败，将继续轮询", "room_id", r.id, "err", err)
			}
		}
	}
}

// stopMonitorLoop stops offline polling and waits for its goroutine to exit.
// stopMonitorLoop 停止未开播轮询并等待后台 goroutine 退出。
func (r *Room) stopMonitorLoop() {
	r.mu.Lock()
	loop := r.monitor
	if loop != nil {
		loop.stop()
	}
	r.mu.Unlock()

	if loop != nil {
		select {
		case <-loop.doneCh:
		case <-time.After(1500 * time.Millisecond):
			r.logger.Warn("等待监控循环退出超时，跳过阻塞等待", "room_id", r.id)
		}
	}
}
