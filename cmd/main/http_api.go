package main

import (
	"context"
	"crypto/rand"
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
	Anchor        *apiAnchor `json:"anchor,omitempty"`
	ClientCount   *int       `json:"client_count,omitempty"`
	UpstreamReady *bool      `json:"upstream_ready,omitempty"`
	StatusUnknown *bool      `json:"status_unknown,omitempty"`
	Source        string     `json:"source"`
	CheckedAt     *time.Time `json:"checked_at,omitempty"`
}

type apiAnchor struct {
	UserUniqueID string `json:"user_unique_id,omitempty"`
	Nickname     string `json:"nickname,omitempty"`
	AvatarThumb  string `json:"avatar_thumb,omitempty"`
}

type apiAnchorProfile struct {
	LiveID      string    `json:"live_id"`
	RoomID      string    `json:"room_id,omitempty"`
	Status      string    `json:"status"`
	HasRoom     *bool     `json:"has_room,omitempty"`
	AccountOnly *bool     `json:"account_only,omitempty"`
	Anchor      apiAnchor `json:"anchor"`
	Source      string    `json:"source"`
	CheckedAt   time.Time `json:"checked_at"`
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

type apiRequestHandler func(http.ResponseWriter, *http.Request, string)

func (a *App) registerHTTPAPI(mux *http.ServeMux) {
	methodsByPath := make(map[string]string)
	register := func(method, pattern string, liveID bool, handler apiRequestHandler) {
		route := a.wrapAPIRequest(true, func(w http.ResponseWriter, r *http.Request, requestID string) {
			if expected := methodsByPath[r.URL.Path]; expected != "" && r.Method != expected {
				w.Header().Set("Allow", expected)
				a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持", "请使用 "+expected)
				return
			}
			if liveID && !isValidLiveID(r.PathValue("live_id")) {
				a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_live_id", "直播间标识无效", "仅支持字母、数字、下划线和短横线")
				return
			}
			if r.Method != method {
				w.Header().Set("Allow", method)
				a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持", "请使用 "+method)
				return
			}
			handler(w, r, requestID)
		})
		mux.Handle(method+" "+pattern, route)
		if !strings.Contains(pattern, "{") {
			methodsByPath[pattern] = method
		}
	}

	health := a.wrapAPIRequest(false, func(w http.ResponseWriter, r *http.Request, requestID string) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "健康检查仅支持 GET 请求", "请使用 GET")
			return
		}
		a.handleHealth(w, r, requestID)
	})
	mux.Handle("GET /health", health)
	mux.Handle("/health", health)

	metrics := a.wrapAPIRequest(false, func(w http.ResponseWriter, r *http.Request, requestID string) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "指标接口仅支持 GET 请求", "请使用 GET")
			return
		}
		if !a.authorizeAPI(r) {
			a.writeAPIError(w, requestID, http.StatusUnauthorized, "unauthorized", "缺少或无效的 API Key", "请使用 Authorization: Bearer <key>")
			return
		}
		a.handleMetrics(w, r, requestID)
	})
	mux.Handle("GET /metrics", metrics)
	mux.Handle("/metrics", metrics)

	register(http.MethodGet, "/api/v1/health", false, a.handleHealth)
	register(http.MethodGet, "/api/v1/capabilities", false, a.handleCapabilities)
	register(http.MethodGet, "/api/v1/rooms", false, a.handleRoomList)
	register(http.MethodGet, "/api/v1/rooms/resolve", false, a.handleRoomResolve)
	register(http.MethodPost, "/api/v1/rooms/status:batch", false, a.handleBatchRoomStatus)
	roomProbe := func(w http.ResponseWriter, r *http.Request, requestID string) {
		a.handleRoomProbe(w, r, requestID, r.PathValue("live_id"), false)
	}
	roomStatus := func(w http.ResponseWriter, r *http.Request, requestID string) {
		a.handleRoomProbe(w, r, requestID, r.PathValue("live_id"), true)
	}
	anchorProfile := func(w http.ResponseWriter, r *http.Request, requestID string) {
		a.handleAnchorProfile(w, r, requestID, r.PathValue("live_id"))
	}
	register(http.MethodGet, "/api/v1/rooms/{live_id}", true, roomProbe)
	register(http.MethodGet, "/api/v1/rooms/{live_id}/status", true, roomStatus)
	register(http.MethodGet, "/api/v1/rooms/{live_id}/anchor", true, anchorProfile)

	// The former path parser accepted a trailing slash on live-ID routes.
	register(http.MethodGet, "/api/v1/rooms/{live_id}/{$}", true, roomProbe)
	register(http.MethodGet, "/api/v1/rooms/{live_id}/status/{$}", true, roomStatus)
	register(http.MethodGet, "/api/v1/rooms/{live_id}/anchor/{$}", true, anchorProfile)

	notFound := a.wrapAPIRequest(true, func(w http.ResponseWriter, r *http.Request, requestID string) {
		allowedMethod, matchedPattern := methodsByPath[r.URL.Path], ""
		if allowedMethod == "" {
			for _, method := range []string{http.MethodGet, http.MethodPost} {
				probe := r.Clone(r.Context())
				probe.Method = method
				_, pattern := mux.Handler(probe)
				if strings.HasPrefix(pattern, method+" ") {
					allowedMethod, matchedPattern = method, pattern
					break
				}
			}
		}

		roomPath, isRoomPath := strings.CutPrefix(r.URL.Path, "/api/v1/rooms/")
		liveID, _, _ := strings.Cut(roomPath, "/")
		if isRoomPath && liveID != "" && (matchedPattern == "" || strings.Contains(matchedPattern, "{live_id}")) && !isValidLiveID(liveID) {
			a.writeAPIError(w, requestID, http.StatusBadRequest, "invalid_live_id", "直播间标识无效", "仅支持字母、数字、下划线和短横线")
			return
		}
		if allowedMethod != "" {
			w.Header().Set("Allow", allowedMethod)
			a.writeAPIError(w, requestID, http.StatusMethodNotAllowed, "method_not_allowed", "请求方法不受支持", "请使用 "+allowedMethod)
			return
		}
		a.writeAPIError(w, requestID, http.StatusNotFound, "not_found", "接口不存在", "请查看 API 文档")
	})
	mux.Handle("/api/v1", notFound)
	mux.Handle("/api/v1/", notFound)
}

func (a *App) wrapAPIRequest(requireAuth bool, next apiRequestHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		if requireAuth && !a.authorizeAPI(r) {
			a.writeAPIError(w, requestID, http.StatusUnauthorized, "unauthorized", "缺少或无效的 API Key", "请使用 Authorization: Bearer <key>")
			return
		}
		next(w, r, requestID)
	})
}

func requestIDForRequest(r *http.Request) string {
	requestID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
	if !validRequestID.MatchString(requestID) {
		return rand.Text()
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
		w.WriteHeader(status)
		_, _ = w.Write(body)
		return
	}
	w.Header().Set("Cache-Control", fmt.Sprintf("private, max-age=%d", int(maxAge/time.Second)))
	stable, err := json.Marshal(apiEnvelope{OK: status < 400, Data: data})
	if err != nil {
		a.writeAPIError(w, requestID, http.StatusInternalServerError, "internal_error", "响应编码失败", "请稍后重试")
		return
	}
	hash := sha256.Sum256(stable)
	// request_id intentionally changes for every request. Publish a weak ETag
	// for the stable representation data instead of claiming byte identity for
	// the complete envelope.
	// request_id 每次请求都会变化，因此对稳定业务数据发布弱 ETag，避免错误
	// 声称完整 envelope 的字节完全一致。
	etag := fmt.Sprintf("W/\"%x\"", hash[:])
	w.Header().Set("ETag", etag)
	if r != nil && (r.Method == http.MethodGet || r.Method == http.MethodHead) && ifNoneMatchMatches(r.Header.Get("If-None-Match"), etag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func ifNoneMatchMatches(header, etag string) bool {
	target, ok := weakETagOpaque(etag)
	if !ok {
		return false
	}
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		if opaque, valid := weakETagOpaque(candidate); valid && opaque == target {
			return true
		}
	}
	return false
}

func weakETagOpaque(value string) (string, bool) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "W/")
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", false
	}
	for _, ch := range value[1 : len(value)-1] {
		if ch == '"' || ch < 0x21 || ch == 0x7f {
			return "", false
		}
	}
	return value, true
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
		"endpoints":      []string{"GET /health", "GET /metrics", "GET /api/v1/health", "GET /api/v1/capabilities", "GET /api/v1/rooms", "GET /api/v1/rooms/{live_id}", "GET /api/v1/rooms/{live_id}/status", "GET /api/v1/rooms/{live_id}/anchor", "POST /api/v1/rooms/status:batch", "GET /api/v1/rooms/resolve"},
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
	status, checkedAt, ok := a.lookupRoomForAPI(w, r, requestID, liveID)
	if !ok {
		return
	}
	if statusOnly {
		a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"live_id": status.LiveID, "status": status.Code, "is_live": status.Live, "has_room": status.HasRoom, "checked_at": checkedAt}, 5*time.Second)
		return
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, liveStatusToAPI(status, "probe", checkedAt), 5*time.Second)
}

func (a *App) handleAnchorProfile(w http.ResponseWriter, r *http.Request, requestID, liveID string) {
	status, checkedAt, ok := a.lookupRoomForAPI(w, r, requestID, liveID)
	if !ok {
		return
	}
	anchor := anchorFromLiveStatus(status)
	if !anchor.available() {
		a.writeAPIError(w, requestID, http.StatusServiceUnavailable, "anchor_unverified", "暂时无法确认主播资料", "稍后重试")
		return
	}
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, apiAnchorProfile{
		LiveID: status.LiveID, RoomID: status.RoomID, Status: string(status.Code),
		HasRoom: status.HasRoom, AccountOnly: status.AccountOnly, Anchor: anchor,
		Source: "probe", CheckedAt: checkedAt,
	}, 5*time.Second)
}

func (a *App) lookupRoomForAPI(w http.ResponseWriter, r *http.Request, requestID, liveID string) (douyinLive.LiveStatus, time.Time, bool) {
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
		return douyinLive.LiveStatus{}, time.Time{}, false
	}
	checkedAt := time.Now().UTC().Truncate(5 * time.Second)
	return status, checkedAt, true
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
	var maxBytesErr *http.MaxBytesError
	if err := decoder.Decode(&request); err != nil {
		_ = controller.SetReadDeadline(time.Time{})
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
	} else if errors.As(err, &maxBytesErr) {
		_ = controller.SetReadDeadline(time.Time{})
		a.writeAPIError(w, requestID, http.StatusRequestEntityTooLarge, "request_too_large", "请求体超过 1 MiB 限制", "请减少批量请求内容")
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
	var wg sync.WaitGroup
	for index, liveID := range ids {
		wg.Go(func() {
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
		})
	}
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
	a.writeAPIJSONCached(w, r, requestID, http.StatusOK, map[string]interface{}{"items": items, "total": len(items), "online": online, "offline": offline, "account_no_room": accountNoRoom, "not_found": notFound, "unknown": unknown}, 0)
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
	anchor := apiAnchor{UserUniqueID: s.UserUniqueID, Nickname: s.LiveName, AvatarThumb: s.AvatarThumb}
	var anchorPtr *apiAnchor
	if anchor.available() {
		anchorPtr = &anchor
	}
	return apiRoom{LiveID: s.LiveID, Status: s.Status, IsLive: s.IsLive, HasRoom: s.HasRoom, AccountOnly: s.AccountOnly, RoomID: s.RoomID, Title: s.Title, Anchor: anchorPtr, ClientCount: &clientCount, UpstreamReady: &upstreamReady, StatusUnknown: &statusUnknown, Source: source, CheckedAt: checkedAtPtr}
}

func liveStatusToAPI(s douyinLive.LiveStatus, source string, checkedAt time.Time) apiRoom {
	hasRoom := s.HasRoom
	if hasRoom == nil && (s.Code == douyinLive.LiveStatusOnline || s.Code == douyinLive.LiveStatusOffline) {
		value := s.RoomID != ""
		hasRoom = &value
	}
	accountOnly := s.AccountOnly
	anchor := anchorFromLiveStatus(s)
	var anchorPtr *apiAnchor
	if anchor.available() {
		anchorPtr = &anchor
	}
	return apiRoom{LiveID: s.LiveID, Status: string(s.Code), IsLive: s.Live, HasRoom: hasRoom, AccountOnly: accountOnly, RoomID: s.RoomID, Title: s.Title, Anchor: anchorPtr, Source: source, CheckedAt: &checkedAt}
}

func anchorFromLiveStatus(s douyinLive.LiveStatus) apiAnchor {
	return apiAnchor{UserUniqueID: s.UserUniqueID, Nickname: s.LiveName, AvatarThumb: s.AvatarThumb}
}

func (a apiAnchor) available() bool {
	return a.UserUniqueID != "" || a.Nickname != "" || a.AvatarThumb != ""
}
