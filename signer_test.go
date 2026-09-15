package douyinLive

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	tikhub "github.com/jwwsjlm/Tikhub"
)

type failingWebsocketSigner struct {
	err error
}

func (s failingWebsocketSigner) Name() string { return "failing" }

func (s failingWebsocketSigner) Sign(context.Context, string, string, string) (string, error) {
	return "", s.err
}

func (s failingWebsocketSigner) UpdateUserAgent(string) {}

func TestNewDouyinLiveDefaultsLocalSigner(t *testing.T) {
	dl, err := NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	if got := dl.signer.Name(); got != SignProviderLocal {
		t.Fatalf("signer = %q, want %q", got, SignProviderLocal)
	}
}

type trackingReadCloser struct {
	closed bool
}

func (r *trackingReadCloser) Read([]byte) (int, error) { return 0, io.EOF }

func (r *trackingReadCloser) Close() error {
	r.closed = true
	return nil
}

func TestCloseWebSocketHandshakeResponseClosesBody(t *testing.T) {
	body := &trackingReadCloser{}
	closeWebSocketHandshakeResponse(&http.Response{Body: body})
	if !body.closed {
		t.Fatal("closeWebSocketHandshakeResponse() did not close response body")
	}
}

func TestCloseWebSocketHandshakeResponseAcceptsNil(t *testing.T) {
	closeWebSocketHandshakeResponse(nil)
	closeWebSocketHandshakeResponse(&http.Response{})
}

func TestLocalWebsocketSignerKeepsRoomStateIsolatedWithoutEagerGoja(t *testing.T) {
	signerA := newLocalWebsocketSigner().(*localWebsocketSigner)
	signerB := newLocalWebsocketSigner().(*localWebsocketSigner)
	defer signerA.Close()
	defer signerB.Close()

	const (
		uaA     = "Mozilla/5.0 room-a Chrome/150.0.0.0 Safari/537.36"
		uaB     = "Mozilla/5.0 room-b Chrome/150.0.0.0 Safari/537.36"
		cookieA = "ttwid=room-a; msToken=token-a"
		cookieB = "ttwid=room-b; msToken=token-b"
	)
	if err := signerA.Prepare(uaA, cookieA); err != nil {
		t.Fatalf("signerA.Prepare() failed: %v", err)
	}
	if err := signerB.Prepare(uaB, cookieB); err != nil {
		t.Fatalf("signerB.Prepare() failed: %v", err)
	}
	if signerA.native == nil || signerB.native == nil || signerA.native == signerB.native {
		t.Fatal("different rooms do not own isolated native signer state")
	}
	if signerA.runtime != nil || signerB.runtime != nil {
		t.Fatal("native mode eagerly initialized a Goja runtime")
	}
	if signerA.userAgent != uaA || signerA.cookie != cookieA {
		t.Fatalf("signer A profile changed: ua=%q cookie=%q", signerA.userAgent, signerA.cookie)
	}
	if signerB.userAgent != uaB || signerB.cookie != cookieB {
		t.Fatalf("signer B profile changed: ua=%q cookie=%q", signerB.userAgent, signerB.cookie)
	}

	firstA, err := signerA.Sign(context.Background(), "room-a", "user-a", uaA)
	if err != nil || len(firstA) != 16 {
		t.Fatalf("signerA.Sign() = %q, %v", firstA, err)
	}
	if _, err := signerB.Sign(context.Background(), "room-b", "user-b", uaB); err != nil {
		t.Fatalf("signerB.Sign() failed: %v", err)
	}
	secondA, err := signerA.Sign(context.Background(), "room-a", "user-a", uaA)
	if err != nil {
		t.Fatalf("second signerA.Sign() failed: %v", err)
	}
	if len(secondA) != 16 || secondA == firstA {
		t.Fatalf("second signerA.Sign() = %q, want a new 16-character signature", secondA)
	}
	if signerA.userAgent != uaA || signerA.cookie != cookieA {
		t.Fatalf("signer A profile changed after signer B use: ua=%q cookie=%q", signerA.userAgent, signerA.cookie)
	}
}

func TestLocalWebsocketSignerLazilyCreatesAndRotatesFallbackRuntime(t *testing.T) {
	signer := newLocalWebsocketSigner().(*localWebsocketSigner)
	defer signer.Close()

	if err := signer.Prepare("ua-a", "ttwid=a"); err != nil {
		t.Fatalf("Prepare(first) failed: %v", err)
	}
	if signer.runtime != nil {
		t.Fatal("Prepare() eagerly initialized Goja in native mode")
	}
	firstNative := signer.native
	switched, err := signer.ActivateFallback()
	if err != nil || !switched {
		t.Fatalf("ActivateFallback() = %v, %v", switched, err)
	}
	firstRuntime := signer.runtime
	if firstRuntime == nil || signer.Implementation() != "goja_fallback" {
		t.Fatal("fallback mode did not initialize Goja")
	}
	if err := signer.Prepare("ua-a", "ttwid=a"); err != nil {
		t.Fatalf("Prepare(reuse) failed: %v", err)
	}
	if signer.runtime != firstRuntime {
		t.Fatal("unchanged profile rebuilt the JS runtime")
	}

	if err := signer.Prepare("ua-b", "ttwid=a"); err != nil {
		t.Fatalf("Prepare(rotated) failed: %v", err)
	}
	if signer.runtime == firstRuntime {
		t.Fatal("changed UA did not rotate the JS runtime")
	}
	if signer.native != firstNative {
		t.Fatal("profile rotation unexpectedly replaced native signer state")
	}
	if firstRuntime.ProfileMatches("ua-a", "ttwid=a") {
		t.Fatal("old runtime remained active after rotation")
	}
}

func TestLocalWebsocketSignerActivatesFallbackOnlyOnceConcurrently(t *testing.T) {
	signer := newLocalWebsocketSigner().(*localWebsocketSigner)
	defer signer.Close()
	if err := signer.Prepare("ua-concurrent", "ttwid=concurrent"); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}

	const callers = 8
	var waitGroup sync.WaitGroup
	results := make(chan bool, callers)
	errorsCh := make(chan error, callers)
	for range callers {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			switched, err := signer.ActivateFallback()
			results <- switched
			errorsCh <- err
		}()
	}
	waitGroup.Wait()
	close(results)
	close(errorsCh)

	activated := 0
	for switched := range results {
		if switched {
			activated++
		}
	}
	for err := range errorsCh {
		if err != nil {
			t.Fatalf("ActivateFallback() failed: %v", err)
		}
	}
	if activated != 1 {
		t.Fatalf("ActivateFallback() switched %d times, want exactly once", activated)
	}
	if signer.runtime == nil || signer.Implementation() != "goja_fallback" {
		t.Fatal("concurrent fallback activation did not leave signer in Goja fallback mode")
	}
}

func TestLocalWebsocketSignerRejectsMismatchedUserAgent(t *testing.T) {
	signer := newLocalWebsocketSigner().(*localWebsocketSigner)
	defer signer.Close()
	if err := signer.Prepare("ua-a", "ttwid=a"); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}
	if _, err := signer.Sign(context.Background(), "room", "user", "ua-b"); !errors.Is(err, ErrLocalSignerProfile) {
		t.Fatalf("Sign() err = %v, want ErrLocalSignerProfile", err)
	}
}

func TestDisposeClosesLocalWebsocketSigner(t *testing.T) {
	dl, err := NewDouyinLive("live-id", nil, "ttwid=dispose-test")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	signer := dl.signer.(*localWebsocketSigner)
	if err := signer.Prepare(dl.userAgent, dl.getCookieString()); err != nil {
		t.Fatalf("Prepare() failed: %v", err)
	}
	dl.Dispose()
	if _, err := signer.Sign(context.Background(), "room", "user", dl.userAgent); !errors.Is(err, ErrLocalSignerClosed) {
		t.Fatalf("Sign() after Dispose err = %v, want ErrLocalSignerClosed", err)
	}
}

func TestSelectUserAgentAvoidsPreviousValue(t *testing.T) {
	candidates := webImpersonatedUserAgents
	if len(candidates) < 2 {
		t.Skip("requires at least two UA candidates")
	}
	excluded := candidates[0]
	for range 100 {
		if got := selectUserAgent(candidates, excluded); got == excluded {
			t.Fatalf("selectUserAgent() returned excluded UA %q", got)
		}
	}
}

func TestSelectUserAgentReturnsCandidate(t *testing.T) {
	candidates := webImpersonatedUserAgents
	allowed := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		allowed[candidate] = struct{}{}
	}
	for range 100 {
		if _, ok := allowed[selectUserAgent(candidates, "")]; !ok {
			t.Fatalf("selectUserAgent() returned a value outside the candidate pool")
		}
	}
}

// TestSelectUserAgentPinsPCProfile 确认 PC 画像始终返回同一个客户端 UA。
// 抓包实测桌面客户端全程只发送一个固定 UA，因此即使传入旧值也不应轮换。
func TestSelectUserAgentPinsPCProfile(t *testing.T) {
	candidates := ProtocolModePC.userAgents()
	if len(candidates) != 1 {
		t.Fatalf("PC 画像应有 1 个 UA 候选，实际 %d 个", len(candidates))
	}
	for range 10 {
		if got := selectUserAgent(candidates, candidates[0]); got != pcClientUserAgent() {
			t.Fatalf("selectUserAgent() = %q, want PC 客户端 UA", got)
		}
	}
}

func TestNewDouyinLiveWithTikHubUsesTikHubSigner(t *testing.T) {
	dl, err := NewDouyinLiveWithTikHub("live-id", nil, "", "api-key")
	if err != nil {
		t.Fatalf("NewDouyinLiveWithTikHub() failed: %v", err)
	}
	defer dl.Dispose()

	if got := dl.signer.Name(); got != SignProviderTikHub {
		t.Fatalf("signer = %q, want %q", got, SignProviderTikHub)
	}
}

func TestTikHubSignerLogDoesNotExposeKeyDetails(t *testing.T) {
	const token = "PFX!0123456789!SFX"
	var output bytes.Buffer
	dl, err := NewDouyinLiveWithTikHub("live-id", log.New(&output, "", 0), "", token)
	if err != nil {
		t.Fatalf("NewDouyinLiveWithTikHub() failed: %v", err)
	}
	dl.Dispose()

	logs := output.String()
	for _, forbidden := range []string{token, "PFX!", "!SFX", "key_mask", "key_sha256", "key_len"} {
		if strings.Contains(logs, forbidden) {
			t.Fatalf("TikHub status log exposed %q: %s", forbidden, logs)
		}
	}
}

func TestDisposeClosesTikHubWebsocketSigner(t *testing.T) {
	dl, err := NewDouyinLiveWithTikHub("live-id", nil, "", "api-key")
	if err != nil {
		t.Fatalf("NewDouyinLiveWithTikHub() failed: %v", err)
	}
	signer := dl.signer.(*tikhubWebsocketSigner)
	dl.Dispose()
	dl.Dispose()

	signer.mu.Lock()
	closed := signer.closed
	client := signer.client
	token := signer.token
	signer.mu.Unlock()
	if !closed || client != nil || token != "" {
		t.Fatalf("TikHub signer was not fully released: closed=%v client_nil=%v token_empty=%v", closed, client == nil, token == "")
	}
	if _, err := signer.Sign(context.Background(), "room", "user", "Mozilla/5.0"); !errors.Is(err, ErrDouyinLiveClosed) {
		t.Fatalf("Sign() after Dispose err = %v, want ErrDouyinLiveClosed", err)
	}
	signer.UpdateUserAgent("Mozilla/5.0 changed")
	signer.mu.Lock()
	client = signer.client
	signer.mu.Unlock()
	if client != nil {
		t.Fatal("UpdateUserAgent() recreated the TikHub client after Dispose")
	}
}

func TestTikHubWebsocketSignerCloseClosesIdleConnections(t *testing.T) {
	idle := make(chan struct{}, 1)
	closed := make(chan struct{}, 1)
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, "ok")
	}))
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		switch state {
		case http.StateIdle:
			select {
			case idle <- struct{}{}:
			default:
			}
		case http.StateClosed:
			select {
			case closed <- struct{}{}:
			default:
			}
		}
	}
	server.Start()
	defer server.Close()

	signer := newTikHubWebsocketSigner("api-key", "Mozilla/5.0").(*tikhubWebsocketSigner)
	response, err := signer.client.ReqClient().R().Get(server.URL)
	if err != nil {
		signer.Close()
		t.Fatalf("create idle TikHub HTTP connection: %v", err)
	}
	if _, err := response.ToBytes(); err != nil {
		signer.Close()
		t.Fatalf("read TikHub test response: %v", err)
	}
	select {
	case <-idle:
	case <-time.After(time.Second):
		signer.Close()
		t.Fatal("TikHub HTTP connection did not become idle")
	}

	signer.Close()
	signer.Close()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("TikHub signer Close() did not close its idle HTTP connection")
	}
}

func TestTikHubWebsocketSignerCloseDoesNotBlockOrCancelActiveSign(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseResponse := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/douyin/web/generate_wss_xb_signature" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		close(requestStarted)
		<-releaseResponse
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"signature":"ACTIVE_SIGNATURE"}}`)
	}))
	defer server.Close()

	signer := newTikHubWebsocketSigner("api-key", "Mozilla/5.0").(*tikhubWebsocketSigner)
	signer.mu.Lock()
	oldClient := signer.client
	signer.client = tikhub.NewClient(
		"api-key",
		tikhub.WithBaseURL(server.URL),
		tikhub.WithTimeout(5*time.Second),
		tikhub.WithUserAgent("Mozilla/5.0"),
	)
	signer.mu.Unlock()
	closeHTTPClientIdleConnections(oldClient.ReqClient())

	type signResult struct {
		signature string
		err       error
	}
	resultCh := make(chan signResult, 1)
	go func() {
		signature, err := signer.Sign(context.Background(), "room-id", "user-id", "Mozilla/5.0")
		resultCh <- signResult{signature: signature, err: err}
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("TikHub Sign request did not reach the test server")
	}

	closeDone := make(chan struct{})
	go func() {
		signer.Close()
		close(closeDone)
	}()
	select {
	case <-closeDone:
	case <-time.After(200 * time.Millisecond):
		close(releaseResponse)
		t.Fatal("Close() blocked on an active TikHub Sign request")
	}
	select {
	case result := <-resultCh:
		t.Fatalf("active Sign completed before the server released it: signature=%q err=%v", result.signature, result.err)
	default:
	}

	close(releaseResponse)
	select {
	case result := <-resultCh:
		if result.err != nil || result.signature != "ACTIVE_SIGNATURE" {
			t.Fatalf("active Sign after concurrent Close = %q, %v", result.signature, result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("active Sign did not complete after its response was released")
	}
}

func TestTikHubSignerRequiresToken(t *testing.T) {
	signer := newTikHubWebsocketSigner("", "Mozilla/5.0")

	_, err := signer.Sign(context.Background(), "room-id", "user-id", "Mozilla/5.0")
	if !errors.Is(err, ErrTikHubTokenEmpty) {
		t.Fatalf("Sign() err = %v, want ErrTikHubTokenEmpty", err)
	}
}

func TestBuildWebsocketURLReturnsSignerError(t *testing.T) {
	wantErr := errors.New("sign failed")
	dl, err := newDouyinLive("live-id", nil, "", failingWebsocketSigner{err: wantErr})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	_, err = dl.buildWebsocketURL()
	if !errors.Is(err, wantErr) {
		t.Fatalf("buildWebsocketURL() err = %v, want %v", err, wantErr)
	}
}

func TestBuildWebsocketURLEscapesSignerOutput(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "ab+c/d e"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	url, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if !strings.Contains(url, "signature=ab+c/d%20e") {
		t.Fatalf("buildWebsocketURL() did not escape signature correctly: %s", url)
	}
}

func TestBuildWebsocketURLUsesCurrentWebcastSDKVersion(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	url, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if !strings.Contains(url, "webcast_sdk_version=1.0.15") {
		t.Fatalf("buildWebsocketURL() missing current SDK version: %s", url)
	}
	if !strings.Contains(url, "update_version_code=1.0.15") {
		t.Fatalf("buildWebsocketURL() missing current update version: %s", url)
	}
	if strings.Contains(url, "1.0.14-beta.0") {
		t.Fatalf("buildWebsocketURL() contains stale SDK version: %s", url)
	}
}

func TestWebsocketDialContextReusesPreparedContext(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")
	dl.headers.Set("User-Agent", dl.userAgent)
	dl.contextPrepared = true

	wsURL, headers, err := dl.websocketDialContext()
	if err != nil {
		t.Fatalf("websocketDialContext() failed with prepared context: %v", err)
	}
	if wsURL == "" {
		t.Fatal("websocketDialContext() returned an empty URL")
	}
	if headers.Get("User-Agent") == "" {
		t.Fatal("websocketDialContext() returned headers without User-Agent")
	}
}

func TestBuildWebsocketURLUsesTrackedCursorAndInternalExt(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")
	dl.wsCursor = "cursor-from-response"
	dl.wsInternalExt = "internal_src:pushserver|seq:9|wss_msg_type:r"

	url, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if !strings.Contains(url, "cursor=cursor-from-response") {
		t.Fatalf("buildWebsocketURL() did not use tracked cursor: %s", url)
	}
	if !strings.Contains(url, "internal_ext=internal_src:pushserver|seq:9|wss_msg_type:r") {
		t.Fatalf("buildWebsocketURL() did not use tracked internal_ext: %s", url)
	}
}

func TestBuildInitialIMFetchParamsMatchesBrowserShape(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	params := dl.buildInitialIMFetchParams(roomInfoSnapshot{
		roomID: "room-id",
		pushID: "user-id",
	}, "ms-token")

	for _, want := range []string{
		"resp_content_type=protobuf",
		"endpoint=live_pc",
		"support_wrds=1",
		"user_unique_id=user-id",
		"room_id=room-id",
		"version_code=180800",
		"live_id=1",
		"aid=6383",
		"fetch_rule=1",
		"cursor=",
		"internal_ext=",
		"browser_name=Mozilla",
		"browser_version=5.0+%28Windows+NT+10.0%3B+Win64%3B+x64%29+AppleWebKit%2F537.36+%28KHTML%2C+like+Gecko%29+Chrome%2F150.0.0.0+Safari%2F537.36",
		"msToken=ms-token",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("buildInitialIMFetchParams() missing %q in %s", want, params)
		}
	}
}

func TestInitialIMFetchParamsIncludeAllBrowserObservedKeys(t *testing.T) {
	params := newInitialIMFetchParams(roomInfoSnapshot{
		roomID: "room-id",
		pushID: "user-id",
	}, "Mozilla/5.0 UA", "ms-token")
	query := params.QueryString()

	wantKeys := []string{
		"resp_content_type",
		"did_rule",
		"device_id",
		"app_name",
		"endpoint",
		"support_wrds",
		"user_unique_id",
		"identity",
		"need_persist_msg_count",
		"insert_task_id",
		"live_reason",
		"room_id",
		"version_code",
		"last_rtt",
		"live_id",
		"aid",
		"fetch_rule",
		"cursor",
		"internal_ext",
		"device_platform",
		"cookie_enabled",
		"screen_width",
		"screen_height",
		"browser_language",
		"browser_platform",
		"browser_name",
		"browser_version",
		"browser_online",
		"tz_name",
		"msToken",
	}
	if got := queryKeys(query); strings.Join(got, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("QueryString() keys = %#v, want %#v", got, wantKeys)
	}
	for _, key := range wantKeys {
		if !strings.Contains(query, key+"=") {
			t.Fatalf("QueryString() missing %q in %s", key+"=", query)
		}
	}
	if !strings.Contains(query, "device_id=&") {
		t.Fatalf("device_id should be present and empty: %s", query)
	}
	if strings.Contains(query, "webcast_sdk_version") {
		t.Fatalf("initial im fetch params should not include webcast_sdk_version: %s", query)
	}
	if strings.Contains(query, "%20") {
		t.Fatalf("initial im fetch params should use URLSearchParams-style + spaces: %s", query)
	}
}

func TestWebsocketSignatureParamsMatchBrowserCapture(t *testing.T) {
	params := newWebsocketSignatureParams("7659772534023654196", "7659776308930922010")

	wantJoined := "live_id=1,aid=6383,version_code=180800,webcast_sdk_version=1.0.15,room_id=7659772534023654196,sub_room_id=,sub_channel_id=,did_rule=3,user_unique_id=7659776308930922010,device_platform=web,device_type=,ac=,identity=audience"
	if got := params.Joined(); got != wantJoined {
		t.Fatalf("Joined() = %q, want %q", got, wantJoined)
	}
	if got := params.XMSStub(); got != "94d8b625e851f0a1f70db875514e621c" {
		t.Fatalf("XMSStub() = %q", got)
	}
}

func TestWebsocketURLParamsDoNotIncludeDeviceID(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")

	wsURL, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if strings.Contains(wsURL, "device_id") {
		t.Fatalf("websocket URL must not include device_id: %s", wsURL)
	}
}

func TestWebsocketURLParamsUseBrowserWebsocketEscaping(t *testing.T) {
	params := newWebsocketURLParams(roomInfoSnapshot{
		roomID: "room-id",
		pushID: "user-id",
	}, "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36", "cursor", "internal", "64PfoT6J/GCWD+wr")

	query := params.QueryString()
	if !strings.Contains(query, "browser_version=5.0%20(Windows%20NT%2010.0;%20Win64;%20x64)%20AppleWebKit/537.36%20(KHTML,%20like%20Gecko)%20Chrome/150.0.0.0%20Safari/537.36") {
		t.Fatalf("browser_version should use browser websocket escaping: %s", query)
	}
	if !strings.Contains(query, "signature=64PfoT6J/GCWD+wr") {
		t.Fatalf("signature should use browser websocket escaping: %s", query)
	}
}

func TestWebsocketURLParamsMatchBrowserKeyOrder(t *testing.T) {
	params := newWebsocketURLParams(roomInfoSnapshot{
		roomID: "room-id",
		pushID: "user-id",
	}, "Mozilla/5.0 UA", "cursor", "internal", "signature")

	wantKeys := []string{
		"app_name",
		"version_code",
		"webcast_sdk_version",
		"update_version_code",
		"compress",
		"device_platform",
		"cookie_enabled",
		"screen_width",
		"screen_height",
		"browser_language",
		"browser_platform",
		"browser_name",
		"browser_version",
		"browser_online",
		"tz_name",
		"cursor",
		"internal_ext",
		"host",
		"aid",
		"live_id",
		"did_rule",
		"endpoint",
		"support_wrds",
		"user_unique_id",
		"im_path",
		"identity",
		"need_persist_msg_count",
		"insert_task_id",
		"live_reason",
		"room_id",
		"heartbeatDuration",
		"signature",
	}
	if got := queryKeys(params.QueryString()); strings.Join(got, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("websocket keys = %#v, want %#v", got, wantKeys)
	}
}

func TestBuildWebsocketURLUsesTrackedPushServer(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("room-id", "user-id", "live-name", "title", "avatar")
	dl.wsPushURL = "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/"

	wsURL, err := dl.buildWebsocketURL()
	if err != nil {
		t.Fatalf("buildWebsocketURL() failed: %v", err)
	}
	if !strings.HasPrefix(wsURL, "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/?") {
		t.Fatalf("buildWebsocketURL() did not use tracked push server: %s", wsURL)
	}
}

func TestNormalizeWebsocketPushURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "host", in: "webcast100-ws-web-hl.douyin.com", want: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/"},
		{name: "https", in: "https://webcast100-ws-web-hl.douyin.com", want: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/"},
		{name: "full wss", in: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/", want: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/"},
		{name: "list", in: "webcast100-ws-web-hl.douyin.com,webcast5-ws-web-lf.douyin.com", want: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeWebsocketPushURL(tt.in); got != tt.want {
				t.Fatalf("normalizeWebsocketPushURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

type staticWebsocketSigner struct {
	signature string
}

func (s staticWebsocketSigner) Name() string { return "static" }

func (s staticWebsocketSigner) Sign(context.Context, string, string, string) (string, error) {
	return s.signature, nil
}

func (s staticWebsocketSigner) UpdateUserAgent(string) {}

func TestExtractTikHubSignature(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "data string", body: `{"code":200,"request_id":"req-id","data":"XB_SIGNATURE"}`, want: "XB_SIGNATURE"},
		{name: "data object xb", body: `{"code":200,"data":{"xb":"XB_OBJECT"}}`, want: "XB_OBJECT"},
		{name: "data object x bogus", body: `{"code":200,"data":{"X-Bogus":"XB_BOGUS"}}`, want: "XB_BOGUS"},
		{name: "ignore request id", body: `{"code":200,"request_id":"req-id","message":"Request successful."}`, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractTikHubSignature([]byte(tt.body)); got != tt.want {
				t.Fatalf("extractTikHubSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeTikHubSignature(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "raw", in: "XB_RAW", want: "XB_RAW"},
		{name: "query signature", in: "signature=XB_QUERY", want: "XB_QUERY"},
		{name: "query x bogus", in: "X-Bogus=XB_BOGUS", want: "XB_BOGUS"},
		{name: "url", in: "https://example.com/path?signature=XB_URL", want: "XB_URL"},
		{name: "raw plus", in: "XB+RAW/VALUE", want: "XB+RAW/VALUE"},
		{name: "escaped raw", in: "XB%2BVALUE", want: "XB+VALUE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTikHubSignature(tt.in); got != tt.want {
				t.Fatalf("normalizeTikHubSignature() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTikHubAPIResponseSuccessCode(t *testing.T) {
	tests := []struct {
		name string
		code int
		want bool
	}{
		{name: "zero", code: 0, want: true},
		{name: "http ok", code: 200, want: true},
		{name: "unauthorized", code: 401, want: false},
		{name: "business error", code: 10001, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTikHubSuccessCode(tt.code); got != tt.want {
				t.Fatalf("isTikHubSuccessCode(%d) = %v, want %v", tt.code, got, tt.want)
			}
		})
	}
}

func TestLiveStatusGuardRequiresConsecutiveOfflineConfirmations(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(true) {
		t.Fatalf("Record(true) closed connection")
	}
	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if !guard.Record(false) {
		t.Fatalf("second consecutive offline confirmation did not close connection")
	}
}

func TestLiveStatusGuardResetsAfterOnlineConfirmation(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if guard.Record(true) {
		t.Fatalf("online confirmation closed connection")
	}
	if guard.Record(false) {
		t.Fatalf("offline confirmation after reset closed connection")
	}
}

func TestLiveStatusGuardCanResetForNewConnection(t *testing.T) {
	guard := liveStatusGuard{}

	if guard.Record(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	guard.Reset()
	if guard.Record(false) {
		t.Fatalf("offline confirmation after connection reset closed connection")
	}
}

func TestStatusCheckKeepsLiveStateUntilSecondOfflineConfirmation(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("first offline confirmation changed live state")
	}
	if !dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("second offline confirmation did not close connection")
	}
	if dl.isLiveStatus() {
		t.Fatalf("second offline confirmation did not change live state")
	}
}

func TestStatusCheckOnlineConfirmationResetsOfflineGuard(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	if dl.shouldCloseAfterStatusCheck(true) {
		t.Fatalf("online confirmation closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("online confirmation changed live state")
	}
	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("offline confirmation after online reset closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("offline confirmation after online reset changed live state")
	}
}

func TestSetLiveStatusOnlineResetsOfflineGuard(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)

	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("first offline confirmation closed connection")
	}
	dl.setLiveStatus(true)
	if dl.shouldCloseAfterStatusCheck(false) {
		t.Fatalf("offline confirmation after setLiveStatus(true) closed connection")
	}
	if !dl.isLiveStatus() {
		t.Fatalf("offline confirmation after setLiveStatus(true) changed live state")
	}
}
