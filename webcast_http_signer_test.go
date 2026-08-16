package douyinLive

import (
	"net/url"
	"strings"
	"testing"
)

const testChromeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"

func TestSignWebcastHTTPURLPreservesParamsAndAddsABogus(t *testing.T) {
	const endpoint = "https://live.douyin.com/webcast/im/fetch/"
	const params = "aid=6383&app_name=douyin_web&live_id=1&room_id=123&msToken=token"

	result := signWebcastHTTPURL(endpoint, params, testChromeUserAgent)
	if result.Provider != webcastHTTPABogusProvider {
		t.Fatalf("Provider = %q, want %q", result.Provider, webcastHTTPABogusProvider)
	}
	if result.ABogusLength == 0 {
		t.Fatal("a_bogus is empty")
	}
	if result.Duration < 0 {
		t.Fatalf("Duration = %s, want non-negative", result.Duration)
	}
	if !strings.HasPrefix(result.URL, endpoint+"?"+params+"&a_bogus=") {
		t.Fatalf("URL did not preserve original parameter order: %s", result.URL)
	}

	parsed, err := url.Parse(result.URL)
	if err != nil {
		t.Fatalf("url.Parse() failed: %v", err)
	}
	if got := parsed.Query().Get("a_bogus"); got == "" {
		t.Fatal("signed URL is missing a_bogus")
	}
}

func BenchmarkSignWebcastHTTPURL(b *testing.B) {
	const endpoint = "https://live.douyin.com/webcast/im/fetch/"
	const params = "aid=6383&app_name=douyin_web&live_id=1&device_platform=web&room_id=7674618643562220334&user_unique_id=7660042561771406874&msToken=abcdefghijklmnopqrstuvwxyz"
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = signWebcastHTTPURL(endpoint, params, testChromeUserAgent)
	}
}
