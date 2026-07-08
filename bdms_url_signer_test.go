package douyinLive

import (
	"context"
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
