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
	"time"

	"github.com/jwwsjlm/req/v3"
)

const issue71EnterBody = `{"status_code":0,"data":{"data":[{"id_str":"9001","status":2,"title":"current title","owner":{"id_str":"123","nickname":"current anchor","avatar_thumb":{"url_list":["current avatar"]}}}]}}`

func issue71Client(t *testing.T, cookie string) *DouyinLive {
	t.Helper()
	dl, err := newDouyinLive("812195156626", log.New(io.Discard, "", 0), cookie, staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dl.Dispose)
	return dl
}

func issue71Response(request *req.Request, status int, body string, cookies ...string) *req.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json")
	for _, cookie := range cookies {
		header.Add("Set-Cookie", cookie)
	}
	response := &req.Response{Response: &http.Response{StatusCode: status, Header: header}, Request: request}
	response.SetBodyString(body)
	return response
}

func TestIssue71TTWIDRegistrationAndHomepageFallback(t *testing.T) {
	for _, tc := range []struct {
		name       string
		status     int
		cookie     string
		requestErr error
		fallback   bool
	}{
		{name: "registered", status: http.StatusOK, cookie: "ttwid=registered; Path=/"},
		{name: "transport error", requestErr: errors.New("registration unavailable"), fallback: true},
		{name: "server error with cookie", status: http.StatusServiceUnavailable, cookie: "ttwid=untrusted; Path=/", fallback: true},
		{name: "missing cookie", status: http.StatusOK, cookie: "other=value; Path=/", fallback: true},
		{name: "empty cookie", status: http.StatusOK, cookie: "ttwid=; Path=/", fallback: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dl := issue71Client(t, "")
			dl.additionalCookies["stale"] = "old"
			registerCalls, homepageCalls := 0, 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					switch request.URL.Host + request.URL.Path {
					case "ttwid.bytedance.com/ttwid/union/register/":
						registerCalls++
						if request.Method != http.MethodPost {
							t.Errorf("registration method=%s, want POST", request.Method)
						}
						if tc.requestErr != nil {
							return nil, tc.requestErr
						}
						return issue71Response(request, tc.status, `{}`, tc.cookie), nil
					case "live.douyin.com/":
						homepageCalls++
						if !tc.fallback || registerCalls != 1 {
							t.Error("homepage requested before registration or after registration succeeded")
						}
						return issue71Response(request, http.StatusOK, "homepage", "ttwid=homepage; Path=/", "msToken=homepage-extra; Path=/"), nil
					default:
						return nil, fmt.Errorf("unexpected request %s", request.URL.Host+request.URL.Path)
					}
				}
			})
			if err := dl.fetchTTWID(context.Background()); err != nil {
				t.Fatal(err)
			}
			wantCookie, wantHomepage := "registered", 0
			if tc.fallback {
				wantCookie, wantHomepage = "homepage", 1
				if dl.cookieValue("msToken") != "homepage-extra" || dl.cookieValue("stale") != "" {
					t.Errorf("homepage cookies not refreshed: %#v", dl.additionalCookies)
				}
			}
			if registerCalls != 1 || homepageCalls != wantHomepage || dl.cookieValue("ttwid") != wantCookie {
				t.Fatalf("register=%d homepage=%d ttwid=%q, want 1/%d/%q", registerCalls, homepageCalls, dl.cookieValue("ttwid"), wantHomepage, wantCookie)
			}
			if dl.shouldFetchTTWID() {
				t.Error("automatically fetched ttwid was not retained for this session")
			}
		})
	}
}

func TestIssue71TTWIDRegistrationCancellationStopsFallback(t *testing.T) {
	dl := issue71Client(t, "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
		return func(request *req.Request) (*req.Response, error) {
			calls++
			if request.URL.Host != "ttwid.bytedance.com" {
				t.Error("canceled registration fetched the homepage")
			}
			cancel()
			return nil, ctx.Err()
		}
	})
	if err := dl.fetchTTWID(ctx); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("fetchTTWID()=%v, calls=%d; want context.Canceled/1", err, calls)
	}
}

func TestIssue71OptionalEndpointTimeoutLeavesFallbackBudget(t *testing.T) {
	for _, registration := range []bool{true, false} {
		t.Run(fmt.Sprint("registration=", registration), func(t *testing.T) {
			cookie, optionalPath, fallbackPath, budget := "ttwid=configured", "/webcast/room/web/enter/", "/812195156626", 3*time.Second
			if registration {
				cookie, optionalPath, fallbackPath, budget = "", "/ttwid/union/register/", "/", 2*time.Second
			}
			dl := issue71Client(t, cookie)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			var optionalDeadline time.Time
			optionalCalls, fallbackCalls := 0, 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					switch request.URL.Path {
					case optionalPath:
						optionalCalls++
						var ok bool
						optionalDeadline, ok = request.Context().Deadline()
						if remaining := time.Until(optionalDeadline); !ok || remaining <= 0 || remaining > budget {
							t.Errorf("optional request deadline has %v remaining, want at most %v", remaining, budget)
						}
						return nil, context.DeadlineExceeded
					case fallbackPath:
						fallbackCalls++
						deadline, ok := request.Context().Deadline()
						if request.Context().Err() != nil || !ok || deadline.Sub(optionalDeadline) < 5*time.Second {
							t.Errorf("fallback inherited exhausted optional budget: deadline=%v err=%v", deadline, request.Context().Err())
						}
						return issue71Response(request, http.StatusOK, `{"roomInfo":{"room":{"id_str":"9002","status":2,"owner":{"id_str":"456","nickname":"fallback anchor"}}}}`, "ttwid=homepage; Path=/"), nil
					default:
						return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
					}
				}
			})
			var err error
			if registration {
				err = dl.fetchTTWID(ctx)
			} else {
				var status LiveStatus
				status, err = dl.CheckLiveStatus(ctx)
				if status.Code != LiveStatusOnline {
					t.Errorf("fallback status=%+v, want online", status)
				}
			}
			if err != nil || ctx.Err() != nil || optionalCalls != 1 || fallbackCalls != 1 {
				t.Fatalf("err=%v parent=%v optional=%d fallback=%d, want success and one of each request", err, ctx.Err(), optionalCalls, fallbackCalls)
			}
		})
	}
}

func TestIssue71StatusParentCancellationStopsFallback(t *testing.T) {
	dl := issue71Client(t, "ttwid=configured")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
		return func(request *req.Request) (*req.Response, error) {
			calls++
			if request.URL.Path != "/webcast/room/web/enter/" {
				t.Error("canceled status request fetched the live page")
			}
			cancel()
			return nil, ctx.Err()
		}
	})
	status, err := dl.CheckLiveStatus(ctx)
	if !errors.Is(err, context.Canceled) || status.Code != LiveStatusUnknown || status.Live != nil || calls != 1 {
		t.Fatalf("status=%+v err=%v calls=%d, want unknown/context.Canceled/1", status, err, calls)
	}
}

func TestIssue71StatusReusesAnonymousCookieAndHonorsConfiguredCookie(t *testing.T) {
	for _, cookie := range []string{"", "sessionid=configured"} {
		t.Run(cookie, func(t *testing.T) {
			dl := issue71Client(t, cookie)
			registerCalls, enterCalls := 0, 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					switch request.URL.Host + request.URL.Path {
					case "ttwid.bytedance.com/ttwid/union/register/":
						registerCalls++
						return issue71Response(request, http.StatusOK, `{}`, "ttwid=registered; Path=/"), nil
					case "live.douyin.com/webcast/room/web/enter/":
						enterCalls++
						wantCookie := "ttwid=registered"
						if cookie != "" {
							wantCookie = cookie
						}
						if !strings.Contains(request.Headers.Get("Cookie"), wantCookie) {
							t.Errorf("request Cookie=%q, want %q", request.Headers.Get("Cookie"), wantCookie)
						}
						if got := request.URL.Query().Get("room_id_str"); got != "" {
							t.Errorf("status refresh retained previous room_id_str=%q", got)
						}
						return issue71Response(request, http.StatusOK, issue71EnterBody), nil
					default:
						return nil, fmt.Errorf("unexpected page request %s", request.URL.Path)
					}
				}
			})
			for range 2 {
				status, err := dl.CheckLiveStatus(context.Background())
				if err != nil || status.Code != LiveStatusOnline {
					t.Fatalf("CheckLiveStatus()=%+v, %v", status, err)
				}
			}
			wantRegister := 1
			if cookie != "" {
				wantRegister = 0
			}
			if registerCalls != wantRegister || enterCalls != 2 {
				t.Fatalf("register=%d enter=%d, want %d/2", registerCalls, enterCalls, wantRegister)
			}
		})
	}
}

func TestIssue71VerifiedAPIStatusSkipsLivePage(t *testing.T) {
	for _, tc := range []struct {
		value string
		live  bool
	}{
		{value: "2", live: true},
		{value: "4"},
		{value: `"2"`, live: true},
		{value: `"4"`},
	} {
		t.Run(tc.value, func(t *testing.T) {
			dl := issue71Client(t, "ttwid=configured")
			dl.updateRoomInfo("old-room", "", "old anchor", "old title", "old avatar")
			dl.setLiveStatus(true)
			enterCalls := 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					if request.URL.Path != "/webcast/room/web/enter/" {
						return nil, fmt.Errorf("verified API response triggered page request %s", request.URL.Path)
					}
					enterCalls++
					if request.URL.Query().Get("room_id_str") != "" || request.URL.Query().Get("web_rid") != dl.liveID {
						t.Error("fresh status probe did not discover the current room by web_rid")
					}
					if request.URL.Query().Get("a_bogus") == "" {
						t.Error("web/enter request is unsigned")
					}
					body := strings.Replace(issue71EnterBody, `"status":2`, `"status":`+tc.value, 1)
					return issue71Response(request, http.StatusOK, body), nil
				}
			})
			status, err := dl.CheckLiveStatus(context.Background())
			wantCode := LiveStatusOnline
			if !tc.live {
				wantCode = LiveStatusOffline
			}
			if err != nil || status.Code != wantCode || status.Live == nil || *status.Live != tc.live {
				t.Fatalf("CheckLiveStatus()=%+v, %v", status, err)
			}
			if enterCalls != 1 || status.RoomID != "9001" || status.UserUniqueID != "123" || status.LiveName != "current anchor" || status.Title != "current title" || status.AvatarThumb != "current avatar" || status.HasRoom == nil || !*status.HasRoom || status.AccountOnly == nil || *status.AccountOnly {
				t.Fatalf("API status lost metadata or triggered extra queries: calls=%d status=%+v", enterCalls, status)
			}
		})
	}
}

func TestIssue71UnverifiedAPIStatusFallsBackToLivePage(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "empty"},
		{name: "HTML", body: "<html>verification required</html>"},
		{name: "missing status", body: strings.Replace(issue71EnterBody, `"status":2,`, "", 1)},
		{name: "null status", body: strings.Replace(issue71EnterBody, `"status":2`, `"status":null`, 1)},
		{name: "boolean status", body: strings.Replace(issue71EnterBody, `"status":2`, `"status":true`, 1)},
		{name: "nonnumeric status", body: strings.Replace(issue71EnterBody, `"status":2`, `"status":"garbage"`, 1)},
		{name: "fractional status", body: strings.Replace(issue71EnterBody, `"status":2`, `"status":2.5`, 1)},
		{name: "missing status_code", body: strings.Replace(issue71EnterBody, `"status_code":0,`, "", 1)},
		{name: "null status_code", body: strings.Replace(issue71EnterBody, `"status_code":0`, `"status_code":null`, 1)},
		{name: "boolean status_code", body: strings.Replace(issue71EnterBody, `"status_code":0`, `"status_code":false`, 1)},
		{name: "numeric string status_code", body: strings.Replace(issue71EnterBody, `"status_code":0`, `"status_code":"0"`, 1)},
		{name: "nonnumeric status_code", body: strings.Replace(issue71EnterBody, `"status_code":0`, `"status_code":"garbage"`, 1)},
		{name: "missing room ID", body: strings.Replace(issue71EnterBody, `"id_str":"9001",`, "", 1)},
		{name: "missing user ID", body: strings.Replace(issue71EnterBody, `"id_str":"123",`, "", 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dl := issue71Client(t, "ttwid=configured")
			enterCalls, pageCalls := 0, 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					switch request.URL.Path {
					case "/webcast/room/web/enter/":
						enterCalls++
						if pageCalls != 0 {
							t.Error("online page fallback did not stop further API requests")
						}
						return issue71Response(request, http.StatusOK, tc.body), nil
					case "/" + dl.liveID:
						pageCalls++
						if enterCalls != 1 {
							t.Error("live page was fetched before the lightweight API")
						}
						return issue71Response(request, http.StatusOK, `{"roomInfo":{"room":{"id_str":"9002","status":2,"title":"fallback title","owner":{"id_str":"456","nickname":"fallback anchor"}}}}`), nil
					default:
						return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
					}
				}
			})
			status, err := dl.CheckLiveStatus(context.Background())
			if err != nil || status.Code != LiveStatusOnline || status.RoomID != "9002" || status.LiveName != "fallback anchor" || status.Title != "fallback title" || pageCalls != 1 || enterCalls != 1 {
				t.Fatalf("page fallback=%+v, %v; API=%d page=%d", status, err, enterCalls, pageCalls)
			}
		})
	}
}

func TestIssue71EmptyAPIFallbackPreservesOfflineAndUnknown(t *testing.T) {
	for _, tc := range []struct {
		name string
		page string
		code LiveStatusCode
	}{
		{name: "verified offline page", page: `{"roomInfo":{"room":{"id_str":"9002","status":4,"owner":{"id_str":"456","nickname":"offline anchor"}}}}`, code: LiveStatusOffline},
		{name: "unverified page after online", page: "<html>verification required</html>", code: LiveStatusUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dl := issue71Client(t, "ttwid=configured")
			dl.updateRoomInfo("old-room", "old-user", "previous anchor", "previous title", "")
			dl.setLiveStatus(true)
			enterCalls, pageCalls := 0, 0
			dl.client.WrapRoundTripFunc(func(_ req.RoundTripper) req.RoundTripFunc {
				return func(request *req.Request) (*req.Response, error) {
					switch request.URL.Path {
					case "/webcast/room/web/enter/":
						enterCalls++
						if request.URL.Query().Get("room_id_str") != "" {
							t.Error("unverified status probe reused the previous live room ID")
						}
						return issue71Response(request, http.StatusOK, ""), nil
					case "/" + dl.liveID:
						pageCalls++
						return issue71Response(request, http.StatusOK, tc.page), nil
					default:
						return nil, fmt.Errorf("unexpected request %s", request.URL.Path)
					}
				}
			})
			status, err := dl.CheckLiveStatus(context.Background())
			if status.Code != tc.code || pageCalls != 1 || enterCalls != 2 {
				t.Fatalf("fallback=%+v, %v; API=%d page=%d", status, err, enterCalls, pageCalls)
			}
			if tc.code == LiveStatusUnknown {
				if !errors.Is(err, ErrLiveStatusUnknown) || status.Live != nil || dl.IsKnownLiveStatus() || dl.IsKnownOfflineStatus() {
					t.Fatalf("failed refresh reused previous online state: %+v, %v", status, err)
				}
			} else if err != nil || status.Live == nil || *status.Live || status.RoomID != "9002" {
				t.Fatalf("offline fallback=%+v, %v", status, err)
			}
		})
	}
}
