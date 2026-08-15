package main

import (
	"testing"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func TestMarkUpstreamReadyOnlyMarksTheCurrentRoomSession(t *testing.T) {
	current, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive() failed: %v", err)
	}
	defer current.Dispose()

	other, err := douyinLive.NewDouyinLive("other-live-id", nil, "")
	if err != nil {
		t.Fatalf("NewDouyinLive(other) failed: %v", err)
	}
	defer other.Dispose()

	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", 30, 30, nil)
	room.douyinLive = current

	if room.markUpstreamReady(other) {
		t.Fatal("markUpstreamReady(other) = true, want false")
	}
	if room.upstreamReady {
		t.Fatal("room became ready for a stale session")
	}

	if !room.markUpstreamReady(current) {
		t.Fatal("markUpstreamReady(current) = false, want true")
	}
	if !room.upstreamReady {
		t.Fatal("room did not become ready for the current session")
	}
}
