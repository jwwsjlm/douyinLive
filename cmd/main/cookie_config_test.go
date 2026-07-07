package main

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
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

func TestRoomCloseAllClientsClosesEveryWaitingClient(t *testing.T) {
	room := NewRoom("1001", nil, false, "", signProviderLocal, "", time.Second, time.Second, nil)
	first := NewClient("client-1", nil)
	second := NewClient("client-2", nil)
	room.addClient(first)
	room.addClient(second)

	room.closeAllClients(roomInvalidMessage)

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

	client.close(nil)
	client.close(nil)
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
