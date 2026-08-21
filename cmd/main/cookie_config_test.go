package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/lxzan/gws"
)

func TestRoomManagerCookieForRoomPriority(t *testing.T) {
	rm := NewRoomManager(nil, false, "global-cookie", map[string]string{
		"1001": "room-cookie",
	}, signProviderLocal, "", 0, 0)

	if got := rm.cookieForRoom("1001", "override-cookie"); got != "override-cookie" {
		t.Fatalf("override cookie should win, got %q", got)
	}
	if got := rm.cookieForRoom("1001", ""); got != "room-cookie" {
		t.Fatalf("room cookie should win over global cookie, got %q", got)
	}
	if got := rm.cookieForRoom("1002", ""); got != "global-cookie" {
		t.Fatalf("missing room cookie should fallback to global cookie, got %q", got)
	}
}

func TestRoomManagerCanDisableStoredCookies(t *testing.T) {
	rm := NewRoomManager(nil, false, "global-cookie", map[string]string{
		"1001": "room-cookie",
	}, signProviderLocal, "", 0, 0, false)
	if got := rm.cookieForRoom("1001", ""); got != "" {
		t.Fatalf("stored cookies should be disabled, got %q", got)
	}
	if got := rm.cookieForRoom("1001", "override-cookie"); got != "override-cookie" {
		t.Fatalf("explicit override should remain usable, got %q", got)
	}
}

func TestRoomManagerWithOptionsCopiesCookieConfiguration(t *testing.T) {
	useStored := true
	cookies := map[string]string{"1001": "room-cookie"}
	rm := NewRoomManagerWithOptions(RoomManagerOptions{
		RoomCookies: cookies, Cookie: "global-cookie", SignProvider: signProviderLocal,
		PollInterval: time.Second, NotifyInterval: time.Second, UseStoredCookie: &useStored,
	})
	cookies["1001"] = "mutated"
	if got := rm.cookieForRoom("1001", ""); got != "room-cookie" {
		t.Fatalf("RoomManagerOptions did not copy cookie map, got %q", got)
	}
}

func TestParseCookieOverride(t *testing.T) {
	cookie := "ttwid=abc; sessionid=xyz"
	encoded := base64.RawURLEncoding.EncodeToString([]byte(cookie))

	req := httptest.NewRequest("GET", "/ws/1001?cookie_b64="+encoded, nil)
	got, err := parseCookieOverride(req)
	if err != nil {
		t.Fatalf("parse cookie_b64 failed: %v", err)
	}
	if got != cookie {
		t.Fatalf("unexpected cookie: got %q want %q", got, cookie)
	}

	req = httptest.NewRequest("GET", "/ws/1001?cookie=ttwid%3Dabc%3B+sessionid%3Dxyz", nil)
	got, err = parseCookieOverride(req)
	if err != nil {
		t.Fatalf("parse cookie failed: %v", err)
	}
	if got != cookie {
		t.Fatalf("unexpected cookie: got %q want %q", got, cookie)
	}
}

func TestParseCookieOverrideRejectsAmbiguousAndUnsafeValues(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws/1001?cookie=a&cookie_b64=Yg", nil)
	if _, err := parseCookieOverride(req); err == nil {
		t.Fatal("cookie and cookie_b64 should not be accepted together")
	}

	req = httptest.NewRequest(http.MethodGet, "/ws/1001?cookie=ttwid%3Dabc%0D%0AX-Test%3A+bad", nil)
	if _, err := parseCookieOverride(req); err == nil {
		t.Fatal("control characters in cookie should be rejected")
	}

	longCookie := strings.Repeat("a", maxCookieOverrideBytes+1)
	req = httptest.NewRequest(http.MethodGet, "/ws/1001?cookie="+url.QueryEscape(longCookie), nil)
	if _, err := parseCookieOverride(req); err == nil {
		t.Fatal("oversized cookie should be rejected")
	}
}

func TestRoomManagerKeySeparatesCookie(t *testing.T) {
	keyA := roomManagerKey("1001", "cookie-a")
	keyB := roomManagerKey("1001", "cookie-b")
	if keyA == keyB {
		t.Fatalf("different cookies should produce different room keys")
	}
	if got := roomManagerKey("1001", ""); got != "1001" {
		t.Fatalf("empty cookie should keep legacy room key, got %q", got)
	}
}

func TestRoomManagerSharesOnlyMatchingRoomProfiles(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	first := rm.GetOrCreateRoom("1001", "cookie-a")
	shared := rm.GetOrCreateRoom("1001", "cookie-a")
	isolated := rm.GetOrCreateRoom("1001", "cookie-b")

	if first != shared {
		t.Fatal("matching room and Cookie did not reuse the same upstream room")
	}
	if first == isolated {
		t.Fatal("different Cookies unexpectedly shared one upstream room")
	}

	rm.CloseAll()
}

func TestSnapshotRoomsAggregatesCookieSpecificSessionsByLiveID(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	defer rm.Close()
	first := rm.GetOrCreateRoom("1001", "cookie-a")
	second := rm.GetOrCreateRoom("1001", "cookie-b")
	if first == second {
		t.Fatal("different cookies unexpectedly shared one internal room session")
	}
	first.clientsMu.Lock()
	first.clients["a"] = NewClient("a", nil)
	first.clientsMu.Unlock()
	second.clientsMu.Lock()
	second.clients["b"] = NewClient("b", nil)
	second.clientsMu.Unlock()
	first.mu.Lock()
	first.knownValid = true
	first.title = "标题"
	first.mu.Unlock()
	second.mu.Lock()
	second.upstreamReady = true
	second.mu.Unlock()

	snapshots := rm.SnapshotRooms()
	if len(snapshots) != 1 {
		t.Fatalf("SnapshotRooms() count = %d, want one logical room: %+v", len(snapshots), snapshots)
	}
	snapshot := snapshots[0]
	if snapshot.LiveID != "1001" || snapshot.Status != "online" || snapshot.ClientCount != 2 || !snapshot.UpstreamReady || snapshot.Title != "标题" {
		t.Fatalf("aggregated snapshot = %+v", snapshot)
	}
}

func TestRoomRemoveIfIdleRemovesRoomFromManager(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	room := rm.GetOrCreateRoom("1001", "")

	room.removeIfIdle()

	rm.roomsMu.RLock()
	_, ok := rm.rooms["1001"]
	rm.roomsMu.RUnlock()
	if ok {
		t.Fatalf("idle room was not removed from manager")
	}
}

func TestRoomRemoveIfIdleKeepsRoomWithClients(t *testing.T) {
	removed := false
	room := NewRoom("1001", nil, false, "", signProviderLocal, "", time.Second, time.Second, func() {
		removed = true
	})
	room.clients["client-1"] = NewClient("client-1", nil)

	room.removeIfIdle()

	if removed {
		t.Fatalf("room with clients should not be removed")
	}
	if room.closed {
		t.Fatalf("room with clients should not be marked closed")
	}
}

func TestPendingWebSocketUpgradePreventsRoomRemoval(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	room := rm.AcquireRoom("1001", "")

	room.removeIfIdle()
	if room.isClosed() {
		t.Fatal("room closed while a WebSocket upgrade reservation was pending")
	}
	rm.roomsMu.RLock()
	got := rm.rooms["1001"]
	rm.roomsMu.RUnlock()
	if got != room {
		t.Fatal("reserved room was removed from manager")
	}

	room.releaseClientReservation()
	if !room.isClosed() {
		t.Fatal("idle room was not closed after releasing the failed upgrade reservation")
	}
}

func TestRoomManagerReplacesClosedRoom(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	oldRoom := rm.GetOrCreateRoom("1001", "")
	oldRoom.mu.Lock()
	oldRoom.closed = true
	oldRoom.mu.Unlock()

	newRoom := rm.GetOrCreateRoom("1001", "")
	if newRoom == oldRoom {
		t.Fatalf("GetOrCreateRoom returned a closed room")
	}
	if newRoom.closed {
		t.Fatalf("new room is closed")
	}
}

func TestOldRoomOnCloseDoesNotRemoveReplacementRoom(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderLocal, "", time.Second, time.Second)
	oldRoom := rm.GetOrCreateRoom("1001", "")
	oldRoom.mu.Lock()
	oldRoom.closed = true
	oldRoom.mu.Unlock()

	newRoom := rm.GetOrCreateRoom("1001", "")
	oldRoom.onClose()

	rm.roomsMu.RLock()
	got := rm.rooms["1001"]
	rm.roomsMu.RUnlock()
	if got != newRoom {
		t.Fatalf("old room onClose removed replacement room")
	}
}

func TestRoomCloseIsIdempotent(t *testing.T) {
	closeCalls := 0
	room := NewRoom("1001", nil, false, "", signProviderLocal, "", time.Second, time.Second, func() {
		closeCalls++
	})

	room.Close()
	room.Close()

	if closeCalls != 1 {
		t.Fatalf("onClose called %d times, want 1", closeCalls)
	}
}

func TestRoomRemembersPreviouslyValidatedIdentity(t *testing.T) {
	room := NewRoom("1001", nil, false, "", signProviderLocal, "", time.Second, time.Second, nil)
	if room.hasKnownValidRoom() {
		t.Fatal("new room unexpectedly starts as validated")
	}
	room.markKnownValid()
	if !room.hasKnownValidRoom() {
		t.Fatal("room did not retain validated identity")
	}
}

func TestRoomCloseAllClientsClosesEveryWaitingClient(t *testing.T) {
	room := NewRoom("1001", nil, false, "", signProviderLocal, "", time.Second, time.Second, nil)
	first := NewClient("client-1", nil)
	second := NewClient("client-2", nil)
	addTestClient(room, first)
	addTestClient(room, second)

	room.closeAllClients(invalidRoomClientClose)

	if got := room.clientCount(); got != 0 {
		t.Fatalf("client count after closeAllClients = %d, want 0", got)
	}
	for _, client := range []*Client{first, second} {
		select {
		case <-client.stopCh:
		default:
			t.Fatalf("client %s was not closed", client.id)
		}
	}
}

func TestRoomLiveStatusMessagesExposeValidityAndStatus(t *testing.T) {
	room := NewRoom("386395296025", nil, false, "", signProviderLocal, "", time.Second, 30*time.Second, nil)
	room.liveName = "CACA-anchor"
	room.title = "offline-room-title"
	room.avatarThumb = "https://example.test/avatar.jpeg"

	offline := string(room.offlineStatusMessage())
	for _, want := range []string{`"event":"live_status"`, `"code":"ROOM_OFFLINE"`, `"valid":true`, `"live":false`, `"status":"offline"`, `"status_text":"直播间未开播"`, `"live_name":"CACA-anchor"`, `"title":"offline-room-title"`, `"avatar_thumb":"https://example.test/avatar.jpeg"`, `"suggestion":"客户端不需要重连`, `"retry_interval_seconds":30`} {
		if !strings.Contains(offline, want) {
			t.Fatalf("offlineStatusMessage() = %s, missing %s", offline, want)
		}
	}

	ended := string(room.offlineEndedStatusMessage())
	for _, want := range []string{`"event":"live_status"`, `"code":"ROOM_ENDED"`, `"valid":true`, `"live":false`, `"status":"ended"`, `"status_text":"直播间已下播"`, `"live_name":"CACA-anchor"`, `"title":"offline-room-title"`, `"avatar_thumb":"https://example.test/avatar.jpeg"`, `"suggestion":"客户端不需要重连`, `"retry_interval_seconds":30`} {
		if !strings.Contains(ended, want) {
			t.Fatalf("offlineEndedStatusMessage() = %s, missing %s", ended, want)
		}
	}

	online := string(room.onlineStatusMessage())
	for _, want := range []string{`"event":"live_status"`, `"code":"ROOM_ONLINE"`, `"valid":true`, `"live":true`, `"status":"online"`, `"status_text":"直播间已开播"`, `"live_name":"CACA-anchor"`, `"title":"offline-room-title"`, `"avatar_thumb":"https://example.test/avatar.jpeg"`, `"suggestion":"客户端可以开始正常处理直播消息"`} {
		if !strings.Contains(online, want) {
			t.Fatalf("onlineStatusMessage() = %s, missing %s", online, want)
		}
	}
}

func TestRoomAnchorOnlyStatusMessageExplainsAccountExistsButNoRoom(t *testing.T) {
	room := NewRoom("32536162943", nil, false, "", signProviderLocal, "", time.Second, 30*time.Second, nil)
	room.liveName = "一只喵动漫"
	room.avatarThumb = "https://example.test/avatar.jpeg"
	room.accountOnly = true

	offline := string(room.offlineStatusMessage())
	for _, want := range []string{`"code":"ACCOUNT_OFFLINE_NO_ROOM"`, `"status":"account_offline"`, `"status_text":"账号存在但当前没有直播间"`, `"has_room":false`, `"account_only":true`, `"live_name":"一只喵动漫"`, `"message":"账号存在，但网页没有返回直播间房间对象，可能是该账号从未开播或当前未创建直播间，当前按未开播处理"`, `"suggestion":"客户端不需要重连`} {
		if !strings.Contains(offline, want) {
			t.Fatalf("offlineStatusMessage() = %s, missing %s", offline, want)
		}
	}
}

func TestStatusUnknownMessageKeepsClientInRetryableState(t *testing.T) {
	room := NewRoom("139819566957", nil, false, "", signProviderLocal, "", time.Second, 30*time.Second, nil)
	room.liveName = "亮一嗓·郝晓亮"
	room.title = "亮一嗓大舞台"
	message := string(room.statusUnknownMessage())
	for _, want := range []string{
		`"code":"ROOM_STATUS_UNKNOWN"`,
		`"valid":false`,
		`"live":null`,
		`"status":"unknown"`,
		`"has_room":null`,
		`"account_only":null`,
		`"live_name":"亮一嗓·郝晓亮"`,
		`"suggestion":"客户端保持当前 WebSocket 连接，不要立即重连"`,
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("statusUnknownMessage() = %s, missing %s", message, want)
		}
	}
}

func TestStatusUnknownMessagePreservesPreviouslyValidatedRoomIdentity(t *testing.T) {
	room := NewRoom("139819566957", nil, false, "", signProviderLocal, "", time.Second, 30*time.Second, nil)
	room.liveName = "亮一嗓·郝晓亮"
	room.markKnownValid()
	message := string(room.statusUnknownMessage())
	for _, want := range []string{`"has_room":true`, `"account_only":false`, `"live_name":"亮一嗓·郝晓亮"`} {
		if !strings.Contains(message, want) {
			t.Fatalf("statusUnknownMessage() = %s, missing %s", message, want)
		}
	}
}

func TestRoomMetadataUpdatePreservesExistingWhenSourceFieldsEmpty(t *testing.T) {
	room := NewRoom("386395296025", nil, false, "", signProviderLocal, "", time.Second, 30*time.Second, nil)
	room.liveName = "CACA-anchor"
	room.title = "offline-room-title"
	room.avatarThumb = "https://example.test/avatar.jpeg"

	room.updateMetadataFromDouyinLive(&douyinLive.DouyinLive{})

	liveName, title, avatarThumb, _ := room.metadataSnapshot()
	if liveName != "CACA-anchor" || title != "offline-room-title" || avatarThumb != "https://example.test/avatar.jpeg" {
		t.Fatalf("metadata was overwritten by empty DouyinLive fields: liveName=%q title=%q avatarThumb=%q", liveName, title, avatarThumb)
	}
}

func TestRoomCloseMessagesUseReadableChineseCodes(t *testing.T) {
	for name, payload := range map[string]string{
		"roomInvalidMessage":       string(roomInvalidMessage),
		"liveStartFailedMessage":   string(liveStartFailedMessage),
		"serviceClosingMessage":    string(serviceClosingMessage),
		"slowClientClosingMessage": string(slowClientClosingMessage),
	} {
		for _, want := range []string{`"code":`, `"message":`, `"suggestion":`} {
			if !strings.Contains(payload, want) {
				t.Fatalf("%s = %s, missing %s", name, payload, want)
			}
		}
	}
}

func TestClientCloseAllowsNilConn(t *testing.T) {
	client := NewClient("client-1", nil)

	client.close(normalClientClose)
	client.close(normalClientClose)
}

func TestClientEnqueueRejectsMessagesAfterClose(t *testing.T) {
	client := NewClient("client-1", nil)
	client.close(normalClientClose)
	if client.enqueue(gws.OpcodeText, []byte("late")) {
		t.Fatal("enqueue unexpectedly accepted a message after close")
	}
}

func TestClientEnqueueRejectsOversizedMessages(t *testing.T) {
	client := NewClient("client-1", nil)
	if got := client.enqueueWithResult(gws.OpcodeText, make([]byte, maxClientMessageBytes+1)); got != enqueueMessageTooLarge {
		t.Fatalf("enqueue result = %v, want enqueueMessageTooLarge", got)
	}
	if client.enqueue(gws.OpcodeText, make([]byte, maxClientMessageBytes+1)) {
		t.Fatal("oversized message was accepted")
	}
}

func TestClientEnqueueBoundsQueuedBytes(t *testing.T) {
	client := NewClient("client-1", nil)
	payload := make([]byte, maxClientMessageBytes)
	if got := client.enqueueWithResult(gws.OpcodeText, payload); got != enqueueAccepted {
		t.Fatalf("first enqueue result = %v", got)
	}
	if got := client.enqueueWithResult(gws.OpcodeText, payload); got != enqueueAccepted {
		t.Fatalf("second enqueue result = %v", got)
	}
	if got := client.enqueueWithResult(gws.OpcodeText, []byte("extra")); got != enqueueQueueBytesExceeded {
		t.Fatalf("third enqueue result = %v, want enqueueQueueBytesExceeded", got)
	}
}

func TestRoomManagerPassesSignConfigToRoom(t *testing.T) {
	rm := NewRoomManager(nil, false, "", nil, signProviderTikHub, " api-key ", time.Second, time.Second)
	room := rm.GetOrCreateRoom("1001", "")

	if room.signProvider != signProviderTikHub {
		t.Fatalf("room.signProvider = %q, want %q", room.signProvider, signProviderTikHub)
	}
	if room.tikHubKey != "api-key" {
		t.Fatalf("room.tikHubKey = %q, want trimmed key", room.tikHubKey)
	}
}
