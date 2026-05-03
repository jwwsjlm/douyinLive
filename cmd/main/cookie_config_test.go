package main

import (
	"encoding/base64"
	"net/http/httptest"
	"testing"
)

func TestRoomManagerCookieForRoomPriority(t *testing.T) {
	rm := NewRoomManager(nil, false, "global-cookie", map[string]string{
		"1001": "room-cookie",
	}, 0, 0)

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
