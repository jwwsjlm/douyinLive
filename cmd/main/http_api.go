package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

var (
	validRequestID = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)
)

const (
	apiSingleProbeTimeout     = 20 * time.Second
	apiBatchProbeTimeout      = 30 * time.Second
	apiRequestBodyReadTimeout = 5 * time.Second
)

func isValidLiveID(value string) bool {
	normalized, err := douyinLive.ValidateLiveID(value)
	return err == nil && normalized == value
}

type apiEnvelope struct {
	OK        bool        `json:"ok"`
	Data      interface{} `json:"data"`
	Error     *apiError   `json:"error"`
	RequestID string      `json:"request_id"`
}

type apiError struct {
	Code       string `json:"code"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

type apiRoom struct {
	LiveID        string     `json:"live_id"`
	Status        string     `json:"status"`
	IsLive        *bool      `json:"is_live,omitempty"`
	HasRoom       *bool      `json:"has_room,omitempty"`
	AccountOnly   *bool      `json:"account_only,omitempty"`
	RoomID        string     `json:"room_id,omitempty"`
	Title         string     `json:"title,omitempty"`
	ClientCount   *int       `json:"client_count,omitempty"`
	UpstreamReady *bool      `json:"upstream_ready,omitempty"`
	StatusUnknown *bool      `json:"status_unknown,omitempty"`
	Source        string     `json:"source"`
	CheckedAt     *time.Time `json:"checked_at,omitempty"`
}

// parseLiveIDPath validates a single live-room path segment for HTTP and WebSocket routes.
// parseLiveIDPath 为 HTTP 和 WebSocket 路由校验单一直播间路径段。
func parseLiveIDPath(path, prefix string) (string, error) {
	if prefix == "" || !strings.HasPrefix(path, prefix) {
		return "", fmt.Errorf("路径前缀无效")
	}
	suffix := strings.TrimPrefix(path, prefix)
	suffix = strings.TrimSuffix(suffix, "/")
	if suffix == "" || strings.Contains(suffix, "/") || !isValidLiveID(suffix) {
		return "", fmt.Errorf("直播间标识无效")
	}
	return suffix, nil
}

func (a *App) registerHTTPAPI(mux *http.ServeMux) {
	mux.HandleFunc("/health", a.handleHealthAlias)
	mux.HandleFunc("/metrics", a.handleMetrics)
	mux.HandleFunc("/api/v1/", a.handleAPI)
}

func (a *App) handleAPI(w http.ResponseWriter, r *http.Request) {
	startedAt := time.Now()
	defer func() {
		if a.metrics != nil {
			a.metrics.observeHTTPDuration(time.Since(startedAt))
		}
	}()
	if a.metrics != nil {
		a.metrics.httpRequests.Add(1)
	}
	requestID := requestIDForRequest(r)
	w.Header().Set("X-Request-ID", requestID)
	if !a.authorizeAPI(r) {
		a.writeAPIError(w, requestID, http.StatusUnauthorized, "unauthorized", "缺少或无效的 API Key", "请使用 Authorization: Bearer <key>")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1")
	if path == "/rooms/status:batch" {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "批量状态接口仅支持 POST", "请使用 POST")
			return
		}
		a.handleBatchRoomStatus(w, r, requestID)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "仅支持 GET 请求", "请使用 GET")
		return
	}
	switch path {
	case "/health":
		a.handleHealth(w, r, requestID)
		return
	case "/capabilities":
		a.handleCapabilities(w, r, requestID)
		return
	case "/rooms":
		a.handleRoomList(w, r, requestID)
		return
	case "/rooms/resolve":
		a.handleRoomResolve(w, r, requestID)
		return
	}
	if !strings.HasPrefix(path, "/rooms/") {
		a.writeAPIError(w, requestID, http.StatusNotFound, "not_found", "接口不存在", "请查看 API 文档")
		return
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] != "rooms" || !isValidLiveID(parts[1]) {
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_live_id", "直播间标识无效", "仅支持字母、数字、下划线和短横线")
		return
	}
	statusOnly := len(parts) == 3 && parts[2] == "status"
	if len(parts) == 3 && !statusOnly {
		a.writeAPIError(w, requestID, http.StatusNotFound, "not_found", "接口不存在", "请查看 API 文档")
		return
	}
	a.handleRoomProbe(w, r, requestID, parts[1], statusOnly)
}

func (a *App) handleHealthAlias(w http.ResponseWriter, r *http.Request) {
	if a.metrics != nil {
		a.metrics.httpRequests.Add(1)
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		requestID := requestIDForRequest(r)
		w.Header().Set("X-Request-ID", requestID)
		a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "健康检查仅支持 GET 请求", "请使用 GET")
		return
	}
	requestID := requestIDForRequest(r)
	w.Header().Set("X-Request-ID", requestID)
	startedAt := time.Now()
	defer func() {
		if a.metrics != nil {
			a.metrics.observeHTTPDuration(time.Since(startedAt))
		}
	}()
	a.handleHealth(w, r, requestID)
}

func requestIDForRequest(r *http.Request) string {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if !validRequestID.MatchString(requestID) {
		return uuid.NewString()
	}
	return requestID
}

func (a *App) authorizeAPI(r *http.Request) bool {
	key := strings.TrimSpace(a.config.API.Key)
	if key == "" {
		return true
	}
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) <= len(prefix) || !strings.EqualFold(header[:len(prefix)], prefix) {
		return false
	}
	token := strings.TrimSpace(header[len(prefix):])
	if token == "" {
		return false
	}
	// Compare fixed-size digests so a length mismatch does not short-circuit
	// the constant-time comparison.
	keyDigest := sha256.Sum256([]byte(key))
	tokenDigest := sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(keyDigest[:], tokenDigest[:]) == 1
}

func (a *App) writeAPIJSONCached(w http.ResponseWriter, r *http.Request, requestID string, status int, data interface{}, maxAge time.Duration) {
	body, err := json.Marshal(apiEnvelope{OK: status < 400, Data: data, RequestID: requestID})
	if err != nil {
		a.writeAPIError(w, requestID, http.StatusInternalServerError, "internal_error", "响应编码失败", "请稍后重试")
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if status >= 400 || maxAge <= 0 {
		w.Header().Set("Cache-Control", "no-store")
	} else {
		w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", int(maxAge/time.Second)))
	}
	stable, err := json.Marshal(apiEnvelope{OK: status < 400, Data: data})
	if err != nil {
		a.writeAPIError(w, requestID, http.StatusInternalServerError, "internal_error", "响应编码失败", "请稍后重试")
		return
	}
	hash := sha256.Sum256(stable)
	etag := fmt.Sprintf("\"%x\"", hash[:])
	w.Header().Set("ETag", etag)
	if r != nil && status < 400 && maxAge > 0 && ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func ifNoneMatchMatches(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

func (a *App) writeAPIError(w http.ResponseWriter, requestID string, status int, code, message, suggestion string) {
	if a.metrics != nil {
		a.metrics.httpErrors.Add(1)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if status == http.StatusUnauthorized {
		w.Header().Set("WWW-Authenticate", "Bearer realm=\"douyinlive\"")
	}
	if status == http.StatusServiceUnavailable {
		w.Header().Set("Retry-After", "5")
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(apiEnvelope{OK: false, Data: nil, Error: &apiError{Code: code, Message: message, Suggestion: suggestion}, RequestID: requestID})
}

func (a *App) handleHealth(w http.ResponseWriter, r *http.Request, requestID string) {
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"status": "ok", "version": VersionString(), "tag": buildTag, "commit": buildCommit, "build_date": buildDate, "port": a.runningPort, "sign_provider": a.config.Sign.Provider}, 30*time.Second)
}

func (a *App) handleCapabilities(w http.ResponseWriter, r *http.Request, requestID string) {
	apiKeyConfigured := strings.TrimSpace(a.config.API.Key) != ""
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{
		"api_version": "v1", "read_only": true, "message_transport": "websocket", "http_send_supported": false,
		"websocket_path": strings.TrimSuffix(a.websocketRoutePrefix(), "/"), "websocket_endpoint": "GET " + a.websocketRoutePrefix() + "{live_id}",
		"websocket_auth": map[string]interface{}{"required": apiKeyConfigured, "scheme": "bearer", "header": "Authorization", "browser_native_supported": !apiKeyConfigured},
		"endpoints":      []string{"GET /health", "GET /metrics", "GET /api/v1/health", "GET /api/v1/capabilities", "GET /api/v1/rooms", "GET /api/v1/rooms/{live_id}", "GET /api/v1/rooms/{live_id}/status", "POST /api/v1/rooms/status:batch", "GET /api/v1/rooms/resolve"},
		"message_types":  []string{douyinLive.WebcastChatMessage, douyinLive.WebcastGiftMessage, douyinLive.WebcastLikeMessage, douyinLive.WebcastMemberMessage, douyinLive.WebcastSocialMessage, douyinLive.WebcastRoomUserSeqMessage, douyinLive.WebcastFansclubMessage, douyinLive.WebcastControlMessage, douyinLive.WebcastEmojiChatMessage, douyinLive.WebcastRoomStatsMessage, douyinLive.WebcastRoomMessage, douyinLive.WebcastRoomRankMessage},
	}, time.Minute)
}

func (a *App) handleRoomList(w http.ResponseWriter, r *http.Request, requestID string) {
	snapshots := a.roomManager.SnapshotRooms()
	sort.Slice(snapshots, func(i, j int) bool { return snapshots[i].LiveID < snapshots[j].LiveID })
	rooms := make([]apiRoom, 0, len(snapshots))
	for _, s := range snapshots {
		rooms = append(rooms, roomSnapshotToAPI(s, "active", time.Time{}))
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"rooms": rooms, "count": len(rooms)}, 2*time.Second)
}

func (a *App) handleRoomProbe(w http.ResponseWriter, r *http.Request, requestID, liveID string, statusOnly bool) {
	ctx, cancel := context.WithTimeout(r.Context(), apiSingleProbeTimeout)
	defer cancel()
	if a.metrics != nil {
		a.metrics.roomProbes.Add(1)
	}
	status, err := a.roomManager.LookupRoom(ctx, liveID)
	if probeResultIsFailure(status, err) {
		if a.metrics != nil {
			a.metrics.roomProbeErrors.Add(1)
		}
		code, httpStatus, message, suggestion := apiProbeFailure(status, err)
		a.writeAPIError(w, requestID, httpStatus, code, message, suggestion)
		return
	}
	checkedAt := time.Now().UTC().Truncate(5 * time.Second)
	if statusOnly {
		a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"live_id": status.LiveID, "status": status.Code, "is_live": status.Live, "has_room": status.HasRoom, "checked_at": checkedAt}, 5*time.Second)
		return
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, liveStatusToAPI(status, "probe", checkedAt), 5*time.Second)
}

type batchStatusRequest struct {
	LiveIDs []string `json:"live_ids"`
}

type batchStatusItem struct {
	LiveID      string    `json:"live_id"`
	Status      string    `json:"status"`
	IsLive      *bool     `json:"is_live,omitempty"`
	HasRoom     *bool     `json:"has_room,omitempty"`
	AccountOnly *bool     `json:"account_only,omitempty"`
	RoomID      string    `json:"room_id,omitempty"`
	Title       string    `json:"title,omitempty"`
	Error       *apiError `json:"error,omitempty"`
}

func (a *App) handleBatchRoomStatus(w http.ResponseWriter, r *http.Request, requestID string) {
	if a.metrics != nil {
		a.metrics.batchRequests.Add(1)
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(apiRequestBodyReadTimeout))
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	decoder.DisallowUnknownFields()
	var request batchStatusRequest
	if err := decoder.Decode(&request); err != nil {
		_ = controller.SetReadDeadline(time.Time{})
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			a.writeAPIError(w, requestID, http.StatusRequestEntityTooLarge, "request_too_large", "请求体超过 1 MiB 限制", "请减少批量请求内容")
			return
		}
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_json", "请求体不是有效 JSON", "请提交 {\"live_ids\":[...]}")
		return
	}
	var extra interface{}
	if err := decoder.Decode(&extra); err == nil {
		_ = controller.SetReadDeadline(time.Time{})
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_json", "请求体只能包含一个 JSON 对象", "请移除多余内容")
		return
	} else if !errors.Is(err, io.EOF) {
		_ = controller.SetReadDeadline(time.Time{})
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_json", "请求体不是有效 JSON", "请提交 {\"live_ids\":[...]}")
		return
	}
	// The deadline only protects request-body reads. Clear it before potentially
	// long upstream probes so it cannot affect a reused keep-alive connection.
	// 该 deadline 只用于保护请求体读取；开始耗时探测前立即清除，避免影响复用连接。
	_ = controller.SetReadDeadline(time.Time{})
	ids := make([]string, 0, len(request.LiveIDs))
	seen := make(map[string]struct{}, len(request.LiveIDs))
	for _, raw := range request.LiveIDs {
		liveID := strings.TrimSpace(raw)
		if !isValidLiveID(liveID) {
			a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_live_id", "批量请求中包含无效直播间标识", "仅支持字母、数字、下划线和短横线")
			return
		}
		if _, ok := seen[liveID]; ok {
			a.writeAPIError(w, requestID, http.StatusBadRequest, "duplicate_live_id", "批量请求中包含重复直播间标识", "请确保 live_ids 中每个标识只出现一次")
			return
		}
		seen[liveID] = struct{}{}
		ids = append(ids, liveID)
	}
	if len(ids) == 0 || len(ids) > 50 {
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_batch_size", "live_ids 数量必须在 1 到 50 之间", "请减少批量数量")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiBatchProbeTimeout)
	defer cancel()
	items := make([]batchStatusItem, len(ids))
	workerCount := min(8, len(ids))
	type batchJob struct {
		index  int
		liveID string
	}
	jobs := make(chan batchJob)
	var wg sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				index, liveID := job.index, job.liveID
				if a.metrics != nil {
					a.metrics.roomProbes.Add(1)
				}
				status, err := a.roomManager.LookupRoom(ctx, liveID)
				item := batchStatusItem{LiveID: liveID}
				if probeResultIsFailure(status, err) {
					if a.metrics != nil {
						a.metrics.roomProbeErrors.Add(1)
					}
					item.Error = apiErrorForProbeResult(status, err)
					item.Status = statusCodeForProbeResult(status, err)
				} else {
					item.Status = string(status.Code)
					item.IsLive = status.Live
					item.HasRoom = status.HasRoom
					item.AccountOnly = status.AccountOnly
					item.RoomID = status.RoomID
					item.Title = status.Title
				}
				items[index] = item
			}
		}()
	}
	for index, liveID := range ids {
		jobs <- batchJob{index: index, liveID: liveID}
	}
	close(jobs)
	wg.Wait()
	online, offline, accountNoRoom, notFound, unknown := 0, 0, 0, 0, 0
	for _, item := range items {
		switch item.Status {
		case "online":
			online++
		case "offline":
			offline++
		case "account_no_room":
			accountNoRoom++
		case "not_found":
			notFound++
		default:
			unknown++
		}
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"items": items, "total": len(items), "online": online, "offline": offline, "account_no_room": accountNoRoom, "not_found": notFound, "unknown": unknown}, 3*time.Second)
}

// probeResultIsFailure reports whether a probe result cannot be safely exposed as a verified status.
// probeResultIsFailure 判断探测结果是否无法安全地作为已验证状态返回。
func probeResultIsFailure(status douyinLive.LiveStatus, err error) bool {
	return err != nil || status.Code == "" || status.Code == douyinLive.LiveStatusUnknown || status.Code == douyinLive.LiveStatusNotFound
}

func apiErrorForProbeResult(status douyinLive.LiveStatus, err error) *apiError {
	code, _, message, suggestion := apiProbeFailure(status, err)
	return &apiError{Code: code, Message: message, Suggestion: suggestion}
}

func apiProbeFailure(status douyinLive.LiveStatus, err error) (code string, httpStatus int, message, suggestion string) {
	if status.Code == douyinLive.LiveStatusNotFound || errors.Is(err, douyinLive.ErrRoomNotFound) {
		return "not_found", http.StatusNotFound, "直播间不存在", "请检查直播间标识"
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return "upstream_timeout", http.StatusServiceUnavailable, "上游查询超时或请求已取消", "稍后重试"
	}
	return "upstream_unverified", http.StatusServiceUnavailable, "暂时无法确认直播状态", "稍后重试"
}

func statusCodeForProbeResult(status douyinLive.LiveStatus, err error) string {
	if status.Code == douyinLive.LiveStatusNotFound || errors.Is(err, douyinLive.ErrRoomNotFound) {
		return string(douyinLive.LiveStatusNotFound)
	}
	if status.Code == douyinLive.LiveStatusOnline || status.Code == douyinLive.LiveStatusOffline || status.Code == douyinLive.LiveStatusNoRoom {
		return string(status.Code)
	}
	if status.Code == douyinLive.LiveStatusUnknown || status.Code == "" || err != nil {
		return string(douyinLive.LiveStatusUnknown)
	}
	return string(douyinLive.LiveStatusUnknown)
}

func (a *App) handleRoomResolve(w http.ResponseWriter, r *http.Request, requestID string) {
	if a.metrics != nil {
		a.metrics.resolveRequests.Add(1)
	}
	resolved, err := resolveDouyinURL(r.URL.Query().Get("url"), a.config.API.AllowedDomains)
	if err != nil {
		a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_url", err.Error(), "仅支持 douyin.com 域名下的直播间 URL")
		return
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, resolved, time.Hour)
}

type resolvedRoomURL struct {
	InputURL     string `json:"input_url"`
	LiveID       string `json:"live_id"`
	CanonicalURL string `json:"canonical_url"`
}

func resolveDouyinURL(raw string, allowedDomains []string) (resolvedRoomURL, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return resolvedRoomURL{}, fmt.Errorf("url 不能为空")
	}
	if len(raw) > 2048 {
		return resolvedRoomURL{}, fmt.Errorf("URL 长度不能超过 2048 个字符")
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil || u.User != nil || (strings.ToLower(u.Scheme) != "http" && strings.ToLower(u.Scheme) != "https") {
		return resolvedRoomURL{}, fmt.Errorf("URL 格式无效")
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || net.ParseIP(host) != nil || !hostAllowed(host, allowedDomains) {
		return resolvedRoomURL{}, fmt.Errorf("URL 域名不在允许范围内")
	}
	if u.Port() != "" {
		return resolvedRoomURL{}, fmt.Errorf("URL 不应包含显式端口")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return resolvedRoomURL{}, fmt.Errorf("URL 不应包含 query 或 fragment")
	}
	if strings.Contains(u.Path, "//") {
		return resolvedRoomURL{}, fmt.Errorf("URL 路径包含重复分隔符")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 1 || parts[0] == "" {
		return resolvedRoomURL{}, fmt.Errorf("URL 中未找到直播间标识")
	}
	for _, part := range parts {
		decoded, err := url.PathUnescape(part)
		if err != nil || decoded == "." || decoded == ".." || strings.ContainsAny(decoded, "/?#\\") {
			return resolvedRoomURL{}, fmt.Errorf("URL 路径包含非法片段")
		}
	}
	liveID, err := url.PathUnescape(parts[len(parts)-1])
	if err != nil || !isValidLiveID(liveID) {
		return resolvedRoomURL{}, fmt.Errorf("URL 中的直播间标识无效")
	}
	return resolvedRoomURL{InputURL: raw, LiveID: liveID, CanonicalURL: "https://live.douyin.com/" + liveID}, nil
}

func hostAllowed(host string, allowedDomains []string) bool {
	domains, err := normalizeAllowedDomains(allowedDomains)
	if err != nil {
		return false
	}
	for _, raw := range domains {
		domain := strings.ToLower(strings.TrimSpace(raw))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func roomSnapshotToAPI(s roomSnapshot, source string, checkedAt time.Time) apiRoom {
	var checkedAtPtr *time.Time
	if !checkedAt.IsZero() {
		checkedAtPtr = &checkedAt
	}
	clientCount := s.ClientCount
	upstreamReady := s.UpstreamReady
	statusUnknown := s.StatusUnknown
	return apiRoom{LiveID: s.LiveID, Status: s.Status, IsLive: s.IsLive, HasRoom: s.HasRoom, AccountOnly: s.AccountOnly, RoomID: s.RoomID, Title: s.Title, ClientCount: &clientCount, UpstreamReady: &upstreamReady, StatusUnknown: &statusUnknown, Source: source, CheckedAt: checkedAtPtr}
}

func liveStatusToAPI(s douyinLive.LiveStatus, source string, checkedAt time.Time) apiRoom {
	hasRoom := s.HasRoom
	if hasRoom == nil && (s.Code == douyinLive.LiveStatusOnline || s.Code == douyinLive.LiveStatusOffline) {
		value := s.RoomID != ""
		hasRoom = &value
	}
	accountOnly := s.AccountOnly
	return apiRoom{LiveID: s.LiveID, Status: string(s.Code), IsLive: s.Live, HasRoom: hasRoom, AccountOnly: accountOnly, RoomID: s.RoomID, Title: s.Title, Source: source, CheckedAt: &checkedAt}
}
