package douyinLive

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"google.golang.org/protobuf/proto"
)

type captureLogSink struct {
	messages []string
	args     [][]interface{}
}

func (l *captureLogSink) Print(v ...interface{})                 {}
func (l *captureLogSink) Printf(format string, v ...interface{}) {}
func (l *captureLogSink) Println(v ...interface{})               {}

func (l *captureLogSink) Debug(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
	l.args = append(l.args, args)
}

func (l *captureLogSink) Info(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
	l.args = append(l.args, args)
}

func (l *captureLogSink) Warn(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
	l.args = append(l.args, args)
}

func (l *captureLogSink) Error(msg string, args ...interface{}) {
	l.messages = append(l.messages, msg)
	l.args = append(l.args, args)
}

func TestQueryEscapeValuePreservesSignatureCharacters(t *testing.T) {
	got := queryEscapeValue("ab+c/d e")
	want := "ab%2Bc%2Fd%20e"
	if got != want {
		t.Fatalf("queryEscapeValue() = %q, want %q", got, want)
	}
}

func TestLogFlowArgsAddsStageAndStep(t *testing.T) {
	got := logFlowArgs("ws", "dial", "live_id", "161022647108", "status_code", 101)
	want := []interface{}{"stage", "ws", "step", "dial", "live_id", "161022647108", "status_code", 101}
	if fmt.Sprint(got...) != fmt.Sprint(want...) {
		t.Fatalf("logFlowArgs() = %#v, want %#v", got, want)
	}
}

func TestWebsocketHostForLogDoesNotExposeQuery(t *testing.T) {
	got := websocketHostForLog("wss://webcast100-ws-web-lq.douyin.com/webcast/im/push/v2/?signature=secret")
	want := "webcast100-ws-web-lq.douyin.com"
	if got != want {
		t.Fatalf("websocketHostForLog() = %q, want %q", got, want)
	}
}

func TestClassifyReadErrorDetectsTimeout(t *testing.T) {
	err := &net.DNSError{IsTimeout: true}
	if got := classifyReadError(err); got != "timeout" {
		t.Fatalf("classifyReadError() = %q, want timeout", got)
	}
}

func TestParseRoomInfoSupportsRoomObject(t *testing.T) {
	body := `{
		"data": {
			"room": {
				"id": "room-id",
				"title": "room-title",
				"owner": {
					"id": "owner-id",
					"nickname": "owner-name",
					"avatar_thumb": {
						"url_list": ["avatar-0", "avatar-1", "avatar-2"]
					}
				}
			}
		}
	}`

	info, err := parseRoomInfo(body)
	if err != nil {
		t.Fatalf("parseRoomInfo() failed: %v", err)
	}
	if info.roomID != "room-id" || info.pushID != "owner-id" {
		t.Fatalf("unexpected ids: roomID=%q pushID=%q", info.roomID, info.pushID)
	}
	if info.liveName != "owner-name" || info.title != "room-title" || info.avatarThumb != "avatar-2" {
		t.Fatalf("unexpected room info: %#v", info)
	}
}

func TestParseRoomIDFromLivePageSupportsHTMLQueryState(t *testing.T) {
	html := `anchor_id_str=64995611209&amp;enter_method=direct_open&amp;room_id=7659792511015177001&amp;sec_anchor_id=abc`
	if got := parseRoomIDFromLivePage(html); got != "7659792511015177001" {
		t.Fatalf("parseRoomIDFromLivePage() = %q", got)
	}
}

func TestParseRoomIDFromLivePageSupportsJSONState(t *testing.T) {
	html := `{"room_id":"7659792511015177001","status":2}`
	if got := parseRoomIDFromLivePage(html); got != "7659792511015177001" {
		t.Fatalf("parseRoomIDFromLivePage() = %q", got)
	}
}

func TestParseRoomIDFromLivePageSupportsRoomStoreState(t *testing.T) {
	html := `\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659792511015177001\",\"status\":2}}}`
	if got := parseRoomIDFromLivePage(html); got != "7659792511015177001" {
		t.Fatalf("parseRoomIDFromLivePage() = %q", got)
	}
}

func TestParseRoomIDFromLivePageSupportsStatusOnlyPayload(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"id_str\":\"7659631024187493163\",\"status\":2,\"status_str\":\"2\",\"title\":\"直播标题\",\"user_count_str\":\"68\","])`
	if got := parseRoomIDFromLivePage(html); got != "7659631024187493163" {
		t.Fatalf("parseRoomIDFromLivePage() = %q", got)
	}
}

func TestParseRoomIDFromLivePageSupportsEndedGiftEffectID(t *testing.T) {
	html := `<div id="gift_effect_bg_7659772543859215154"></div><span>直播已结束</span>`
	if got := parseRoomIDFromLivePage(html); got != "7659772543859215154" {
		t.Fatalf("parseRoomIDFromLivePage() = %q", got)
	}
}

func TestParseRoomInfoFromLivePageSupportsEmbeddedRoomStore(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659786040097426226\",\"status\":2,\"status_str\":\"2\",\"title\":\"梦三国娱乐解说\",\"owner\":{\"id_str\":\"101220697463\",\"nickname\":\"走秀（梦三国解说）\",\"avatar_thumb\":{\"url_list\":[\"https://p3.douyinpic.com/avatar.jpeg\",\"https://p11.douyinpic.com/avatar.jpeg\"]}}}}}}"])`

	info := parseRoomInfoFromLivePage(html)
	if info.roomID != "7659786040097426226" {
		t.Fatalf("roomID = %q", info.roomID)
	}
	if info.pushID != "101220697463" {
		t.Fatalf("pushID = %q", info.pushID)
	}
	if info.liveName != "走秀（梦三国解说）" {
		t.Fatalf("liveName = %q", info.liveName)
	}
	if info.title != "梦三国娱乐解说" {
		t.Fatalf("title = %q", info.title)
	}
	if info.avatarThumb != "https://p11.douyinpic.com/avatar.jpeg" {
		t.Fatalf("avatarThumb = %q", info.avatarThumb)
	}
}

func TestParseRoomInfoFromLivePageUsesRoomInfoAnchorSibling(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659772543859215154\",\"status\":4,\"status_str\":\"4\",\"title\":\"offline-room-title\"},\"roomId\":\"7659772543859215154\",\"web_rid\":\"386395296025\",\"anchor\":{\"id_str\":\"68252455312\",\"nickname\":\"CACA-anchor\",\"avatar_thumb\":{\"url_list\":[\"https://p3.douyinpic.com/a.jpeg\",\"https://p11.douyinpic.com/b.jpeg\",\"https://p26.douyinpic.com/c.jpeg\"]}}}}}"])`

	info := parseRoomInfoFromLivePage(html)
	if info.roomID != "7659772543859215154" {
		t.Fatalf("roomID = %q", info.roomID)
	}
	if info.pushID != "68252455312" {
		t.Fatalf("pushID = %q", info.pushID)
	}
	if info.liveName != "CACA-anchor" {
		t.Fatalf("liveName = %q", info.liveName)
	}
	if info.title != "offline-room-title" {
		t.Fatalf("title = %q", info.title)
	}
	if info.avatarThumb != "https://p26.douyinpic.com/c.jpeg" {
		t.Fatalf("avatarThumb = %q", info.avatarThumb)
	}
}

func TestParseRoomInfoFromLivePageSkipsEmptyRoomInfoBeforeValidState(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{},\"liveStatus\":\"normal\"}}"])
self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659772543859215154\",\"status\":4,\"status_str\":\"4\",\"title\":\"offline-room-title\"},\"roomId\":\"7659772543859215154\",\"web_rid\":\"386395296025\",\"anchor\":{\"id_str\":\"68252455312\",\"nickname\":\"CACA-anchor\",\"avatar_thumb\":{\"url_list\":[\"https://p3.douyinpic.com/a.jpeg\",\"https://p11.douyinpic.com/b.jpeg\",\"https://p26.douyinpic.com/c.jpeg\"]}}}}}"])`

	info := parseRoomInfoFromLivePage(html)
	if info.roomID != "7659772543859215154" {
		t.Fatalf("roomID = %q", info.roomID)
	}
	if info.liveName != "CACA-anchor" {
		t.Fatalf("liveName = %q", info.liveName)
	}
	if info.title != "offline-room-title" {
		t.Fatalf("title = %q", info.title)
	}
}

func TestParseLivePageStateAcceptsAnchorOnlyRoomInfoAsOfflineAccount(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{},\"liveStatus\":\"normal\"}}"])
self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"roomId\":\"\",\"web_rid\":\"32536162943\",\"anchor\":{\"id_str\":\"3872957772872119\",\"nickname\":\"anchor-only-name\",\"avatar_thumb\":{\"url_list\":[\"https://p3.douyinpic.com/a.jpeg\"]}}}}}"])`

	state := parseLivePageState(html)
	if state.hasRoomIdentity() {
		t.Fatalf("hasRoomIdentity() = true, want false for anchor-only page: %#v", state)
	}
	if !state.hasKnownPageIdentity() {
		t.Fatalf("hasKnownPageIdentity() = false, want true for anchor-only page: %#v", state)
	}
	if !state.hasAnchorIdentity {
		t.Fatalf("hasAnchorIdentity = false, want true: %#v", state)
	}
	if !state.statusKnown || state.isLive {
		t.Fatalf("status = (%v, %v), want known offline", state.isLive, state.statusKnown)
	}
	if state.info.liveName != "anchor-only-name" {
		t.Fatalf("liveName = %q", state.info.liveName)
	}
	if state.info.roomID != "" {
		t.Fatalf("roomID = %q, want empty until room appears", state.info.roomID)
	}
}

func TestParseLivePageStateTreatsAnchorWithoutRoomObjectAsAccountOffline(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"roomId\":\"7659700000000000000\",\"web_rid\":\"32536162943\",\"anchor\":{\"id_str\":\"3872957772872119\",\"nickname\":\"?????\",\"avatar_thumb\":{\"url_list\":[\"https://p3.douyinpic.com/a.jpeg\"]}}}}}"])`

	state := parseLivePageState(html)
	if state.hasRoomIdentity() {
		t.Fatalf("hasRoomIdentity() = true, want false when roomInfo.room is missing: %#v", state)
	}
	if !state.info.anchorOnly || !state.hasAnchorIdentity {
		t.Fatalf("anchor-only identity not detected: %#v", state)
	}
	if !state.statusKnown || state.isLive {
		t.Fatalf("status = (%v, %v), want known offline account", state.isLive, state.statusKnown)
	}
	if state.info.roomID != "" {
		t.Fatalf("roomID = %q, want empty because roomInfo.room is missing", state.info.roomID)
	}
	if state.info.liveName != "?????" {
		t.Fatalf("liveName = %q", state.info.liveName)
	}
}

func TestParseRoomInfoFromLivePagePrefersRoomObjectOverEarlierAnchorOnlyState(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"web_rid\":\"386395296025\",\"anchor\":{\"id_str\":\"68252455312\",\"nickname\":\"CACA??\"}}}}"])
self.__pace_f.push([1,"{\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659772543859215154\",\"status\":4,\"status_str\":\"4\",\"title\":\"?????\"},\"anchor\":{\"id_str\":\"68252455312\",\"nickname\":\"CACA??\"}}}}}"])`

	state := parseLivePageState(html)
	if !state.hasRoomIdentity() {
		t.Fatalf("hasRoomIdentity() = false, want room object to win: %#v", state)
	}
	if state.info.anchorOnly {
		t.Fatalf("anchorOnly = true, want false when later roomInfo.room exists: %#v", state)
	}
	if state.info.roomID != "7659772543859215154" || state.info.liveName != "CACA??" || state.info.title != "?????" {
		t.Fatalf("unexpected parsed room info: %#v", state.info)
	}
}

func TestParseUserUniqueIDFromLivePageSupportsLogState(t *testing.T) {
	html := `setPageViewLog({"odin":"{\"user_id\":\"1561766825835499\",\"user_unique_id\":\"7659797852999091746\"}"});`
	if got := parseUserUniqueIDFromLivePage(html); got != "7659797852999091746" {
		t.Fatalf("parseUserUniqueIDFromLivePage() = %q", got)
	}
}

func TestParseLiveStatusFromLivePageSupportsRoomStoreState(t *testing.T) {
	html := `\"roomStore\":{\"roomInfo\":{\"room\":{\"id_str\":\"7659792511015177001\",\"status\":2,\"status_str\":\"2\"}}}`
	status, ok := parseLiveStatusFromLivePage(html, "7659792511015177001")
	if !ok || !status {
		t.Fatalf("parseLiveStatusFromLivePage() = (%v, %v), want (true, true)", status, ok)
	}
}

func TestParseLiveStatusFromLivePageSupportsIssue14EscapedStatusRegex(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"id_str\":\"7659831978830154559\",\"status\":2,\"status_str\":\"2\",\"title\":\"3070ti 到货\",\"user_count_str\":\"68\","])`
	status, ok := parseLiveStatusFromLivePage(html, "7659831978830154559")
	if !ok || !status {
		t.Fatalf("parseLiveStatusFromLivePage() = (%v, %v), want (true, true)", status, ok)
	}
}

func TestParseLiveStatusFromLivePageTreatsNonTwoStatusAsOffline(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"id_str\":\"7659772543859215154\",\"status\":4,\"status_str\":\"4\",\"title\":\"CACA呆夫\",\"user_count_str\":\"\","])`
	status, ok := parseLiveStatusFromLivePage(html, "7659772543859215154")
	if !ok || status {
		t.Fatalf("parseLiveStatusFromLivePage() = (%v, %v), want (false, true)", status, ok)
	}
}

func TestParseLiveStatusFromLivePageDetectsEndedRoomText(t *testing.T) {
	html := `<title>CACA呆夫（无畏契约）的抖音直播间 - 抖音直播</title><div id="gift_effect_bg_7659772543859215154"></div><span>直播已结束</span>`
	status, ok := parseLiveStatusFromLivePage(html, "7659772543859215154")
	if !ok || status {
		t.Fatalf("parseLiveStatusFromLivePage() = (%v, %v), want (false, true)", status, ok)
	}
}

func TestInvalidLivePageWithOnlyUserUniqueIDIsNotAValidRoom(t *testing.T) {
	html := `<html><script>window.__log={"user_unique_id":"7659776308930922010"}; window.endpoint="/webcast/room/web/enter/";</script><body></body></html>`
	if got := parseRoomIDFromLivePage(html); got != "" {
		t.Fatalf("parseRoomIDFromLivePage() = %q, want empty for invalid page without room state", got)
	}
	if _, ok := parseLiveStatusFromLivePage(html, ""); ok {
		t.Fatal("parseLiveStatusFromLivePage() reported a known status for an invalid page without room state")
	}
	state := parseLivePageState(html)
	if state.hasRoomIdentity() {
		t.Fatalf("parseLivePageState() marked invalid page as valid: %#v", state)
	}
	if state.userUniqueID != "7659776308930922010" {
		t.Fatalf("userUniqueID = %q", state.userUniqueID)
	}
}

func TestParseLivePageStateAcceptsEndedRoomStatusAsExistingRoom(t *testing.T) {
	html := `self.__pace_f.push([1,"{\"id_str\":\"7659772543859215154\",\"status\":4,\"status_str\":\"4\",\"title\":\"CACA呆夫\",\"user_count_str\":\"\","])`
	state := parseLivePageState(html)
	if !state.hasRoomIdentity() {
		t.Fatalf("parseLivePageState() did not mark ended room as valid: %#v", state)
	}
	if state.info.roomID != "7659772543859215154" {
		t.Fatalf("roomID = %q", state.info.roomID)
	}
	if !state.statusKnown || state.isLive {
		t.Fatalf("status = (%v, %v), want known offline", state.isLive, state.statusKnown)
	}
}

func TestSetLivePageIDsKeepsExistingMetadata(t *testing.T) {
	dl := &DouyinLive{}
	dl.updateRoomInfo("", "push-id", "live-name", "title", "avatar")

	dl.setLivePageIDs("room-id", "page-push-id")
	info := dl.roomInfoSnapshot()
	if info.roomID != "room-id" || info.pushID != "page-push-id" || info.liveName != "live-name" || info.title != "title" || info.avatarThumb != "avatar" {
		t.Fatalf("unexpected room info after setLivePageIDs: %#v", info)
	}
}

func TestRoomEnterUpdatePreservesLivePageUserUniqueID(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLivePageIDs("page-room-id", "page-user-unique-id")

	dl.updateRoomInfoFromEnter(roomInfoSnapshot{
		roomID:      "enter-room-id",
		pushID:      "anchor-id",
		liveName:    "live-name",
		title:       "title",
		avatarThumb: "avatar",
	})

	info := dl.roomInfoSnapshot()
	if info.roomID != "enter-room-id" {
		t.Fatalf("roomID = %q", info.roomID)
	}
	if info.pushID != "page-user-unique-id" {
		t.Fatalf("pushID = %q, want page user_unique_id", info.pushID)
	}
	if info.liveName != "live-name" || info.title != "title" || info.avatarThumb != "avatar" {
		t.Fatalf("metadata not updated: %#v", info)
	}
}

func TestRoomEnterUpdatePreservesLivePageMetadataWhenEnterMissingDisplayFields(t *testing.T) {
	dl := &DouyinLive{}
	dl.updateRoomInfoFromLivePage(roomInfoSnapshot{
		roomID:      "page-room-id",
		pushID:      "page-user-unique-id",
		liveName:    "CACA-anchor",
		title:       "offline-room-title",
		avatarThumb: "page-avatar",
	})

	dl.updateRoomInfoFromEnter(roomInfoSnapshot{
		roomID: "enter-room-id",
		pushID: "enter-user-id",
	})

	info := dl.roomInfoSnapshot()
	if info.roomID != "enter-room-id" {
		t.Fatalf("roomID = %q", info.roomID)
	}
	if info.pushID != "page-user-unique-id" {
		t.Fatalf("pushID = %q, want page user_unique_id", info.pushID)
	}
	if info.liveName != "CACA-anchor" || info.title != "offline-room-title" || info.avatarThumb != "page-avatar" {
		t.Fatalf("metadata was overwritten by empty web/enter fields: %#v", info)
	}
}

func TestRoomEnterEmptyResponseDoesNotRetryWhenFallbackAvailable(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLiveStatus(true)
	dl.setLivePageIDs("7659792511015177001", "7601036345435309606")

	err := fmt.Errorf("%w status=200 content_type=%q content_length=0 raw_len=0", errRoomInfoEmpty, "application/json")
	if dl.shouldRetryRoomEnter(err) {
		t.Fatal("shouldRetryRoomEnter() = true, want false when live page state can be used")
	}

	body, ok := dl.roomEnterFallbackBody(err)
	if !ok {
		t.Fatal("roomEnterFallbackBody() did not provide fallback body")
	}
	info, parseErr := parseRoomInfo(body)
	if parseErr != nil {
		t.Fatalf("fallback body does not parse: %v", parseErr)
	}
	if info.roomID != "7659792511015177001" || info.pushID != "7601036345435309606" {
		t.Fatalf("fallback ids = (%q, %q)", info.roomID, info.pushID)
	}
}

func TestRoomEnterEmptyResponseDoesNotRetryWhenKnownOffline(t *testing.T) {
	dl := &DouyinLive{}
	dl.liveID = "386395296025"
	dl.setLivePageIDs("7659772543859215154", "")
	dl.setLiveStatus(false)

	err := fmt.Errorf("%w status=200 content_type=%q content_length=0 raw_len=0", errRoomInfoEmpty, "application/json")
	if dl.shouldRetryRoomEnter(err) {
		t.Fatal("shouldRetryRoomEnter() = true, want false when live page confirms ended/offline status")
	}
	if _, ok := dl.roomEnterFallbackBody(err); ok {
		t.Fatal("roomEnterFallbackBody() returned live fallback for a known offline room")
	}
}

func TestRoomEnterEmptyAfterMissingLivePageStateIsRoomNotFound(t *testing.T) {
	dl := &DouyinLive{}
	err := fmt.Errorf("%w status=200 content_type=%q content_length=0 raw_len=0", errRoomInfoEmpty, "application/json")
	livePageErr := fmt.Errorf("%w: %s", errLivePageStateNotFound, "9122185334341")

	if got := dl.roomNotFoundErrorAfterRoomEnter(err, livePageErr); !errors.Is(got, ErrRoomNotFound) {
		t.Fatalf("roomNotFoundErrorAfterRoomEnter() = %v, want ErrRoomNotFound", got)
	}
}

func TestRoomEnterEmptyAfterKnownOfflineIsNotRoomNotFound(t *testing.T) {
	dl := &DouyinLive{}
	dl.setLivePageIDs("7659772543859215154", "")
	dl.setLiveStatus(false)
	err := fmt.Errorf("%w status=200 content_type=%q content_length=0 raw_len=0", errRoomInfoEmpty, "application/json")

	if got := dl.roomNotFoundErrorAfterRoomEnter(err, nil); got != nil {
		t.Fatalf("roomNotFoundErrorAfterRoomEnter() = %v, want nil for known offline room", got)
	}
}

func TestLiveStatusSnapshotDistinguishesUnknownAndKnownOffline(t *testing.T) {
	dl := &DouyinLive{}
	if _, known := dl.liveStatusSnapshot(); known {
		t.Fatal("new DouyinLive zero state should not have a known live status")
	}

	dl.setLiveStatus(false)
	isLive, known := dl.liveStatusSnapshot()
	if !known || isLive {
		t.Fatalf("liveStatusSnapshot() = (%v, %v), want (false, true)", isLive, known)
	}

	dl.clearLiveStatus()
	if _, known := dl.liveStatusSnapshot(); known {
		t.Fatal("clearLiveStatus() did not clear known status")
	}
}

func TestBuildHeartbeatFrameMatchesBrowserCapture(t *testing.T) {
	data, err := buildHeartbeatFrame()
	if err != nil {
		t.Fatalf("buildHeartbeatFrame() failed: %v", err)
	}
	got := hex.EncodeToString(data)
	want := "3a026862"
	if got != want {
		t.Fatalf("buildHeartbeatFrame() = %s, want %s", got, want)
	}
}

func TestInitialIMFetchMSTokenPrefersUserCookie(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "ttwid=ttwid-cookie; msToken=COOKIE_MS_TOKEN; odin_tt=odin", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	if got := dl.initialIMFetchMSToken(); got != "COOKIE_MS_TOKEN" {
		t.Fatalf("initialIMFetchMSToken() = %q, want cookie msToken", got)
	}
}

func TestInitialIMFetchMSTokenReusesGeneratedSessionToken(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "ttwid=ttwid-cookie", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	first := dl.initialIMFetchMSToken()
	second := dl.initialIMFetchMSToken()
	if first == "" {
		t.Fatalf("initialIMFetchMSToken() returned empty token")
	}
	if first != second {
		t.Fatalf("initialIMFetchMSToken() generated different tokens: %q != %q", first, second)
	}
}

func TestCookieValueReadsConfiguredCookieBeforeFetchedCookies(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "ttwid=config-ttwid; msToken=config-token", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.ttwid = "fetched-ttwid"
	dl.additionalCookies["msToken"] = "fetched-token"

	if got := dl.cookieValue("ttwid"); got != "config-ttwid" {
		t.Fatalf("cookieValue(ttwid) = %q, want configured cookie", got)
	}
	if got := dl.cookieValue("msToken"); got != "config-token" {
		t.Fatalf("cookieValue(msToken) = %q, want configured cookie", got)
	}
}

func TestShouldFetchTTWIDSkipsWhenUserCookieProvided(t *testing.T) {
	withCookie, err := newDouyinLive("live-id", nil, "ttwid=user-ttwid; sessionid=user-session", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer withCookie.Dispose()
	if withCookie.shouldFetchTTWID() {
		t.Fatalf("shouldFetchTTWID() = true with user cookie")
	}

	withoutCookie, err := newDouyinLive("live-id", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer withoutCookie.Dispose()
	if !withoutCookie.shouldFetchTTWID() {
		t.Fatalf("shouldFetchTTWID() = false without user cookie")
	}
}

func TestBuildRoomEnterParamsUsesCurrentUserAgentAndCookieToken(t *testing.T) {
	dl, err := newDouyinLive("161022647108", nil, "ttwid=user-ttwid; msToken=COOKIE_MS_TOKEN", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

	params := dl.buildRoomEnterParams()
	for _, want := range []string{
		"web_rid=161022647108",
		"enter_from=link_share",
		"cookie_enabled=true",
		"screen_width=1920",
		"screen_height=1080",
		"browser_name=Chrome",
		"browser_version=150.0.0.0",
		"os_name=Windows",
		"os_version=10",
		"is_need_double_stream=false",
		"msToken=COOKIE_MS_TOKEN",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("buildRoomEnterParams() missing %q in %s", want, params)
		}
	}
	if strings.Contains(params, "116.0.0.0") {
		t.Fatalf("buildRoomEnterParams() contains stale browser version: %s", params)
	}
}

func TestBuildRoomEnterParamsIncludesKnownRoomID(t *testing.T) {
	dl, err := newDouyinLive("161022647108", nil, "ttwid=user-ttwid; msToken=COOKIE_MS_TOKEN", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.updateRoomInfo("7659792511015177001", "7659797852999091746", "", "", "")

	params := dl.buildRoomEnterParams()
	if !strings.Contains(params, "room_id_str=7659792511015177001") {
		t.Fatalf("buildRoomEnterParams() missing room_id_str in %s", params)
	}
}

func TestBuildRoomEnterParamsMatchesBrowserKeyOrder(t *testing.T) {
	dl, err := newDouyinLive("161022647108", nil, "ttwid=user-ttwid; msToken=COOKIE_MS_TOKEN", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	dl.updateRoomInfo("7659792511015177001", "7659797852999091746", "", "", "")

	wantKeys := []string{
		"aid",
		"app_name",
		"live_id",
		"device_platform",
		"language",
		"enter_from",
		"cookie_enabled",
		"screen_width",
		"screen_height",
		"browser_language",
		"browser_platform",
		"browser_name",
		"browser_version",
		"os_name",
		"os_version",
		"web_rid",
		"room_id_str",
		"enter_source",
		"is_need_double_stream",
		"insert_task_id",
		"live_reason",
		"msToken",
	}
	if got := queryKeys(dl.buildRoomEnterParams()); strings.Join(got, ",") != strings.Join(wantKeys, ",") {
		t.Fatalf("room enter keys = %#v, want %#v", got, wantKeys)
	}
}

func TestShouldRetryRoomEnterAcceptsWrappedEmptyResponse(t *testing.T) {
	err := fmt.Errorf("%w status=200 raw_len=0", errRoomInfoEmpty)
	if !isRoomInfoEmptyError(err) {
		t.Fatalf("isRoomInfoEmptyError() = false for wrapped empty response")
	}
	if !isRoomInfoEmptyError(fmt.Errorf("retry: all attempts failed: %s", errRoomInfoEmpty.Error())) {
		t.Fatalf("isRoomInfoEmptyError() = false for retry wrapper text")
	}
	if isRoomInfoEmptyError(errors.New("other error")) {
		t.Fatalf("isRoomInfoEmptyError() = true for unrelated error")
	}
}

func TestRoomEnterFallbackBodyUsesLivePageState(t *testing.T) {
	dl := &DouyinLive{}
	dl.liveID = "161022647108"
	dl.setLivePageIDs("7659792511015177001", "7601036345435309606")
	dl.setLiveStatus(true)

	body, ok := dl.roomEnterFallbackBody(fmt.Errorf("%w status=200 raw_len=0", errRoomInfoEmpty))
	if !ok {
		t.Fatalf("roomEnterFallbackBody() ok = false")
	}
	info, err := parseRoomInfo(body)
	if err != nil {
		t.Fatalf("parseRoomInfo(fallback) failed: %v body=%s", err, body)
	}
	if info.roomID != "7659792511015177001" || info.pushID != "7601036345435309606" {
		t.Fatalf("fallback info = %#v", info)
	}
	if status := firstNonEmptyGJSON(body, "data.data.0.status"); status != "2" {
		t.Fatalf("fallback status = %q", status)
	}
}

func TestRoomEnterFallbackBodyDoesNotUseLiveIDAsNickname(t *testing.T) {
	dl, err := newDouyinLive("1144632524", nil, "", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.setLivePageIDs("7659786040097426226", "7601036345435309606")
	dl.setLiveStatus(true)

	body, ok := dl.roomEnterFallbackBody(fmt.Errorf("%w status=200 raw_len=0", errRoomInfoEmpty))
	if !ok {
		t.Fatalf("roomEnterFallbackBody() ok = false")
	}
	info, err := parseRoomInfo(body)
	if err != nil {
		t.Fatalf("parseRoomInfo(fallback) failed: %v body=%s", err, body)
	}
	if info.liveName == dl.liveID {
		t.Fatalf("fallback liveName = liveID %q, should stay empty when nickname is unknown", info.liveName)
	}
}

func TestLogMissingLiveNameWarnsWithRoomContext(t *testing.T) {
	logger := &captureLogSink{}
	dl := &DouyinLive{liveID: "1144632524", logger: logger}

	dl.logMissingLiveName("live_page_fallback", roomInfoSnapshot{
		roomID: "7659786040097426226",
		pushID: "7601036345435309606",
	})

	if len(logger.messages) != 1 {
		t.Fatalf("log count = %d, want 1", len(logger.messages))
	}
	if logger.messages[0] != "直播间名称未获取到，已继续连接" {
		t.Fatalf("message = %q", logger.messages[0])
	}
	args := logger.args[0]
	want := map[string]interface{}{
		"live_id":        "1144632524",
		"room_id":        "7659786040097426226",
		"user_unique_id": "7601036345435309606",
		"source":         "live_page_fallback",
	}
	for i := 0; i+1 < len(args); i += 2 {
		key, ok := args[i].(string)
		if !ok {
			continue
		}
		if value, exists := want[key]; exists && args[i+1] == value {
			delete(want, key)
		}
	}
	if len(want) > 0 {
		t.Fatalf("log args %#v missing %v", args, want)
	}
}

func TestBrowserClientHintHeadersUseChromeMajorVersion(t *testing.T) {
	ua := "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	headers := browserClientHintHeaders(ua)

	if got := headers["sec-ch-ua"]; !strings.Contains(got, `"Chromium";v="150"`) || !strings.Contains(got, `"Google Chrome";v="150"`) {
		t.Fatalf("sec-ch-ua = %q", got)
	}
	if got := headers["sec-ch-ua-mobile"]; got != "?0" {
		t.Fatalf("sec-ch-ua-mobile = %q", got)
	}
	if got := headers["sec-ch-ua-platform"]; got != `"Windows"` {
		t.Fatalf("sec-ch-ua-platform = %q", got)
	}
}

func TestReconnectPlanDoesNotRefreshUAWhenUserCookieProvided(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "ttwid=user-ttwid; sessionid=user-session", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	_, changeUA, rebuildHTTP := dl.reconnectPlan("try_again_later_1013", 4, time.Second, true)
	if changeUA {
		t.Fatalf("reconnectPlan() changeUA = true with user cookie")
	}
	if !rebuildHTTP {
		t.Fatalf("reconnectPlan() rebuildHTTP = false, want true for high failure count")
	}
}

func TestDefaultHeartbeatIntervalMatchesBrowserCadence(t *testing.T) {
	if heartbeatInterval != 10*time.Second {
		t.Fatalf("heartbeatInterval = %v, want %v", heartbeatInterval, 10*time.Second)
	}
}

func TestApplyWebsocketResponseStateTracksCursorInternalExtAndHeartbeat(t *testing.T) {
	dl := newTestDouyinLiveWithBufferPool()

	dl.applyWebsocketResponseState(&new_douyin.Webcast_Im_Response{
		Cursor:            "cursor-1",
		InternalExt:       "internal_src:pushserver|seq:1",
		HeartbeatDuration: 3,
		PushServerV2:      "webcast100-ws-web-hl.douyin.com",
	})

	cursor, internalExt, pushURL := dl.websocketStateSnapshot()
	if cursor != "cursor-1" {
		t.Fatalf("cursor = %q, want %q", cursor, "cursor-1")
	}
	if internalExt != "internal_src:pushserver|seq:1" {
		t.Fatalf("internalExt = %q, want %q", internalExt, "internal_src:pushserver|seq:1")
	}
	if pushURL != "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/" {
		t.Fatalf("pushURL = %q", pushURL)
	}
	if got := dl.currentHeartbeatInterval(); got != 10*time.Second {
		t.Fatalf("currentHeartbeatInterval() = %v, want %v", got, 10*time.Second)
	}

	dl.applyWebsocketResponseState(&new_douyin.Webcast_Im_Response{
		HeartbeatDuration: 15,
	})
	if got := dl.currentHeartbeatInterval(); got != 15*time.Second {
		t.Fatalf("currentHeartbeatInterval() = %v, want %v", got, 15*time.Second)
	}
}

func TestWebsocketPushURLFromResponsePrefersDynamicServer(t *testing.T) {
	pushURL, source := websocketPushURLFromResponseWithSource(&new_douyin.Webcast_Im_Response{
		PushServerV2: "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/",
		PushServer:   "wss://webcast100-ws-web-lf.douyin.com/webcast/im/push/v2/",
		ProxyServer:  "wss://webcast100-ws-web-lq.douyin.com/webcast/im/push/v2/",
	})
	if source != "push_server_v2" {
		t.Fatalf("source = %q, want push_server_v2", source)
	}
	if pushURL != "wss://webcast100-ws-web-hl.douyin.com/webcast/im/push/v2/" {
		t.Fatalf("pushURL = %q", pushURL)
	}
}

func TestDefaultWebsocketPushURLUsesCurrentWebHostFamily(t *testing.T) {
	if !strings.HasPrefix(websocketPushURL, "wss://webcast100-ws-web-") {
		t.Fatalf("websocketPushURL = %q, want webcast100 fallback", websocketPushURL)
	}
}

func TestSendHeartbeatRefreshesReadDeadline(t *testing.T) {
	wsConn, recorder, cleanup := newRecordingWebsocketConn(t)
	defer cleanup()

	dl := newTestDouyinLiveWithBufferPool()
	dl.conn = wsConn

	before := recorder.readDeadline()
	if err := dl.sendHeartbeat(); err != nil {
		t.Fatalf("sendHeartbeat() failed: %v", err)
	}
	after := recorder.readDeadline()

	if after.IsZero() {
		t.Fatal("read deadline was not refreshed after heartbeat")
	}
	if !before.IsZero() && !after.After(before) {
		t.Fatalf("read deadline = %v, want after %v", after, before)
	}
}

func TestDecodeGzipResponseAcceptsNormalPayload(t *testing.T) {
	payload, err := proto.Marshal(&new_douyin.Webcast_Im_Response{})
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	compressed := gzipTestPayload(t, payload)
	dl := newTestDouyinLiveWithBufferPool()

	err = dl.decodeGzipResponse(
		compressed,
		&new_douyin.Webcast_Im_PushFrame{},
		&new_douyin.Webcast_Im_Response{},
		&new_douyin.Webcast_Im_ControlMessage{},
	)
	if err != nil {
		t.Fatalf("decodeGzipResponse() failed: %v", err)
	}
}

func TestDecodeGzipResponseRejectsOversizedPayload(t *testing.T) {
	if maxGzipPayloadSize < 32<<20 {
		t.Fatalf("maxGzipPayloadSize = %d, want at least 32MiB", maxGzipPayloadSize)
	}
	compressed := gzipTestPayload(t, []byte(strings.Repeat("x", maxGzipPayloadSize+1)))
	dl := newTestDouyinLiveWithBufferPool()

	err := dl.decodeGzipResponse(
		compressed,
		&new_douyin.Webcast_Im_PushFrame{},
		&new_douyin.Webcast_Im_Response{},
		&new_douyin.Webcast_Im_ControlMessage{},
	)
	if err == nil {
		t.Fatalf("decodeGzipResponse() returned nil error for oversized payload")
	}
}

func newTestDouyinLiveWithBufferPool() *DouyinLive {
	return &DouyinLive{
		bufferPool: &sync.Pool{
			New: func() interface{} {
				return bytes.NewBuffer(make([]byte, 0, gzipBufferSize))
			},
		},
	}
}

type recordingConn struct {
	net.Conn
	mu               sync.Mutex
	lastReadDeadline time.Time
}

func (c *recordingConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.lastReadDeadline = t
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(t)
}

func (c *recordingConn) readDeadline() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastReadDeadline
}

func newRecordingWebsocketConn(t *testing.T) (*websocket.Conn, *recordingConn, func()) {
	t.Helper()

	clientSide, serverSide := net.Pipe()
	recorder := &recordingConn{Conn: clientSide}
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)
		defer serverSide.Close()

		reader := bufio.NewReader(serverSide)
		var key string
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				break
			}
			if value, ok := strings.CutPrefix(trimmed, "Sec-WebSocket-Key:"); ok {
				key = strings.TrimSpace(value)
			}
		}
		if key == "" {
			return
		}

		acceptHash := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		accept := base64.StdEncoding.EncodeToString(acceptHash[:])
		_, _ = fmt.Fprintf(serverSide,
			"HTTP/1.1 101 Switching Protocols\r\n"+
				"Upgrade: websocket\r\n"+
				"Connection: Upgrade\r\n"+
				"Sec-WebSocket-Accept: %s\r\n\r\n",
			accept,
		)
		_, _ = io.Copy(io.Discard, reader)
	}()

	u, err := url.Parse("ws://example.test/webcast/im/push/v2/")
	if err != nil {
		t.Fatalf("parse websocket URL: %v", err)
	}
	wsConn, _, err := websocket.NewClient(recorder, u, nil, 1024, 1024)
	if err != nil {
		serverSide.Close()
		clientSide.Close()
		t.Fatalf("NewClient() failed: %v", err)
	}

	cleanup := func() {
		_ = wsConn.Close()
		_ = clientSide.Close()
		select {
		case <-serverDone:
		case <-time.After(time.Second):
			t.Fatal("websocket test server did not stop")
		}
	}
	return wsConn, recorder, cleanup
}

func gzipTestPayload(t *testing.T, payload []byte) []byte {
	t.Helper()

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func queryKeys(query string) []string {
	parts := strings.Split(query, "&")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		key, _, _ := strings.Cut(part, "=")
		keys = append(keys, key)
	}
	return keys
}
