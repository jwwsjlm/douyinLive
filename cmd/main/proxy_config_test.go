package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	douyinLive "github.com/jwwsjlm/douyinLive/v2"
)

func TestProxyConfigPriorityAndValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("proxy:\n  url: http://yaml:80\n  rooms:\n    AbC123: socks5://room:1080\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resetConfigGlobalsForTest(t, "--config", path, "--proxy-url", "http://flag:80")
	t.Setenv("APP_PROXY_URL", "http://env:80")
	t.Setenv("APP_PROXY_ROOMS", `{"AbC123":"http://env-room:80"}`)
	config, err := NewConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.Proxy.URL != "http://flag:80" || config.Proxy.Rooms["AbC123"] != "http://env-room:80" {
		t.Fatal("proxy overrides did not preserve flag priority or room ID casing")
	}
	t.Setenv("APP_PROXY_ROOMS", "")
	resetConfigGlobalsForTest(t, "--config", path)
	config, err = NewConfig()
	if err != nil || config.Proxy.URL != "http://env:80" || len(config.Proxy.Rooms) != 0 {
		t.Fatalf("environment priority/clear: %v", err)
	}
	t.Setenv("APP_PROXY_ROOMS", `{"room":"http://u:secret@host:99999"}`)
	if _, err := NewConfig(); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("bad proxy configuration: %v", err)
	}
	t.Setenv("APP_PROXY_ROOMS", `not json`)
	if _, err := NewConfig(); err == nil {
		t.Fatal("invalid proxy map accepted")
	}
	for _, config := range []ProxyConfig{
		{URL: "https://u:secret@host"},
		{Rooms: map[string]string{"bad/id": "http://host"}},
		{Rooms: map[string]string{"room": "http://one", " room ": "http://two"}},
	} {
		if _, err := normalizeProxyConfig(config); err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatalf("normalization accepted bad config or leaked credentials: %v", err)
		}
		if _, err := NewApp(context.Background(), &Config{Proxy: config}, nil); err == nil {
			t.Fatal("programmatic NewApp bypassed proxy validation")
		}
	}
}

func TestRoomProxyRoutingIncludesAPIAndProbeRotation(t *testing.T) {
	var counts [2]atomic.Int64
	proxies := make([]*httptest.Server, 2)
	for i := range proxies {
		proxies[i] = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodConnect || r.Host != "live.douyin.com:443" {
				t.Errorf("unexpected proxy request: %s %s", r.Method, r.Host)
			}
			counts[i].Add(1)
			w.WriteHeader(http.StatusProxyAuthRequired)
		}))
		defer proxies[i].Close()
	}
	config := &Config{
		Proxy:   ProxyConfig{URL: proxies[0].URL, Rooms: map[string]string{"1002": proxies[1].URL, "1003": ""}},
		Monitor: MonitorConfig{PollInterval: time.Second, NotifyInterval: time.Second},
	}
	app, err := NewApp(context.Background(), config, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer app.roomManager.Close()
	config.Proxy.Rooms["1002"] = "http://modified:80"
	if app.roomManager.proxyForRoom("1002") != proxies[1].URL || app.roomManager.proxyForRoom("1003") != proxies[0].URL {
		t.Fatal("room proxy map was not copied or empty room did not inherit")
	}
	for i, id := range []string{"1001", "1002"} {
		room := app.roomManager.GetOrCreateRoom(id, "")
		var previous *douyinLive.DouyinLive
		for round := 0; round < 2; round++ {
			before := counts[i].Load()
			dl, err := room.acquireProbeLive(room.sessionGeneration)
			if err != nil {
				t.Fatal(err)
			}
			if dl == previous {
				t.Fatal("anonymous probe was not rotated")
			}
			status, err := dl.CheckLiveStatus(context.Background())
			if err == nil || status.Code != douyinLive.LiveStatusUnknown || counts[i].Load() <= before {
				t.Fatalf("room probe did not fail through proxy %d: %+v %v", i, status, err)
			}
			for failure := 0; failure < anonymousProbeRotateFailures; failure++ {
				room.recordProbeFailure(dl)
			}
			previous = dl
		}
		before := counts[i].Load()
		status, err := app.roomManager.LookupRoom(context.Background(), id)
		if err == nil || status.Code != douyinLive.LiveStatusUnknown || counts[i].Load() <= before {
			t.Fatalf("API probe did not use proxy %d: %+v %v", i, status, err)
		}
	}
}
