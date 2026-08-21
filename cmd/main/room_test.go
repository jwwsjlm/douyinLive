package main

import (
	"strings"
	"sync"
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
	addTestClient(room, client)
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

func addTestClient(room *Room, client *Client) {
	room.clientsMu.Lock()
	room.clients[client.id] = client
	room.clientsMu.Unlock()
}

func TestNewRoomNormalizesInvalidMonitorIntervals(t *testing.T) {
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", 0, -time.Second, nil)
	if room.pollInterval != defaultRoomPollInterval || room.notifyInterval != defaultRoomNotifyInterval {
		t.Fatalf("monitor intervals = (%s, %s), want (%s, %s)", room.pollInterval, room.notifyInterval, defaultRoomPollInterval, defaultRoomNotifyInterval)
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

func TestRoomCloseWaitsForOwnedTaskAndRejectsNewTasks(t *testing.T) {
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	started := make(chan struct{})
	release := make(chan struct{})
	if !room.startTask(func() {
		close(started)
		<-release
	}) {
		t.Fatal("startTask unexpectedly rejected an open room")
	}
	<-started

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		room.Close()
	}()

	select {
	case <-room.closeDone:
		t.Fatal("Room.Close returned before the owned task exited")
	case <-time.After(50 * time.Millisecond):
	}
	if room.startTask(func() {}) {
		t.Fatal("startTask accepted a task after Room.Close began")
	}
	close(release)
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Room.Close did not finish after the owned task exited")
	}
}

func TestRoomCloseIsSafeWhenCalledConcurrently(t *testing.T) {
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)

	const callers = 16
	var wg sync.WaitGroup
	wg.Add(callers)
	for range callers {
		go func() {
			defer wg.Done()
			room.Close()
		}()
	}
	wg.Wait()

	if !room.isClosed() {
		t.Fatal("room remained open after concurrent Close calls")
	}
	select {
	case <-room.closeDone:
	default:
		t.Fatal("room close completion was not signaled")
	}
}
