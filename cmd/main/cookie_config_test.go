package main

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
	"time"
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
