package douyinLive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"testing"

	"github.com/jwwsjlm/douyinlive-proto/generated/new_douyin"
	"github.com/jwwsjlm/req/v3"
	"google.golang.org/protobuf/proto"
)

func TestRoomRefreshFindsNextLiveSession(t *testing.T) {
	const liveID = "812195156626"
	const oldRoom = "7690809912363977523"
	const newRoom = "7690926943562304282"
	offlinePage := fmt.Sprintf(`{"roomInfo":{"room":{"id_str":%q,"status":4,"owner":{"id_str":"123456789012","nickname":"anchor"}}}}`, oldRoom)
	onlinePage := strings.ReplaceAll(strings.ReplaceAll(offlinePage, oldRoom, newRoom), `"status":4`, `"status":2`)
	onlineEnter := fmt.Sprintf(`{"status_code":0,"data":{"data":[{"id_str":%q,"status":2,"owner":{"id_str":"123456789012","nickname":"anchor"}}]}}`, newRoom)
	offlineEnter := strings.ReplaceAll(strings.ReplaceAll(onlineEnter, newRoom, oldRoom), `"status":2`, `"status":4`)

	for _, api := range []string{"websocket", "status"} {
		for _, tt := range []struct {
			name         string
			page         string
			pageErr      error
			enter        string
			previousLive bool
			wantLive     bool
			wantUnknown  bool
		}{
			{name: "old offline page", page: offlinePage, enter: onlineEnter, wantLive: true},
			{name: "page request failed", pageErr: errors.New("page unavailable"), enter: onlineEnter, wantLive: true},
			{name: "page has no status", page: strings.ReplaceAll(offlinePage, `,"status":4`, ""), enter: onlineEnter, wantLive: true},
			{name: "challenge page", page: "<html>verification required</html>", enter: onlineEnter, wantLive: true},
			{name: "failed refresh after offline", pageErr: errors.New("page unavailable"), wantUnknown: true},
			{name: "failed refresh after online", pageErr: errors.New("page unavailable"), previousLive: true, wantUnknown: true},
			{name: "confirmed offline", page: offlinePage, enter: offlineEnter},
			{name: "offline page with empty enter", page: offlinePage},
			{name: "online page with empty enter", page: onlinePage, wantLive: true},
		} {
			t.Run(api+"/"+tt.name, func(t *testing.T) {
				dl, err := newDouyinLive(liveID, log.New(io.Discard, "", 0), "ttwid=test", staticWebsocketSigner{signature: "sig"})
				if err != nil {
					t.Fatal(err)
				}
				defer dl.Dispose()
				dl.updateRoomInfo(oldRoom, "123456789012", "anchor", "previous session", "")
				dl.setLiveStatus(tt.previousLive)
				dl.storeRoomEnterData(offlineEnter)

				pageCalls, enterCalls, imCalls := 0, 0, 0
				dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
					return func(request *req.Request) (*req.Response, error) {
						var body []byte
						switch request.URL.Path {
						case "/" + liveID:
							pageCalls++
							if dl.isLiveStatus() != tt.previousLive {
								t.Error("HTTP refresh changed listener state before receiving a response")
							}
							if tt.pageErr != nil {
								return nil, tt.pageErr
							}
							body = []byte(tt.page)
						case "/webcast/room/web/enter/":
							enterCalls++
							wantRoomID := ""
							if tt.page == onlinePage {
								wantRoomID = newRoom
							}
							if got := request.URL.Query().Get("room_id_str"); got != wantRoomID {
								t.Errorf("room_id_str=%s, want %s", got, wantRoomID)
							}
							if got := request.URL.Query().Get("web_rid"); got != liveID {
								t.Errorf("web_rid=%s, want %s", got, liveID)
							}
							body = []byte(tt.enter)
						case "/webcast/im/fetch/":
							imCalls++
							if got := request.URL.Query().Get("room_id"); got != newRoom {
								t.Errorf("IM requested room_id=%s, want %s", got, newRoom)
							}
							body, err = proto.Marshal(&new_douyin.Webcast_Im_Response{Cursor: "next-session-cursor", PushServer: websocketPushURL})
							if err != nil {
								return nil, err
							}
						default:
							t.Errorf("unexpected request: %s", request.URL.Path)
							return nil, fmt.Errorf("unexpected request: %s", request.URL.Path)
						}
						response := &req.Response{Response: &http.Response{StatusCode: http.StatusOK, Header: make(http.Header)}, Request: request}
						response.SetBody(body)
						return response, nil
					}
				})

				var live bool
				if api == "websocket" {
					err = dl.PrepareWebSocketContext()
					live = dl.IsKnownLiveStatus()
				} else {
					status, statusErr := dl.CheckLiveStatus(context.Background())
					err = statusErr
					live = status.Code == LiveStatusOnline
					if tt.wantUnknown && status.Live != nil {
						t.Errorf("unverified refresh exposed live=%v", *status.Live)
					}
				}
				var wantErr error
				if tt.wantUnknown {
					wantErr = ErrLiveStatusUnknown
				} else if api == "websocket" && !tt.wantLive {
					wantErr = ErrLiveNotStarted
				}
				if !errors.Is(err, wantErr) || live != tt.wantLive {
					t.Fatalf("live=%v err=%v, want live=%v err=%v", live, err, tt.wantLive, wantErr)
				}
				if pageCalls != 1 {
					t.Errorf("page requests=%d, want 1", pageCalls)
				}
				if !(api == "status" && tt.page == onlinePage) && enterCalls == 0 {
					t.Error("refresh skipped web/enter")
				}
				if tt.wantLive && dl.GetRoomID() != newRoom {
					t.Errorf("room_id=%s, want %s", dl.GetRoomID(), newRoom)
				}
				wantIMCalls := 0
				if api == "websocket" && tt.wantLive {
					wantIMCalls = 1
				}
				if imCalls != wantIMCalls {
					t.Errorf("IM requests=%d, want %d", imCalls, wantIMCalls)
				}
				if tt.wantUnknown && (dl.IsKnownOfflineStatus() || dl.IsKnownLiveStatus()) {
					t.Error("failed refresh reused a previous live status")
				}
			})
		}
	}
}
