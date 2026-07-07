package douyinLive

import (
	"context"
	"errors"
	"strings"
	"testing"
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
