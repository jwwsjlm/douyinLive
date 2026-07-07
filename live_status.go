package douyinLive

// liveStatusGuard 通过多次确认降低直播状态误判概率。
// liveStatusGuard reduces live-status false positives by requiring repeated confirmation.
type liveStatusGuard struct {
	offlineConfirmations int
}

// Record 记录一次直播状态检查结果，并返回是否确认下播。
// Record stores one live-status check result and reports whether offline is confirmed.
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

// setLiveStatus 设置直播间状态（线程安全）
// setLiveStatus 更新内部直播状态。
// setLiveStatus updates the internal live status.
func (dl *DouyinLive) setLiveStatus(status bool) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.isLiveClosed = status
	if status {
		dl.liveStatusGuard.Reset()
	}
}

// isLiveStatus 获取直播间状态（线程安全）
// isLiveStatus 返回内部直播状态。
// isLiveStatus returns the internal live status.
func (dl *DouyinLive) isLiveStatus() bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.isLiveClosed
}

// shouldCloseAfterStatusCheck 根据连续状态检查结果判断是否应关闭连接。
// shouldCloseAfterStatusCheck decides whether to close the connection based on repeated status checks.
func (dl *DouyinLive) shouldCloseAfterStatusCheck(isLive bool) bool {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	shouldClose := dl.liveStatusGuard.Record(isLive)
	if shouldClose {
		dl.isLiveClosed = false
	} else if isLive {
		dl.isLiveClosed = true
	}
	return shouldClose
}
