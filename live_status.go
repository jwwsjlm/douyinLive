package douyinLive

// liveStatusGuard 通过多次确认降低直播状态误判概率。
// liveStatusGuard reduces live-status false positives by requiring repeated confirmation.
type liveStatusGuard struct {
	offlineConfirmations int
}

// Record 记录一次直播状态检查结果，并返回是否确认下播。
// Record stores one live-status check result and reports whether offline is confirmed.
// 参数/Parameters:
//   - isLive: 本次检查得到的直播状态。 Live status returned by the current check.
func (g *liveStatusGuard) Record(isLive bool) bool {
	if isLive {
		g.offlineConfirmations = 0
		return false
	}
	g.offlineConfirmations++
	return g.offlineConfirmations >= 2
}

// Reset 清空直播状态确认计数。
// Reset clears the live-status confirmation counter.
func (g *liveStatusGuard) Reset() {
	g.offlineConfirmations = 0
}

// setLiveStatus 更新内部直播状态。
// setLiveStatus updates the internal live status.
// 参数/Parameters:
//   - status: true 表示直播间在线。 true means the live room is online.
func (dl *DouyinLive) setLiveStatus(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.isLiveClosed = status
	dl.liveStatusKnown = true
	if status {
		dl.liveStatusGuard.Reset()
	}
}

// clearLiveStatus 清空已知直播状态，保留为“未知”而不是“确认未开播”。
// clearLiveStatus clears the known live status, keeping it unknown rather than confirmed offline.
func (dl *DouyinLive) clearLiveStatus() {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.isLiveClosed = false
	dl.liveStatusKnown = false
}

// isLiveStatus 返回内部直播状态。
// isLiveStatus returns the internal live status.
func (dl *DouyinLive) isLiveStatus() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.isLiveClosed
}

// liveStatusSnapshot 返回直播状态和该状态是否来自有效页面或接口。
// liveStatusSnapshot returns the live status and whether it came from a valid page or API response.
func (dl *DouyinLive) liveStatusSnapshot() (bool, bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.isLiveClosed, dl.liveStatusKnown
}

// isKnownOfflineStatus 判断当前是否已确认未开播或已下播。
// isKnownOfflineStatus reports whether the room is confirmed offline or ended.
func (dl *DouyinLive) isKnownOfflineStatus() bool {
	isLive, known := dl.liveStatusSnapshot()
	return known && !isLive
}

// IsKnownOfflineStatus 返回直播页或接口是否已确认房间当前不在直播。
// IsKnownOfflineStatus reports whether the page or API confirmed the room is not currently live.
func (dl *DouyinLive) IsKnownOfflineStatus() bool {
	return dl.isKnownOfflineStatus()
}

// shouldCloseAfterStatusCheck 根据连续状态检查结果判断是否应关闭连接。
// shouldCloseAfterStatusCheck decides whether to close the connection based on repeated status checks.
// 参数/Parameters:
//   - isLive: 本次直播状态检查结果。 Current live-status check result.
func (dl *DouyinLive) shouldCloseAfterStatusCheck(isLive bool) bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	shouldClose := dl.liveStatusGuard.Record(isLive)
	if shouldClose {
		dl.isLiveClosed = false
		dl.liveStatusKnown = true
	} else if isLive {
		dl.isLiveClosed = true
		dl.liveStatusKnown = true
	}
	return shouldClose
}
