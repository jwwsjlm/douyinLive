package main

import (
	"strings"
	"testing"
	"time"

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

func TestMonitorStatusKeepsUnknownSemantics(t *testing.T) {
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	client := NewClient("client", nil)
	room.addClient(client)
	room.setStatusUnknown(true)

	room.notifyMonitorStatus()

	select {
	case message := <-client.sendQueue:
		if payload := string(message.payload); !strings.Contains(payload, `"code":"ROOM_STATUS_UNKNOWN"`) || strings.Contains(payload, `"code":"ROOM_OFFLINE"`) {
			t.Fatalf("monitor payload = %s", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("monitor did not enqueue status message")
	}
}

func TestAnonymousProbeRotatesOnlyAfterRepeatedFailures(t *testing.T) {
	probe, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	room.probeLive = probe

	room.recordProbeFailure(probe)
	room.recordProbeFailure(probe)
	if room.probeLive != probe {
		t.Fatal("anonymous probe rotated before reaching the failure threshold")
	}
	room.recordProbeFailure(probe)
	if room.probeLive != nil || room.probeFailures != 0 {
		t.Fatal("anonymous probe was not rotated after repeated failures")
	}
}

func TestConfiguredCookieProbeDoesNotRotateOnStatusUnknown(t *testing.T) {
	probe, err := douyinLive.NewDouyinLive("live-id", nil, "ttwid=configured")
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Dispose()
	room := NewRoom("live-id", nil, false, "ttwid=configured", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	room.probeLive = probe

	for range anonymousProbeRotateFailures + 2 {
		room.recordProbeFailure(probe)
	}
	if room.probeLive != probe {
		t.Fatal("configured-Cookie probe unexpectedly rotated")
	}
}

func TestRoomCloseDisposesProbeSession(t *testing.T) {
	probe, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	room.probeLive = probe

	room.Close()

	if room.probeLive != nil {
		t.Fatal("Room.Close() retained the probe session")
	}
}
