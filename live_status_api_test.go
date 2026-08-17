package douyinLive

import (
	"context"
	"errors"
	"testing"
)

func TestStatusCodeForSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		info      roomInfoSnapshot
		isLive    bool
		known     bool
		wantCode  LiveStatusCode
		wantRoom  bool
		wantOwner bool
	}{
		{name: "online", info: roomInfoSnapshot{roomID: "room-1"}, isLive: true, known: true, wantCode: LiveStatusOnline, wantRoom: true, wantOwner: false},
		{name: "offline", info: roomInfoSnapshot{roomID: "room-1"}, isLive: false, known: true, wantCode: LiveStatusOffline, wantRoom: true, wantOwner: false},
		{name: "account only", info: roomInfoSnapshot{anchorOnly: true, liveName: "主播"}, isLive: false, known: true, wantCode: LiveStatusNoRoom, wantRoom: false, wantOwner: true},
		{name: "offline without identity", info: roomInfoSnapshot{}, isLive: false, known: true, wantCode: LiveStatusUnknown},
		{name: "unknown", info: roomInfoSnapshot{}, isLive: false, known: false, wantCode: LiveStatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusCodeForSnapshot(tt.info, tt.isLive, tt.known); got != tt.wantCode {
				t.Fatalf("statusCodeForSnapshot() = %q, want %q", got, tt.wantCode)
			}
			dl := &DouyinLive{liveID: "live-1"}
			dl.updateRoomInfo(tt.info.roomID, tt.info.pushID, tt.info.liveName, tt.info.title, tt.info.avatarThumb)
			if tt.info.anchorOnly {
				dl.mu.Lock()
				dl.anchorOnlyPageIdentity = true
				dl.mu.Unlock()
			}
			if tt.known {
				dl.setLiveStatus(tt.isLive)
			}
			status := dl.liveStatusResult(tt.wantCode)
			if status.Code != tt.wantCode {
				t.Fatalf("LiveStatus.Code = %q, want %q", status.Code, tt.wantCode)
			}
			if tt.known && tt.wantCode != LiveStatusUnknown && tt.wantCode != LiveStatusNotFound {
				if status.HasRoom == nil || *status.HasRoom != tt.wantRoom {
					t.Fatalf("LiveStatus.HasRoom = %v, want %v", status.HasRoom, tt.wantRoom)
				}
				if status.AccountOnly == nil || *status.AccountOnly != tt.wantOwner {
					t.Fatalf("LiveStatus.AccountOnly = %v, want %v", status.AccountOnly, tt.wantOwner)
				}
			} else if status.Live != nil || status.HasRoom != nil || status.AccountOnly != nil {
				t.Fatalf("unknown status should not expose boolean fields: %+v", status)
			}
		})
	}
}

func TestCheckLiveStatusHonorsCanceledContext(t *testing.T) {
	dl, err := newDouyinLive("live-id", nil, "ttwid=test", staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	status, err := dl.CheckLiveStatus(ctx)
	if err == nil {
		t.Fatal("CheckLiveStatus() error = nil, want cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CheckLiveStatus() error = %v, want context.Canceled", err)
	}
	if status.Code != LiveStatusUnknown {
		t.Fatalf("CheckLiveStatus() code = %q, want %q", status.Code, LiveStatusUnknown)
	}
}
