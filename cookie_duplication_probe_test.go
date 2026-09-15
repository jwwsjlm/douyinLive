package douyinLive

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestCookieHeaderIsNotDuplicated 校验请求头中的 Cookie 不存在重复键。
// 抓包工具会把请求链上的 Cookie 合并展示，容易误判为重复；这里用本地探针直接观察
// 服务端收到的原始 Cookie 头，确认实际发送的 Cookie 是干净的。
// TestCookieHeaderIsNotDuplicated verifies the outgoing Cookie header has no duplicate names.
// Capture tooling merges cookies across a request chain, which easily looks like duplication.
func TestCookieHeaderIsNotDuplicated(t *testing.T) {
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Cookie"))
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()

	dl, err := newDouyinLiveWithProxy("1001", nil, "", staticWebsocketSigner{signature: "sig"}, "", string(ProtocolModePC))
	if err != nil {
		t.Fatal(err)
	}
	defer dl.Dispose()

	dl.ttwid = "ttwid-A"
	dl.additionalCookies["UIFID_TEMP"] = "UIFID-B"

	cookieString := dl.getCookieString()
	for range 2 {
		if _, err := dl.client.R().
			SetHeaders(map[string]string{"Cookie": cookieString}).
			Get(server.URL); err != nil {
			t.Fatal(err)
		}
	}

	wantParts := len(strings.Split(cookieString, "; "))
	if wantParts != len(pcClientCookieSeeds)+2 {
		t.Fatalf("Cookie 片段数 = %d, want %d", wantParts, len(pcClientCookieSeeds)+2)
	}
	for index, cookie := range seen {
		counts := map[string]int{}
		for _, part := range strings.Split(cookie, "; ") {
			name, _, found := strings.Cut(part, "=")
			if !found {
				continue
			}
			counts[name]++
		}
		for name, count := range counts {
			if count > 1 {
				t.Errorf("第 %d 次请求 Cookie 中 %q 出现 %d 次", index+1, name, count)
			}
		}
		if got := len(strings.Split(cookie, "; ")); got != wantParts {
			t.Errorf("第 %d 次请求服务端收到 %d 个片段, want %d: %q", index+1, got, wantParts, cookie)
		}
	}
}
