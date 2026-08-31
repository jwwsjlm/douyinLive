package main

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"github.com/lxzan/gws"
	"google.golang.org/protobuf/types/known/emptypb"
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
	client := NewClient("client", nil)
	addTestClient(room, client)

	if room.markUpstreamReady(other, room.sessionGeneration) {
		t.Fatal("markUpstreamReady(other) = true, want false")
	}
	if room.upstreamReady {
		t.Fatal("room became ready for a stale session")
	}
	requireNoQueuedMessage(t, client)

	if !room.markUpstreamReady(current, room.sessionGeneration) {
		t.Fatal("markUpstreamReady(current) = false, want true")
	}
	if !room.upstreamReady {
		t.Fatal("room did not become ready for the current session")
	}
	requireQueuedMessageContaining(t, client, `"code":"ROOM_ONLINE"`)
}

func addTestClient(room *Room, client *Client) {
	room.clientsMu.Lock()
	room.clients[client.id] = client
	room.clientsMu.Unlock()
}

func requireNoQueuedMessage(t *testing.T, client *Client) {
	t.Helper()
	select {
	case message := <-client.sendQueue:
		t.Fatalf("unexpected queued message: %s", message.payload)
	default:
	}
}

func requireQueuedMessageContaining(t *testing.T, client *Client, substring string) {
	t.Helper()
	select {
	case message := <-client.sendQueue:
		if payload := string(message.payload); !strings.Contains(payload, substring) {
			t.Fatalf("queued message = %s, want substring %q", payload, substring)
		}
	case <-time.After(time.Second):
		t.Fatalf("no queued message containing %q", substring)
	}
}

func TestStaleStartResultCannotTransitionReplacementGeneration(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		statusUnknown bool
	}{
		{name: "offline", err: douyinLive.ErrLiveNotStarted, statusUnknown: true},
		{name: "unknown", err: douyinLive.ErrLiveStatusUnknown, statusUnknown: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
			oldGeneration := room.sessionGeneration
			client := NewClient("replacement-client", nil)

			room.mu.Lock()
			room.sessionGeneration++
			room.starting = true
			room.statusUnknown = test.statusUnknown
			room.clientsMu.Lock()
			room.clients[client.id] = client
			room.clientsMu.Unlock()
			room.mu.Unlock()

			room.handleLiveSessionStartResult(oldGeneration, test.err)

			room.mu.Lock()
			starting := room.starting
			statusUnknown := room.statusUnknown
			monitor := room.monitor
			room.mu.Unlock()
			if !starting {
				t.Fatal("stale result cleared replacement generation startup state")
			}
			if statusUnknown != test.statusUnknown {
				t.Fatalf("stale result changed replacement statusUnknown to %v, want %v", statusUnknown, test.statusUnknown)
			}
			if monitor != nil {
				t.Fatal("stale result installed a monitor for the replacement generation")
			}
			requireNoQueuedMessage(t, client)
		})
	}
}

func TestCurrentStartResultInstallsGenerationBoundMonitor(t *testing.T) {
	tests := []struct {
		name          string
		err           error
		statusUnknown bool
		code          string
	}{
		{name: "offline", err: douyinLive.ErrLiveNotStarted, statusUnknown: false, code: `"code":"ROOM_OFFLINE"`},
		{name: "unknown", err: douyinLive.ErrLiveStatusUnknown, statusUnknown: true, code: `"code":"ROOM_STATUS_UNKNOWN"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
			client := NewClient("client", nil)
			addTestClient(room, client)
			room.starting = true
			generation := room.sessionGeneration

			room.handleLiveSessionStartResult(generation, test.err)

			room.mu.Lock()
			monitor := room.monitor
			starting := room.starting
			statusUnknown := room.statusUnknown
			room.mu.Unlock()
			if monitor == nil || monitor.generation != generation {
				t.Fatalf("monitor = %#v, want generation %d", monitor, generation)
			}
			if starting {
				t.Fatal("current startup result left room in starting state")
			}
			if statusUnknown != test.statusUnknown {
				t.Fatalf("statusUnknown = %v, want %v", statusUnknown, test.statusUnknown)
			}
			requireQueuedMessageContaining(t, client, test.code)
			room.Close()
		})
	}
}

func TestRetiredMonitorCannotAffectOrClearReplacementMonitor(t *testing.T) {
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
	oldGeneration := room.sessionGeneration
	oldMonitor := newRoomMonitorLoop(oldGeneration, time.Hour, time.Hour)
	replacementClient := NewClient("replacement-client", nil)

	room.mu.Lock()
	room.monitor = oldMonitor
	room.sessionGeneration++
	room.statusUnknown = false
	replacementMonitor := newRoomMonitorLoop(room.sessionGeneration, time.Hour, time.Hour)
	room.monitor = replacementMonitor
	room.clientsMu.Lock()
	room.clients[replacementClient.id] = replacementClient
	room.clientsMu.Unlock()
	room.mu.Unlock()

	if clients, changed, ok := room.commitMonitorStatus(oldMonitor, true); ok || changed || len(clients) != 0 {
		t.Fatalf("retired monitor committed status: ok=%v changed=%v clients=%d", ok, changed, len(clients))
	}
	if clients, _, ok := room.snapshotClientsForMonitor(oldMonitor); ok || len(clients) != 0 {
		t.Fatalf("retired monitor captured replacement clients: ok=%v clients=%d", ok, len(clients))
	}
	oldMonitor.finish(room)

	room.mu.Lock()
	gotMonitor := room.monitor
	statusUnknown := room.statusUnknown
	room.mu.Unlock()
	if gotMonitor != replacementMonitor {
		t.Fatal("retired monitor finish cleared the replacement monitor")
	}
	if statusUnknown {
		t.Fatal("retired monitor changed replacement monitor status")
	}
	requireNoQueuedMessage(t, replacementClient)

	replacementMonitor.finish(room)
}

func TestLiveSessionClientSnapshotDoesNotCrossGeneration(t *testing.T) {
	oldLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer oldLive.Dispose()
	newLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
	defer room.Close()
	oldClient := NewClient("old-client", nil)
	newClient := NewClient("new-client", nil)
	room.douyinLive = oldLive
	addTestClient(room, oldClient)
	oldGeneration := room.sessionGeneration

	clients, ok := room.snapshotClientsForLiveSession(oldLive, oldGeneration)
	if !ok || len(clients) != 1 || clients[0] != oldClient {
		t.Fatalf("old session snapshot = (%v, %d clients), want old client", ok, len(clients))
	}

	room.mu.Lock()
	room.sessionGeneration++
	newGeneration := room.sessionGeneration
	room.douyinLive = newLive
	room.clientsMu.Lock()
	room.clients = map[string]*Client{newClient.id: newClient}
	room.connIDs = make(map[*gws.Conn]string)
	room.clientsMu.Unlock()
	room.mu.Unlock()

	event := &douyinLive.LiveMessage{
		Raw:    &new_douyin.Webcast_Im_Message{Method: "GenerationTestMessage"},
		Parsed: &emptypb.Empty{},
	}
	room.handleDouyinEventForClients(clients, event)
	requireQueuedMessageContaining(t, oldClient, `"method":"GenerationTestMessage"`)
	requireNoQueuedMessage(t, newClient)

	room.handleDouyinEventForSession(oldLive, oldGeneration, event)
	requireNoQueuedMessage(t, newClient)
	room.handleDouyinEventForSession(newLive, newGeneration, event)
	requireQueuedMessageContaining(t, newClient, `"method":"GenerationTestMessage"`)
}

func TestStaleLiveSessionEndCannotTransitionReplacementGeneration(t *testing.T) {
	oldLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer oldLive.Dispose()
	newLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}

	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
	defer room.Close()
	room.douyinLive = oldLive
	oldGeneration := room.sessionGeneration
	replacementClient := NewClient("replacement-client", nil)

	room.mu.Lock()
	room.sessionGeneration++
	room.douyinLive = newLive
	room.upstreamReady = true
	room.statusUnknown = true
	room.clientsMu.Lock()
	room.clients[replacementClient.id] = replacementClient
	room.clientsMu.Unlock()
	room.mu.Unlock()

	monitor, clients, installed, ok := room.transitionLiveSessionToMonitor(oldLive, oldGeneration)
	if ok || installed || monitor != nil || len(clients) != 0 {
		t.Fatalf("stale live end transitioned replacement generation: ok=%v installed=%v monitor=%p clients=%d", ok, installed, monitor, len(clients))
	}
	room.mu.Lock()
	gotLive := room.douyinLive
	ready := room.upstreamReady
	statusUnknown := room.statusUnknown
	gotMonitor := room.monitor
	room.mu.Unlock()
	if gotLive != newLive || !ready || !statusUnknown || gotMonitor != nil {
		t.Fatalf("stale live end changed replacement state: live=%p want=%p ready=%v unknown=%v monitor=%p", gotLive, newLive, ready, statusUnknown, gotMonitor)
	}
	requireNoQueuedMessage(t, replacementClient)
}

func TestProbeMetadataCommitRejectsRetiredGeneration(t *testing.T) {
	probe, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	defer probe.Dispose()

	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Hour, time.Hour, nil)
	room.probeLive = probe
	oldGeneration := room.sessionGeneration
	room.sessionGeneration++
	room.liveName = "replacement-anchor"
	room.title = "replacement-title"
	room.accountOnly = true
	room.knownValid = false

	if room.commitProbeMetadataForGeneration(probe, oldGeneration, true) {
		t.Fatal("retired probe metadata was committed")
	}
	room.mu.Lock()
	liveName := room.liveName
	title := room.title
	accountOnly := room.accountOnly
	knownValid := room.knownValid
	room.mu.Unlock()
	if liveName != "replacement-anchor" || title != "replacement-title" || !accountOnly || knownValid {
		t.Fatalf("retired probe changed replacement metadata: live_name=%q title=%q account_only=%v known_valid=%v", liveName, title, accountOnly, knownValid)
	}
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

func TestTerminalStartFailureDisposesRetainedProbeAndRemovesRoom(t *testing.T) {
	removed := make(chan struct{}, 1)
	room := NewRoom("live-id", nil, false, "ttwid=configured", douyinLive.SignProviderLocal, "", time.Second, time.Second, func() {
		removed <- struct{}{}
	})
	probe, err := douyinLive.NewDouyinLive("live-id", nil, "ttwid=configured")
	if err != nil {
		t.Fatal(err)
	}
	room.probeLive = probe
	room.starting = true
	client := NewClient("client", nil)
	addTestClient(room, client)

	room.handleLiveSessionStartResult(room.sessionGeneration, errors.New("deterministic terminal startup failure"))

	room.mu.Lock()
	retainedProbe := room.probeLive
	room.mu.Unlock()
	if retainedProbe != nil {
		t.Fatal("terminal startup failure retained its probe session")
	}
	if got := room.clientCount(); got != 0 {
		t.Fatalf("client count after terminal startup failure = %d, want 0", got)
	}
	select {
	case <-client.stopCh:
	default:
		t.Fatal("terminal startup failure did not close the waiting client")
	}
	if !room.isClosed() {
		t.Fatal("room remained managed after terminal startup failure cleanup")
	}
	select {
	case <-removed:
	default:
		t.Fatal("terminal startup failure did not remove the room")
	}
}

func TestStaleClientlessCleanupDoesNotKillImmediateReconnect(t *testing.T) {
	oldLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	newLive, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		oldLive.Dispose()
		t.Fatal(err)
	}

	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	defer room.Close()
	room.douyinLive = oldLive
	room.upstreamReady = true
	room.starting = true
	oldGeneration := room.sessionGeneration

	workers, ok := room.detachBackgroundWorkersIfIdle()
	if !ok {
		t.Fatal("clientless room did not retire its old session generation")
	}
	if room.sessionGeneration == oldGeneration {
		t.Fatal("clientless cleanup did not advance the session generation")
	}

	client := NewClient("reconnected-client", nil)
	addTestClient(room, client)
	room.mu.Lock()
	room.douyinLive = newLive
	room.upstreamReady = true
	room.starting = true
	room.mu.Unlock()

	workers.close(room)
	if room.closeClientsForFailedGeneration(oldGeneration, invalidRoomClientClose) {
		t.Fatal("stale failed-generation completion retired the replacement session")
	}
	// Simulate the old initialization completing after the replacement client
	// and session have already been installed.
	room.initializeLiveSession(oldGeneration)

	room.mu.Lock()
	gotLive := room.douyinLive
	gotReady := room.upstreamReady
	gotStarting := room.starting
	room.mu.Unlock()
	if gotLive != newLive || !gotReady || !gotStarting {
		t.Fatalf("stale cleanup changed replacement session: live=%p want=%p ready=%v starting=%v", gotLive, newLive, gotReady, gotStarting)
	}
	if got := room.clientCount(); got != 1 {
		t.Fatalf("stale cleanup removed replacement clients: count=%d", got)
	}
	select {
	case <-client.stopCh:
		t.Fatal("stale cleanup closed the immediately reconnected client")
	default:
	}
}

func TestClientArrivalBeforeScheduledCleanupKeepsCurrentSession(t *testing.T) {
	current, err := douyinLive.NewDouyinLive("live-id", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	room := NewRoom("live-id", nil, false, "", douyinLive.SignProviderLocal, "", time.Second, time.Second, nil)
	defer room.Close()
	room.douyinLive = current
	originalGeneration := room.sessionGeneration

	// Hold mu so the cleanup goroutine is definitely pending while the new
	// client is installed using the documented mu -> clientsMu lock order.
	room.mu.Lock()
	cleanupDone := make(chan struct{})
	go func() {
		room.closeBackgroundWorkersIfIdle()
		close(cleanupDone)
	}()
	client := NewClient("reconnected-client", nil)
	room.clientsMu.Lock()
	room.clients[client.id] = client
	room.clientsMu.Unlock()
	room.mu.Unlock()

	select {
	case <-cleanupDone:
	case <-time.After(time.Second):
		t.Fatal("scheduled cleanup did not finish")
	}
	room.mu.Lock()
	gotLive := room.douyinLive
	gotGeneration := room.sessionGeneration
	room.mu.Unlock()
	if gotLive != current || gotGeneration != originalGeneration {
		t.Fatalf("cleanup retired an in-use session: live=%p want=%p generation=%d want=%d", gotLive, current, gotGeneration, originalGeneration)
	}
	select {
	case <-client.stopCh:
		t.Fatal("scheduled cleanup closed the newly arrived client")
	default:
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
