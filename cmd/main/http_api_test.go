package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func testAPIApp(t *testing.T, key string) *App {
	t.Helper()
	app, err := NewApp(context.Background(), &Config{Port: "1088", API: APIConfig{Key: key}, Sign: SignConfig{Provider: signProviderLocal}, Monitor: MonitorConfig{PollInterval: time.Second, NotifyInterval: time.Second}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	app.runningPort = "1088"
	return app
}

func performAPIRequest(t *testing.T, app *App, method, path, auth string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	app.registerHTTPAPI(mux)
	req := httptest.NewRequest(method, path, nil)
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder) apiEnvelope {
	t.Helper()
	var envelope apiEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, rec.Body.String())
	}
	return envelope
}

func TestHTTPAPIHealthAndCapabilities(t *testing.T) {
	app := testAPIApp(t, "")
	health := performAPIRequest(t, app, http.MethodGet, "/api/v1/health", "")
	if health.Code != http.StatusOK || !decodeEnvelope(t, health).OK {
		t.Fatalf("health status=%d body=%s", health.Code, health.Body.String())
	}
	if health.Header().Get("X-Request-ID") == "" {
		t.Fatal("health response missing request id")
	}
	capabilities := performAPIRequest(t, app, http.MethodGet, "/api/v1/capabilities", "")
	if capabilities.Code != http.StatusOK || !strings.Contains(capabilities.Body.String(), `"read_only":true`) {
		t.Fatalf("capabilities status=%d body=%s", capabilities.Code, capabilities.Body.String())
	}
	if !strings.Contains(capabilities.Body.String(), `"browser_native_supported":true`) {
		t.Fatalf("capabilities should advertise browser-native WebSocket support without API key: %s", capabilities.Body.String())
	}
}

func TestHTTPAPIHealthAliasUsesSameEnvelopeAndETag(t *testing.T) {
	app := testAPIApp(t, "")
	apiHealth := performAPIRequest(t, app, http.MethodGet, "/api/v1/health", "")
	aliasHealth := performAPIRequest(t, app, http.MethodGet, "/health", "")
	if apiHealth.Code != http.StatusOK || aliasHealth.Code != http.StatusOK {
		t.Fatalf("health statuses: api=%d alias=%d", apiHealth.Code, aliasHealth.Code)
	}
	apiEnvelopeValue := decodeEnvelope(t, apiHealth)
	aliasEnvelopeValue := decodeEnvelope(t, aliasHealth)
	if !apiEnvelopeValue.OK || !aliasEnvelopeValue.OK || aliasEnvelopeValue.Error != nil {
		t.Fatalf("unexpected health envelopes: api=%+v alias=%+v", apiEnvelopeValue, aliasEnvelopeValue)
	}
	if apiHealth.Header().Get("ETag") == "" || aliasHealth.Header().Get("ETag") == "" {
		t.Fatal("health response missing ETag")
	}
	if apiHealth.Header().Get("Cache-Control") == "" || aliasHealth.Header().Get("Cache-Control") == "" {
		t.Fatal("health response missing Cache-Control")
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("If-None-Match", apiHealth.Header().Get("ETag"))
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	app.registerHTTPAPI(mux)
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 {
		t.Fatalf("conditional health status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestHTTPAPIOptionalBearerAuthentication(t *testing.T) {
	app := testAPIApp(t, "secret-token")
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/health", ""); got.Code != http.StatusUnauthorized {
		t.Fatalf("missing auth status=%d", got.Code)
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/health", "Bearer wrong"); got.Code != http.StatusUnauthorized {
		t.Fatalf("wrong auth status=%d", got.Code)
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/health", "Bearer secret-token"); got.Code != http.StatusOK {
		t.Fatalf("valid auth status=%d body=%s", got.Code, got.Body.String())
	}
	capabilities := performAPIRequest(t, app, http.MethodGet, "/api/v1/capabilities", "Bearer secret-token")
	if capabilities.Code != http.StatusOK || strings.Contains(capabilities.Body.String(), "secret-token") {
		t.Fatalf("capabilities leaked API key or failed: status=%d body=%s", capabilities.Code, capabilities.Body.String())
	}
	if !strings.Contains(capabilities.Body.String(), `"browser_native_supported":false`) {
		t.Fatalf("capabilities should explain browser auth limitation: %s", capabilities.Body.String())
	}
}

func TestHealthAliasRemainsUnauthenticatedWhenAPIKeyConfigured(t *testing.T) {
	app := testAPIApp(t, "secret-token")
	if got := performAPIRequest(t, app, http.MethodGet, "/health", ""); got.Code != http.StatusOK {
		t.Fatalf("health alias should remain public for probes, status=%d body=%s", got.Code, got.Body.String())
	}
}

func TestWebSocketAuthenticationUsesAPIKey(t *testing.T) {
	app := testAPIApp(t, "secret-token")
	req := httptest.NewRequest(http.MethodGet, "/ws/123", nil)
	rec := httptest.NewRecorder()
	app.handleWebSocket(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("missing WebSocket auth status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("missing WebSocket WWW-Authenticate header")
	}
}

func TestNormalizeWebSocketPath(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{"", "/ws", true},
		{"ws", "/ws", true},
		{"/live-stream/", "/live-stream", true},
		{"/api/custom", "", false},
		{"/health", "", false},
		{"/../ws", "", false},
		{"/live stream", "", false},
	}
	for _, tt := range tests {
		got, err := normalizeWebSocketPath(tt.raw)
		if tt.ok {
			if err != nil || got != tt.want {
				t.Fatalf("normalizeWebSocketPath(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
			}
		} else if err == nil {
			t.Fatalf("normalizeWebSocketPath(%q) unexpectedly succeeded with %q", tt.raw, got)
		}
	}
}

func TestParseLiveIDPathRejectsNestedOrEncodedPathSegments(t *testing.T) {
	tests := []struct {
		path string
		ok   bool
	}{
		{path: "/ws/123456", ok: true},
		{path: "/ws/123456/", ok: true},
		{path: "/ws/123/456", ok: false},
		{path: "/ws/foo%2Fbar", ok: false},
		{path: "/ws/foo?bar", ok: false},
		{path: "/ws/../123", ok: false},
	}
	for _, tc := range tests {
		_, err := parseLiveIDPath(tc.path, "/ws/")
		if (err == nil) != tc.ok {
			t.Fatalf("parseLiveIDPath(%q) err=%v, want ok=%v", tc.path, err, tc.ok)
		}
	}
}

func TestWebSocketOriginAllowlist(t *testing.T) {
	app := testAPIApp(t, "")
	app.config.WebSocket.AllowedOrigins = []string{"https://client.example.com"}
	allowed := httptest.NewRequest(http.MethodGet, "/ws/123", nil)
	allowed.Header.Set("Origin", "https://client.example.com")
	if !app.authorizeWebSocketOrigin(allowed) {
		t.Fatal("configured origin was rejected")
	}
	rejected := httptest.NewRequest(http.MethodGet, "/ws/123", nil)
	rejected.Header.Set("Origin", "https://evil.example.com")
	if app.authorizeWebSocketOrigin(rejected) {
		t.Fatal("unconfigured origin was accepted")
	}
}

func TestHTTPAPIRejectsMethodsAndInvalidRoomIDs(t *testing.T) {
	app := testAPIApp(t, "")
	if got := performAPIRequest(t, app, http.MethodPost, "/api/v1/health", ""); got.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status=%d", got.Code)
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/rooms/bad$id", ""); got.Code != http.StatusBadRequest {
		t.Fatalf("invalid room status=%d body=%s", got.Code, got.Body.String())
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/nope", ""); got.Code != http.StatusNotFound {
		t.Fatalf("unknown path status=%d", got.Code)
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/api/v1/anchors/123", ""); got.Code != http.StatusNotFound {
		t.Fatalf("removed anchor endpoint status=%d body=%s", got.Code, got.Body.String())
	}
	for _, path := range []string{"/api/v1/rooms/../123", "/api/v1/rooms/foo%2Fbar", "/api/v1/rooms/123/unknown"} {
		if got := performAPIRequest(t, app, http.MethodGet, path, ""); got.Code == http.StatusOK {
			t.Fatalf("invalid path %q unexpectedly succeeded: %s", path, got.Body.String())
		}
	}
}

func TestHTTPAPIBatchRejectsTrailingJSON(t *testing.T) {
	app := testAPIApp(t, "")
	mux := http.NewServeMux()
	app.registerHTTPAPI(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/status:batch", bytes.NewBufferString("{\"live_ids\":[\"123\"]} {}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "\"code\":\"invalid_json\"") {
		t.Fatalf("unexpected trailing JSON response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAPIBatchRejectsDuplicateLiveIDs(t *testing.T) {
	app := testAPIApp(t, "")
	mux := http.NewServeMux()
	app.registerHTTPAPI(mux)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/status:batch", bytes.NewBufferString(`{"live_ids":["123","123"]}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), `"code":"duplicate_live_id"`) {
		t.Fatalf("unexpected duplicate response: status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHTTPAPIBatchRejectsOversizedRequestBody(t *testing.T) {
	app := testAPIApp(t, "")
	body := `{"live_ids":["` + strings.Repeat("1", (1<<20)+1) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/status:batch", strings.NewReader(body))
	rec := httptest.NewRecorder()
	app.handleAPI(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), `"code":"request_too_large"`) {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

type readDeadlineRecorder struct {
	*httptest.ResponseRecorder
	mu        sync.Mutex
	deadlines []time.Time
}

func (r *readDeadlineRecorder) SetReadDeadline(deadline time.Time) error {
	r.mu.Lock()
	r.deadlines = append(r.deadlines, deadline)
	r.mu.Unlock()
	return nil
}

func (r *readDeadlineRecorder) readDeadlines() []time.Time {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Time(nil), r.deadlines...)
}

type callbackStatusProbe struct {
	check func() error
}

func (p callbackStatusProbe) CheckLiveStatus(context.Context) (douyinLive.LiveStatus, error) {
	if p.check != nil {
		if err := p.check(); err != nil {
			return douyinLive.LiveStatus{Code: douyinLive.LiveStatusUnknown, LiveID: "123"}, err
		}
	}
	live := true
	hasRoom := true
	return douyinLive.LiveStatus{Code: douyinLive.LiveStatusOnline, Live: &live, HasRoom: &hasRoom, LiveID: "123", RoomID: "9001"}, nil
}

func (p callbackStatusProbe) Dispose() {}

func TestHTTPAPIBatchClearsBodyReadDeadlineBeforeProbing(t *testing.T) {
	app := testAPIApp(t, "")
	defer app.roomManager.Close()
	recorder := &readDeadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
	app.roomManager.probeFactory = func(string, string) (statusProbe, error) {
		return callbackStatusProbe{check: func() error {
			deadlines := recorder.readDeadlines()
			if len(deadlines) < 2 || !deadlines[len(deadlines)-1].IsZero() {
				return errors.New("request body read deadline was not cleared before upstream probe")
			}
			return nil
		}}, nil
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/rooms/status:batch", strings.NewReader(`{"live_ids":["123"]}`))
	app.handleAPI(recorder, req)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	deadlines := recorder.readDeadlines()
	if len(deadlines) < 2 || deadlines[0].IsZero() || !deadlines[len(deadlines)-1].IsZero() {
		t.Fatalf("unexpected read deadline sequence: %#v", deadlines)
	}
}

func TestRoomManagerProbeContextHonorsCallerCancellation(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	for i := 0; i < cap(rm.probeSem); i++ {
		rm.probeSem <- struct{}{}
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := rm.LookupRoom(ctx, "123")
		done <- err
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("LookupRoom error=%v, want cancellation", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("LookupRoom did not honor caller cancellation while waiting for a probe slot")
	}
	for i := 0; i < cap(rm.probeSem); i++ {
		<-rm.probeSem
	}
	rm.Close()
}

func TestAPIErrorForProbeMapping(t *testing.T) {
	if got := apiErrorForProbeResult(douyinLive.LiveStatus{}, douyinLive.ErrRoomNotFound); got.Code != "not_found" {
		t.Fatalf("not-found mapping=%+v", got)
	}
	if got := apiErrorForProbeResult(douyinLive.LiveStatus{}, context.DeadlineExceeded); got.Code != "upstream_timeout" {
		t.Fatalf("timeout mapping=%+v", got)
	}
	if got := statusCodeForProbeResult(douyinLive.LiveStatus{}, context.DeadlineExceeded); got != "unknown" {
		t.Fatalf("timeout status=%q, want unknown", got)
	}
	if got := statusCodeForProbeResult(douyinLive.LiveStatus{Code: douyinLive.LiveStatusNotFound}, nil); got != "not_found" {
		t.Fatalf("status-code-only not-found mapping=%q", got)
	}
	if got := statusCodeForProbeResult(douyinLive.LiveStatus{}, nil); got != "unknown" {
		t.Fatalf("empty status mapping=%q, want unknown", got)
	}
	if got := statusCodeForProbeResult(douyinLive.LiveStatus{Code: "unexpected"}, nil); got != "unknown" {
		t.Fatalf("unexpected status mapping=%q, want unknown", got)
	}
	if !probeResultIsFailure(douyinLive.LiveStatus{}, nil) {
		t.Fatal("empty status should be treated as a probe failure")
	}
}

func TestHTTPAPIRoomListDoesNotProbeUpstream(t *testing.T) {
	app := testAPIApp(t, "")
	room := app.roomManager.GetOrCreateRoom("123", "")
	room.mu.Lock()
	room.knownValid = true
	room.liveName = "主播"
	room.title = "标题"
	room.upstreamReady = false
	room.mu.Unlock()
	rec := performAPIRequest(t, app, http.MethodGet, "/api/v1/rooms", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"live_id":"123"`) || !strings.Contains(rec.Body.String(), `"source":"active"`) {
		t.Fatalf("room list status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, field := range []string{`"client_count":0`, `"upstream_ready":false`, `"status_unknown":false`} {
		if !strings.Contains(rec.Body.String(), field) {
			t.Fatalf("active room response missing stable field %s: %s", field, rec.Body.String())
		}
	}
	app.roomManager.CloseAll()
}

func TestLiveStatusToAPIIncludesRoomIdentity(t *testing.T) {
	status := douyinLive.LiveStatus{Code: douyinLive.LiveStatusOnline, Live: boolPtrForTest(true), HasRoom: boolPtrForTest(true), LiveID: "123", RoomID: "999", UserUniqueID: "777", LiveName: "主播", Title: "标题", AvatarThumb: "avatar"}
	room := liveStatusToAPI(status, "probe", time.Now())
	if room.RoomID != "999" || room.Title != "标题" || room.LiveID != "123" {
		t.Fatalf("room=%+v", room)
	}
	body, err := json.Marshal(room)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"anchor", "user_unique_id", "avatar_thumb", "nickname"} {
		if strings.Contains(string(body), field) {
			t.Fatalf("HTTP room response unexpectedly exposed independent profile field %q: %s", field, body)
		}
	}
	for _, field := range []string{"client_count", "upstream_ready", "status_unknown"} {
		if strings.Contains(string(body), field) {
			t.Fatalf("probe response unexpectedly exposed active-room field %q: %s", field, body)
		}
	}
}

func TestLiveStatusToAPIIncludesAccountWithoutRoom(t *testing.T) {
	status := douyinLive.LiveStatus{Code: douyinLive.LiveStatusNoRoom, Live: boolPtrForTest(false), HasRoom: boolPtrForTest(false), AccountOnly: boolPtrForTest(true), LiveID: "32536162943", UserUniqueID: "777", LiveName: "主播"}
	room := liveStatusToAPI(status, "probe", time.Now())
	if room.Status != "account_no_room" || room.HasRoom == nil || *room.HasRoom {
		t.Fatalf("account-only room=%+v", room)
	}
}

func TestResolveDouyinURLAllowsConfiguredDomainAndRejectsOthers(t *testing.T) {
	resolved, err := resolveDouyinURL("https://live.douyin.com/123456", []string{"douyin.com"})
	if err != nil || resolved.LiveID != "123456" || resolved.CanonicalURL != "https://live.douyin.com/123456" {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if _, err := resolveDouyinURL("https://example.com/123456", []string{"douyin.com"}); err == nil {
		t.Fatal("expected non-douyin domain to be rejected")
	}
	if _, err := resolveDouyinURL("https://live.douyin.com/123456?x=1", []string{"douyin.com"}); err == nil {
		t.Fatal("expected query URL to be rejected")
	}
}

func TestResolveDouyinURLRejectsTraversalAndEncodedSeparators(t *testing.T) {
	for _, raw := range []string{
		"https://live.douyin.com/../123456",
		"https://live.douyin.com/%2e%2e/123456",
		"https://live.douyin.com/foo%2Fbar",
		"https://live.douyin.com/%2F123456",
	} {
		if _, err := resolveDouyinURL(raw, []string{"douyin.com"}); err == nil {
			t.Fatalf("resolveDouyinURL(%q) unexpectedly succeeded", raw)
		}
	}
}

func TestNormalizeAllowedDomainsRejectsURLSyntax(t *testing.T) {
	if got, err := normalizeAllowedDomains([]string{"douyin.com", "LIVE.DOUYIN.COM", "douyin.com"}); err != nil || len(got) != 2 {
		t.Fatalf("normalized domains=%#v err=%v", got, err)
	}
	for _, raw := range []string{"https://douyin.com", "douyin.com/path", "douyin..com", "douyin.com:443", "example.com"} {
		if _, err := normalizeAllowedDomains([]string{raw}); err == nil {
			t.Fatalf("invalid domain %q was accepted", raw)
		}
	}
}

func TestHTTPAPIAdditionalRoutes(t *testing.T) {
	app := testAPIApp(t, "")
	if got := performAPIRequest(t, app, http.MethodGet, "/health", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), `"status":"ok"`) {
		t.Fatalf("health alias status=%d body=%s", got.Code, got.Body.String())
	}
	if got := performAPIRequest(t, app, http.MethodGet, "/metrics", ""); got.Code != http.StatusOK || !strings.Contains(got.Body.String(), "douyinlive_active_rooms") {
		t.Fatalf("metrics status=%d body=%s", got.Code, got.Body.String())
	}
	resolve := performAPIRequest(t, app, http.MethodGet, "/api/v1/rooms/resolve?url=https%3A%2F%2Flive.douyin.com%2F123456", "")
	if resolve.Code != http.StatusOK || !strings.Contains(resolve.Body.String(), `"live_id":"123456"`) {
		t.Fatalf("resolve status=%d body=%s", resolve.Code, resolve.Body.String())
	}
	batch := performAPIRequest(t, app, http.MethodPost, "/api/v1/rooms/status:batch", "")
	if batch.Code != http.StatusBadRequest {
		t.Fatalf("batch invalid body status=%d body=%s", batch.Code, batch.Body.String())
	}
}

func boolPtrForTest(v bool) *bool { return &v }

func TestHTTPAPIHealthETagAndUnifiedEnvelope(t *testing.T) {
	app := testAPIApp(t, "")
	mux := http.NewServeMux()
	app.registerHTTPAPI(mux)

	firstReq := httptest.NewRequest(http.MethodGet, "/health", nil)
	firstRec := httptest.NewRecorder()
	mux.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusOK {
		t.Fatalf("first health status=%d", firstRec.Code)
	}
	var first apiEnvelope
	if err := json.Unmarshal(firstRec.Body.Bytes(), &first); err != nil || !first.OK || first.Error != nil {
		t.Fatalf("unexpected health envelope: %s", firstRec.Body.String())
	}
	etag := firstRec.Header().Get("ETag")
	if etag == "" || firstRec.Header().Get("Cache-Control") == "" || firstRec.Header().Get("X-Request-ID") == "" {
		t.Fatalf("missing cache/request headers: %v", firstRec.Header())
	}

	secondReq := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	secondReq.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	mux.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusNotModified {
		t.Fatalf("expected 304, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if !ifNoneMatchMatches("W/"+etag+", \"other\"", etag) || !ifNoneMatchMatches("*", etag) {
		t.Fatal("If-None-Match standard validators were not recognized")
	}
}

func TestURLResolveSamples(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{"canonical", "https://live.douyin.com/123456", true},
		{"trailing slash", "https://live.douyin.com/123456/", true},
		{"www", "https://www.douyin.com/123456", true},
		{"evil suffix", "https://douyin.com.evil.com/123456", false},
		{"other domain", "https://evil.com/123456", false},
		{"explicit port", "https://live.douyin.com:8080/123456", false},
		{"query", "https://live.douyin.com/123456?x=1", false},
		{"fragment", "https://live.douyin.com/123456#fragment", false},
		{"missing id", "https://live.douyin.com/", false},
		{"nested path", "https://live.douyin.com/category/123456", false},
		{"double slash", "https://live.douyin.com//123456", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveDouyinURL(tc.raw, []string{"douyin.com"})
			if (err == nil) != tc.ok {
				t.Fatalf("raw=%q err=%v", tc.raw, err)
			}
		})
	}
}

func TestHTTPMetricsExposeRequestDuration(t *testing.T) {
	app := testAPIApp(t, "")
	_ = performAPIRequest(t, app, http.MethodGet, "/api/v1/health", "")
	rec := performAPIRequest(t, app, http.MethodGet, "/metrics", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status=%d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "douyinlive_http_request_duration_seconds_sum") ||
		!strings.Contains(body, "douyinlive_http_request_duration_seconds_count") {
		t.Fatalf("duration metrics missing: %s", body)
	}
	if !strings.Contains(body, "douyinlive_probe_upstream_calls_total") || !strings.Contains(body, "douyinlive_probe_merged_waiters_total") {
		t.Fatalf("probe metrics missing: %s", body)
	}
}

type blockingStatusProbe struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingStatusProbe) CheckLiveStatus(ctx context.Context) (douyinLive.LiveStatus, error) {
	p.once.Do(func() { close(p.started) })
	select {
	case <-p.release:
		live := true
		return douyinLive.LiveStatus{Code: douyinLive.LiveStatusOnline, Live: &live, LiveID: "123"}, nil
	case <-ctx.Done():
		return douyinLive.LiveStatus{Code: douyinLive.LiveStatusUnknown, LiveID: "123"}, ctx.Err()
	}
}

func (p *blockingStatusProbe) Dispose() {}

type staticStatusProbe struct {
	status douyinLive.LiveStatus
	err    error
}

func (p staticStatusProbe) CheckLiveStatus(context.Context) (douyinLive.LiveStatus, error) {
	return p.status, p.err
}

func (p staticStatusProbe) Dispose() {}

type shutdownStatusProbe struct {
	started  chan struct{}
	disposed chan struct{}
	once     sync.Once
}

func (p *shutdownStatusProbe) CheckLiveStatus(ctx context.Context) (douyinLive.LiveStatus, error) {
	p.once.Do(func() { close(p.started) })
	<-ctx.Done()
	return douyinLive.LiveStatus{Code: douyinLive.LiveStatusUnknown}, ctx.Err()
}

func (p *shutdownStatusProbe) Dispose() { close(p.disposed) }

func TestHTTPAPIRoomProbeStatusMatrix(t *testing.T) {
	tests := []struct {
		name       string
		status     douyinLive.LiveStatus
		err        error
		wantCode   int
		wantStatus string
		wantBody   string
	}{
		{name: "online", status: douyinLive.LiveStatus{Code: douyinLive.LiveStatusOnline, Live: boolPtrForTest(true), HasRoom: boolPtrForTest(true), LiveID: "online", RoomID: "9001", UserUniqueID: "u1", LiveName: "主播", Title: "标题", AvatarThumb: "avatar"}, wantCode: http.StatusOK, wantStatus: "online", wantBody: `"room_id":"9001"`},
		{name: "offline", status: douyinLive.LiveStatus{Code: douyinLive.LiveStatusOffline, Live: boolPtrForTest(false), HasRoom: boolPtrForTest(true), LiveID: "offline", RoomID: "9002"}, wantCode: http.StatusOK, wantStatus: "offline"},
		{name: "account_no_room", status: douyinLive.LiveStatus{Code: douyinLive.LiveStatusNoRoom, Live: boolPtrForTest(false), HasRoom: boolPtrForTest(false), AccountOnly: boolPtrForTest(true), LiveID: "account", UserUniqueID: "u3", LiveName: "账号"}, wantCode: http.StatusOK, wantStatus: "account_no_room", wantBody: `"has_room":false`},
		{name: "not_found", status: douyinLive.LiveStatus{Code: douyinLive.LiveStatusNotFound, LiveID: "missing"}, err: douyinLive.ErrRoomNotFound, wantCode: http.StatusNotFound, wantStatus: "not_found"},
		{name: "unknown", status: douyinLive.LiveStatus{Code: douyinLive.LiveStatusUnknown, LiveID: "unknown"}, err: douyinLive.ErrLiveStatusUnknown, wantCode: http.StatusServiceUnavailable, wantStatus: "upstream_unverified"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			app := testAPIApp(t, "")
			app.roomManager.probeFactory = func(string, string) (statusProbe, error) {
				return staticStatusProbe{status: tc.status, err: tc.err}, nil
			}
			rec := performAPIRequest(t, app, http.MethodGet, "/api/v1/rooms/"+tc.name, "")
			if rec.Code != tc.wantCode {
				t.Fatalf("status=%d body=%s want=%d", rec.Code, rec.Body.String(), tc.wantCode)
			}
			if !strings.Contains(rec.Body.String(), `"status":"`+tc.wantStatus+`"`) && !strings.Contains(rec.Body.String(), `"code":"`+tc.wantStatus+`"`) {
				t.Fatalf("body=%s missing status/code %q", rec.Body.String(), tc.wantStatus)
			}
			if tc.wantBody != "" && !strings.Contains(rec.Body.String(), tc.wantBody) {
				t.Fatalf("body=%s missing %s", rec.Body.String(), tc.wantBody)
			}
			app.roomManager.Close()
		})
	}
}

func TestRoomManagerProbeLeaderCancellationDoesNotCancelWaiters(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	defer rm.Close()

	probe := &blockingStatusProbe{started: make(chan struct{}), release: make(chan struct{})}
	rm.probeFactory = func(string, string) (statusProbe, error) { return probe, nil }

	leaderCtx, cancelLeader := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := rm.LookupRoom(leaderCtx, "123")
		leaderDone <- err
	}()
	select {
	case <-probe.started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}

	followerDone := make(chan error, 1)
	go func() {
		_, err := rm.LookupRoom(context.Background(), "123")
		followerDone <- err
	}()
	cancelLeader()
	select {
	case err := <-leaderDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("leader error=%v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("leader did not honor cancellation")
	}

	select {
	case err := <-followerDone:
		t.Fatalf("follower returned before shared probe completed: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	close(probe.release)
	select {
	case err := <-followerDone:
		if err != nil {
			t.Fatalf("follower error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("follower did not receive shared probe result")
	}
}

func TestRoomManagerCancelsProbeAfterLastWaiterLeaves(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	defer rm.Close()
	probe := &shutdownStatusProbe{started: make(chan struct{}), disposed: make(chan struct{})}
	rm.probeFactory = func(string, string) (statusProbe, error) { return probe, nil }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := rm.LookupRoom(ctx, "123")
		done <- err
	}()
	select {
	case <-probe.started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("LookupRoom error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LookupRoom did not honor cancellation")
	}
	select {
	case <-probe.disposed:
	case <-time.After(time.Second):
		t.Fatal("shared probe was not canceled and disposed after the last waiter left")
	}
}

func TestRoomManagerProbeFactoryNilProbeReturnsError(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	defer rm.Close()
	rm.probeFactory = func(string, string) (statusProbe, error) { return nil, nil }

	status, err := rm.LookupRoom(context.Background(), "nil-probe")
	if err == nil || !strings.Contains(err.Error(), "nil probe") {
		t.Fatalf("LookupRoom() status=%+v err=%v, want nil-probe error", status, err)
	}
}

func TestRoomManagerCloseWaitsForProbeDisposal(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	probe := &shutdownStatusProbe{started: make(chan struct{}), disposed: make(chan struct{})}
	rm.probeFactory = func(string, string) (statusProbe, error) { return probe, nil }

	lookupDone := make(chan error, 1)
	go func() {
		_, err := rm.LookupRoom(context.Background(), "123")
		lookupDone <- err
	}()
	select {
	case <-probe.started:
	case <-time.After(time.Second):
		t.Fatal("probe did not start")
	}

	rm.Close()
	select {
	case <-probe.disposed:
	default:
		t.Fatal("RoomManager.Close returned before probe Dispose completed")
	}
	select {
	case err := <-lookupDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("LookupRoom error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LookupRoom did not return after manager shutdown")
	}
}
