package douyinLive

import (
	"context"
	"strings"
	"testing"
)

func TestLocalBDMSSignerWithGoja(t *testing.T) {
	u := "https://live.douyin.com/webcast/im/fetch/?aid=6383&app_name=douyin_web&live_id=1&device_platform=web&language=zh-CN&room_id=7660004188205714182"
	r, err := signURLWithLocalBDMS(context.Background(), u, "ttwid=local", "", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36")
	if err != nil {
		t.Fatal(err)
	}
	if r.Lengths["msToken"] != 172 {
		t.Fatalf("msToken len=%d", r.Lengths["msToken"])
	}
	if r.Lengths["a_bogus"] != 188 {
		t.Fatalf("a_bogus len=%d url=%s", r.Lengths["a_bogus"], r.SignedURLRedacted)
	}
}

func TestDouyinLiveReusesBDMSRuntimeForMatchingContext(t *testing.T) {
	const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	msToken := strings.Repeat("a", 172)
	dl, err := newDouyinLive("live-id", nil, "ttwid=local; msToken="+msToken, staticWebsocketSigner{signature: "sig"})
	if err != nil {
		t.Fatalf("newDouyinLive() failed: %v", err)
	}
	defer dl.Dispose()
	dl.userAgent = userAgent

	unsignedURL := "https://live.douyin.com/webcast/im/fetch/?aid=6383&app_name=douyin_web&live_id=1&device_platform=web&room_id=7660004188205714182&msToken=" + msToken
	first, err := dl.signWebcastURL(context.Background(), unsignedURL, msToken)
	if err != nil {
		t.Fatalf("first signWebcastURL() failed: %v", err)
	}
	if first.Lengths["a_bogus"] != 188 {
		t.Fatalf("first a_bogus len=%d", first.Lengths["a_bogus"])
	}
	firstRuntime := dl.bdmsRuntime
	if firstRuntime == nil {
		t.Fatal("BDMS runtime was not created")
	}

	second, err := dl.signWebcastURL(context.Background(), unsignedURL, msToken)
	if err != nil {
		t.Fatalf("second signWebcastURL() failed: %v", err)
	}
	if second.Lengths["a_bogus"] != 188 {
		t.Fatalf("second a_bogus len=%d", second.Lengths["a_bogus"])
	}
	if dl.bdmsRuntime != firstRuntime {
		t.Fatal("BDMS runtime was recreated despite an unchanged cookie and User-Agent")
	}
}
