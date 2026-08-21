package main

import (
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// apiMetrics stores low-cardinality process metrics for HTTP and probe operations.
// apiMetrics 保存 HTTP 和房间探测的低基数运行指标。
type apiMetrics struct {
	httpRequests       atomic.Uint64
	httpErrors         atomic.Uint64
	roomProbes         atomic.Uint64
	probeUpstreamCalls atomic.Uint64
	probeMergedWaiters atomic.Uint64
	roomProbeErrors    atomic.Uint64
	batchRequests      atomic.Uint64
	resolveRequests    atomic.Uint64
	httpDurationNanos  atomic.Uint64
	httpDurationCount  atomic.Uint64
}

func newAPIMetrics() *apiMetrics { return &apiMetrics{} }

func (m *apiMetrics) observeHTTPDuration(d time.Duration) {
	if m == nil {
		return
	}
	m.httpDurationNanos.Add(uint64(d))
	m.httpDurationCount.Add(1)
}

func (a *App) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if a.metrics != nil {
		a.metrics.httpRequests.Add(1)
	}
	startedAt := time.Now()
	defer func() {
		if a.metrics != nil {
			a.metrics.observeHTTPDuration(time.Since(startedAt))
		}
	}()
	requestID := requestIDForRequest(r)
	w.Header().Set("X-Request-ID", requestID)
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !a.authorizeAPI(r) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="douyinlive"`)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	snapshots := a.roomManager.SnapshotRooms()
	online, monitoring, clients := 0, 0, 0
	for _, snapshot := range snapshots {
		clients += snapshot.ClientCount
		if snapshot.Status == "online" {
			online++
		}
		if snapshot.Status == "offline" || snapshot.Status == "unknown" {
			monitoring++
		}
	}
	m := a.metrics
	if m == nil {
		m = newAPIMetrics()
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	fmt.Fprintf(w, "# HELP douyinlive_active_rooms Number of managed active rooms.\n# TYPE douyinlive_active_rooms gauge\ndouyinlive_active_rooms %d\n", len(snapshots))
	fmt.Fprintf(w, "# HELP douyinlive_online_rooms Number of active rooms confirmed online.\n# TYPE douyinlive_online_rooms gauge\ndouyinlive_online_rooms %d\n", online)
	fmt.Fprintf(w, "# HELP douyinlive_monitoring_rooms Number of rooms in offline or unknown monitoring state.\n# TYPE douyinlive_monitoring_rooms gauge\ndouyinlive_monitoring_rooms %d\n", monitoring)
	fmt.Fprintf(w, "# HELP douyinlive_active_clients Number of downstream WebSocket clients.\n# TYPE douyinlive_active_clients gauge\ndouyinlive_active_clients %d\n", clients)
	fmt.Fprintf(w, "# HELP douyinlive_http_requests_total Total HTTP API requests.\n# TYPE douyinlive_http_requests_total counter\ndouyinlive_http_requests_total %d\n", m.httpRequests.Load())
	fmt.Fprintf(w, "# HELP douyinlive_http_errors_total Total versioned HTTP API errors.\n# TYPE douyinlive_http_errors_total counter\ndouyinlive_http_errors_total %d\n", m.httpErrors.Load())
	fmt.Fprintf(w, "# HELP douyinlive_http_request_duration_seconds HTTP API request duration in seconds.\n# TYPE douyinlive_http_request_duration_seconds summary\ndouyinlive_http_request_duration_seconds_sum %.9f\ndouyinlive_http_request_duration_seconds_count %d\n", float64(m.httpDurationNanos.Load())/float64(time.Second), m.httpDurationCount.Load())
	fmt.Fprintf(w, "# HELP douyinlive_room_probes_total Total one-shot room probes.\n# TYPE douyinlive_room_probes_total counter\ndouyinlive_room_probes_total %d\n", m.roomProbes.Load())
	fmt.Fprintf(w, "# HELP douyinlive_probe_upstream_calls_total Total actual upstream room probe calls after request coalescing.\n# TYPE douyinlive_probe_upstream_calls_total counter\ndouyinlive_probe_upstream_calls_total %d\n", m.probeUpstreamCalls.Load())
	fmt.Fprintf(w, "# HELP douyinlive_probe_merged_waiters_total Total room probe requests served by an existing in-flight probe.\n# TYPE douyinlive_probe_merged_waiters_total counter\ndouyinlive_probe_merged_waiters_total %d\n", m.probeMergedWaiters.Load())
	fmt.Fprintf(w, "# HELP douyinlive_room_probe_errors_total Total failed one-shot room probes.\n# TYPE douyinlive_room_probe_errors_total counter\ndouyinlive_room_probe_errors_total %d\n", m.roomProbeErrors.Load())
	fmt.Fprintf(w, "# HELP douyinlive_batch_requests_total Total batch status requests.\n# TYPE douyinlive_batch_requests_total counter\ndouyinlive_batch_requests_total %d\n", m.batchRequests.Load())
	fmt.Fprintf(w, "# HELP douyinlive_resolve_requests_total Total URL resolve requests.\n# TYPE douyinlive_resolve_requests_total counter\ndouyinlive_resolve_requests_total %d\n", m.resolveRequests.Load())
}
